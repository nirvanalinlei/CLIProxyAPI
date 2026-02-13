package responses

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertOpenAIResponsesToChat_NonStream_ToolCalls(t *testing.T) {
	in := []byte(`{"id":"resp_1","object":"response","created_at":1,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]},{"type":"function_call","id":"call_1","name":"do","arguments":"{\"a\":1}"}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`)
	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream(context.Background(), "gpt-4", nil, nil, in, nil)
	if gjson.Get(out, "choices.0.message.tool_calls.0.id").String() != "call_1" {
		t.Fatalf("missing tool_call: %s", out)
	}
	if gjson.Get(out, "choices.0.finish_reason").String() != "tool_calls" {
		t.Fatalf("finish_reason = %s", out)
	}
}

func TestConvertOpenAIResponsesToChat_Stream(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":true}}`)
	line1 := []byte(`data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message","role":"assistant"},"output_index":0}`)
	line2 := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hi","output_index":0}`)
	line2b := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_2","delta":"yo","output_index":1}`)
	line3 := []byte(`data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","name":"do"},"output_index":1}`)
	line4 := []byte(`data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"a\":1}","output_index":1}`)
	line5 := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","created_at":1,"status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`)

	c1 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line1, &param)
	if gjson.Get(strings.TrimPrefix(c1[0], "data: "), "choices.0.delta.role").String() != "assistant" {
		t.Fatalf("role delta missing: %s", c1[0])
	}
	c2 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line2, &param)
	if gjson.Get(strings.TrimPrefix(c2[0], "data: "), "choices.0.delta.content").String() != "hi" {
		t.Fatalf("content delta missing: %s", c2[0])
	}
	c2b := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line2b, &param)
	if gjson.Get(strings.TrimPrefix(c2b[0], "data: "), "choices.0.index").Int() != 0 {
		t.Fatalf("choice index missing: %s", c2b[0])
	}
	c3 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line3, &param)
	if gjson.Get(strings.TrimPrefix(c3[0], "data: "), "choices.0.delta.tool_calls.0.function.name").String() != "do" {
		t.Fatalf("tool name missing: %s", c3[0])
	}
	c4 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line4, &param)
	if gjson.Get(strings.TrimPrefix(c4[0], "data: "), "choices.0.delta.tool_calls.0.function.arguments").String() != "{\"a\":1}" {
		t.Fatalf("tool args delta missing: %s", c4[0])
	}
	c5 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line5, &param)
	if gjson.Get(strings.TrimPrefix(c5[0], "data: "), "choices.0.finish_reason").String() != "tool_calls" {
		t.Fatalf("finish_reason missing: %s", c5[0])
	}
	if !strings.HasPrefix(c5[1], "data: [DONE]") {
		t.Fatalf("missing DONE: %s", c5[1])
	}
}

func TestConvertOpenAIResponsesToChat_StreamStop(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	line := []byte(`data: {"type":"response.completed","response":{"id":"resp_2","created_at":2,"status":"completed"}}`)
	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line, &param)
	if gjson.Get(strings.TrimPrefix(out[0], "data: "), "choices.0.finish_reason").String() != "stop" {
		t.Fatalf("finish_reason = %s", out[0])
	}
}

func TestResponsesToChat_Stream_ToolOnlyEmitsRoleFirst(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":true}}`)

	add := []byte(`data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","name":"do"},"output_index":0}`)
	delta := []byte(`data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"a\":1}","output_index":0}`)
	done := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","created_at":1,"status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`)

	c1 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, add, &param)
	if gjson.Get(strings.TrimPrefix(c1[0], "data: "), "choices.0.delta.role").String() != "assistant" {
		t.Fatalf("role delta missing: %s", c1[0])
	}

	c2 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, delta, &param)
	if gjson.Get(strings.TrimPrefix(c2[0], "data: "), "choices.0.delta.tool_calls.0.function.arguments").String() != "{\"a\":1}" {
		t.Fatalf("tool args delta missing: %s", c2[0])
	}

	c3 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, done, &param)
	if gjson.Get(strings.TrimPrefix(c3[0], "data: "), "choices.0.finish_reason").String() != "tool_calls" {
		t.Fatalf("finish_reason missing: %s", c3[0])
	}
	if gjson.Get(strings.TrimPrefix(c3[0], "data: "), "usage.prompt_tokens").Int() != 1 {
		t.Fatalf("usage mapping missing: %s", c3[0])
	}
}

func TestResponsesToChat_Stream_MultiOutputIndexAggregatesToChoice0(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	line0 := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"a","output_index":0}`)
	line1 := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_2","delta":"b","output_index":1}`)

	c0 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line0, &param)
	c1 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line1, &param)

	if gjson.Get(strings.TrimPrefix(c0[0], "data: "), "choices.0.index").Int() != 0 {
		t.Fatalf("choice index 0 missing: %s", c0[0])
	}
	if gjson.Get(strings.TrimPrefix(c1[0], "data: "), "choices.0.index").Int() != 0 {
		t.Fatalf("choice index not aggregated: %s", c1[0])
	}
}

func TestResponsesToChat_Stream_ToolArgsBeforeItemAdded(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	delta := []byte(`data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{}","output_index":0}`)
	add := []byte(`data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","name":"do"},"output_index":0}`)

	c1 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, delta, &param)
	if gjson.Get(strings.TrimPrefix(c1[0], "data: "), "choices.0.delta.role").String() != "assistant" {
		t.Fatalf("role delta missing: %s", c1[0])
	}
	if gjson.Get(strings.TrimPrefix(c1[1], "data: "), "choices.0.delta.tool_calls.0.index").Int() != 0 {
		t.Fatalf("tool index missing: %s", c1[1])
	}
	if gjson.Get(strings.TrimPrefix(c1[1], "data: "), "choices.0.delta.tool_calls.0.function.name").Exists() {
		t.Fatalf("unexpected tool name: %s", c1[1])
	}

	c2 := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, add, &param)
	if gjson.Get(strings.TrimPrefix(c2[0], "data: "), "choices.0.delta.tool_calls.0.function.name").String() != "do" {
		t.Fatalf("tool name missing: %s", c2[0])
	}
}

func TestResponsesToChat_Stream_ContentDeltaFirstEmitsRole(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	line := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hi","output_index":0}`)

	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line, &param)
	if gjson.Get(strings.TrimPrefix(out[0], "data: "), "choices.0.delta.role").String() != "assistant" {
		t.Fatalf("role delta missing: %s", out[0])
	}
	if gjson.Get(strings.TrimPrefix(out[1], "data: "), "choices.0.delta.content").String() != "hi" {
		t.Fatalf("content delta missing: %s", out[1])
	}
}

func TestResponsesToChat_NonStream_UsageMappedToChat(t *testing.T) {
	in := []byte(`{"id":"resp_1","object":"response","created_at":1,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3,"input_tokens_details":{"cached_tokens":1},"output_tokens_details":{"reasoning_tokens":4}}}`)
	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream(context.Background(), "gpt-4", nil, nil, in, nil)
	if gjson.Get(out, "usage.prompt_tokens").Int() != 1 || gjson.Get(out, "usage.completion_tokens").Int() != 2 {
		t.Fatalf("usage mapping missing: %s", out)
	}
	if gjson.Get(out, "usage.prompt_tokens_details.cached_tokens").Int() != 1 {
		t.Fatalf("cached usage missing: %s", out)
	}
	if gjson.Get(out, "usage.completion_tokens_details.reasoning_tokens").Int() != 4 {
		t.Fatalf("reasoning usage missing: %s", out)
	}
}

func TestResponsesToChat_Stream_ParamNilAndDoneIgnored(t *testing.T) {
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	line := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hi","output_index":0}`)
	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line, nil)
	if len(out) == 0 {
		t.Fatalf("expected output with nil param")
	}

	done := []byte("data: [DONE]")
	out = ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, done, nil)
	if out != nil {
		t.Fatalf("expected nil for done marker: %v", out)
	}
}

func TestResponsesToChat_Stream_ResponseCreatedUpdatesState(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	line := []byte(`data: {"type":"response.created","response":{"id":"resp_1","created_at":123}}`)
	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletions(context.Background(), "gpt-4", req, req, line, &param)
	if out != nil {
		t.Fatalf("expected no output: %v", out)
	}
	rp := param.(*responsesToChatParam)
	if rp.State.ResponseID != "resp_1" || rp.State.Created != 123 {
		t.Fatalf("state not updated: %+v", rp.State)
	}
}

func TestResponsesToChat_MapUsageFallback(t *testing.T) {
	base := `{"id":"x"}`
	out := mapResponsesUsageToChat(base, gjson.Result{})
	if out != base {
		t.Fatalf("expected unchanged output")
	}
	usage := gjson.Parse(`{"foo":1}`)
	out = mapResponsesUsageToChat(base, usage)
	if gjson.Get(out, "usage.foo").Int() != 1 {
		t.Fatalf("usage passthrough missing: %s", out)
	}
}

func TestResponsesToChat_NonStream_TextType(t *testing.T) {
	in := []byte(`{"id":"resp_2","object":"response","created_at":1,"output":[{"type":"message","role":"assistant","content":[{"type":"text","text":"hi"}]}]}`)
	out := ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream(context.Background(), "gpt-4", nil, nil, in, nil)
	if gjson.Get(out, "choices.0.message.content").String() != "hi" {
		t.Fatalf("content missing: %s", out)
	}
}
