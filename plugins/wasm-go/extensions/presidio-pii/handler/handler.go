package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/presidio-pii/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type PresidioHandler struct{}

var Handler = &PresidioHandler{}

func (h *PresidioHandler) generateRandomChatID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 29)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return "chatcmpl-" + string(b)
}

func (h *PresidioHandler) generateHexID(length int) (string, error) {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	result := ""
	for _, v := range b {
		result += fmt.Sprintf("%02x", v)
	}
	return result, nil
}

func HandleRequestBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte) types.Action {
	content := gjson.GetBytes(body, config.RequestContentJsonPath).String()

	log.Debugf("Raw request content for PII check: %s", content)
	if len(content) == 0 {
		log.Info("request content is empty, skip PII check")
		return types.ActionContinue
	}

	sessionID, _ := Handler.generateHexID(20)
	startTime := time.Now().UnixMilli()

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("Presidio analyze response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("Presidio analyze failed with status: %d, response: %s", statusCode, string(responseBody))
			proxywasm.ResumeHttpRequest()
			return
		}

		var analyzeResp cfg.AnalyzeResponse
		err := json.Unmarshal(responseBody, &analyzeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal Presidio analyze response: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}

		blockedEntities := Handler.getBlockedEntities(config, analyzeResp)
		maskedEntities := Handler.getMaskedEntities(config, analyzeResp)

		if len(blockedEntities) > 0 {
			Handler.sendDenyResponse(ctx, config, body, false, blockedEntities, startTime)
			return
		}

		if len(maskedEntities) > 0 {
			Handler.anonymizeAndReplaceRequest(ctx, config, body, content, maskedEntities, sessionID)
			return
		}

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("presidio_pii_request_rt", endTime-startTime)
		ctx.SetUserAttribute("presidio_pii_status", "request pass")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpRequest()
	}

	entityTypes := config.GetAllEntityTypes()
	if len(entityTypes) == 0 {
		log.Info("No PII entity types configured, skip PII check")
		return types.ActionContinue
	}

	analyzeReq := cfg.AnalyzeRequest{
		Text:           content,
		Entities:       entityTypes,
		Language:       config.Language,
		ScoreThreshold: config.DefaultScoreThreshold,
	}

	reqBody, _ := json.Marshal(analyzeReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	err := config.AnalyzerClient.Post(config.AnalyzerPath, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call Presidio analyze service: %v", err)
		proxywasm.ResumeHttpRequest()
	}

	return types.ActionPause
}

func (h *PresidioHandler) getBlockedEntities(config cfg.PresidioPIIConfig, results []cfg.AnalyzeResult) []cfg.AnalyzeResult {
	blocked := make([]cfg.AnalyzeResult, 0)
	for _, result := range results {
		action := config.GetEntityTypeAction(result.EntityType)
		if cfg.BlockAction(action) {
			threshold := config.GetEntityTypeScoreThreshold(result.EntityType)
			if result.Score >= threshold {
				blocked = append(blocked, result)
			}
		}
	}
	return blocked
}

func (h *PresidioHandler) getMaskedEntities(config cfg.PresidioPIIConfig, results []cfg.AnalyzeResult) []cfg.AnalyzeResult {
	masked := make([]cfg.AnalyzeResult, 0)
	for _, result := range results {
		action := config.GetEntityTypeAction(result.EntityType)
		if cfg.MaskAction(action) {
			threshold := config.GetEntityTypeScoreThreshold(result.EntityType)
			if result.Score >= threshold {
				masked = append(masked, result)
			}
		}
	}
	return masked
}

func (h *PresidioHandler) buildAnAnonymizers(config cfg.PresidioPIIConfig) map[string]cfg.AnonymizerConfig {
	anonymizerConfig := cfg.AnonymizerConfig{
		Type: config.Anonymizer,
	}
	if config.Anonymizer == cfg.AnonymizerHash {
		anonymizerConfig.HashType = "sha256"
	}
	return map[string]cfg.AnonymizerConfig{
		"DEFAULT": anonymizerConfig,
	}
}

func (h *PresidioHandler) anonymizeAndReplaceRequest(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, originalBody []byte, content string, entities []cfg.AnalyzeResult, sessionID string) {
	startTime := time.Now().UnixMilli()
	anonymizeResults := make([]cfg.AnonymizeResult, 0)
	for _, entity := range entities {
		anonymizeResults = append(anonymizeResults, cfg.AnonymizeResult{
			Start:      entity.Start,
			End:        entity.End,
			EntityType: entity.EntityType,
		})
	}

	anonymizers := h.buildAnAnonymizers(config)

	anonymizeReq := cfg.AnonymizeRequest{
		Text:             content,
		AnonymizeResults: anonymizeResults,
		Anonymizers:      anonymizers,
	}

	reqBody, _ := json.Marshal(anonymizeReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("Presidio anonymize response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("Presidio anonymize failed with status: %d", statusCode)
			proxywasm.ResumeHttpRequest()
			return
		}

		var anonymizeResp cfg.AnonymizeResponse
		err := json.Unmarshal(responseBody, &anonymizeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal Presidio anonymize response: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}

		modifiedBody := h.replaceContentInJSON(originalBody, config.RequestContentJsonPath, anonymizeResp.Text)
		proxywasm.ReplaceHttpRequestBody(modifiedBody)

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("presidio_pii_request_rt", endTime-startTime)
		ctx.SetUserAttribute("presidio_pii_status", "request masked")
		ctx.SetUserAttribute("presidio_pii_masked_count", len(entities))
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpRequest()
	}

	err := config.AnonymizerClient.Post(config.AnonymizerPath, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call Presidio anonymize service: %v", err)
		proxywasm.ResumeHttpRequest()
	}
}

func (h *PresidioHandler) replaceContentInJSON(originalBody []byte, jsonPath string, newContent string) []byte {
	if jsonPath == "" {
		return originalBody
	}

	result := gjson.GetBytes(originalBody, jsonPath)
	if !result.Exists() {
		return originalBody
	}

	modified := gjson.ParseBytes(originalBody)
	modifiedStr := modified.Raw

	startIdx := result.Index
	endIdx := startIdx + len(result.Raw)

	newBody := make([]byte, 0, len(originalBody))
	newBody = append(newBody, modifiedStr[:startIdx]...)
	newBody = append(newBody, []byte(fmt.Sprintf("%q", newContent))...)
	newBody = append(newBody, modifiedStr[endIdx:]...)

	return newBody
}

func (h *PresidioHandler) sendDenyResponse(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte, isStreaming bool, blockedEntities []cfg.AnalyzeResult, startTime int64) {
	randomID := h.generateRandomChatID()
	denyMessage := config.DenyMessage
	if denyMessage == "" {
		denyMessage = cfg.DefaultDenyMessage
	}

	marshalledDenyMessage := escapeJSONString(denyMessage)

	if config.ProtocolOriginal {
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, []byte(denyMessage), -1)
	} else if isStreaming {
		jsonData := []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormat, randomID, marshalledDenyMessage, randomID))
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "text/event-stream;charset=UTF-8"}}, jsonData, -1)
	} else {
		jsonData := []byte(fmt.Sprintf(cfg.OpenAIResponseFormat, randomID, marshalledDenyMessage))
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, jsonData, -1)
	}

	ctx.DontReadResponseBody()
	config.IncrementCounter("presidio_pii_request_deny", 1)

	endTime := time.Now().UnixMilli()
	ctx.SetUserAttribute("presidio_pii_request_rt", endTime-startTime)
	ctx.SetUserAttribute("presidio_pii_status", "request deny")

	entityTypes := make([]string, 0)
	for _, entity := range blockedEntities {
		entityTypes = append(entityTypes, entity.EntityType)
	}
	ctx.SetUserAttribute("presidio_pii_blocked_entities", strings.Join(entityTypes, ","))
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
}

func escapeJSONString(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

func HandleResponseBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, body []byte) types.Action {
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	isStreaming := strings.Contains(contentType, "event-stream")

	var content string
	if isStreaming {
		content = extractMessageFromStreamingBody(body, config.ResponseStreamContentJsonPath)
	} else {
		content = gjson.GetBytes(body, config.ResponseContentJsonPath).String()
	}

	log.Debugf("Raw response content for PII check: %s", content)
	if len(content) == 0 {
		log.Info("response content is empty, skip PII check")
		return types.ActionContinue
	}

	sessionID, _ := Handler.generateHexID(20)
	startTime := time.Now().UnixMilli()

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("Presidio analyze response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("Presidio analyze failed with status: %d", statusCode)
			proxywasm.ResumeHttpResponse()
			return
		}

		var analyzeResp cfg.AnalyzeResponse
		err := json.Unmarshal(responseBody, &analyzeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal Presidio analyze response: %v", err)
			proxywasm.ResumeHttpResponse()
			return
		}

		blockedEntities := Handler.getBlockedEntities(config, analyzeResp)
		maskedEntities := Handler.getMaskedEntities(config, analyzeResp)

		if len(blockedEntities) > 0 {
			Handler.sendDenyResponse(ctx, config, body, isStreaming, blockedEntities, startTime)
			return
		}

		if len(maskedEntities) > 0 {
			Handler.anonymizeAndReplaceResponse(ctx, config, body, content, maskedEntities, sessionID, isStreaming)
			return
		}

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("presidio_pii_response_rt", endTime-startTime)
		ctx.SetUserAttribute("presidio_pii_status", "response pass")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpResponse()
	}

	entityTypes := config.GetAllEntityTypes()
	if len(entityTypes) == 0 {
		log.Info("No PII entity types configured, skip PII check")
		return types.ActionContinue
	}

	analyzeReq := cfg.AnalyzeRequest{
		Text:           content,
		Entities:       entityTypes,
		Language:       config.Language,
		ScoreThreshold: config.DefaultScoreThreshold,
	}

	reqBody, _ := json.Marshal(analyzeReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	err := config.AnalyzerClient.Post(config.AnalyzerPath, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call Presidio analyze service: %v", err)
		proxywasm.ResumeHttpResponse()
	}

	return types.ActionPause
}

func (h *PresidioHandler) anonymizeAndReplaceResponse(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, originalBody []byte, content string, entities []cfg.AnalyzeResult, sessionID string, isStreaming bool) {
	anonymizeResults := make([]cfg.AnonymizeResult, 0)
	for _, entity := range entities {
		anonymizeResults = append(anonymizeResults, cfg.AnonymizeResult{
			Start:      entity.Start,
			End:        entity.End,
			EntityType: entity.EntityType,
		})
	}

	anonymizers := h.buildAnAnonymizers(config)

	anonymizeReq := cfg.AnonymizeRequest{
		Text:             content,
		AnonymizeResults: anonymizeResults,
		Anonymizers:      anonymizers,
	}

	reqBody, _ := json.Marshal(anonymizeReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("Presidio anonymize response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("Presidio anonymize failed with status: %d", statusCode)
			proxywasm.ResumeHttpResponse()
			return
		}

		var anonymizeResp cfg.AnonymizeResponse
		err := json.Unmarshal(responseBody, &anonymizeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal Presidio anonymize response: %v", err)
			proxywasm.ResumeHttpResponse()
			return
		}

		jsonPath := config.ResponseContentJsonPath
		if isStreaming {
			jsonPath = config.ResponseStreamContentJsonPath
		}

		modifiedBody := Handler.replaceContentInJSON(originalBody, jsonPath, anonymizeResp.Text)
		proxywasm.ReplaceHttpResponseBody(modifiedBody)

		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("presidio_pii_response_rt", endTime)
		ctx.SetUserAttribute("presidio_pii_status", "response masked")
		ctx.SetUserAttribute("presidio_pii_masked_count", len(entities))
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		proxywasm.ResumeHttpResponse()
	}

	err := config.AnonymizerClient.Post(config.AnonymizerPath, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call Presidio anonymize service: %v", err)
		proxywasm.ResumeHttpResponse()
	}
}

func extractMessageFromStreamingBody(data []byte, jsonPath string) string {
	chunks := bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n"))
	strChunks := []string{}
	for _, chunk := range chunks {
		strChunks = append(strChunks, gjson.GetBytes(chunk, jsonPath).String())
	}
	return strings.Join(strChunks, "")
}

func HandleStreamingResponseBody(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, data []byte, endOfStream bool) []byte {
	var sessionID string
	if ctx.GetContext("sessionID") == nil {
		sessionID, _ = Handler.generateHexID(20)
		ctx.SetContext("sessionID", sessionID)
	} else {
		sessionID, _ = ctx.GetContext("sessionID").(string)
	}

	var bufferQueue [][]byte
	var singleCall func()

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("Presidio analyze response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("Presidio analyze failed with status: %d", statusCode)
			if ctx.GetContext("end_of_stream_received").(bool) {
				proxywasm.ResumeHttpResponse()
			}
			ctx.SetContext("during_call", false)
			return
		}

		var analyzeResp cfg.AnalyzeResponse
		err := json.Unmarshal(responseBody, &analyzeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal Presidio analyze response: %v", err)
			if ctx.GetContext("end_of_stream_received").(bool) {
				proxywasm.ResumeHttpResponse()
			}
			ctx.SetContext("during_call", false)
			return
		}

		blockedEntities := Handler.getBlockedEntities(config, analyzeResp)
		maskedEntities := Handler.getMaskedEntities(config, analyzeResp)

		if len(blockedEntities) > 0 {
			Handler.sendStreamingDenyResponse(ctx, config, blockedEntities)
			return
		}

		if len(maskedEntities) > 0 {
			Handler.handleStreamingAnonymize(ctx, config, bufferQueue, maskedEntities, sessionID)
			return
		}

		endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
		proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
		bufferQueue = [][]byte{}
		if !endStream {
			ctx.SetContext("during_call", false)
			singleCall()
		}
	}

	singleCall = func() {
		if ctx.GetContext("during_call").(bool) {
			return
		}

		if ctx.BufferQueueSize() >= config.BufferLimit || ctx.GetContext("end_of_stream_received").(bool) {
			var buffer string
			for ctx.BufferQueueSize() > 0 {
				front := ctx.PopBuffer()
				bufferQueue = append(bufferQueue, front)
				msg := gjson.GetBytes(front, config.ResponseStreamContentJsonPath).String()
				buffer += msg
				if len([]rune(buffer)) >= config.BufferLimit {
					break
				}
			}

			if len(buffer) == 0 {
				buffer = "[empty content]"
			}

			ctx.SetContext("during_call", true)

			entityTypes := config.GetAllEntityTypes()
			if len(entityTypes) == 0 {
				endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
				proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
				bufferQueue = [][]byte{}
				ctx.SetContext("during_call", false)
				if ctx.GetContext("end_of_stream_received").(bool) {
					proxywasm.ResumeHttpResponse()
				}
				return
			}

			analyzeReq := cfg.AnalyzeRequest{
				Text:           buffer,
				Entities:       entityTypes,
				Language:       config.Language,
				ScoreThreshold: config.DefaultScoreThreshold,
			}

			reqBody, _ := json.Marshal(analyzeReq)
			headers := [][2]string{
				{"Content-Type", "application/json"},
			}

			err := config.AnalyzerClient.Post(config.AnalyzerPath, headers, reqBody, callback, config.Timeout)
			if err != nil {
				log.Errorf("Failed to call Presidio analyze service: %v", err)
				if ctx.GetContext("end_of_stream_received").(bool) {
					proxywasm.ResumeHttpResponse()
				}
			}
		}
	}

	if !ctx.GetContext("risk_detected").(bool) {
		unifiedChunk := wrapper.UnifySSEChunk(data)
		hasTrailingSeparator := bytes.HasSuffix(unifiedChunk, []byte("\n\n"))
		trimmedChunk := bytes.TrimSpace(unifiedChunk)
		chunks := bytes.Split(trimmedChunk, []byte("\n\n"))

		nonEmptyChunks := make([][]byte, 0, len(chunks))
		for _, chunk := range chunks {
			if len(chunk) > 0 {
				nonEmptyChunks = append(nonEmptyChunks, chunk)
			}
		}

		for i := range len(nonEmptyChunks) - 1 {
			nonEmptyChunks[i] = append(nonEmptyChunks[i], []byte("\n\n")...)
		}
		if hasTrailingSeparator && len(nonEmptyChunks) > 0 {
			nonEmptyChunks[len(nonEmptyChunks)-1] = append(nonEmptyChunks[len(nonEmptyChunks)-1], []byte("\n\n")...)
		}

		for _, chunk := range nonEmptyChunks {
			ctx.PushBuffer(chunk)
		}

		ctx.SetContext("end_of_stream_received", endOfStream)
		if !ctx.GetContext("during_call").(bool) {
			singleCall()
		}
	} else if endOfStream {
		proxywasm.ResumeHttpResponse()
	}

	return []byte{}
}

func (h *PresidioHandler) sendStreamingDenyResponse(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, blockedEntities []cfg.AnalyzeResult) {
	randomID := h.generateRandomChatID()
	denyMessage := config.DenyMessage
	if denyMessage == "" {
		denyMessage = cfg.DefaultDenyMessage
	}

	marshalledDenyMessage := escapeJSONString(denyMessage)
	jsonData := []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormat, randomID, marshalledDenyMessage, randomID))

	proxywasm.InjectEncodedDataToFilterChain(jsonData, true)

	config.IncrementCounter("presidio_pii_response_deny", 1)
	ctx.SetContext("risk_detected", true)

	entityTypes := make([]string, 0)
	for _, entity := range blockedEntities {
		entityTypes = append(entityTypes, entity.EntityType)
	}
	ctx.SetUserAttribute("presidio_pii_blocked_entities", strings.Join(entityTypes, ","))
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
}

func (h *PresidioHandler) handleStreamingAnonymize(ctx wrapper.HttpContext, config cfg.PresidioPIIConfig, bufferQueue [][]byte, entities []cfg.AnalyzeResult, sessionID string) {
	var buffer string
	for _, chunk := range bufferQueue {
		msg := gjson.GetBytes(chunk, config.ResponseStreamContentJsonPath).String()
		buffer += msg
	}

	anonymizeResults := make([]cfg.AnonymizeResult, 0)
	for _, entity := range entities {
		anonymizeResults = append(anonymizeResults, cfg.AnonymizeResult{
			Start:      entity.Start,
			End:        entity.End,
			EntityType: entity.EntityType,
		})
	}

	anonymizers := h.buildAnAnonymizers(config)

	anonymizeReq := cfg.AnonymizeRequest{
		Text:             buffer,
		AnonymizeResults: anonymizeResults,
		Anonymizers:      anonymizers,
	}

	reqBody, _ := json.Marshal(anonymizeReq)
	headers := [][2]string{
		{"Content-Type", "application/json"},
	}

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Debugf("Presidio anonymize response: %s", string(responseBody))

		if statusCode != 200 {
			log.Errorf("Presidio anonymize failed with status: %d", statusCode)
			endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
			proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
			return
		}

		var anonymizeResp cfg.AnonymizeResponse
		err := json.Unmarshal(responseBody, &anonymizeResp)
		if err != nil {
			log.Errorf("Failed to unmarshal Presidio anonymize response: %v", err)
			endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
			proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
			return
		}

		modifiedChunks := make([][]byte, 0)
		for _, chunk := range bufferQueue {
			modifiedChunk := Handler.replaceContentInJSON(chunk, config.ResponseStreamContentJsonPath, anonymizeResp.Text)
			modifiedChunks = append(modifiedChunks, modifiedChunk)
		}

		endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
		proxywasm.InjectEncodedDataToFilterChain(bytes.Join(modifiedChunks, []byte("")), endStream)

		ctx.SetUserAttribute("presidio_pii_status", "response masked")
		ctx.SetUserAttribute("presidio_pii_masked_count", len(entities))
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	}

	err := config.AnonymizerClient.Post(config.AnonymizerPath, headers, reqBody, callback, config.Timeout)
	if err != nil {
		log.Errorf("Failed to call Presidio anonymize service: %v", err)
		endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
		proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
	}
}
