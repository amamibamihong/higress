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
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, body []byte) types.Action {
	log.Debugf("checking request body for LLM Guard...")
	return handler.HandleRequestBody(ctx, config, body)
}
