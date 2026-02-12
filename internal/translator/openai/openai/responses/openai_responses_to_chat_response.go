package responses

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type responsesToChatState struct {
	ResponseID    string
	Created       int64
	RoleSent      bool
	ToolCallSeen  bool
	NextToolIndex int
	ToolIndexByID map[string]int
	ToolNameByID  map[string]string
}

type responsesToChatParam struct {
	IncludeUsage bool
	State        *responsesToChatState
}

var responsesIDCounter uint64

func ensureResponsesToChatParam(param *any, requestRawJSON []byte) *responsesToChatParam {
	if param == nil {
		local := any(nil)
		param = &local
	}
	if *param == nil {
		includeUsage := gjson.GetBytes(requestRawJSON, "stream_options.include_usage").Bool()
		*param = &responsesToChatParam{
			IncludeUsage: includeUsage,
			State: &responsesToChatState{
				ToolIndexByID: map[string]int{},
				ToolNameByID:  map[string]string{},
			},
		}
	}
	return (*param).(*responsesToChatParam)
}

func mapResponsesUsageToChat(out string, usage gjson.Result) string {
	if !usage.Exists() {
		return out
	}
	mapped := false
	if v := usage.Get("input_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "usage.prompt_tokens", v.Int())
		mapped = true
	}
	if v := usage.Get("input_tokens_details.cached_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "usage.prompt_tokens_details.cached_tokens", v.Int())
		mapped = true
	}
	if v := usage.Get("output_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "usage.completion_tokens", v.Int())
		mapped = true
	}
	if v := usage.Get("output_tokens_details.reasoning_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "usage.completion_tokens_details.reasoning_tokens", v.Int())
		mapped = true
	}
	if v := usage.Get("total_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "usage.total_tokens", v.Int())
		mapped = true
	}
	if !mapped {
		out, _ = sjson.Set(out, "usage", usage.Value())
	}
	return out
}

// ConvertOpenAIResponsesResponseToOpenAIChatCompletions converts OpenAI Responses SSE
// payloads into OpenAI Chat Completions streaming chunks.
func ConvertOpenAIResponsesResponseToOpenAIChatCompletions(ctx context.Context, modelName string, originalReq, requestRawJSON, rawJSON []byte, param *any) []string {
	_ = ctx
	rp := ensureResponsesToChatParam(param, requestRawJSON)
	st := rp.State

	line := bytes.TrimSpace(rawJSON)
	if bytes.HasPrefix(line, []byte("data:")) {
		line = bytes.TrimSpace(line[5:])
	}
	if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
		return nil
	}
	if st.ResponseID == "" {
		st.ResponseID = fmt.Sprintf("resp_%d", atomic.AddUint64(&responsesIDCounter, 1))
	}

	evt := gjson.GetBytes(line, "type").String()
	switch evt {
	case "response.created", "response.in_progress":
		if resp := gjson.GetBytes(line, "response"); resp.Exists() {
			if v := resp.Get("id"); v.Exists() {
				st.ResponseID = v.String()
			}
			if v := resp.Get("created_at"); v.Exists() {
				st.Created = v.Int()
			}
		}
		return nil
	case "response.output_item.added":
		item := gjson.GetBytes(line, "item")
		choiceIndex := 0
		if item.Get("type").String() == "message" && item.Get("role").String() == "assistant" && !st.RoleSent {
			st.RoleSent = true
			return []string{"data: " + buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
				"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
			})}
		}
		if item.Get("type").String() == "function_call" {
			st.ToolCallSeen = true
			itemID := item.Get("id").String()
			if _, ok := st.ToolIndexByID[itemID]; !ok {
				st.ToolIndexByID[itemID] = st.NextToolIndex
				st.NextToolIndex++
			}
			st.ToolNameByID[itemID] = item.Get("name").String()
			out := make([]string, 0, 2)
			if !st.RoleSent {
				st.RoleSent = true
				out = append(out, "data: "+buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
					"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
				}))
			}
			out = append(out, "data: "+buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
				"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{
					"tool_calls": []any{map[string]any{
						"index": st.ToolIndexByID[itemID],
						"id":    itemID,
						"type":  "function",
						"function": map[string]any{
							"name": st.ToolNameByID[itemID],
						},
					}},
				}, "finish_reason": nil}},
			}))
			return out
		}
	case "response.output_text.delta":
		choiceIndex := 0
		delta := gjson.GetBytes(line, "delta").String()
		if delta != "" {
			out := make([]string, 0, 2)
			if !st.RoleSent {
				st.RoleSent = true
				out = append(out, "data: "+buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
					"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
				}))
			}
			out = append(out, "data: "+buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
				"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{"content": delta}, "finish_reason": nil}},
			}))
			return out
		}
	case "response.function_call_arguments.delta":
		st.ToolCallSeen = true
		choiceIndex := 0
		itemID := gjson.GetBytes(line, "item_id").String()
		if _, ok := st.ToolIndexByID[itemID]; !ok {
			st.ToolIndexByID[itemID] = st.NextToolIndex
			st.NextToolIndex++
		}
		delta := gjson.GetBytes(line, "delta").String()
		out := make([]string, 0, 2)
		if !st.RoleSent {
			st.RoleSent = true
			out = append(out, "data: "+buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
				"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
			}))
		}
		functionPayload := map[string]any{
			"arguments": delta,
		}
		if name := st.ToolNameByID[itemID]; name != "" {
			functionPayload["name"] = name
		}
		out = append(out, "data: "+buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
			"choices": []any{map[string]any{"index": choiceIndex, "delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": st.ToolIndexByID[itemID],
					"id":    itemID,
					"type":  "function",
					"function": functionPayload,
				}},
			}, "finish_reason": nil}},
		}))
		return out
	case "response.completed":
		if resp := gjson.GetBytes(line, "response"); resp.Exists() {
			if v := resp.Get("id"); v.Exists() && v.String() != "" {
				st.ResponseID = v.String()
			}
			if v := resp.Get("created_at"); v.Exists() {
				st.Created = v.Int()
			}
		}
		// If any tool call was emitted, finish with tool_calls to match chat semantics
		// (we don't downgrade to stop after content appears).
		finish := "stop"
		if st.ToolCallSeen {
			finish = "tool_calls"
		}
		chunk := buildChatChunk(modelName, st.ResponseID, st.Created, map[string]any{
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}},
		})
		if rp.IncludeUsage {
			if usage := gjson.GetBytes(line, "response.usage"); usage.Exists() {
				chunk = mapResponsesUsageToChat(chunk, usage)
			}
		}
		return []string{"data: " + chunk, "data: [DONE]"}
	}

	return nil
}

func buildChatChunk(modelName, id string, created int64, override map[string]any) string {
	out := `{"id":"","object":"chat.completion.chunk","created":0,"model":"","choices":[{"index":0,"delta":{},"finish_reason":null}]}`
	if id != "" {
		out, _ = sjson.Set(out, "id", id)
	}
	if created != 0 {
		out, _ = sjson.Set(out, "created", created)
	}
	out, _ = sjson.Set(out, "model", modelName)
	for k, v := range override {
		out, _ = sjson.Set(out, k, v)
	}
	return out
}

// ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream converts OpenAI Responses
// non-stream responses into OpenAI Chat Completions responses.
func ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream(_ context.Context, modelName string, _, _ []byte, rawJSON []byte, _ *any) string {
	root := gjson.ParseBytes(rawJSON)
	out := `{"id":"","object":"chat.completion","created":0,"model":"","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{}}`
	out, _ = sjson.Set(out, "id", root.Get("id").String())
	out, _ = sjson.Set(out, "created", root.Get("created_at").Int())
	out, _ = sjson.Set(out, "model", modelName)

	var text strings.Builder
	toolCalls := make([]any, 0)
	if items := root.Get("output"); items.IsArray() {
		items.ForEach(func(_, it gjson.Result) bool {
			switch it.Get("type").String() {
			case "message":
				if it.Get("role").String() == "assistant" {
					if content := it.Get("content"); content.IsArray() {
						content.ForEach(func(_, part gjson.Result) bool {
							switch part.Get("type").String() {
							case "output_text", "text":
								text.WriteString(part.Get("text").String())
							case "refusal":
								out, _ = sjson.Set(out, "choices.0.message.refusal", part.Get("refusal").String())
							}
							return true
						})
					}
				}
			case "function_call":
				call := `{"id":"","type":"function","function":{"name":"","arguments":""}}`
				call, _ = sjson.Set(call, "id", it.Get("id").String())
				call, _ = sjson.Set(call, "function.name", it.Get("name").String())
				call, _ = sjson.Set(call, "function.arguments", it.Get("arguments").String())
				toolCalls = append(toolCalls, gjson.Parse(call).Value())
			}
			return true
		})
	}
	out, _ = sjson.Set(out, "choices.0.message.content", text.String())
	if len(toolCalls) > 0 {
		out, _ = sjson.Set(out, "choices.0.message.tool_calls", toolCalls)
		// If any tool call exists, finish with tool_calls (even if content is present).
		out, _ = sjson.Set(out, "choices.0.finish_reason", "tool_calls")
	}
	if usage := root.Get("usage"); usage.Exists() {
		out = mapResponsesUsageToChat(out, usage)
	}
	return out
}
