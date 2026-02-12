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
