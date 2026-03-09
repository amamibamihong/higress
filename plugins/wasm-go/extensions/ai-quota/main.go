package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-quota/util"
)

const (
	pluginName = "ai-quota"
)

type ChatMode string

const (
	ChatModeCompletion ChatMode = "completion"
	ChatModeAdmin      ChatMode = "admin"
	ChatModeNone       ChatMode = "none"
)

type AdminMode string

const (
	AdminModeRefresh AdminMode = "refresh"
	AdminModeQuery   AdminMode = "query"
	AdminModeDelta   AdminMode = "delta"
	AdminModeSetRate AdminMode = "setrate"
	AdminModeGetRate AdminMode = "getrate"
	AdminModeNone    AdminMode = "none"
)

func main() {}

func init() {
	wrapper.SetCtx(
		pluginName,
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
	)
}

type QuotaConfig struct {
	redisInfo       RedisInfo         `yaml:"redis"`
	RedisKeyPrefix  string            `yaml:"redis_key_prefix"`
	AdminConsumer   string            `yaml:"admin_consumer"`
	AdminPath       string            `yaml:"admin_path"`
	Precision       int               `yaml:"precision"`
	credential2Name map[string]string `yaml:"-"`
	redisClient     wrapper.RedisClient
}

const (
	TokensPerMillion = 1000000
)

type Consumer struct {
	Name       string `yaml:"name"`
	Credential string `yaml:"credential"`
}

type RedisInfo struct {
	ServiceName string `required:"true" yaml:"service_name" json:"service_name"`
	ServicePort int    `required:"false" yaml:"service_port" json:"service_port"`
	Username    string `required:"false" yaml:"username" json:"username"`
	Password    string `required:"false" yaml:"password" json:"password"`
	Timeout     int    `required:"false" yaml:"timeout" json:"timeout"`
	Database    int    `required:"false" yaml:"database" json:"database"`
}

func parseConfig(json gjson.Result, config *QuotaConfig) error {
	log.Debugf("parse config()")
	// admin
	config.AdminPath = json.Get("admin_path").String()
	config.AdminConsumer = json.Get("admin_consumer").String()
	if config.AdminPath == "" {
		config.AdminPath = "/quota"
	}
	if config.AdminConsumer == "" {
		return errors.New("missing admin_consumer in config")
	}
	// Redis
	config.RedisKeyPrefix = json.Get("redis_key_prefix").String()
	if config.RedisKeyPrefix == "" {
		config.RedisKeyPrefix = "chat_quota:"
	}
	redisConfig := json.Get("redis")
	if !redisConfig.Exists() {
		return errors.New("missing redis in config")
	}
	serviceName := redisConfig.Get("service_name").String()
	if serviceName == "" {
		return errors.New("redis service name must not be empty")
	}
	servicePort := int(redisConfig.Get("service_port").Int())
	if servicePort == 0 {
		if strings.HasSuffix(serviceName, ".static") {
			// use default logic port which is 80 for static service
			servicePort = 80
		} else {
			servicePort = 6379
		}
	}
	username := redisConfig.Get("username").String()
	password := redisConfig.Get("password").String()
	timeout := int(redisConfig.Get("timeout").Int())
	if timeout == 0 {
		timeout = 1000
	}
	database := int(redisConfig.Get("database").Int())
	config.redisInfo.ServiceName = serviceName
	config.redisInfo.ServicePort = servicePort
	config.redisInfo.Username = username
	config.redisInfo.Password = password
	config.redisInfo.Timeout = timeout
	config.redisInfo.Database = database

	// precision configuration
	precision := json.Get("precision").Int()
	if precision <= 0 {
		precision = 9 // default precision is 9 decimal places (nanounits)
	}
	config.Precision = int(precision)

	config.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: int64(servicePort),
	})

	return config.redisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(database))
}

func onHttpRequestHeaders(context wrapper.HttpContext, config QuotaConfig) types.Action {
	context.DisableReroute()
	log.Debugf("onHttpRequestHeaders()")
	// get tokens
	consumer, err := proxywasm.GetHttpRequestHeader("x-mse-consumer")
	if err != nil {
		return deniedNoKeyAuthData()
	}
	if consumer == "" {
		return deniedUnauthorizedConsumer()
	}

	rawPath := context.Path()
	path, _ := url.Parse(rawPath)
	chatMode, adminMode := getOperationMode(path.Path, config.AdminPath)
	context.SetContext("chatMode", chatMode)
	context.SetContext("adminMode", adminMode)
	context.SetContext("consumer", consumer)
	log.Debugf("chatMode:%s, adminMode:%s, consumer:%s", chatMode, adminMode, consumer)
	if chatMode == ChatModeNone {
		return types.ActionContinue
	}
	if chatMode == ChatModeAdmin {
		// query quota
		if adminMode == AdminModeQuery {
			return queryQuota(context, config, consumer, path)
		}
		if adminMode == AdminModeRefresh || adminMode == AdminModeDelta {
			context.BufferRequestBody()
			return types.HeaderStopIteration
		}
		return types.ActionContinue
	}

	// Buffer request body so we can parse model from it in onHttpRequestBody
	context.BufferRequestBody()

	// Get provider from request
	provider := getProviderFromRequest(context)
	context.SetContext("provider", provider)

	// Check dual quota here using hash structure (before we have model name)
	// We'll update rate in onHttpRequestBody after parsing model from request body
	budgetKey := config.RedisKeyPrefix + consumer

	// Check both token and cost budgets using HMGet
	config.redisClient.HMGet(budgetKey, []string{"token_budget", "cost_budget"}, func(response resp.Value) {
		if err := response.Error(); err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis hmget error:%v", err))
			return
		}

		elements := response.Array()
		tokenBudget := 0
		costBudget := 0

		if len(elements) >= 1 && !elements[0].IsNull() {
			tokenBudget = elements[0].Integer()
		}
		if len(elements) >= 2 && !elements[1].IsNull() {
			costBudget = elements[1].Integer()
		}

		// Check token budget
		if tokenBudget <= 0 {
			util.SendResponse(http.StatusForbidden, "ai-quota.notokenleft", "text/plain", "Request denied by ai quota check, No token left")
			return
		}

		// Check cost budget
		if costBudget <= 0 {
			util.SendResponse(http.StatusForbidden, "ai-quota.nocostleft", "text/plain", "Request denied by ai quota check, No cost left")
			return
		}

		costValue := integerToCost(int64(costBudget), config.Precision)
		log.Debugf("consumer:%s tokenBudget:%d costBudget:%.9f (initial check)", consumer, tokenBudget, costValue)

		// Store initial budget for later use
		context.SetContext("initialTokenBudget", tokenBudget)
		context.SetContext("initialCostBudget", costBudget)

		proxywasm.ResumeHttpRequest()
	})
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpRequestBody(ctx wrapper.HttpContext, config QuotaConfig, body []byte) types.Action {
	log.Debugf("onHttpRequestBody()")
	chatMode, ok := ctx.GetContext("chatMode").(ChatMode)
	if !ok {
		return types.ActionContinue
	}
	if chatMode == ChatModeNone {
		return types.ActionContinue
	}
	if chatMode == ChatModeAdmin {
		adminMode, ok := ctx.GetContext("adminMode").(AdminMode)
		if !ok {
			return types.ActionContinue
		}
		adminConsumer, ok := ctx.GetContext("consumer").(string)
		if !ok {
			return types.ActionContinue
		}

		if adminMode == AdminModeRefresh {
			return refreshQuota(ctx, config, adminConsumer, string(body))
		}
		if adminMode == AdminModeDelta {
			return deltaQuota(ctx, config, adminConsumer, string(body))
		}
		if adminMode == AdminModeSetRate {
			return setRate(ctx, config, adminConsumer, string(body))
		}
		if adminMode == AdminModeGetRate {
			return getRate(ctx, config, adminConsumer, string(body))
		}

		return types.ActionContinue
	}

	// Chat completion mode - parse model from request body and get rate
	if chatMode == ChatModeCompletion {
		modelName := gjson.Get(string(body), "model").String()
		if modelName == "" {
			log.Debugf("onHttpRequestBody: model not found in request body")
			return types.ActionContinue
		}
		ctx.SetContext("modelName", modelName)
		log.Debugf("onHttpRequestBody: parsed model from request body: %s", modelName)

		provider := ctx.GetContext("provider").(string)

		// Get model rate from Redis and store in context
		rateKey := config.RedisKeyPrefix + "rate:model:" + provider + ":" + modelName
		log.Debugf("onHttpRequestBody: trying provider-specific rate key: %s", rateKey)
		config.redisClient.Get(rateKey, func(response resp.Value) {
			var rate ModelRate
			if err := response.Error(); err != nil {
				log.Debugf("onHttpRequestBody: redis error for rate key: %v", err)
			} else if response.IsNull() {
				log.Debugf("onHttpRequestBody: provider-specific rate not found, using zero rate")
			} else {
				if err := json.Unmarshal([]byte(response.String()), &rate); err != nil {
					log.Debugf("onHttpRequestBody: failed to parse rate: %s", response.String())
				} else {
					log.Debugf("onHttpRequestBody: found and parsed rate: %+v", rate)
				}
			}
			log.Debugf("onHttpRequestBody: rate result: %+v", rate)

			ctx.SetContext("modelRate", rate)

			// Update budget check with the actual rate
			consumer := ctx.GetContext("consumer").(string)
			budgetKey := config.RedisKeyPrefix + consumer

			config.redisClient.HMGet(budgetKey, []string{"token_budget", "cost_budget"}, func(response resp.Value) {
				if err := response.Error(); err != nil {
					util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis hmget error:%v", err))
					return
				}

				elements := response.Array()
				tokenBudget := 0
				costBudget := 0

				if len(elements) >= 1 && !elements[0].IsNull() {
					tokenBudget = elements[0].Integer()
				}
				if len(elements) >= 2 && !elements[1].IsNull() {
					costBudget = elements[1].Integer()
				}

				if tokenBudget <= 0 {
					util.SendResponse(http.StatusForbidden, "ai-quota.notokenleft", "text/plain", "Request denied by ai quota check, No token left")
					return
				}

				if costBudget <= 0 {
					util.SendResponse(http.StatusForbidden, "ai-quota.nocostleft", "text/plain", "Request denied by ai quota check, No cost left")
					return
				}

				costValue := integerToCost(int64(costBudget), config.Precision)
				log.Debugf("onHttpRequestBody: consumer:%s tokenBudget:%d costBudget:%.9f (after model parsed)", consumer, tokenBudget, costValue)
				proxywasm.ResumeHttpRequest()
			})
		})
		return types.ActionPause
	}

	return types.ActionContinue
}

// processDeduction handles the cost deduction after rate is retrieved
// Uses hash structure: key=prefix+consumer, fields=token_budget,cost_budget
func processDeduction(ctx wrapper.HttpContext, config QuotaConfig, consumer string, inputToken, outputToken int64, modelName, provider string, rate ModelRate, data []byte) []byte {
	totalCost := calculateCost(inputToken, outputToken, rate)

	totalToken := int(inputToken + outputToken)
	log.Debugf("consumer:%s provider:%s model:%s input:%d output:%d total:%d cost:%d",
		consumer, provider, modelName, inputToken, outputToken, totalToken, totalCost)

	// Use hash structure: key=prefix+consumer, fields=token_budget,cost_budget
	budgetKey := config.RedisKeyPrefix + consumer

	completedOps := 0
	totalOps := 2

	sendResponse := func() {
		completedOps++
		if completedOps == totalOps {
			proxywasm.ResumeHttpResponse()
		}
	}

	// Deduct token budget using HIncrBy (negative increment for deduction)
	err := config.redisClient.HIncrBy(budgetKey, "token_budget", -totalToken,
		func(response resp.Value) {
			if err := response.Error(); err != nil {
				log.Errorf("failed to deduct token budget for consumer:%s, err:%v", consumer, err)
			} else {
				newValue := response.Integer()
				log.Debugf("token budget deducted for consumer:%s, new value:%d", consumer, newValue)
			}
			sendResponse()
		})
	if err != nil {
		log.Errorf("failed to call HIncrBy for token budget for consumer:%s, err:%v", consumer, err)
		sendResponse()
	}

	// Deduct cost budget using HIncrBy (negative increment for deduction)
	err = config.redisClient.HIncrBy(budgetKey, "cost_budget", -int(totalCost),
		func(response resp.Value) {
			if err := response.Error(); err != nil {
				log.Errorf("failed to deduct cost budget for consumer:%s, err:%v", consumer, err)
			} else {
				newValue := response.Integer()
				remainingCost := integerToCost(int64(newValue), config.Precision)
				log.Debugf("cost budget deducted for consumer:%s, cost deducted:%d, remaining budget:%.9f", consumer, totalCost, remainingCost)
			}
			sendResponse()
		})
	if err != nil {
		log.Errorf("failed to call HIncrBy for cost budget for consumer:%s, err:%v", consumer, err)
		sendResponse()
	}

	return data
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config QuotaConfig, data []byte, endOfStream bool) []byte {
	chatMode, ok := ctx.GetContext("chatMode").(ChatMode)
	if !ok {
		return data
	}
	if chatMode == ChatModeNone || chatMode == ChatModeAdmin {
		return data
	}
	// Extract token usage
	if usage := tokenusage.GetTokenUsage(ctx, data); usage.TotalToken > 0 {
		ctx.SetContext(tokenusage.CtxKeyInputToken, usage.InputToken)
		ctx.SetContext(tokenusage.CtxKeyOutputToken, usage.OutputToken)
		ctx.SetContext("modelName", usage.Model)
	}

	// chat completion mode
	if !endOfStream {
		return data
	}

	if ctx.GetContext(tokenusage.CtxKeyInputToken) == nil || ctx.GetContext(tokenusage.CtxKeyOutputToken) == nil || ctx.GetContext("consumer") == nil {
		return data
	}

	ctx.NeedPauseStreamingResponse()

	inputToken := ctx.GetContext(tokenusage.CtxKeyInputToken).(int64)
	outputToken := ctx.GetContext(tokenusage.CtxKeyOutputToken).(int64)
	consumer := ctx.GetContext("consumer").(string)

	// Get model name
	modelName := ""
	if ctx.GetContext("modelName") != nil {
		modelName = ctx.GetContext("modelName").(string)
	}
	if modelName == "" {
		modelName = ctx.GetStringContext("model", "")
	}

	// Get provider
	provider := getProviderFromRequest(ctx)

	// Get model rate from context (already fetched in onHttpRequestHeaders)
	rate := ctx.GetContext("modelRate").(ModelRate)

	log.Debugf("onHttpStreamingResponseBody: got rate from context: %+v", rate)

	// Process deduction with rate from context
	processDeduction(ctx, config, consumer, inputToken, outputToken, modelName, provider, rate, data)

	return data
}

func deniedNoKeyAuthData() types.Action {
	util.SendResponse(http.StatusUnauthorized, "ai-quota.no_key", "text/plain", "Request denied by ai quota check. No Key Authentication information found.")
	return types.ActionContinue
}

func deniedUnauthorizedConsumer() types.Action {
	util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized consumer.")
	return types.ActionContinue
}

// costToInteger converts a cost value to integer representation based on precision
func costToInteger(cost float64, precision int) int64 {
	factor := int64(1)
	for i := 0; i < precision; i++ {
		factor *= 10
	}
	return int64(cost*float64(factor) + 0.5)
}

// stringToIntegerCost converts a cost string to integer representation based on precision
func stringToIntegerCost(costStr string, precision int) (int64, error) {
	costStr = strings.TrimSpace(costStr)
	if costStr == "" {
		return 0, errors.New("empty cost string")
	}

	parts := strings.Split(costStr, ".")
	if len(parts) > 2 {
		return 0, errors.New("invalid cost format")
	}

	integerStr := parts[0]
	decimalStr := ""
	if len(parts) == 2 {
		decimalStr = parts[1]
	}

	var isNegative bool
	if strings.HasPrefix(integerStr, "-") {
		isNegative = true
		integerStr = integerStr[1:]
	}

	integerValue, err := strconv.ParseInt(integerStr, 10, 64)
	if err != nil {
		return 0, err
	}

	factor := int64(1)
	for i := 0; i < precision; i++ {
		factor *= 10
	}

	result := integerValue * factor

	for i := 0; i < len(decimalStr) && i < precision; i++ {
		digit, err := strconv.ParseInt(string(decimalStr[i]), 10, 64)
		if err != nil {
			return 0, err
		}
		remainingPrecision := precision - i - 1
		digitFactor := int64(1)
		for j := 0; j < remainingPrecision; j++ {
			digitFactor *= 10
		}
		result += digit * digitFactor
	}

	if isNegative {
		result = -result
	}

	return result, nil
}

// integerToCost converts an integer cost to float64 representation
func integerToCost(integerCost int64, precision int) float64 {
	factor := float64(1)
	for i := 0; i < precision; i++ {
		factor *= 10
	}
	return float64(integerCost) / factor
}

// getProviderFromRequest extracts provider from request
// It first tries to get from cluster_name, then from x-provider header, defaulting to "openai"
func getProviderFromRequest(ctx wrapper.HttpContext) string {
	// First, try to get provider from cluster_name
	if raw, err := proxywasm.GetProperty([]string{"cluster_name"}); err == nil && len(raw) > 0 {
		clusterName := string(raw)
		log.Debugf("getProviderFromRequest:cluster_name:%s", clusterName)
		// Extract provider from cluster_name
		// Format: outbound|80||llm-svc-{providerName}-internal.static or outbound|80||llm-litellm.internal.static
		if provider := extractProviderFromClusterName(clusterName); provider != "" {
			return provider
		}
	}

	// Second, try to get provider from request header
	if provider, err := proxywasm.GetHttpRequestHeader("x-provider"); err == nil && provider != "" {
		return provider
	}

	// Default to openai if not found
	return "openai"
}

// extractProviderFromClusterName extracts provider name from cluster name
// clusterName formats:
// - outbound|80||llm-{providerName}-internal.static
// - outbound|80||llm-{providerName}-internal.dns
// - outbound|80||llm-{providerName}.internal.static
func extractProviderFromClusterName(clusterName string) string {
	// Find the third "|" to get the service name part
	// Format: outbound|80||service-name
	// We want to find the last part after the last "|"
	serviceName := ""

	// Count the "|" separators
	parts := strings.Split(clusterName, "|")
	if len(parts) >= 4 {
		// Format: outbound|80||service-name
		// parts[0] = "outbound", parts[1] = "80", parts[2] = "", parts[3] = "service-name"
		serviceName = parts[3]
	} else if len(parts) >= 2 {
		// Fallback: get the last part
		serviceName = parts[len(parts)-1]
	}

	if serviceName == "" {
		return ""
	}

	// Try format: llm-{providerName}-internal.static or llm-{providerName}-internal.dns
	if strings.HasPrefix(serviceName, "llm-") {
		// Remove prefix "llm-"
		afterPrefix := strings.TrimPrefix(serviceName, "llm-")
		// Remove suffix "-internal.static" or "-internal.dns"
		if idx := strings.LastIndex(afterPrefix, ".internal"); idx > 0 {
			return afterPrefix[:idx]
		}
	}

	return ""
}

// ModelRate represents a model's rate configuration
type ModelRate struct {
	InputRate  int64 `json:"input_rate"`
	OutputRate int64 `json:"output_rate"`
}

// calculateCost calculates the cost based on input/output tokens and rates
func calculateCost(inputToken, outputToken int64, rate ModelRate) int64 {
	inputCost := inputToken * rate.InputRate
	outputCost := outputToken * rate.OutputRate
	totalCost := (inputCost + outputCost)
	return totalCost
}

func getOperationMode(path string, adminPath string) (ChatMode, AdminMode) {
	fullAdminPath := "/v1/chat/completions" + adminPath
	if strings.HasSuffix(path, fullAdminPath+"/refresh") {
		return ChatModeAdmin, AdminModeRefresh
	}
	if strings.HasSuffix(path, fullAdminPath+"/delta") {
		return ChatModeAdmin, AdminModeDelta
	}
	if strings.HasSuffix(path, fullAdminPath+"/setrate") {
		return ChatModeAdmin, AdminModeSetRate
	}
	if strings.HasSuffix(path, fullAdminPath+"/getrate") {
		return ChatModeAdmin, AdminModeGetRate
	}
	if strings.HasSuffix(path, fullAdminPath) {
		return ChatModeAdmin, AdminModeQuery
	}
	if strings.HasSuffix(path, "/v1/chat/completions") {
		return ChatModeCompletion, AdminModeNone
	}
	return ChatModeNone, AdminModeNone
}

func refreshQuota(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, body string) types.Action {
	log.Debugf("[refreshQuota] START: consumer=%s", adminConsumer)

	// check consumer
	if adminConsumer != config.AdminConsumer {
		log.Debugf("[refreshQuota] Unauthorized admin consumer: %s != %s", adminConsumer, config.AdminConsumer)
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}

	log.Debugf("[refreshQuota] Parsing query body: %s", body)
	queryValues, _ := url.ParseQuery(body)
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}
	queryConsumer := values["consumer"]
	log.Debugf("[refreshQuota] Target consumer: %s", queryConsumer)
	if queryConsumer == "" {
		log.Debugf("[refreshQuota] Empty consumer parameter")
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. consumer can't be empty.")
		return types.ActionContinue
	}

	// Parse token budget if provided
	tokenBudget := 0
	tokenBudgetSet := false
	if tokenStr := values["token_budget"]; tokenStr != "" {
		log.Debugf("[refreshQuota] Parsing token_budget: %s", tokenStr)
		if val, err := strconv.Atoi(tokenStr); err == nil && val >= 0 {
			tokenBudget = val
			tokenBudgetSet = true
			log.Debugf("[refreshQuota] Token budget parsed: %d", tokenBudget)
		} else {
			log.Debugf("[refreshQuota] Invalid token_budget format")
			util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. token_budget must be non-negative integer.")
			return types.ActionContinue
		}
	}

	// Parse cost budget if provided
	costBudget := int64(0)
	costBudgetSet := false
	if costStr := values["cost_budget"]; costStr != "" {
		log.Debugf("[refreshQuota] Parsing cost_budget: %s", costStr)
		val, err := stringToIntegerCost(costStr, config.Precision)
		if err == nil && val >= 0 {
			costBudget = val
			costBudgetSet = true
			costValue := integerToCost(costBudget, config.Precision)
			log.Debugf("[refreshQuota] Cost budget parsed: %.9f (int: %d)", costValue, costBudget)
		} else {
			log.Debugf("[refreshQuota] Invalid cost_budget format: %v", err)
			util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. cost_budget must be non-negative cost value.")
			return types.ActionContinue
		}
	}

	if !tokenBudgetSet && !costBudgetSet {
		log.Debugf("[refreshQuota] No budget parameters set")
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. token_budget or cost_budget must be set.")
		return types.ActionContinue
	}

	// Use hash structure: key=prefix+consumer, fields=token_budget,cost_budget
	budgetKey := config.RedisKeyPrefix + queryConsumer
	log.Debugf("[refreshQuota] Using hash key=%s", budgetKey)

	// Prepare HMSet data
	kvMap := make(map[string]interface{})
	if tokenBudgetSet {
		kvMap["token_budget"] = tokenBudget
		log.Debugf("[refreshQuota] Adding token_budget=%d to hash", tokenBudget)
	}
	if costBudgetSet {
		kvMap["cost_budget"] = int(costBudget)
		log.Debugf("[refreshQuota] Adding cost_budget=%d to hash", costBudget)
	}

	// Use HMSet for batch operation
	log.Debugf("[refreshQuota] Calling HMSet for hash key=%s", budgetKey)
	err := config.redisClient.HMSet(budgetKey, kvMap, func(response resp.Value) {
		log.Debugf("[refreshQuota] HMSet callback started")
		if err := response.Error(); err != nil {
			log.Debugf("[refreshQuota] HMSet error: %v", err)
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis hmset error:%v", err))
			return
		}

		if tokenBudgetSet {
			log.Debugf("set token_budget:%d for consumer:%s", tokenBudget, queryConsumer)
		}
		if costBudgetSet {
			costValue := integerToCost(costBudget, config.Precision)
			log.Debugf("set cost_budget:%.9f for consumer:%s", costValue, queryConsumer)
		}
		log.Debugf("[refreshQuota] HMSet success, sending response")
		util.SendResponse(http.StatusOK, "ai-quota.refreshbudget", "text/plain", "refresh budget successful")
	})

	if err != nil {
		log.Debugf("[refreshQuota] HMSet call failed: %v", err)
		util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis hmset error:%v", err))
		return types.ActionContinue
	}

	log.Debugf("[refreshQuota] HMSet called, returning ActionPause")
	return types.ActionPause
}

func queryQuota(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, url *url.URL) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}
	// check url
	queryValues := url.Query()
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}
	if values["consumer"] == "" {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. consumer can't be empty.")
		return types.ActionContinue
	}
	queryConsumer := values["consumer"]

	// Use hash structure: key=prefix+consumer, fields=token_budget,cost_budget
	budgetKey := config.RedisKeyPrefix + queryConsumer
	log.Debugf("[queryQuota] Using hash key=%s", budgetKey)

	// Use HMGet for batch operation
	log.Debugf("[queryQuota] Calling HMGet for hash key=%s", budgetKey)
	err := config.redisClient.HMGet(budgetKey, []string{"token_budget", "cost_budget"}, func(response resp.Value) {
		log.Debugf("[queryQuota] HMGet callback started")
		if err := response.Error(); err != nil {
			log.Debugf("[queryQuota] HMGet error: %v", err)
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis hmget error:%v", err))
			return
		}

		// Parse HMGet response - response is an array
		tokenBudget := 0
		costBudget := 0

		elements := response.Array()
		if len(elements) >= 1 && !elements[0].IsNull() {
			tokenBudget = elements[0].Integer()
			log.Debugf("[queryQuota] Token budget from HMGet: %d", tokenBudget)
		}
		if len(elements) >= 2 && !elements[1].IsNull() {
			costBudget = elements[1].Integer()
			log.Debugf("[queryQuota] Cost budget from HMGet: %d", costBudget)
		}

		result := struct {
			Consumer    string  `json:"consumer"`
			TokenBudget int     `json:"token_budget"`
			CostBudget  float64 `json:"cost_budget"`
		}{
			Consumer:    queryConsumer,
			TokenBudget: tokenBudget,
			CostBudget:  integerToCost(int64(costBudget), config.Precision),
		}
		body, _ := json.Marshal(result)
		log.Debugf("[queryQuota] Sending response: %s", string(body))
		util.SendResponse(http.StatusOK, "ai-quota.querybudget", "application/json", string(body))
	})

	if err != nil {
		log.Debugf("[queryQuota] HMGet call failed: %v", err)
		util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis hmget error:%v", err))
		return types.ActionContinue
	}

	log.Debugf("[queryQuota] HMGet called, returning ActionPause")
	return types.ActionPause
}

func deltaQuota(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, body string) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}

	queryValues, _ := url.ParseQuery(body)
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}
	queryConsumer := values["consumer"]
	if queryConsumer == "" {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. consumer can't be empty.")
		return types.ActionContinue
	}

	// Parse token budget delta if provided
	tokenDelta := 0
	tokenDeltaSet := false
	if tokenStr := values["token_budget_delta"]; tokenStr != "" {
		if val, err := strconv.Atoi(tokenStr); err == nil {
			tokenDelta = val
			tokenDeltaSet = true
		} else {
			util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. token_budget_delta must be integer.")
			return types.ActionContinue
		}
	}

	// Parse cost budget delta if provided
	costDelta := int64(0)
	costDeltaSet := false
	if costStr := values["cost_budget_delta"]; costStr != "" {
		val, err := stringToIntegerCost(costStr, config.Precision)
		if err == nil {
			costDelta = val
			costDeltaSet = true
		} else {
			util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. cost_budget_delta must be valid cost value.")
			return types.ActionContinue
		}
	}

	if !tokenDeltaSet && !costDeltaSet {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. token_budget_delta or cost_budget_delta must be set.")
		return types.ActionContinue
	}

	// Use hash structure: key=prefix+consumer, fields=token_budget,cost_budget
	budgetKey := config.RedisKeyPrefix + queryConsumer
	log.Debugf("[deltaQuota] Using hash key=%s", budgetKey)

	completedOps := 0
	totalOps := 0
	var firstError error
	if tokenDeltaSet {
		totalOps++
	}
	if costDeltaSet {
		totalOps++
	}
	log.Debugf("[deltaQuota] Starting HIncrBy operations, totalOps=%d", totalOps)

	sendResponseGuard := func() {
		log.Debugf("[deltaQuota] Checking if all ops completed: completedOps=%d totalOps=%d", completedOps, totalOps)
		if completedOps == totalOps {
			log.Debugf("[deltaQuota] All ops completed, firstError=%v", firstError)
			if firstError != nil {
				log.Debugf("[deltaQuota] Sending error response: %v", firstError)
				util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", firstError))
			} else {
				log.Debugf("[deltaQuota] Sending success response")
				util.SendResponse(http.StatusOK, "ai-quota.deltabudget", "text/plain", "delta budget successful")
			}
		}
	}

	// Process token budget delta using HIncrBy
	if tokenDeltaSet {
		if tokenDelta >= 0 {
			log.Debugf("[deltaQuota] HIncrBy token_budget field, key=%s delta=%d", budgetKey, tokenDelta)
			config.redisClient.HIncrBy(budgetKey, "token_budget", tokenDelta,
				func(response resp.Value) {
					log.Debugf("[deltaQuota] HIncrBy token_budget callback started")
					if err := response.Error(); err != nil {
						log.Debugf("[deltaQuota] HIncrBy token_budget error: %v", err)
						if firstError == nil {
							firstError = fmt.Errorf("token budget incr: %v", err)
						}
					} else {
						newVal := response.Integer()
						log.Debugf("[deltaQuota] HIncrBy token_budget success, new value=%d", newVal)
						log.Debugf("Redis HIncrBy token_budget key=%s field=token_budget delta=%d, new value=%d", budgetKey, tokenDelta, newVal)
					}
					completedOps++
					sendResponseGuard()
				})
		} else {
			log.Debugf("[deltaQuota] HIncrBy token_budget field (negative), key=%s delta=%d", budgetKey, tokenDelta)
			config.redisClient.HIncrBy(budgetKey, "token_budget", tokenDelta,
				func(response resp.Value) {
					log.Debugf("[deltaQuota] HIncrBy token_budget callback started")
					if err := response.Error(); err != nil {
						log.Debugf("[deltaQuota] HIncrBy token_budget error: %v", err)
						if firstError == nil {
							firstError = fmt.Errorf("token budget decr: %v", err)
						}
					} else {
						newVal := response.Integer()
						log.Debugf("[deltaQuota] HIncrBy token_budget success, new value=%d", newVal)
						log.Debugf("Redis HIncrBy token_budget key=%s field=token_budget delta=%d, new value=%d", budgetKey, tokenDelta, newVal)
					}
					completedOps++
					sendResponseGuard()
				})
		}
	}

	// Process cost budget delta using HIncrBy
	if costDeltaSet {
		if costDelta >= 0 {
			log.Debugf("[deltaQuota] HIncrBy cost_budget field, key=%s delta=%d", budgetKey, costDelta)
			config.redisClient.HIncrBy(budgetKey, "cost_budget", int(costDelta),
				func(response resp.Value) {
					log.Debugf("[deltaQuota] HIncrBy cost_budget callback started")
					if err := response.Error(); err != nil {
						log.Debugf("[deltaQuota] HIncrBy cost_budget error: %v", err)
						if firstError == nil {
							firstError = fmt.Errorf("cost budget incr: %v", err)
						}
					} else {
						newVal := response.Integer()
						log.Debugf("[deltaQuota] HIncrBy cost_budget success, new value=%d", newVal)
						log.Debugf("Redis HIncrBy cost_budget key=%s field=cost_budget delta=%d, new value=%d", budgetKey, costDelta, newVal)
					}
					completedOps++
					sendResponseGuard()
				})
		} else {
			log.Debugf("[deltaQuota] HIncrBy cost_budget field (negative), key=%s delta=%d", budgetKey, costDelta)
			config.redisClient.HIncrBy(budgetKey, "cost_budget", int(costDelta),
				func(response resp.Value) {
					log.Debugf("[deltaQuota] HIncrBy cost_budget callback started")
					if err := response.Error(); err != nil {
						log.Debugf("[deltaQuota] HIncrBy cost_budget error: %v", err)
						if firstError == nil {
							firstError = fmt.Errorf("cost budget decr: %v", err)
						}
					} else {
						newVal := response.Integer()
						log.Debugf("[deltaQuota] HIncrBy cost_budget success, new value=%d", newVal)
						log.Debugf("Redis HIncrBy cost_budget key=%s field=cost_budget delta=%d, new value=%d", budgetKey, costDelta, newVal)
					}
					completedOps++
					sendResponseGuard()
				})
		}
	}

	return types.ActionPause
}

// setRate handles setting model rates
func setRate(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, body string) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}

	queryValues, _ := url.ParseQuery(body)
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}

	// Get provider and model
	provider := values["provider"]
	model := values["model"]

	inputRateStr := values["input_rate"]
	outputRateStr := values["output_rate"]

	inputRate, err1 := stringToIntegerCost(inputRateStr, config.Precision)
	outputRate, err2 := stringToIntegerCost(outputRateStr, config.Precision)

	if err1 != nil || err2 != nil {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. input_rate and output_rate must be valid cost values.")
		return types.ActionContinue
	}

	rate := ModelRate{
		InputRate:  inputRate,
		OutputRate: outputRate,
	}

	// Marshal to JSON
	rateJSON, err := json.Marshal(rate)
	if err != nil {
		util.SendResponse(http.StatusInternalServerError, "ai-quota.error", "text/plain", "Failed to marshal rate to JSON.")
		return types.ActionContinue
	}

	// Determine Redis key - only support provider-specific model rate
	rateKey := config.RedisKeyPrefix + "rate:model:" + provider + ":" + model

	// Store in Redis
	err = config.redisClient.Set(rateKey, string(rateJSON), func(response resp.Value) {
		if err := response.Error(); err != nil {
			util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
			return
		}

		inputRateDisplay := integerToCost(rate.InputRate, config.Precision)
		outputRateDisplay := integerToCost(rate.OutputRate, config.Precision)
		log.Debugf("Set rate for key %s: input=%.9f output=%.9f", rateKey, inputRateDisplay, outputRateDisplay)
		util.SendResponse(http.StatusOK, "ai-quota.setrate", "text/plain", "set rate successful")
	})

	if err != nil {
		util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
		return types.ActionContinue
	}

	return types.ActionPause
}

// getRate handles querying model rates
func getRate(ctx wrapper.HttpContext, config QuotaConfig, adminConsumer string, body string) types.Action {
	// check consumer
	if adminConsumer != config.AdminConsumer {
		util.SendResponse(http.StatusForbidden, "ai-quota.unauthorized", "text/plain", "Request denied by ai quota check. Unauthorized admin consumer.")
		return types.ActionContinue
	}

	queryValues, _ := url.ParseQuery(body)
	values := make(map[string]string, len(queryValues))
	for k, v := range queryValues {
		values[k] = v[0]
	}

	// Get provider and model
	provider := values["provider"]
	model := values["model"]

	// Determine Redis key - only support provider-specific model rate
	rateKey := config.RedisKeyPrefix + "rate:model:" + provider + ":" + model

	// Get from Redis
	err := config.redisClient.Get(rateKey, func(response resp.Value) {
		var rate ModelRate

		if err := response.Error(); err != nil || response.IsNull() {
			// Provider-specific rate not found, use zero rate
			log.Debugf("getRate: provider-specific rate not found, using zero rate")
			rate = ModelRate{InputRate: 0, OutputRate: 0}
		} else {
			if err := json.Unmarshal([]byte(response.String()), &rate); err != nil {
				log.Debugf("getRate: failed to parse rate, using zero rate")
				rate = ModelRate{InputRate: 0, OutputRate: 0}
			}
		}

		returnResponse(ctx, config, provider, model, rate)
	})

	if err != nil {
		util.SendResponse(http.StatusServiceUnavailable, "ai-quota.error", "text/plain", fmt.Sprintf("redis error:%v", err))
		return types.ActionContinue
	}

	return types.ActionPause
}

// returnResponse returns the rate response
func returnResponse(ctx wrapper.HttpContext, config QuotaConfig, provider, model string, rate ModelRate) {
	result := struct {
		Provider   string  `json:"provider"`
		Model      string  `json:"model"`
		InputRate  float64 `json:"input_rate"`
		OutputRate float64 `json:"output_rate"`
	}{
		Provider:   provider,
		Model:      model,
		InputRate:  integerToCost(rate.InputRate, config.Precision),
		OutputRate: integerToCost(rate.OutputRate, config.Precision),
	}

	body, _ := json.Marshal(result)
	util.SendResponse(http.StatusOK, "ai-quota.getrate", "application/json", string(body))
}
