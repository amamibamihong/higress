package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"smart-route",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

type SmartRouteConfig struct {
	RouterID        string
	RequestJsonPath string
	Timeout         uint32
	ServiceName     string
	ServicePort     uint32
	ServicePath     string
	ServiceHost     string
	client          wrapper.HttpClient
}

type SmartRouteRequest struct {
	Query    string `json:"query"`
	RouterID string `json:"router_id"`
}

type SmartRouteResponse struct {
	Code int `json:"code"`
	Data struct {
		SelectedModel string  `json:"selected_model"`
		LatencyMS     float64 `json:"latency_ms"`
	} `json:"data"`
}

func parseConfig(json gjson.Result, config *SmartRouteConfig) error {
	config.RouterID = json.Get("routerID").String()
	if config.RouterID == "" {
		return fmt.Errorf("routerID is required")
	}

	config.ServiceName = json.Get("serviceName").String()
	if config.ServiceName == "" {
		return fmt.Errorf("serviceName is required")
	}

	config.ServicePort = uint32(json.Get("servicePort").Int())
	if config.ServicePort == 0 {
		config.ServicePort = 80
	}

	config.ServicePath = json.Get("servicePath").String()
	if config.ServicePath == "" {
		config.ServicePath = "/v1/smart-route"
	}

	config.ServiceHost = json.Get("serviceHost").String()
	if config.ServiceHost == "" {
		config.ServiceHost = config.ServiceName
	}

	config.RequestJsonPath = json.Get("requestJsonPath").String()
	if config.RequestJsonPath == "" {
		config.RequestJsonPath = "messages.@reverse.0.content"
	}

	config.Timeout = uint32(json.Get("timeout").Int())
	if config.Timeout == 0 {
		config.Timeout = 5000
	}

	config.client = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: config.ServiceName,
		Host: config.ServiceHost,
		Port: int64(config.ServicePort),
	})

	return nil
}

func onHttpRequestBody(ctx wrapper.HttpContext, config SmartRouteConfig, body []byte) types.Action {
	// Extract prompt using gjson
	prompt := gjson.GetBytes(body, config.RequestJsonPath).String()
	if prompt == "" {
		log.Debug("extracted prompt is empty, skip smart route")
		return types.ActionContinue
	}

	log.Debugf("Extracted prompt for SmartRoute: %s", prompt)

	// Prepare API request
	apiReq := SmartRouteRequest{
		Query:    prompt,
		RouterID: config.RouterID,
	}
	reqBody, _ := json.Marshal(apiReq)

	headers := [][2]string{
		{"Content-Type", "application/json"},
		{":authority", config.ServiceHost},
	}

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		if statusCode != 200 {
			log.Errorf("SmartRoute API call failed with status: %d, body: %s", statusCode, string(responseBody))
			proxywasm.SendHttpResponse(500, [][2]string{{"Content-Type", "text/plain"}}, []byte("SmartRoute service unavailable"), -1)
			return
		}

		var apiResp SmartRouteResponse
		if err := json.Unmarshal(responseBody, &apiResp); err != nil {
			log.Errorf("Failed to unmarshal SmartRoute API response: %v, body: %s", err, string(responseBody))
			proxywasm.SendHttpResponse(500, [][2]string{{"Content-Type", "text/plain"}}, []byte("Invalid response from SmartRoute service"), -1)
			return
		}

		if apiResp.Code != 0 || apiResp.Data.SelectedModel == "" {
			log.Warnf("SmartRoute API returned non-zero code (%d) or empty model", apiResp.Code)
			proxywasm.SendHttpResponse(500, [][2]string{{"Content-Type", "text/plain"}}, []byte("SmartRoute service error: decision failed"), -1)
			return
		}

		selectedModel := apiResp.Data.SelectedModel
		log.Infof("SmartRoute raw selected result '%s' for router '%s'", selectedModel, config.RouterID)

		provider := ""
		finalModel := selectedModel

		// Extract provider and model by the first '/'
		if idx := strings.Index(selectedModel, "/"); idx != -1 {
			provider = selectedModel[:idx]
			finalModel = selectedModel[idx+1:]
			log.Infof("Extracted provider: %s, final model: %s", provider, finalModel)
		}

		if provider != "" {
			// Use Envoy Property to pass provider information securely between plugins.
			// This is not visible to the end user and cannot be spoofed.
			_ = proxywasm.SetProperty([]string{"ai_provider"}, []byte(provider))
			ctx.SetContext("provider", provider)
		}

		// Replace model in original request body with the final model name
		newBody, err := sjson.SetBytes(body, "model", finalModel)
		if err != nil {
			log.Errorf("Failed to replace model in request body: %v", err)
			proxywasm.SendHttpResponse(500, [][2]string{{"Content-Type", "text/plain"}}, []byte("Internal error: failed to apply routing decision"), -1)
			return
		}

		proxywasm.ReplaceHttpRequestBody(newBody)
		proxywasm.ResumeHttpRequest()
	}

	err := config.client.Post(config.ServicePath, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to initiate SmartRoute API call: %v", err)
		proxywasm.SendHttpResponse(500, [][2]string{{"Content-Type", "text/plain"}}, []byte("Failed to connect to SmartRoute service"), -1)
		return types.ActionContinue
	}

	return types.ActionPause
}
