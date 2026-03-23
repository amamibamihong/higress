package main

import (
	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/presidio-pii/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"go-wasm-demo",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessResponseBody(onHttpResponseBody),

		// wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
	)
}

func parseConfig(json gjson.Result, config *cfg.PresidioPIIConfig) error {
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig) types.Action {

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte) types.Action {
	log.Debugf("checking request body...%s", string(body))
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig) types.Action {

	return types.ActionContinue
}
func onHttpResponseBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte) types.Action {
	log.Debugf("checking response body...%s", string(body))
	return types.ActionContinue
}
func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, data []byte, endOfStream bool) []byte {
	log.Debugf("checking streaming response body...%s", string(data))
	return data
}
