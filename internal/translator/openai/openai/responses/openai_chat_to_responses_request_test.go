package responses

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIChatToResponses_FullMapping(t *testing.T) {
	in := []byte(`{
		"model":"gpt-4",
		"stream":true,
		"messages":[
			{"role":"system","content":"sys"},
			{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"https://img"}}]},
			{"role":"assistant","content":"ok","tool_calls":[{"id":"call1","type":"function","function":{"name":"do","arguments":"{\"a\":1}"}}]},
			{"role":"tool","tool_call_id":"call1","content":"result"}
		],
		"tools":[{"type":"function","function":{"name":"do","description":"d","parameters":{"type":"object"}}}],
		"tool_choice":{"type":"function","function":{"name":"do"}},
		"parallel_tool_calls":true,
		"response_format":{"type":"json_object"},
		"reasoning_effort":"medium",
		"metadata":{"tenant":"t1"},
		"max_tokens":123
	}`)

	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, true)
	if gjson.GetBytes(out, "input.#").Int() != 5 {
		t.Fatalf("input len = %s", string(out))
	}
	if gjson.GetBytes(out, "input.0.role").String() != "developer" {
		t.Fatalf("role = %s", string(out))
	}
	if gjson.GetBytes(out, "input.1.content.0.type").String() != "input_text" {
		t.Fatalf("text part = %s", string(out))
	}
	if gjson.GetBytes(out, "input.1.content.1.type").String() != "input_image" {
		t.Fatalf("image part = %s", string(out))
	}
	if gjson.GetBytes(out, "input.3.type").String() != "function_call" {
		t.Fatalf("function_call missing = %s", string(out))
	}
	if gjson.GetBytes(out, "input.4.type").String() != "function_call_output" {
		t.Fatalf("function_call_output missing = %s", string(out))
	}
	if gjson.GetBytes(out, "tools.0.name").String() != "do" {
		t.Fatalf("tools = %s", string(out))
	}
	if gjson.GetBytes(out, "tool_choice.function.name").String() != "do" {
		t.Fatalf("tool_choice = %s", string(out))
	}
	if gjson.GetBytes(out, "response_format.type").String() != "json_object" {
		t.Fatalf("response_format = %s", string(out))
	}
	if gjson.GetBytes(out, "reasoning.effort").String() != "medium" {
		t.Fatalf("reasoning = %s", string(out))
	}
	if gjson.GetBytes(out, "max_output_tokens").Int() != 123 {
		t.Fatalf("max_output_tokens = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_EmptyContentArray(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":[]}]}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if !gjson.GetBytes(out, "input.0.content").IsArray() {
		t.Fatalf("content must be array: %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_ToolChoiceString(t *testing.T) {
	in := []byte(`{"model":"gpt-4","tool_choice":"none"}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "tool_choice").String() != "none" {
		t.Fatalf("tool_choice = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_ToolOutputArray(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"tool","tool_call_id":"call_1","content":[{"type":"text","text":"ok"},{"type":"text","text":"!"}]}]}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.0.output").String() != "ok!" {
		t.Fatalf("tool output = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_LegacyFunctionCall(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"assistant","content":"","function_call":{"name":"do","arguments":"{\"a\":1}"}}]}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.#").Int() != 2 {
		t.Fatalf("input len = %s", string(out))
	}
	if gjson.GetBytes(out, "input.1.type").String() != "function_call" {
		t.Fatalf("function_call missing = %s", string(out))
	}
	if gjson.GetBytes(out, "input.1.name").String() != "do" {
		t.Fatalf("name missing = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_ToolCallsMissingID(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"do","arguments":"{}"}}]}]}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.1.call_id").String() != "call_0" {
		t.Fatalf("call_id = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_SystemIgnoresImage(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[{"role":"system","content":[{"type":"text","text":"ok"},{"type":"image_url","image_url":{"url":"https://img"}}]}]}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.0.content.#").Int() != 1 {
		t.Fatalf("expected only text content: %s", string(out))
	}
	if gjson.GetBytes(out, "input.0.content.0.type").String() != "input_text" {
		t.Fatalf("content type = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_MissingMessages(t *testing.T) {
	in := []byte(`{"model":"gpt-4"}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.#").Int() != 0 {
		t.Fatalf("expected empty input: %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_ResponseFormatJsonSchema(t *testing.T) {
	in := []byte(`{"model":"gpt-4","response_format":{"type":"json_schema","json_schema":{"name":"n","schema":{"type":"object"}}}}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "response_format.json_schema.schema.type").String() != "object" {
		t.Fatalf("json_schema missing: %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_StreamOptionsIncludeUsage(t *testing.T) {
	in := []byte(`{"model":"gpt-4","stream_options":{"include_usage":true}}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if !gjson.GetBytes(out, "stream_options.include_usage").Bool() {
		t.Fatalf("stream_options missing: %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_MaxCompletionTokens(t *testing.T) {
	in := []byte(`{"model":"gpt-4","max_completion_tokens":77}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "max_output_tokens").Int() != 77 {
		t.Fatalf("max_output_tokens = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_ReasoningObject(t *testing.T) {
	in := []byte(`{"model":"gpt-4","reasoning":{"effort":"high","summary":"short"}}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "reasoning.effort").String() != "high" {
		t.Fatalf("reasoning.effort = %s", string(out))
	}
	if gjson.GetBytes(out, "reasoning.summary").String() != "short" {
		t.Fatalf("reasoning.summary = %s", string(out))
	}
}

func TestConvertOpenAIChatToResponses_IgnoresNonFunctionTools(t *testing.T) {
	in := []byte(`{"model":"gpt-4","tools":[{"type":"file_search","file_search":{"max_results":1}}]}`)
	out := ConvertOpenAIChatCompletionsRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "tools").Exists() {
		t.Fatalf("expected no tools mapped: %s", string(out))
	}
}
