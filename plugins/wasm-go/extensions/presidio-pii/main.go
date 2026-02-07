package main

import (
	"encoding/json"
	"fmt"
	"math/rand"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/presidio-pii/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/presidio-pii/handler"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"presidio-pii",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
		wrapper.WithRebuildAfterRequests[cfg.PresidioPIIConfig](1000),
	)
}

func parseConfig(json gjson.Result, config *cfg.PresidioPIIConfig) error {
	return config.Parse(json)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig) types.Action {
	consumer, _ := proxywasm.GetHttpRequestHeader("x-mse-consumer")
	ctx.SetContext("consumer", consumer)
	ctx.DisableReroute()

	shouldCheckRequest := config.CheckRequest && (config.FilterScope == cfg.FilterScopeInput || config.FilterScope == cfg.FilterScopeBoth)
	if !shouldCheckRequest {
		log.Debugf("request PII checking is disabled or filter scope does not include input")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte) types.Action {
	shouldCheckRequest := config.CheckRequest && (config.FilterScope == cfg.FilterScopeInput || config.FilterScope == cfg.FilterScopeBoth)
	if !shouldCheckRequest {
		return types.ActionContinue
	}

	log.Debugf("checking request body for PII...")
	return handler.HandleRequestBody(ctx, config, body)
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig) types.Action {
	shouldCheckResponse := config.CheckResponse && (config.FilterScope == cfg.FilterScopeOutput || config.FilterScope == cfg.FilterScopeBoth)

	if !shouldCheckResponse {
		log.Debugf("response PII checking is disabled or filter scope does not include output")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	statusCode, _ := proxywasm.GetHttpResponseHeader(":status")
	if statusCode != "200" {
		log.Debugf("response is not 200, skip response body check")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")

	if !config.ProtocolOriginal {
		if shouldCheckResponse && len(contentType) > 0 {
			if contains(contentType, "text/event-stream") {
				ctx.NeedPauseStreamingResponse()
				return types.ActionContinue
			}
		}
	}

	ctx.BufferResponseBody()
	return types.HeaderStopIteration
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, data []byte, endOfStream bool) []byte {
	return handler.HandleStreamingResponseBody(ctx, config, data, endOfStream)
}

func onHttpResponseBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte) types.Action {
	shouldCheckResponse := config.CheckResponse && (config.FilterScope == cfg.FilterScopeOutput || config.FilterScope == cfg.FilterScopeBoth)
	if !shouldCheckResponse {
		return types.ActionContinue
	}

	log.Debugf("checking response body for PII...")
	return handler.HandleResponseBody(ctx, config, body)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func createDenyMessage(denyMessage string, randomID string, streaming bool) []byte {
	marshalledDenyMessage := escapeJSONString(denyMessage)
	if streaming {
		jsonData := []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormat, randomID, marshalledDenyMessage, randomID))
		return jsonData
	} else {
		jsonData := []byte(fmt.Sprintf(cfg.OpenAIResponseFormat, randomID, marshalledDenyMessage))
		return jsonData
	}
}

func escapeJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

func generateRandomChatID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 29)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "chatcmpl-" + string(b)
}
