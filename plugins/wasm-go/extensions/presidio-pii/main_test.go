package main

import (
	"encoding/json"
	"testing"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/presidio-pii/config"
	"github.com/tidwall/gjson"
)

func TestParseConfig(t *testing.T) {
	configJSON := `{
		"analyzerServiceName": "presidio-analyzer.dns",
		"analyzerServicePort": 8080,
		"analyzerServiceHost": "presidio-analyzer.example.com",
		"anonymizerServiceName": "presidio-anonymizer.dns",
		"anonymizerServicePort": 8080,
		"anonymizerServiceHost": "presidio-anonymizer.example.com",
		"checkRequest": true,
		"checkResponse": true,
		"filterScope": "both",
		"language": "zh",
		"defaultAction": "MASK",
		"defaultScoreThreshold": 0.85,
		"anonymizer": "hash",
		"entities": [
			{
				"entityType": "PERSON",
				"action": "MASK",
				"scoreThreshold": 0.9
			},
			{
				"entityType": "EMAIL_ADDRESS",
				"action": "MASK",
				"scoreThreshold": 0.95
			},
			{
				"entityType": "CREDIT_CARD",
				"action": "BLOCK",
				"scoreThreshold": 0.99
			}
		]
	}`

	var config cfg.PresidioPIIConfig
	result := gjson.Parse(configJSON)
	err := config.Parse(result)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if config.AnalyzerServiceName != "presidio-analyzer.dns" {
		t.Errorf("Expected analyzerServiceName 'presidio-analyzer.dns', got '%s'", config.AnalyzerServiceName)
	}

	if config.AnalyzerServicePort != 8080 {
		t.Errorf("Expected analyzerServicePort 8080, got %d", config.AnalyzerServicePort)
	}

	if config.AnalyzerServiceHost != "presidio-analyzer.example.com" {
		t.Errorf("Expected analyzerServiceHost 'presidio-analyzer.example.com', got '%s'", config.AnalyzerServiceHost)
	}

	if config.AnonymizerServiceName != "presidio-anonymizer.dns" {
		t.Errorf("Expected anonymizerServiceName 'presidio-anonymizer.dns', got '%s'", config.AnonymizerServiceName)
	}

	if config.AnonymizerServicePort != 8080 {
		t.Errorf("Expected anonymizerServicePort 8080, got %d", config.AnonymizerServicePort)
	}

	if config.AnonymizerServiceHost != "presidio-anonymizer.example.com" {
		t.Errorf("Expected anonymizerServiceHost 'presidio-anonymizer.example.com', got '%s'", config.AnonymizerServiceHost)
	}

	if !config.CheckRequest {
		t.Error("Expected checkRequest to be true")
	}

	if !config.CheckResponse {
		t.Error("Expected checkResponse to be true")
	}

	if config.FilterScope != cfg.FilterScopeBoth {
		t.Errorf("Expected filterScope '%s', got '%s'", cfg.FilterScopeBoth, config.FilterScope)
	}

	if config.Language != "zh" {
		t.Errorf("Expected language 'zh', got '%s'", config.Language)
	}

	if config.DefaultAction != cfg.ActionMask {
		t.Errorf("Expected defaultAction '%s', got '%s'", cfg.ActionMask, config.DefaultAction)
	}

	if config.DefaultScoreThreshold != 0.85 {
		t.Errorf("Expected defaultScoreThreshold 0.85, got %f", config.DefaultScoreThreshold)
	}

	if config.Anonymizer != cfg.AnonymizerHash {
		t.Errorf("Expected anonymizer '%s', got '%s'", cfg.AnonymizerHash, config.Anonymizer)
	}

	if len(config.Entities) != 3 {
		t.Fatalf("Expected 3 entities, got %d", len(config.Entities))
	}

	personEntity := config.Entities[0]
	if personEntity.EntityType != "PERSON" {
		t.Errorf("Expected first entity type 'PERSON', got '%s'", personEntity.EntityType)
	}
	if personEntity.Action != cfg.ActionMask {
		t.Errorf("Expected PERSON action '%s', got '%s'", cfg.ActionMask, personEntity.Action)
	}
	if personEntity.ScoreThreshold != 0.9 {
		t.Errorf("Expected PERSON scoreThreshold 0.9, got %f", personEntity.ScoreThreshold)
	}

	creditCardEntity := config.Entities[2]
	if creditCardEntity.Action != cfg.ActionBlock {
		t.Errorf("Expected CREDIT_CARD action '%s', got '%s'", cfg.ActionBlock, creditCardEntity.Action)
	}
}

func TestGetEntityTypeAction(t *testing.T) {
	configJSON := `{
		"analyzerServiceName": "presidio-analyzer.dns",
		"analyzerServicePort": 8080,
		"analyzerServiceHost": "presidio-analyzer.example.com",
		"anonymizerServiceName": "presidio-anonymizer.dns",
		"anonymizerServicePort": 8080,
		"anonymizerServiceHost": "presidio-anonymizer.example.com",
		"defaultAction": "MASK",
		"entities": [
			{
				"entityType": "PERSON",
				"action": "BLOCK"
			},
			{
				"entityType": "EMAIL_ADDRESS",
				"action": "MASK"
			}
		]
	}`

	var config cfg.PresidioPIIConfig
	result := gjson.Parse(configJSON)
	err := config.Parse(result)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	personAction := config.GetEntityTypeAction("PERSON")
	if personAction != cfg.ActionBlock {
		t.Errorf("Expected PERSON action '%s', got '%s'", cfg.ActionBlock, personAction)
	}

	emailAction := config.GetEntityTypeAction("EMAIL_ADDRESS")
	if emailAction != cfg.ActionMask {
		t.Errorf("Expected EMAIL_ADDRESS action '%s', got '%s'", cfg.ActionMask, emailAction)
	}

	phoneAction := config.GetEntityTypeAction("PHONE_NUMBER")
	if phoneAction != cfg.ActionMask {
		t.Errorf("Expected PHONE_NUMBER action '%s' (default), got '%s'", cfg.ActionMask, phoneAction)
	}
}

func TestGetEntityTypeScoreThreshold(t *testing.T) {
	configJSON := `{
		"analyzerServiceName": "presidio-analyzer.dns",
		"analyzerServicePort": 8080,
		"analyzerServiceHost": "presidio-analyzer.example.com",
		"anonymizerServiceName": "presidio-anonymizer.dns",
		"anonymizerServicePort": 8080,
		"anonymizerServiceHost": "presidio-anonymizer.example.com",
		"defaultScoreThreshold": 0.8,
		"entities": [
			{
				"entityType": "PERSON",
				"scoreThreshold": 0.9
			},
			{
				"entityType": "EMAIL_ADDRESS",
				"scoreThreshold": 0.95
			}
		]
	}`

	var config cfg.PresidioPIIConfig
	result := gjson.Parse(configJSON)
	err := config.Parse(result)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	personThreshold := config.GetEntityTypeScoreThreshold("PERSON")
	if personThreshold != 0.9 {
		t.Errorf("Expected PERSON threshold 0.9, got %f", personThreshold)
	}

	emailThreshold := config.GetEntityTypeScoreThreshold("EMAIL_ADDRESS")
	if emailThreshold != 0.95 {
		t.Errorf("Expected EMAIL_ADDRESS threshold 0.95, got %f", emailThreshold)
	}

	phoneThreshold := config.GetEntityTypeScoreThreshold("PHONE_NUMBER")
	if phoneThreshold != 0.8 {
		t.Errorf("Expected PHONE_NUMBER threshold 0.8 (default), got %f", phoneThreshold)
	}
}

func TestAnalyzeRequestSerialization(t *testing.T) {
	req := cfg.AnalyzeRequest{
		Text:           "Hello John Doe",
		Entities:       []string{"PERSON", "EMAIL_ADDRESS"},
		Language:       "en",
		ScoreThreshold: 0.85,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal AnalyzeRequest: %v", err)
	}

	var unmarshaled cfg.AnalyzeRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal AnalyzeRequest: %v", err)
	}

	if unmarshaled.Text != req.Text {
		t.Errorf("Expected text '%s', got '%s'", req.Text, unmarshaled.Text)
	}

	if len(unmarshaled.Entities) != len(req.Entities) {
		t.Errorf("Expected %d entities, got %d", len(req.Entities), len(unmarshaled.Entities))
	}

	if unmarshaled.Language != req.Language {
		t.Errorf("Expected language '%s', got '%s'", req.Language, unmarshaled.Language)
	}

	if unmarshaled.ScoreThreshold != req.ScoreThreshold {
		t.Errorf("Expected scoreThreshold %f, got %f", req.ScoreThreshold, unmarshaled.ScoreThreshold)
	}
}

func TestAnonymizeRequestSerialization(t *testing.T) {
	req := cfg.AnonymizeRequest{
		Text: "Hello John Doe",
		AnonymizeResults: []cfg.AnonymizeResult{
			{
				Start:      6,
				End:        14,
				EntityType: "PERSON",
			},
		},
		Anonymizers: map[string]cfg.AnonymizerConfig{
			"DEFAULT": {
				Type:     "hash",
				HashType: "sha256",
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal AnonymizeRequest: %v", err)
	}

	var unmarshaled cfg.AnonymizeRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal AnonymizeRequest: %v", err)
	}

	if unmarshaled.Text != req.Text {
		t.Errorf("Expected text '%s', got '%s'", req.Text, unmarshaled.Text)
	}

	if len(unmarshaled.AnonymizeResults) != len(req.AnonymizeResults) {
		t.Errorf("Expected %d anonymize results, got %d", len(req.AnonymizeResults), len(unmarshaled.AnonymizeResults))
	}

	if unmarshaled.AnonymizeResults[0].Start != req.AnonymizeResults[0].Start {
		t.Errorf("Expected start %d, got %d", req.AnonymizeResults[0].Start, unmarshaled.AnonymizeResults[0].Start)
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "lo wo", true},
		{"hello world", "xyz", false},
		{"", "", true},
		{"hello", "", true},
		{"", "hello", false},
	}

	for _, tt := range tests {
		got := contains(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestEscapeJSONString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "hello world"},
		{"hello \"world\"", "hello \\\"world\\\""},
		{"hello\nworld", "hello\\nworld"},
	}

	for _, tt := range tests {
		got := escapeJSONString(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSONString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
