package config

import (
	"errors"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	ActionMask  = "MASK"
	ActionBlock = "BLOCK"

	ScopeInput  = "input"
	ScopeOutput = "output"
	ScopeBoth   = "both"

	DefaultTimeout     = 5000
	DefaultDenyCode    = 200
	DefaultDenyMessage = "内容包含风险，已被拦截"

	DefaultRequestJsonPath  = "messages.@reverse.0.content"
	DefaultResponseJsonPath = "choices.0.message.content"

	OpenAIResponseFormat = `{"id":"%s","object":"chat.completion","model":"from-llm-guard","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`

	LLMGuardEndpointAnalyzePrompt = "/analyze/prompt"
	LLMGuardEndpointScanPrompt    = "/scan/prompt"
)

type ScannerConfig struct {
	Name     string
	Action   string
	Entities []EntityConfig
}

type EntityConfig struct {
	EntityType string
	Action     string
}

type LLMGuardConfig struct {
	ServiceName   string
	ServicePort   int64
	ServiceHost   string
	ServiceClient wrapper.HttpClient

	EndpointAnalyzePrompt string
	EndpointScanPrompt    string

	DefaultAction string

	RequestJsonPath  string
	ResponseJsonPath string

	DenyCode    int64
	DenyMessage string
	Timeout     uint32
	Metrics     map[string]proxywasm.MetricCounter

	ScannersInclude      []string
	AnonymizeEntityTypes []string
	ScannerActions       map[string]string
	EntityActions        map[string]string
}

type AnalyzePromptRequest struct {
	Prompt               string   `json:"prompt"`
	ScannersInclude      []string `json:"scanners_include,omitempty"`
	ShowDetails          bool     `json:"show_details,omitempty"`
	AnonymizeEntityTypes []string `json:"anonymize_entity_types,omitempty"`
}

type AnalyzePromptResponse struct {
	SanitizedPrompt string                 `json:"sanitized_prompt"`
	IsValid         bool                   `json:"is_valid"`
	Scanners        map[string]interface{} `json:"scanners"`
}

type ScanPromptRequest struct {
	Prompt           string   `json:"prompt"`
	ScannersSuppress []string `json:"scanners_suppress,omitempty"`
}

type ScanPromptResponse struct {
	IsValid  bool               `json:"is_valid"`
	Scanners map[string]float64 `json:"scanners"`
}

func (c *LLMGuardConfig) Parse(json gjson.Result) error {
	serviceName := json.Get("serviceName").String()
	servicePort := json.Get("servicePort").Int()
	serviceHost := json.Get("serviceHost").String()

	if serviceName == "" || servicePort == 0 || serviceHost == "" {
		return errors.New("invalid service config: serviceName, servicePort, and serviceHost are required")
	}

	c.ServiceName = serviceName
	c.ServicePort = servicePort
	c.ServiceHost = serviceHost

	c.EndpointAnalyzePrompt = json.Get("endpointAnalyzePrompt").String()
	if c.EndpointAnalyzePrompt == "" {
		c.EndpointAnalyzePrompt = LLMGuardEndpointAnalyzePrompt
	}

	c.EndpointScanPrompt = json.Get("endpointScanPrompt").String()
	if c.EndpointScanPrompt == "" {
		c.EndpointScanPrompt = LLMGuardEndpointScanPrompt
	}

	c.DefaultAction = strings.ToUpper(json.Get("defaultAction").String())
	if c.DefaultAction == "" {
		c.DefaultAction = ActionMask
	}

	c.RequestJsonPath = json.Get("requestJsonPath").String()
	if c.RequestJsonPath == "" {
		c.RequestJsonPath = DefaultRequestJsonPath
	}

	c.ResponseJsonPath = json.Get("responseJsonPath").String()
	if c.ResponseJsonPath == "" {
		c.ResponseJsonPath = DefaultResponseJsonPath
	}

	if obj := json.Get("denyCode"); obj.Exists() {
		c.DenyCode = obj.Int()
	} else {
		c.DenyCode = DefaultDenyCode
	}

	c.DenyMessage = json.Get("denyMessage").String()
	if c.DenyMessage == "" {
		c.DenyMessage = DefaultDenyMessage
	}

	if obj := json.Get("timeout"); obj.Exists() {
		c.Timeout = uint32(obj.Int())
	} else {
		c.Timeout = DefaultTimeout
	}

	c.ScannerActions = make(map[string]string)
	c.EntityActions = make(map[string]string)
	if scanners := json.Get("scanners"); scanners.Exists() && scanners.IsArray() {
		for _, scanner := range scanners.Array() {
			name := scanner.Get("name").String()
			action := strings.ToUpper(scanner.Get("action").String())
			if name != "" {
				c.ScannersInclude = append(c.ScannersInclude, name)
				if action != "" {
					c.ScannerActions[name] = action
				}
				if name == "Anonymize" {
					if entities := scanner.Get("entities"); entities.Exists() && entities.IsArray() {
						for _, entity := range entities.Array() {
							eType := entity.Get("entityType").String()
							eAction := strings.ToUpper(entity.Get("action").String())
							if eType != "" {
								c.AnonymizeEntityTypes = append(c.AnonymizeEntityTypes, eType)
								if eAction != "" {
									c.EntityActions[eType] = eAction
								}
							}
						}
					}
				}
			}
		}
	}

	c.ServiceClient = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
		Host: serviceHost,
	})

	c.Metrics = make(map[string]proxywasm.MetricCounter)
	return nil
}

func (c *LLMGuardConfig) IncrementCounter(metricName string, inc uint64) {
	counter, ok := c.Metrics[metricName]
	if !ok {
		counter = proxywasm.DefineCounterMetric(metricName)
		c.Metrics[metricName] = counter
	}
	counter.Increment(inc)
}

func MaskAction(action string) bool {
	return strings.ToUpper(action) == ActionMask
}

func BlockAction(action string) bool {
	return strings.ToUpper(action) == ActionBlock
}
