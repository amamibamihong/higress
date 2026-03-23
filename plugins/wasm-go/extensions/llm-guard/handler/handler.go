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

	log.Debugf("Raw request content for LLM Guard check: %s", content)
	if len(content) == 0 {
		log.Info("request content is empty, skip LLM Guard check")
		return types.ActionContinue
	}

	startTime := time.Now().UnixMilli()

	if cfg.MaskAction(config.DefaultAction) {
		return Handler.handleMaskMode(ctx, config, body, content, startTime)
	}

	if cfg.BlockAction(config.DefaultAction) {
		return Handler.handleBlockMode(ctx, config, content, startTime)
	}

	return types.ActionContinue
}

func (h *LLMGuardHandler) handleMaskMode(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, body []byte, content string, startTime int64) types.Action {
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("LLM Guard analyze response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("LLM Guard analyze failed with status: %d, response: %s", statusCode, string(responseBody))
			proxywasm.ResumeHttpRequest()
			return
		}

		var analyzeResp cfg.AnalyzePromptResponse
		err := json.Unmarshal(responseBody, &analyzeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal LLM Guard analyze response: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}

		if !analyzeResp.IsValid {
			h.replaceRequestContent(ctx, config, body, analyzeResp.SanitizedPrompt, analyzeResp.Scanners, startTime)
			return
		}

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("llm_guard_request_rt", endTime-startTime)
		ctx.SetUserAttribute("llm_guard_status", "request pass")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpRequest()
	}

	analyzeReq := cfg.AnalyzePromptRequest{
		Prompt: content,
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

func (h *LLMGuardHandler) handleBlockMode(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, content string, startTime int64) types.Action {
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("LLM Guard scan response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("LLM Guard scan failed with status: %d, response: %s", statusCode, string(responseBody))
			proxywasm.ResumeHttpRequest()
			return
		}

		var scanResp cfg.ScanPromptResponse
		err := json.Unmarshal(responseBody, &scanResp)
		if err != nil {
			log.Errorf("Failed to unmarshal LLM Guard scan response: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}

		if scanResp.IsValid {
			h.sendDenyResponse(ctx, config, scanResp.Scanners, startTime)
			return
		}

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("llm_guard_request_rt", endTime-startTime)
		ctx.SetUserAttribute("llm_guard_status", "request pass")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpRequest()
	}

	scanReq := cfg.ScanPromptRequest{
		Prompt: content,
	}

	reqBody, _ := json.Marshal(scanReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	err := config.ServiceClient.Post(config.EndpointScanPrompt, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call LLM Guard scan service: %v", err)
		proxywasm.ResumeHttpRequest()
	}

	return types.ActionPause
}

func (h *LLMGuardHandler) replaceRequestContent(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, originalBody []byte, sanitizedPrompt string, scanners map[string]float64, startTime int64) {
	modifiedBody := h.replaceContentInJSON(originalBody, config.RequestJsonPath, sanitizedPrompt)
	proxywasm.ReplaceHttpRequestBody(modifiedBody)

	endTime := time.Now().UnixMilli()
	ctx.SetUserAttribute("llm_guard_request_rt", endTime-startTime)
	ctx.SetUserAttribute("llm_guard_status", "request sanitized")
	ctx.SetUserAttribute("llm_guard_scanners", getScannerNames(scanners))
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

func (h *LLMGuardHandler) sendDenyResponse(ctx wrapper.HttpContext, config cfg.LLMGuardConfig, scanners map[string]float64, startTime int64) {
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
	ctx.SetUserAttribute("llm_guard_scanners", getScannerNames(scanners))
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
}

func escapeJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

func getScannerNames(scanners map[string]float64) string {
	names := make([]string, 0, len(scanners))
	for name := range scanners {
		names = append(names, name)
	}
	return strings.Join(names, ",")
}
