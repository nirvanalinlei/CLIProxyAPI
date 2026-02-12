package claude

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/translator/openai/openai/responses"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type responsesToClaudeState struct {
	ResponseID               string
	Created                  int64
	RoleSent                 bool
	ToolCallSeen             bool
	ChoiceIndexByOutputIndex map[int]int
	ToolIndexByID            map[string]int
	ToolNameByID             map[string]string
}

type responsesToClaudeParam struct {
	IncludeUsage bool
	ClaudeParam  *ConvertOpenAIResponseToAnthropicParams
	State        *responsesToClaudeState
}

var claudeResponseIDCounter uint64

func ensureResponsesToClaudeParam(param *any, requestRawJSON []byte) *responsesToClaudeParam {
	if *param == nil {
		includeUsage := gjson.GetBytes(requestRawJSON, "stream_options.include_usage").Bool()
		*param = &responsesToClaudeParam{
			IncludeUsage: includeUsage,
			ClaudeParam: &ConvertOpenAIResponseToAnthropicParams{
				MessageID:                   "",
				Model:                       "",
				CreatedAt:                   0,
				ContentAccumulator:          strings.Builder{},
				ToolCallsAccumulator:        nil,
				TextContentBlockStarted:     false,
				ThinkingContentBlockStarted: false,
				FinishReason:                "",
				ContentBlocksStopped:        false,
				MessageDeltaSent:            false,
				MessageStarted:              false,
				MessageStopSent:             false,
				ToolCallBlockIndexes:        make(map[int]int),
				TextContentBlockIndex:       -1,
				ThinkingContentBlockIndex:   -1,
				NextContentBlockIndex:       0,
			},
			State: &responsesToClaudeState{
				ChoiceIndexByOutputIndex: map[int]int{},
				ToolIndexByID:            map[string]int{},
				ToolNameByID:             map[string]string{},
			},
		}
	}
	return (*param).(*responsesToClaudeParam)
}

// ConvertClaudeRequestToOpenAIResponses converts Anthropic Claude requests into OpenAI Responses requests.
func ConvertClaudeRequestToOpenAIResponses(modelName string, inputRawJSON []byte, stream bool) []byte {
	root := gjson.ParseBytes(inputRawJSON)
	out := `{"model":"","input":[],"stream":false}`
	out, _ = sjson.Set(out, "model", modelName)
	out, _ = sjson.Set(out, "stream", stream)

	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_output_tokens", maxTokens.Int())
	}

	if temp := root.Get("temperature"); temp.Exists() {
		out, _ = sjson.Set(out, "temperature", temp.Float())
	} else if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.Set(out, "top_p", topP.Float())
	}

	if stopSequences := root.Get("stop_sequences"); stopSequences.Exists() && stopSequences.IsArray() {
		var stops []string
		stopSequences.ForEach(func(_, value gjson.Result) bool {
			stops = append(stops, value.String())
			return true
		})
		if len(stops) == 1 {
			out, _ = sjson.Set(out, "stop", stops[0])
		} else if len(stops) > 1 {
			out, _ = sjson.Set(out, "stop", stops)
		}
	}

	if thinkingConfig := root.Get("thinking"); thinkingConfig.Exists() && thinkingConfig.IsObject() {
		if thinkingType := thinkingConfig.Get("type"); thinkingType.Exists() {
			switch thinkingType.String() {
			case "enabled":
				if budgetTokens := thinkingConfig.Get("budget_tokens"); budgetTokens.Exists() {
					budget := int(budgetTokens.Int())
					if effort, ok := thinking.ConvertBudgetToLevel(budget); ok && effort != "" {
						out, _ = sjson.Set(out, "reasoning.effort", effort)
					}
				} else {
					if effort, ok := thinking.ConvertBudgetToLevel(-1); ok && effort != "" {
						out, _ = sjson.Set(out, "reasoning.effort", effort)
					}
				}
			case "adaptive":
				out, _ = sjson.Set(out, "reasoning.effort", string(thinking.LevelXHigh))
			case "disabled":
				if effort, ok := thinking.ConvertBudgetToLevel(0); ok && effort != "" {
					out, _ = sjson.Set(out, "reasoning.effort", effort)
				}
			}
		}
	}

	if system := root.Get("system"); system.Exists() {
		if systemMsg, ok := buildClaudeMessageItem("developer", system); ok {
			out, _ = sjson.SetRaw(out, "input.-1", systemMsg)
		}
	}

	if messages := root.Get("messages"); messages.Exists() && messages.IsArray() {
		messages.ForEach(func(_, message gjson.Result) bool {
			role := message.Get("role").String()
			content := message.Get("content")

			var toolResults []string
			var toolCalls []string

			if content.Exists() && content.IsArray() {
				var contentItems []string
				content.ForEach(func(_, part gjson.Result) bool {
					switch part.Get("type").String() {
					case "tool_result":
						item := `{"type":"function_call_output","call_id":"","output":""}`
						item, _ = sjson.Set(item, "call_id", part.Get("tool_use_id").String())
						item, _ = sjson.Set(item, "output", convertClaudeToolResultContentToString(part.Get("content")))
						toolResults = append(toolResults, item)
					case "tool_use":
						if role == "assistant" {
							call := `{"type":"function_call","call_id":"","name":"","arguments":""}`
							call, _ = sjson.Set(call, "call_id", part.Get("id").String())
							call, _ = sjson.Set(call, "name", part.Get("name").String())
							if input := part.Get("input"); input.Exists() {
								call, _ = sjson.Set(call, "arguments", input.Raw)
							} else {
								call, _ = sjson.Set(call, "arguments", "{}")
							}
							toolCalls = append(toolCalls, call)
						}
					case "thinking", "redacted_thinking":
						// ignore
					default:
						if item, ok := convertClaudeContentPartToResponses(part, role); ok {
							contentItems = append(contentItems, item)
						}
					}
					return true
				})

				for _, tr := range toolResults {
					out, _ = sjson.SetRaw(out, "input.-1", tr)
				}

				if msg, ok := buildMessageItemWithContent(role, contentItems); ok {
					out, _ = sjson.SetRaw(out, "input.-1", msg)
				}

				for _, tc := range toolCalls {
					out, _ = sjson.SetRaw(out, "input.-1", tc)
				}

				return true
			}

			if content.Exists() && content.Type == gjson.String {
				contentItems := []string{}
				text := strings.TrimSpace(content.String())
				if text != "" {
					item := `{"type":"","text":""}`
					itemType := "input_text"
					if role == "assistant" {
						itemType = "output_text"
					}
					item, _ = sjson.Set(item, "type", itemType)
					item, _ = sjson.Set(item, "text", content.String())
					contentItems = append(contentItems, item)
				}
				if msg, ok := buildMessageItemWithContent(role, contentItems); ok {
					out, _ = sjson.SetRaw(out, "input.-1", msg)
				}
			}

			return true
		})
	}

	if tools := root.Get("tools"); tools.Exists() && tools.IsArray() {
		var toolsJSON = "[]"
		tools.ForEach(func(_, tool gjson.Result) bool {
			item := `{"type":"function","name":"","description":"","parameters":{}}`
			item, _ = sjson.Set(item, "name", tool.Get("name").String())
			item, _ = sjson.Set(item, "description", tool.Get("description").String())
			if inputSchema := tool.Get("input_schema"); inputSchema.Exists() {
				item, _ = sjson.SetRaw(item, "parameters", inputSchema.Raw)
			}
			toolsJSON, _ = sjson.SetRaw(toolsJSON, "-1", item)
			return true
		})
		if gjson.Parse(toolsJSON).IsArray() && len(gjson.Parse(toolsJSON).Array()) > 0 {
			out, _ = sjson.SetRaw(out, "tools", toolsJSON)
		}
	}

	if toolChoice := root.Get("tool_choice"); toolChoice.Exists() {
		switch toolChoice.Get("type").String() {
		case "auto":
			out, _ = sjson.Set(out, "tool_choice", "auto")
		case "any":
			out, _ = sjson.Set(out, "tool_choice", "required")
		case "tool":
			toolName := toolChoice.Get("name").String()
			toolChoiceJSON := `{"type":"function","function":{"name":""}}`
			toolChoiceJSON, _ = sjson.Set(toolChoiceJSON, "function.name", toolName)
			out, _ = sjson.SetRaw(out, "tool_choice", toolChoiceJSON)
		default:
			out, _ = sjson.Set(out, "tool_choice", "auto")
		}
	}

	if user := root.Get("user"); user.Exists() {
		out, _ = sjson.Set(out, "user", user.String())
	}

	return []byte(out)
}

// ConvertOpenAIResponsesResponseToClaude converts OpenAI Responses stream events to Claude SSE.
func ConvertOpenAIResponsesResponseToClaude(ctx context.Context, modelName string, originalReq, requestRawJSON, rawJSON []byte, param *any) []string {
	_ = ctx
	rp := ensureResponsesToClaudeParam(param, requestRawJSON)
	st := rp.State

	line := bytes.TrimSpace(rawJSON)
	if bytes.HasPrefix(line, dataTag) {
		line = bytes.TrimSpace(line[5:])
	}
	if len(line) == 0 {
		return nil
	}
	if bytes.Equal(line, []byte("[DONE]")) {
		return convertOpenAIDoneToAnthropic(rp.ClaudeParam)
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
		outputIndex := int(gjson.GetBytes(line, "output_index").Int())
		choiceIndex := choiceIndexForOutput(st, outputIndex)

		if item.Get("type").String() == "message" && item.Get("role").String() == "assistant" && !st.RoleSent {
			st.RoleSent = true
			chunk := buildOpenAIChatChunk(modelName, st, choiceIndex, map[string]any{"role": "assistant"}, "", gjson.Result{})
			return convertOpenAIStreamingChunkToAnthropic([]byte(chunk), rp.ClaudeParam)
		}

		if item.Get("type").String() == "function_call" {
			st.ToolCallSeen = true
			itemID := item.Get("id").String()
			if itemID == "" {
				itemID = item.Get("call_id").String()
			}
			if _, ok := st.ToolIndexByID[itemID]; !ok {
				st.ToolIndexByID[itemID] = len(st.ToolIndexByID)
			}
			st.ToolNameByID[itemID] = item.Get("name").String()
			toolCall := map[string]any{
				"index": st.ToolIndexByID[itemID],
				"id":    itemID,
				"type":  "function",
				"function": map[string]any{
					"name": st.ToolNameByID[itemID],
				},
			}
			chunk := buildOpenAIChatChunk(modelName, st, choiceIndex, map[string]any{"tool_calls": []any{toolCall}}, "", gjson.Result{})
			return convertOpenAIStreamingChunkToAnthropic([]byte(chunk), rp.ClaudeParam)
		}

	case "response.output_text.delta":
		outputIndex := int(gjson.GetBytes(line, "output_index").Int())
		choiceIndex := choiceIndexForOutput(st, outputIndex)
		delta := gjson.GetBytes(line, "delta").String()
		if delta == "" {
			return nil
		}
		chunk := buildOpenAIChatChunk(modelName, st, choiceIndex, map[string]any{"content": delta}, "", gjson.Result{})
		return convertOpenAIStreamingChunkToAnthropic([]byte(chunk), rp.ClaudeParam)

	case "response.function_call_arguments.delta":
		outputIndex := int(gjson.GetBytes(line, "output_index").Int())
		choiceIndex := choiceIndexForOutput(st, outputIndex)
		itemID := gjson.GetBytes(line, "item_id").String()
		delta := gjson.GetBytes(line, "delta").String()
		if itemID == "" {
			return nil
		}
		if _, ok := st.ToolIndexByID[itemID]; !ok {
			st.ToolIndexByID[itemID] = len(st.ToolIndexByID)
		}
		toolCall := map[string]any{
			"index": st.ToolIndexByID[itemID],
			"id":    itemID,
			"type":  "function",
			"function": map[string]any{
				"arguments": delta,
			},
		}
		chunk := buildOpenAIChatChunk(modelName, st, choiceIndex, map[string]any{"tool_calls": []any{toolCall}}, "", gjson.Result{})
		return convertOpenAIStreamingChunkToAnthropic([]byte(chunk), rp.ClaudeParam)

	case "response.completed":
		if resp := gjson.GetBytes(line, "response"); resp.Exists() {
			if v := resp.Get("id"); v.Exists() && v.String() != "" {
				st.ResponseID = v.String()
			}
			if v := resp.Get("created_at"); v.Exists() {
				st.Created = v.Int()
			}
		}

		finish := "stop"
		if st.ToolCallSeen {
			finish = "tool_calls"
		}

		var usage gjson.Result
		if rp.IncludeUsage {
			usage = gjson.GetBytes(line, "response.usage")
		}
		chunk := buildOpenAIChatChunk(modelName, st, 0, map[string]any{}, finish, usage)
		out := convertOpenAIStreamingChunkToAnthropic([]byte(chunk), rp.ClaudeParam)
		if !usage.Exists() {
			out = append(out, convertOpenAIDoneToAnthropic(rp.ClaudeParam)...)
		}
		return out
	}

	return nil
}

// ConvertOpenAIResponsesResponseToClaudeNonStream converts Responses non-stream to Claude non-stream.
func ConvertOpenAIResponsesResponseToClaudeNonStream(ctx context.Context, modelName string, originalReq, requestRawJSON, rawJSON []byte, _ *any) string {
	chat := responses.ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream(ctx, modelName, originalReq, requestRawJSON, rawJSON, nil)
	return ConvertOpenAIResponseToClaudeNonStream(ctx, modelName, originalReq, requestRawJSON, []byte(chat), nil)
}

func buildClaudeMessageItem(role string, content gjson.Result) (string, bool) {
	var contentItems []string
	if content.Type == gjson.String {
		text := strings.TrimSpace(content.String())
		if text != "" {
			item := `{"type":"input_text","text":""}`
			item, _ = sjson.Set(item, "text", content.String())
			contentItems = append(contentItems, item)
		}
	} else if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			if item, ok := convertClaudeContentPartToResponses(part, role); ok {
				contentItems = append(contentItems, item)
			}
			return true
		})
	}

	return buildMessageItemWithContent(role, contentItems)
}

func buildMessageItemWithContent(role string, contentItems []string) (string, bool) {
	if role == "" {
		return "", false
	}
	msg := `{"type":"message","role":"","content":[]}`
	msg, _ = sjson.Set(msg, "role", role)
	for _, item := range contentItems {
		msg, _ = sjson.SetRaw(msg, "content.-1", item)
	}
	return msg, true
}

func convertClaudeContentPartToResponses(part gjson.Result, role string) (string, bool) {
	partType := part.Get("type").String()
	switch partType {
	case "text":
		text := part.Get("text").String()
		if strings.TrimSpace(text) == "" {
			return "", false
		}
		itemType := "input_text"
		if role == "assistant" {
			itemType = "output_text"
		}
		item := `{"type":"","text":""}`
		item, _ = sjson.Set(item, "type", itemType)
		item, _ = sjson.Set(item, "text", text)
		return item, true
	case "image":
		if role == "assistant" || role == "developer" {
			return "", false
		}
		imageURL := extractClaudeImageURL(part)
		if imageURL == "" {
			return "", false
		}
		item := `{"type":"input_image","image_url":""}`
		item, _ = sjson.Set(item, "image_url", imageURL)
		return item, true
	default:
		return "", false
	}
}

func extractClaudeImageURL(part gjson.Result) string {
	var imageURL string
	if source := part.Get("source"); source.Exists() {
		sourceType := source.Get("type").String()
		switch sourceType {
		case "base64":
			mediaType := source.Get("media_type").String()
			if mediaType == "" {
				mediaType = "application/octet-stream"
			}
			data := source.Get("data").String()
			if data != "" {
				imageURL = "data:" + mediaType + ";base64," + data
			}
		case "url":
			imageURL = source.Get("url").String()
		}
	}
	if imageURL == "" {
		imageURL = part.Get("url").String()
	}
	return imageURL
}

func choiceIndexForOutput(st *responsesToClaudeState, outputIndex int) int {
	if v, ok := st.ChoiceIndexByOutputIndex[outputIndex]; ok {
		return v
	}
	st.ChoiceIndexByOutputIndex[outputIndex] = outputIndex
	return outputIndex
}

func buildOpenAIChatChunk(modelName string, st *responsesToClaudeState, choiceIndex int, delta map[string]any, finishReason string, usage gjson.Result) string {
	if st.ResponseID == "" {
		st.ResponseID = fmt.Sprintf("resp_%d", atomic.AddUint64(&claudeResponseIDCounter, 1))
	}
	if st.Created == 0 {
		st.Created = time.Now().Unix()
	}

	out := `{"id":"","object":"chat.completion.chunk","created":0,"model":"","choices":[{"index":0,"delta":{},"finish_reason":null}]}`
	out, _ = sjson.Set(out, "id", st.ResponseID)
	out, _ = sjson.Set(out, "created", st.Created)
	out, _ = sjson.Set(out, "model", modelName)
	out, _ = sjson.Set(out, "choices.0.index", choiceIndex)
	out, _ = sjson.Set(out, "choices.0.delta", delta)
	if finishReason != "" {
		out, _ = sjson.Set(out, "choices.0.finish_reason", finishReason)
	}
	if usage.Exists() && usage.Type != gjson.Null {
		if mapped := mapResponsesUsageToOpenAI(usage); len(mapped) > 0 {
			out, _ = sjson.Set(out, "usage", mapped)
		}
	}
	return out
}

func mapResponsesUsageToOpenAI(usage gjson.Result) map[string]any {
	m := map[string]any{}
	if v := usage.Get("input_tokens"); v.Exists() {
		m["prompt_tokens"] = v.Int()
	}
	if v := usage.Get("output_tokens"); v.Exists() {
		m["completion_tokens"] = v.Int()
	}
	if v := usage.Get("total_tokens"); v.Exists() {
		m["total_tokens"] = v.Int()
	}
	if v := usage.Get("input_tokens_details.cached_tokens"); v.Exists() {
		m["prompt_tokens_details"] = map[string]any{"cached_tokens": v.Int()}
	}
	if v := usage.Get("output_tokens_details.reasoning_tokens"); v.Exists() {
		m["completion_tokens_details"] = map[string]any{"reasoning_tokens": v.Int()}
	}
	return m
}
