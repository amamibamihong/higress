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
	ActionNone  = "NONE"

	FilterScopeInput  = "input"
	FilterScopeOutput = "output"
	FilterScopeBoth   = "both"

	AnonymizerHash     = "hash"
	AnonymizerAsterisk = "asterisk"
	AnonymizerRedact   = "redact"

	DefaultTimeout     = 2000
	DefaultBufferLimit = 1000
	DefaultDenyCode    = 200
	DefaultDenyMessage = "很抱歉，内容包含敏感信息"
	DefaultLanguage    = "zh"

	DefaultRequestJsonPath           = "messages.@reverse.0.content"
	DefaultResponseJsonPath          = "choices.0.message.content"
	DefaultStreamingResponseJsonPath = "choices.0.delta.content"

	OpenAIResponseFormat       = `{"id":"%s","object":"chat.completion","model":"from-presidio-pii","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	OpenAIStreamResponseChunk  = `data:{"id":"%s","object":"chat.completion.chunk","model":"from-presidio-pii","choices":[{"index":0,"delta":{"content":"%s"},"logprobs":null,"finish_reason":null}]}`
	OpenAIStreamResponseEnd    = `data:{"id":"%s","object":"chat.completion.chunk","model":"from-presidio-pii","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}`
	OpenAIStreamResponseFormat = OpenAIStreamResponseChunk + "\n\n" + OpenAIStreamResponseEnd + "\n\ndata: [DONE]"
)

type PIIEntity struct {
	EntityType     string  `json:"entityType"`
	Action         string  `json:"action"`
	ScoreThreshold float64 `json:"scoreThreshold"`
}

type PresidioPIIConfig struct {
	AnalyzerServiceName string
	AnalyzerServicePort int64
	AnalyzerServiceHost string
	AnalyzerPath        string
	AnalyzerClient      wrapper.HttpClient

	AnonymizerServiceName string
	AnonymizerServicePort int64
	AnonymizerServiceHost string
	AnonymizerPath        string
	AnonymizerClient      wrapper.HttpClient

	CheckRequest                  bool
	CheckResponse                 bool
	FilterScope                   string
	Language                      string
	RequestContentJsonPath        string
	ResponseContentJsonPath       string
	ResponseStreamContentJsonPath string
	Entities                      []PIIEntity
	DefaultAction                 string
	DefaultScoreThreshold         float64
	Anonymizer                    string
	DenyCode                      int64
	DenyMessage                   string
	ProtocolOriginal              bool
	Timeout                       uint32
	BufferLimit                   int
	Metrics                       map[string]proxywasm.MetricCounter
}

type AnalyzeRequest struct {
	Text           string   `json:"text"`
	Entities       []string `json:"entities"`
	Language       string   `json:"language"`
	ScoreThreshold float64  `json:"scoreThreshold"`
}

type AnalyzeResponse []AnalyzeResult

type AnalyzeResult struct {
	EntityType string  `json:"entity_type"`
	Start      int     `json:"start"`
	End        int     `json:"end"`
	Score      float64 `json:"score"`
}

type AnonymizeRequest struct {
	Text             string                      `json:"text"`
	AnonymizeResults []AnonymizeResult           `json:"anonymize_results"`
	Anonymizers      map[string]AnonymizerConfig `json:"anonymizers"`
}

type AnonymizeResult struct {
	Start      int    `json:"start"`
	End        int    `json:"end"`
	EntityType string `json:"entity_type"`
}

type AnonymizerConfig struct {
	Type     string `json:"type"`
	HashType string `json:"hash_type,omitempty"`
}

type AnonymizeResponse struct {
	Text  string                  `json:"text"`
	Items []AnonymizeResponseItem `json:"items"`
}

type AnonymizeResponseItem struct {
	Start      int    `json:"start"`
	End        int    `json:"end"`
	EntityType string `json:"entity_type"`
	Anonymizer string `json:"anonymizer"`
}

func (config *PresidioPIIConfig) Parse(json gjson.Result) error {
	analyzerServiceName := json.Get("analyzerServiceName").String()
	analyzerServicePort := json.Get("analyzerServicePort").Int()
	analyzerServiceHost := json.Get("analyzerServiceHost").String()
	analyzerPath := json.Get("analyzerPath").String()
	if analyzerPath == "" {
		analyzerPath = "/"
	}

	anonymizerServiceName := json.Get("anonymizerServiceName").String()
	anonymizerServicePort := json.Get("anonymizerServicePort").Int()
	anonymizerServiceHost := json.Get("anonymizerServiceHost").String()
	anonymizerPath := json.Get("anonymizerPath").String()
	if anonymizerPath == "" {
		anonymizerPath = "/"
	}

	if analyzerServiceName == "" || analyzerServicePort == 0 || analyzerServiceHost == "" {
		return errors.New("invalid analyzer service config: analyzerServiceName, analyzerServicePort, and analyzerServiceHost are required")
	}

	if anonymizerServiceName == "" || anonymizerServicePort == 0 || anonymizerServiceHost == "" {
		return errors.New("invalid anonymizer service config: anonymizerServiceName, anonymizerServicePort, and anonymizerServiceHost are required")
	}

	config.AnalyzerServiceName = analyzerServiceName
	config.AnalyzerServicePort = analyzerServicePort
	config.AnalyzerServiceHost = analyzerServiceHost
	config.AnalyzerPath = analyzerPath

	config.AnonymizerServiceName = anonymizerServiceName
	config.AnonymizerServicePort = anonymizerServicePort
	config.AnonymizerServiceHost = anonymizerServiceHost
	config.AnonymizerPath = anonymizerPath

	config.DefaultAction = json.Get("defaultAction").String()
	if config.DefaultAction == "" {
		config.DefaultAction = ActionMask
	}

	config.DefaultScoreThreshold = json.Get("defaultScoreThreshold").Float()
	if config.DefaultScoreThreshold == 0 {
		config.DefaultScoreThreshold = 0.85
	}

	config.CheckRequest = json.Get("checkRequest").Bool()
	config.CheckResponse = json.Get("checkResponse").Bool()

	config.FilterScope = json.Get("filterScope").String()
	if config.FilterScope == "" {
		config.FilterScope = FilterScopeBoth
	}

	config.Language = json.Get("language").String()
	if config.Language == "" {
		config.Language = DefaultLanguage
	}

	config.RequestContentJsonPath = json.Get("requestContentJsonPath").String()
	if config.RequestContentJsonPath == "" {
		config.RequestContentJsonPath = DefaultRequestJsonPath
	}

	config.ResponseContentJsonPath = json.Get("responseContentJsonPath").String()
	if config.ResponseContentJsonPath == "" {
		config.ResponseContentJsonPath = DefaultResponseJsonPath
	}

	config.ResponseStreamContentJsonPath = json.Get("responseStreamContentJsonPath").String()
	if config.ResponseStreamContentJsonPath == "" {
		config.ResponseStreamContentJsonPath = DefaultStreamingResponseJsonPath
	}

	config.ProtocolOriginal = json.Get("protocol").String() == "original"
	config.DenyMessage = json.Get("denyMessage").String()
	if config.DenyMessage == "" {
		config.DenyMessage = DefaultDenyMessage
	}

	if obj := json.Get("denyCode"); obj.Exists() {
		config.DenyCode = obj.Int()
	} else {
		config.DenyCode = DefaultDenyCode
	}

	config.Anonymizer = json.Get("anonymizer").String()
	if config.Anonymizer == "" {
		config.Anonymizer = AnonymizerHash
	}

	if obj := json.Get("timeout"); obj.Exists() {
		config.Timeout = uint32(obj.Int())
	} else {
		config.Timeout = DefaultTimeout
	}

	if obj := json.Get("bufferLimit"); obj.Exists() {
		config.BufferLimit = int(obj.Int())
	} else {
		config.BufferLimit = DefaultBufferLimit
	}

	entitiesConfig := json.Get("entities")
	if entitiesConfig.Exists() && entitiesConfig.IsArray() {
		config.Entities = make([]PIIEntity, 0)
		for _, entity := range entitiesConfig.Array() {
			piientity := PIIEntity{
				EntityType:     entity.Get("entityType").String(),
				Action:         entity.Get("action").String(),
				ScoreThreshold: entity.Get("scoreThreshold").Float(),
			}
			if piientity.Action == "" {
				piientity.Action = config.DefaultAction
			}
			if piientity.ScoreThreshold == 0 {
				piientity.ScoreThreshold = config.DefaultScoreThreshold
			}
			config.Entities = append(config.Entities, piientity)
		}
	}

	config.AnalyzerClient = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: analyzerServiceName,
		Port: analyzerServicePort,
		Host: analyzerServiceHost,
	})

	config.AnonymizerClient = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: anonymizerServiceName,
		Port: anonymizerServicePort,
		Host: anonymizerServiceHost,
	})

	config.Metrics = make(map[string]proxywasm.MetricCounter)
	return nil
}

func (config *PresidioPIIConfig) GetEntityTypeAction(entityType string) string {
	for _, entity := range config.Entities {
		if entity.EntityType == entityType {
			return entity.Action
		}
	}
	return config.DefaultAction
}

func (config *PresidioPIIConfig) GetEntityTypeScoreThreshold(entityType string) float64 {
	for _, entity := range config.Entities {
		if entity.EntityType == entityType {
			return entity.ScoreThreshold
		}
	}
	return config.DefaultScoreThreshold
}

func (config *PresidioPIIConfig) GetAllEntityTypes() []string {
	entityTypes := make([]string, 0)
	for _, entity := range config.Entities {
		entityTypes = append(entityTypes, entity.EntityType)
	}
	return entityTypes
}

func (config *PresidioPIIConfig) IncrementCounter(metricName string, inc uint64) {
	counter, ok := config.Metrics[metricName]
	if !ok {
		counter = proxywasm.DefineCounterMetric(metricName)
		config.Metrics[metricName] = counter
	}
	counter.Increment(inc)
}

func BlockAction(action string) bool {
	return strings.ToUpper(action) == ActionBlock
}

func MaskAction(action string) bool {
	return strings.ToUpper(action) == ActionMask
}

func NoneAction(action string) bool {
	return strings.ToUpper(action) == ActionNone
}
