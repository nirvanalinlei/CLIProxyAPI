package responses

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIChatCompletionsRequestToOpenAIResponsesConvertsFunctionTools(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"messages":[{"role":"user","content":"hi"}],
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"Task",
					"description":"Run task",
					"parameters":{"type":"object","properties":{"prompt":{"type":"string"}}}
				}
			},
			{"type":"web_search_preview"}
		]
	}`)

	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4.1", raw, false)
	if got := gjson.GetBytes(out, "tools.0.type").String(); got != "function" {
		t.Fatalf("tools.0.type = %q, want %q", got, "function")
	}
	if got := gjson.GetBytes(out, "tools.0.name").String(); got != "Task" {
		t.Fatalf("tools.0.name = %q, want %q", got, "Task")
	}
	if got := gjson.GetBytes(out, "tools.0.description").String(); got != "Run task" {
		t.Fatalf("tools.0.description = %q, want %q", got, "Run task")
	}
	if !gjson.GetBytes(out, "tools.0.parameters").Exists() {
		t.Fatalf("expected tools.0.parameters to exist")
	}
	if gjson.GetBytes(out, "tools.0.function").Exists() {
		t.Fatalf("unexpected nested tools.0.function after conversion")
	}
	if got := gjson.GetBytes(out, "tools.1.type").String(); got != "web_search_preview" {
		t.Fatalf("tools.1.type = %q, want %q", got, "web_search_preview")
	}
}

func TestConvertOpenAIChatCompletionsRequestToOpenAIResponsesConvertsFunctionToolChoice(t *testing.T) {
	raw := []byte(`{
		"model":"gpt-4.1",
		"messages":[{"role":"user","content":"hi"}],
		"tool_choice":{"type":"function","function":{"name":"Task"}}
	}`)

	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4.1", raw, false)
	if got := gjson.GetBytes(out, "tool_choice.type").String(); got != "function" {
		t.Fatalf("tool_choice.type = %q, want %q", got, "function")
	}
	if got := gjson.GetBytes(out, "tool_choice.name").String(); got != "Task" {
		t.Fatalf("tool_choice.name = %q, want %q", got, "Task")
	}
	if gjson.GetBytes(out, "tool_choice.function").Exists() {
		t.Fatalf("unexpected nested tool_choice.function after conversion")
	}
}
