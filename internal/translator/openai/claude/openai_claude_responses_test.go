package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertClaudeToResponses_Request(t *testing.T) {
	in := []byte(`{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.#").Int() == 0 {
		t.Fatalf("expected input: %s", string(out))
	}
	if gjson.GetBytes(out, "input.0.role").String() != "user" {
		t.Fatalf("role = %s", string(out))
	}
	if gjson.GetBytes(out, "input.0.content.0.type").String() != "input_text" {
		t.Fatalf("content type = %s", string(out))
	}
}

func TestConvertOpenAIResponsesToClaude_Stream(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":true}}`)
	add := []byte(`data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message","role":"assistant"},"output_index":0}`)
	delta := []byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","delta":"hi","output_index":0}`)
	done := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","created_at":1,"status":"completed"}}`)

	out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, add, &param)
	out = append(out, ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, delta, &param)...)
	out = append(out, ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, done, &param)...)
	joined := strings.Join(out, "")
	if !strings.Contains(joined, "event: message_start") || !strings.Contains(joined, "event: content_block_delta") {
		t.Fatalf("missing claude sse events: %s", joined)
	}
}

func TestConvertOpenAIResponsesToClaude_ToolCallStream(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":true}}`)
	add := []byte(`data: {"type":"response.output_item.added","item":{"id":"call_1","type":"function_call","name":"do"},"output_index":0}`)
	delta := []byte(`data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"a\":1}","output_index":0}`)
	done := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","created_at":1,"status":"completed"}}`)

	out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, add, &param)
	out = append(out, ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, delta, &param)...)
	out = append(out, ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, done, &param)...)
	joined := strings.Join(out, "")
	if !strings.Contains(joined, "\"type\":\"tool_use\"") {
		t.Fatalf("missing tool_use: %s", joined)
	}
}

func TestConvertClaudeRequestToOpenAIResponses_ThinkingStopsAndTopP(t *testing.T) {
	in := []byte(`{"max_tokens":10,"temperature":0.2,"stop_sequences":["END"],"thinking":{"type":"enabled","budget_tokens":600}}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, true)
	if gjson.GetBytes(out, "max_output_tokens").Int() != 10 {
		t.Fatalf("max_output_tokens missing: %s", string(out))
	}
	if gjson.GetBytes(out, "temperature").Float() != 0.2 {
		t.Fatalf("temperature missing: %s", string(out))
	}
	if gjson.GetBytes(out, "stop").String() != "END" {
		t.Fatalf("stop sequence missing: %s", string(out))
	}
	if gjson.GetBytes(out, "reasoning.effort").String() != "low" {
		t.Fatalf("reasoning effort mismatch: %s", string(out))
	}
	if !gjson.GetBytes(out, "stream").Bool() {
		t.Fatalf("stream flag missing: %s", string(out))
	}

	in = []byte(`{"top_p":0.9,"stop_sequences":["a","b"],"thinking":{"type":"disabled"}}`)
	out = ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "top_p").Float() != 0.9 {
		t.Fatalf("top_p missing: %s", string(out))
	}
	if gjson.GetBytes(out, "temperature").Exists() {
		t.Fatalf("temperature should not be set: %s", string(out))
	}
	if gjson.GetBytes(out, "stop.#").Int() != 2 {
		t.Fatalf("stop array missing: %s", string(out))
	}
	if gjson.GetBytes(out, "reasoning.effort").String() != "none" {
		t.Fatalf("disabled reasoning mismatch: %s", string(out))
	}

	in = []byte(`{"thinking":{"type":"adaptive"}}`)
	out = ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "reasoning.effort").String() != "xhigh" {
		t.Fatalf("adaptive reasoning mismatch: %s", string(out))
	}

	in = []byte(`{"thinking":{"type":"enabled"}}`)
	out = ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "reasoning.effort").String() != "auto" {
		t.Fatalf("enabled reasoning default mismatch: %s", string(out))
	}
}

func TestConvertClaudeRequestToOpenAIResponses_ToolsChoiceAndUser(t *testing.T) {
	in := []byte(`{"tools":[{"name":"calc","description":"d","input_schema":{"type":"object","properties":{"a":{"type":"number"}}}}],"tool_choice":{"type":"tool","name":"calc"},"user":"u1"}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "tools.0.type").String() != "function" {
		t.Fatalf("tool type mismatch: %s", string(out))
	}
	if gjson.GetBytes(out, "tools.0.parameters.properties.a.type").String() != "number" {
		t.Fatalf("tool schema mismatch: %s", string(out))
	}
	if gjson.GetBytes(out, "tool_choice.type").String() != "function" {
		t.Fatalf("tool_choice type mismatch: %s", string(out))
	}
	if gjson.GetBytes(out, "tool_choice.function.name").String() != "calc" {
		t.Fatalf("tool_choice name mismatch: %s", string(out))
	}
	if gjson.GetBytes(out, "user").String() != "u1" {
		t.Fatalf("user missing: %s", string(out))
	}

	in = []byte(`{"tool_choice":{"type":"any"}}`)
	out = ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "tool_choice").String() != "required" {
		t.Fatalf("tool_choice any mismatch: %s", string(out))
	}
}

func TestConvertClaudeRequestToOpenAIResponses_MessageOrderingAndToolUse(t *testing.T) {
	in := []byte(`{
  "system":"sys",
  "messages":[
    {"role":"user","content":[
      {"type":"text","text":"hi"},
      {"type":"image","source":{"type":"url","url":"https://example.com/a.png"}},
      {"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"ok"}]}
    ]},
    {"role":"assistant","content":[
      {"type":"tool_use","id":"call_2","name":"do"},
      {"type":"text","text":"done"},
      {"type":"thinking","text":"internal"},
      {"type":"redacted_thinking","text":"nope"}
    ]}
  ]
}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	items := gjson.GetBytes(out, "input").Array()
	if len(items) < 5 {
		t.Fatalf("unexpected input length: %s", string(out))
	}
	if items[0].Get("type").String() != "message" || items[0].Get("role").String() != "developer" {
		t.Fatalf("system mapping missing: %s", string(out))
	}
	if items[1].Get("type").String() != "function_call_output" || items[1].Get("output").String() != "ok" {
		t.Fatalf("tool_result mapping missing: %s", string(out))
	}
	if items[2].Get("type").String() != "message" || items[2].Get("role").String() != "user" {
		t.Fatalf("user message missing: %s", string(out))
	}
	if items[3].Get("type").String() != "message" || items[3].Get("role").String() != "assistant" {
		t.Fatalf("assistant message missing: %s", string(out))
	}
	if items[4].Get("type").String() != "function_call" || items[4].Get("name").String() != "do" {
		t.Fatalf("tool_use mapping missing: %s", string(out))
	}
	if items[4].Get("arguments").String() != "{}" {
		t.Fatalf("tool_use arguments default missing: %s", string(out))
	}

	userContent := items[2].Get("content").Array()
	contentTypes := map[string]bool{}
	for _, part := range userContent {
		contentTypes[part.Get("type").String()] = true
	}
	if !contentTypes["input_text"] || !contentTypes["input_image"] {
		t.Fatalf("user content missing text/image: %s", string(out))
	}
	if img := items[2].Get("content.1.image_url").String(); img != "https://example.com/a.png" {
		t.Fatalf("image url mismatch: %s", string(out))
	}

	if items[3].Get("content.0.type").String() != "output_text" || items[3].Get("content.0.text").String() != "done" {
		t.Fatalf("assistant content mapping missing: %s", string(out))
	}
}

func TestConvertClaudeContentPartToResponses_ImageRoles(t *testing.T) {
	part := gjson.Parse(`{"type":"image","source":{"type":"base64","data":"aaaa"}}`)
	if _, ok := convertClaudeContentPartToResponses(part, "assistant"); ok {
		t.Fatalf("assistant images should be rejected")
	}
	item, ok := convertClaudeContentPartToResponses(part, "user")
	if !ok {
		t.Fatalf("user image should be accepted")
	}
	if gjson.Get(item, "image_url").String() != "data:application/octet-stream;base64,aaaa" {
		t.Fatalf("image url mismatch: %s", item)
	}
}

func TestExtractClaudeImageURL_Fallback(t *testing.T) {
	part := gjson.Parse(`{"type":"image","url":"https://example.com/fallback.png"}`)
	if got := extractClaudeImageURL(part); got != "https://example.com/fallback.png" {
		t.Fatalf("fallback url mismatch: %s", got)
	}
}

func TestBuildMessageItemWithContent_EmptyRole(t *testing.T) {
	if _, ok := buildMessageItemWithContent("", nil); ok {
		t.Fatalf("expected empty role to fail")
	}
}

func TestConvertOpenAIResponsesResponseToClaude_StateAndToolArgs(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	created := []byte(`data: {"type":"response.created","response":{"id":"resp_1","created_at":123}}`)
	if out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, created, &param); len(out) != 0 {
		t.Fatalf("created should not emit: %v", out)
	}
	rp := param.(*responsesToClaudeParam)
	if rp.State.ResponseID != "resp_1" || rp.State.Created != 123 {
		t.Fatalf("state not updated: %+v", rp.State)
	}

	emptyDelta := []byte(`data: {"type":"response.output_text.delta","output_index":0,"delta":""}`)
	if out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, emptyDelta, &param); out != nil {
		t.Fatalf("empty delta should be ignored: %v", out)
	}

	add := []byte(`data: {"type":"response.output_item.added","item":{"call_id":"call_1","type":"function_call","name":"do"},"output_index":0}`)
	out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, add, &param)
	joined := strings.Join(out, "")
	if !strings.Contains(joined, "\"type\":\"tool_use\"") || !strings.Contains(joined, "\"id\":\"call_1\"") {
		t.Fatalf("tool_use mapping missing: %s", joined)
	}

	noID := []byte(`data: {"type":"response.function_call_arguments.delta","delta":"{}","output_index":0}`)
	if out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, noID, &param); out != nil {
		t.Fatalf("missing item_id should be ignored: %v", out)
	}

	args := []byte(`data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"a\":1}","output_index":0}`)
	_ = ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, args, &param)

	completed := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","created_at":123,"status":"completed"}}`)
	out = ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, completed, &param)
	joined = strings.Join(out, "")
	if !strings.Contains(joined, "\"partial_json\"") {
		t.Fatalf("tool args not flushed on completion: %s", joined)
	}
}

func TestConvertOpenAIResponsesResponseToClaude_UsageAndDone(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":true}}`)
	completed := []byte(`data: {"type":"response.completed","response":{"id":"resp_1","created_at":1,"status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`)
	out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, completed, &param)
	joined := strings.Join(out, "")
	if !strings.Contains(joined, "\"input_tokens\":1") || !strings.Contains(joined, "\"output_tokens\":2") {
		t.Fatalf("usage missing: %s", joined)
	}

	var param2 any
	req2 := []byte(`{"stream_options":{"include_usage":false}}`)
	out = ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req2, req2, completed, &param2)
	joined = strings.Join(out, "")
	if strings.Contains(joined, "\"input_tokens\":1") {
		t.Fatalf("usage should be omitted when include_usage=false: %s", joined)
	}

	var param3 any
	done := []byte("data: [DONE]")
	out = ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req2, req2, done, &param3)
	joined = strings.Join(out, "")
	if !strings.Contains(joined, "event: message_stop") {
		t.Fatalf("done marker missing: %s", joined)
	}
}

func TestMapResponsesUsageToOpenAI(t *testing.T) {
	usage := gjson.Parse(`{"input_tokens":2,"output_tokens":3,"total_tokens":5,"input_tokens_details":{"cached_tokens":1},"output_tokens_details":{"reasoning_tokens":4}}`)
	mapped := mapResponsesUsageToOpenAI(usage)
	if mapped["prompt_tokens"].(int64) != 2 {
		t.Fatalf("prompt_tokens missing")
	}
	if mapped["completion_tokens"].(int64) != 3 {
		t.Fatalf("completion_tokens missing")
	}
	if mapped["total_tokens"].(int64) != 5 {
		t.Fatalf("total_tokens missing")
	}
	if mapped["prompt_tokens_details"].(map[string]any)["cached_tokens"].(int64) != 1 {
		t.Fatalf("cached_tokens missing")
	}
	if mapped["completion_tokens_details"].(map[string]any)["reasoning_tokens"].(int64) != 4 {
		t.Fatalf("reasoning_tokens missing")
	}
}

func TestConvertClaudeRequestToOpenAIResponses_StringContent(t *testing.T) {
	in := []byte(`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"ok"}]}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	items := gjson.GetBytes(out, "input").Array()
	if len(items) < 2 {
		t.Fatalf("expected two messages: %s", string(out))
	}
	if items[0].Get("content.0.type").String() != "input_text" || items[0].Get("content.0.text").String() != "hi" {
		t.Fatalf("user string content mismatch: %s", string(out))
	}
	if items[1].Get("content.0.type").String() != "output_text" || items[1].Get("content.0.text").String() != "ok" {
		t.Fatalf("assistant string content mismatch: %s", string(out))
	}
}

func TestConvertClaudeRequestToOpenAIResponses_ToolUseInput(t *testing.T) {
	in := []byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"do","input":{"a":1}}]}]}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "input.1.type").String() != "function_call" {
		t.Fatalf("tool_use missing: %s", string(out))
	}
	if gjson.GetBytes(out, "input.1.arguments").String() != "{\"a\":1}" {
		t.Fatalf("tool_use arguments mismatch: %s", string(out))
	}
}

func TestConvertClaudeRequestToOpenAIResponses_ToolChoiceAutoAndDefault(t *testing.T) {
	in := []byte(`{"tool_choice":{"type":"auto"}}`)
	out := ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "tool_choice").String() != "auto" {
		t.Fatalf("tool_choice auto mismatch: %s", string(out))
	}

	in = []byte(`{"tool_choice":{"type":"weird"}}`)
	out = ConvertClaudeRequestToOpenAIResponses("gpt-4", in, false)
	if gjson.GetBytes(out, "tool_choice").String() != "auto" {
		t.Fatalf("tool_choice default mismatch: %s", string(out))
	}
}

func TestConvertOpenAIResponsesResponseToClaude_EmptyLineAndArgsBeforeAdd(t *testing.T) {
	var param any
	req := []byte(`{"stream_options":{"include_usage":false}}`)
	empty := []byte("   ")
	if out := ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, empty, &param); out != nil {
		t.Fatalf("empty line should be ignored: %v", out)
	}

	args := []byte(`data: {"type":"response.function_call_arguments.delta","item_id":"call_9","delta":"{}","output_index":0}`)
	_ = ConvertOpenAIResponsesResponseToClaude(context.Background(), "gpt-4", req, req, args, &param)
	rp := param.(*responsesToClaudeParam)
	if _, ok := rp.State.ToolIndexByID["call_9"]; !ok {
		t.Fatalf("expected tool index to be created")
	}
}

func TestBuildClaudeMessageItem_ArrayContent(t *testing.T) {
	content := gjson.Parse(`[{"type":"text","text":"sys"}]`)
	msg, ok := buildClaudeMessageItem("developer", content)
	if !ok {
		t.Fatalf("expected message")
	}
	if gjson.Get(msg, "content.0.type").String() != "input_text" {
		t.Fatalf("content type mismatch: %s", msg)
	}
}

func TestConvertClaudeContentPartToResponses_EmptyTextAndUnknown(t *testing.T) {
	if _, ok := convertClaudeContentPartToResponses(gjson.Parse(`{"type":"text","text":"   "}`), "user"); ok {
		t.Fatalf("expected empty text to be rejected")
	}
	if _, ok := convertClaudeContentPartToResponses(gjson.Parse(`{"type":"unknown"}`), "user"); ok {
		t.Fatalf("expected unknown type to be rejected")
	}
	if _, ok := convertClaudeContentPartToResponses(gjson.Parse(`{"type":"image"}`), "user"); ok {
		t.Fatalf("expected empty image to be rejected")
	}
}
