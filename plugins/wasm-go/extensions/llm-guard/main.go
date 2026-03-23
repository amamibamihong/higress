package main

import (
	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/llm-guard/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/llm-guard/handler"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"llm-guard",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.WithRebuildAfterRequests[cfg.LLMGuardConfig](1000),
	)
}

func parseConfig(json gjson.Result, config *cfg.LLMGuardConfig) error {
	return config.Parse(json)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config cfg.LLMGuardConfig) types.Action {
	consumer, _ := proxywasm.GetHttpRequestHeader("x-mse-consumer")
	ctx.SetContext("consumer", consumer)
	ctx.DisableReroute()

	shouldCheckRequest := config.CheckRequest && (config.Scope == cfg.ScopeInput || config.Scope == cfg.ScopeBoth)
	if !shouldCheckRequest {
		log.Debugf("request LLM Guard checking is disabled or scope does not include input")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, body []byte) types.Action {
	shouldCheckRequest := config.CheckRequest && (config.Scope == cfg.ScopeInput || config.Scope == cfg.ScopeBoth)
	if !shouldCheckRequest {
		return types.ActionContinue
	}

	log.Debugf("checking request body for LLM Guard...")
	return handler.HandleRequestBody(ctx, config, body)
}
