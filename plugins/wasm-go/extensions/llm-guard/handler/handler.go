package handler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/llm-guard/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type LLMGuardHandler struct{}

var Handler = &LLMGuardHandler{}

func (h *LLMGuardHandler) GenerateRandomChatID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 29)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "chatcmpl-" + string(b)
}

func HandleRequestBody(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, body []byte) types.Action {
	content := gjson.GetBytes(body, config.RequestJsonPath).String()

	if len(content) == 0 {
		log.Warnf("Extracted request content is empty using path: %s. Skip LLM Guard check.", config.RequestJsonPath)
		return types.ActionContinue
	}
	log.Debugf("Raw request content for LLM Guard check: %s", content)

	startTime := time.Now().UnixMilli()

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("LLM Guard analyze response status: %d, body: %s", statusCode, string(responseBody))

		// 200 means safe or sanitized, 422 means risk detected but result is valid
		if statusCode != 200 && statusCode != 422 {
			log.Errorf("LLM Guard analyze failed with status: %d, response: %s", statusCode, string(responseBody))
			proxywasm.ResumeHttpRequest()
			return
		}

		res := gjson.ParseBytes(responseBody)
		isValid := true
		if val := res.Get("is_valid"); val.Exists() {
			isValid = val.Bool()
		}

		if !isValid {
			shouldBlock := false
			shouldMask := false
			triggeredScanners := make([]string, 0)

			scanners := res.Get("scanners")
			log.Infof("LLM Guard found risks, analyzing scanners: %s", scanners.Raw)

			if scanners.IsObject() {
				scanners.ForEach(func(key, value gjson.Result) bool {
					scannerName := key.String()
					// Default action for scanner
					scannerAction, ok := config.ScannerActions[scannerName]
					if !ok {
						scannerAction = config.DefaultAction
					}

					log.Infof("Processing scanner: %s, configured action: %s", scannerName, scannerAction)

					if scannerName == "Anonymize" && value.Get("detected_entities").Exists() {
						entities := value.Get("detected_entities").Array()
						log.Infof("Anonymize scanner triggered with %d entities", len(entities))
						for _, entity := range entities {
							entityType := entity.String()
							action, ok := config.EntityActions[entityType]
							if !ok {
								action = scannerAction
							}
							log.Infof("  Entity: %s, matched action: %s", entityType, action)
							if action == cfg.ActionBlock {
								shouldBlock = true
							} else if action == cfg.ActionMask {
								shouldMask = true
							}
							triggeredScanners = append(triggeredScanners, fmt.Sprintf("Anonymize(%s:%s)", entityType, action))
						}
					} else {
						// For non-Anonymize scanners or when details are simple
						score := 0.0
						if value.Type == gjson.Number {
							score = value.Float()
						} else {
							score = value.Get("score").Float()
						}

						log.Infof("  Scanner %s score: %f", scannerName, score)
						if score > 0 {
							if scannerAction == cfg.ActionBlock {
								shouldBlock = true
							} else if scannerAction == cfg.ActionMask {
								shouldMask = true
							}
							triggeredScanners = append(triggeredScanners, fmt.Sprintf("%s:%s", scannerName, scannerAction))
						}
					}
					return true
				})
			}

			log.Infof("Decision result - shouldBlock: %v, shouldMask: %v, triggered: %v", shouldBlock, shouldMask, triggeredScanners)

			if shouldBlock {
				Handler.sendDenyResponse(ctx, config, triggeredScanners, startTime)
				return
			}

			if shouldMask {
				sanitizedPrompt := res.Get("sanitized_prompt").String()
				Handler.replaceRequestContent(ctx, config, body, sanitizedPrompt, triggeredScanners, startTime)
				return
			}
		}

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("llm_guard_request_rt", endTime-startTime)
		ctx.SetUserAttribute("llm_guard_status", "request pass")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpRequest()
	}

	analyzeReq := cfg.AnalyzePromptRequest{
		Prompt:               content,
		ScannersInclude:      config.ScannersInclude,
		ShowDetails:          true,
		AnonymizeEntityTypes: config.AnonymizeEntityTypes,
	}

	reqBody, _ := json.Marshal(analyzeReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	err := config.ServiceClient.Post(config.EndpointAnalyzePrompt, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call LLM Guard analyze service: %v", err)
		proxywasm.ResumeHttpRequest()
	}

	return types.ActionPause
}

func (h *LLMGuardHandler) replaceRequestContent(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, originalBody []byte, sanitizedPrompt string, triggeredScanners []string, startTime int64) {
	modifiedBody := h.replaceContentInJSON(originalBody, config.RequestJsonPath, sanitizedPrompt)
	proxywasm.ReplaceHttpRequestBody(modifiedBody)

	endTime := time.Now().UnixMilli()
	ctx.SetUserAttribute("llm_guard_request_rt", endTime-startTime)
	ctx.SetUserAttribute("llm_guard_status", "request sanitized")
	ctx.SetUserAttribute("llm_guard_scanners", strings.Join(triggeredScanners, ","))
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	proxywasm.ResumeHttpRequest()
}

func (h *LLMGuardHandler) replaceContentInJSON(originalBody []byte, jsonPath string, newContent string) []byte {
	if jsonPath == "" {
		return originalBody
	}

	result := gjson.GetBytes(originalBody, jsonPath)
	if !result.Exists() {
		log.Debugf("Path %s does not exist in JSON", jsonPath)
		return originalBody
	}

	replacePath := jsonPath
	if strings.Contains(jsonPath, "@reverse") {
		messagesResult := gjson.GetBytes(originalBody, "messages")
		if messagesResult.Exists() && messagesResult.Type == gjson.JSON {
			messages := messagesResult.Array()
			if len(messages) > 0 {
				lastIndex := len(messages) - 1
				replacePath = strings.Replace(jsonPath, "@reverse.0", fmt.Sprintf("%d", lastIndex), 1)
			}
		}
	}

	modifiedBody, err := sjson.SetBytes(originalBody, replacePath, newContent)
	if err != nil {
		log.Errorf("Failed to replace content in JSON: %v", err)
		return originalBody
	}

	return modifiedBody
}

func (h *LLMGuardHandler) sendDenyResponse(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, triggeredScanners []string, startTime int64) {
	randomID := h.GenerateRandomChatID()
	denyMessage := config.DenyMessage
	if denyMessage == "" {
		denyMessage = cfg.DefaultDenyMessage
	}

	marshalledDenyMessage := escapeJSONString(denyMessage)
	jsonData := []byte(fmt.Sprintf(cfg.OpenAIResponseFormat, randomID, marshalledDenyMessage))

	proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, jsonData, -1)

	ctx.DontReadResponseBody()
	config.IncrementCounter("llm_guard_request_deny", 1)

	endTime := time.Now().UnixMilli()
	ctx.SetUserAttribute("llm_guard_request_rt", endTime-startTime)
	ctx.SetUserAttribute("llm_guard_status", "request deny")
	ctx.SetUserAttribute("llm_guard_scanners", strings.Join(triggeredScanners, ","))
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
}

func escapeJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}
