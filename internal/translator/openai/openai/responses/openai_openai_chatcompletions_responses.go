package responses

import (
	"bytes"
	"context"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func ConvertOpenAIChatCompletionsRequestToOpenAIResponses(modelName string, inputRawJSON []byte, stream bool) []byte {
	rawJSON := inputRawJSON
	root := gjson.ParseBytes(rawJSON)
	out := `{"model":"","input":[],"stream":false}`

	out, _ = sjson.Set(out, "model", modelName)
	out, _ = sjson.Set(out, "stream", stream)

	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_output_tokens", maxTokens.Int())
	}
	if maxTokens := root.Get("max_completion_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_output_tokens", maxTokens.Int())
	}

	for _, key := range []string{"temperature", "top_p", "presence_penalty", "frequency_penalty", "seed", "user"} {
		if val := root.Get(key); val.Exists() {
			out, _ = sjson.SetRaw(out, key, val.Raw)
		}
	}
	if stop := root.Get("stop"); stop.Exists() {
		out, _ = sjson.SetRaw(out, "stop", stop.Raw)
	}

	if tools := root.Get("tools"); tools.Exists() {
		convertedTools := convertChatCompletionsToolsToResponses(tools)
		if len(convertedTools) > 0 {
			out, _ = sjson.Set(out, "tools", convertedTools)
		}
	}
	if toolChoice := root.Get("tool_choice"); toolChoice.Exists() {
		if convertedChoice, ok := convertChatCompletionsToolChoiceToResponses(toolChoice); ok {
			out, _ = sjson.Set(out, "tool_choice", convertedChoice)
		}
	}
	if parallelToolCalls := root.Get("parallel_tool_calls"); parallelToolCalls.Exists() {
		out, _ = sjson.Set(out, "parallel_tool_calls", parallelToolCalls.Bool())
	}

	var instructions []string
	if messages := root.Get("messages"); messages.Exists() && messages.IsArray() {
		messages.ForEach(func(_, message gjson.Result) bool {
			role := strings.TrimSpace(message.Get("role").String())
			content := message.Get("content")
			if role == "system" {
				if text := extractContentText(content); text != "" {
					instructions = append(instructions, text)
				}
				return true
			}
			if role == "developer" {
				role = "user"
			}
			if role == "tool" {
				output := extractContentText(content)
				toolItem := `{"type":"function_call_output","output":""}`
				if callID := message.Get("tool_call_id"); callID.Exists() {
					toolItem, _ = sjson.Set(toolItem, "call_id", callID.String())
				}
				toolItem, _ = sjson.Set(toolItem, "output", output)
				out, _ = sjson.SetRaw(out, "input.-1", toolItem)
				return true
			}

			msg := `{"type":"message","role":"","content":[]}`
			msg, _ = sjson.Set(msg, "role", role)
			msg = appendResponsesContent(msg, role, content)
			out, _ = sjson.SetRaw(out, "input.-1", msg)

			if toolCalls := message.Get("tool_calls"); toolCalls.Exists() && toolCalls.IsArray() {
				toolCalls.ForEach(func(_, call gjson.Result) bool {
					callItem := `{"type":"function_call","name":"","arguments":""}`
					if callID := call.Get("id"); callID.Exists() {
						callItem, _ = sjson.Set(callItem, "call_id", callID.String())
					}
					if name := call.Get("function.name"); name.Exists() {
						callItem, _ = sjson.Set(callItem, "name", name.String())
					}
					if args := call.Get("function.arguments"); args.Exists() {
						callItem, _ = sjson.Set(callItem, "arguments", args.String())
					}
					out, _ = sjson.SetRaw(out, "input.-1", callItem)
					return true
				})
			}

			if legacy := message.Get("function_call"); legacy.Exists() {
				callItem := `{"type":"function_call","name":"","arguments":""}`
				if name := legacy.Get("name"); name.Exists() {
					callItem, _ = sjson.Set(callItem, "name", name.String())
				}
				if args := legacy.Get("arguments"); args.Exists() {
					callItem, _ = sjson.Set(callItem, "arguments", args.String())
				}
				out, _ = sjson.SetRaw(out, "input.-1", callItem)
			}

			return true
		})
	} else if prompt := root.Get("prompt"); prompt.Exists() {
		msg := `{"type":"message","role":"user","content":[]}`
		msg = appendResponsesContent(msg, "user", prompt)
		out, _ = sjson.SetRaw(out, "input.-1", msg)
	}

	if len(instructions) > 0 {
		out, _ = sjson.Set(out, "instructions", strings.Join(instructions, "\n"))
	}

	return []byte(out)
}

func convertChatCompletionsToolsToResponses(tools gjson.Result) []any {
	if !tools.Exists() || !tools.IsArray() {
		return nil
	}
	converted := make([]any, 0, len(tools.Array()))
	tools.ForEach(func(_, tool gjson.Result) bool {
		convertedTool, ok := convertChatCompletionsToolToResponses(tool)
		if ok {
			converted = append(converted, convertedTool)
		}
		return true
	})
	return converted
}

func convertChatCompletionsToolToResponses(tool gjson.Result) (any, bool) {
	if !tool.Exists() || !tool.IsObject() {
		return nil, false
	}
	toolType := strings.TrimSpace(tool.Get("type").String())
	if toolType != "" && toolType != "function" {
		return tool.Value(), true
	}
	function := tool.Get("function")
	if function.Exists() && function.IsObject() {
		mapped := map[string]any{
			"type": "function",
		}
		name := strings.TrimSpace(function.Get("name").String())
		if name == "" {
			name = strings.TrimSpace(tool.Get("name").String())
		}
		if name != "" {
			mapped["name"] = name
		}
		if description := function.Get("description"); description.Exists() {
			mapped["description"] = description.Value()
		} else if description := tool.Get("description"); description.Exists() {
			mapped["description"] = description.Value()
		}
		if parameters := function.Get("parameters"); parameters.Exists() {
			mapped["parameters"] = parameters.Value()
		} else if parameters := function.Get("parametersJsonSchema"); parameters.Exists() {
			mapped["parameters"] = parameters.Value()
		} else if parameters := tool.Get("parameters"); parameters.Exists() {
			mapped["parameters"] = parameters.Value()
		}
		if strict := function.Get("strict"); strict.Exists() {
			mapped["strict"] = strict.Value()
		}
		return mapped, true
	}

	// Already in responses function-tool shape.
	if name := strings.TrimSpace(tool.Get("name").String()); name != "" {
		return tool.Value(), true
	}
	return tool.Value(), true
}

func convertChatCompletionsToolChoiceToResponses(toolChoice gjson.Result) (any, bool) {
	if !toolChoice.Exists() {
		return nil, false
	}
	switch toolChoice.Type {
	case gjson.String:
		return toolChoice.String(), true
	case gjson.JSON:
		if !toolChoice.IsObject() {
			return toolChoice.Value(), true
		}
		choiceType := strings.TrimSpace(toolChoice.Get("type").String())
		if choiceType != "function" {
			return toolChoice.Value(), true
		}
		name := strings.TrimSpace(toolChoice.Get("name").String())
		if name == "" {
			name = strings.TrimSpace(toolChoice.Get("function.name").String())
		}
		choice := map[string]any{
			"type": "function",
		}
		if name != "" {
			choice["name"] = name
		}
		return choice, true
	default:
		return toolChoice.Value(), true
	}
}

func appendResponsesContent(message string, role string, content gjson.Result) string {
	if !content.Exists() {
		return message
	}
	partType := responsesTextTypeForRole(role)

	if content.Type == gjson.String {
		part := `{"type":"","text":""}`
		part, _ = sjson.Set(part, "type", partType)
		part, _ = sjson.Set(part, "text", content.String())
		message, _ = sjson.SetRaw(message, "content.-1", part)
		return message
	}
	if content.IsArray() {
		content.ForEach(func(_, item gjson.Result) bool {
			kind := item.Get("type").String()
			switch kind {
			case "text":
				part := `{"type":"","text":""}`
				part, _ = sjson.Set(part, "type", partType)
				part, _ = sjson.Set(part, "text", item.Get("text").String())
				message, _ = sjson.SetRaw(message, "content.-1", part)
			case "image_url":
				imageURL := item.Get("image_url.url").String()
				if imageURL != "" {
					part := `{"type":"input_image","image_url":""}`
					part, _ = sjson.Set(part, "image_url", imageURL)
					message, _ = sjson.SetRaw(message, "content.-1", part)
				}
			case "input_text", "output_text":
				part := `{"type":"","text":""}`
				part, _ = sjson.Set(part, "type", kind)
				part, _ = sjson.Set(part, "text", item.Get("text").String())
				message, _ = sjson.SetRaw(message, "content.-1", part)
			default:
				if text := item.Get("text"); text.Exists() {
					part := `{"type":"","text":""}`
					part, _ = sjson.Set(part, "type", partType)
					part, _ = sjson.Set(part, "text", text.String())
					message, _ = sjson.SetRaw(message, "content.-1", part)
				}
			}
			return true
		})
		return message
	}
	if content.Type == gjson.JSON {
		part := `{"type":"","text":""}`
		part, _ = sjson.Set(part, "type", partType)
		part, _ = sjson.Set(part, "text", content.Raw)
		message, _ = sjson.SetRaw(message, "content.-1", part)
	}
	return message
}

func responsesTextTypeForRole(role string) string {
	if role == "assistant" {
		return "output_text"
	}
	return "input_text"
}

func extractContentText(content gjson.Result) string {
	if !content.Exists() {
		return ""
	}
	if content.Type == gjson.String {
		return content.String()
	}
	if content.IsArray() {
		var buf strings.Builder
		content.ForEach(func(_, item gjson.Result) bool {
			if text := item.Get("text"); text.Exists() {
				buf.WriteString(text.String())
			}
			return true
		})
		return buf.String()
	}
	if content.Type == gjson.JSON {
		return content.String()
	}
	return ""
}

func ConvertOpenAIResponsesResponseToOpenAIChatCompletions(_ context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	line := bytes.TrimSpace(rawJSON)
	if bytes.HasPrefix(line, []byte("data:")) {
		line = bytes.TrimSpace(line[5:])
	}
	if len(line) == 0 {
		return nil
	}
	if bytes.Equal(line, []byte("[DONE]")) {
		return [][]byte{[]byte("data: [DONE]")}
	}

	root := gjson.ParseBytes(line)
	if !root.Exists() {
		return [][]byte{bytes.Clone(rawJSON)}
	}
	eventType := root.Get("type").String()
	switch eventType {
	case "response.output_text.delta":
		delta := root.Get("delta").String()
		if delta == "" {
			return nil
		}
		chunk := `{"id":"","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":""}}]}`
		if id := root.Get("response_id"); id.Exists() {
			chunk, _ = sjson.Set(chunk, "id", id.String())
		}
		chunk, _ = sjson.Set(chunk, "choices.0.delta.content", delta)
		return [][]byte{[]byte("data: " + chunk)}
	case "response.completed":
		return [][]byte{[]byte("data: [DONE]")}
	default:
		return nil
	}
}

func ConvertOpenAIResponsesResponseToOpenAIChatCompletionsNonStream(_ context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, _ *any) []byte {
	root := gjson.ParseBytes(rawJSON)
	if response := root.Get("response"); response.Exists() && response.IsObject() {
		root = response
	}

	out := `{"id":"","object":"chat.completion","created":0,"model":"","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`

	if id := root.Get("id"); id.Exists() {
		out, _ = sjson.Set(out, "id", id.String())
	}
	if created := root.Get("created_at"); created.Exists() {
		out, _ = sjson.Set(out, "created", created.Int())
	}
	if model := root.Get("model"); model.Exists() {
		out, _ = sjson.Set(out, "model", model.String())
	} else if modelName != "" {
		out, _ = sjson.Set(out, "model", modelName)
	}

	var content strings.Builder
	var toolCalls []interface{}
	if output := root.Get("output"); output.Exists() && output.IsArray() {
		output.ForEach(func(_, item gjson.Result) bool {
			itemType := item.Get("type").String()
			switch itemType {
			case "message":
				if parts := item.Get("content"); parts.Exists() && parts.IsArray() {
					parts.ForEach(func(_, part gjson.Result) bool {
						if part.Get("type").String() == "output_text" {
							content.WriteString(part.Get("text").String())
						}
						return true
					})
				}
			case "function_call":
				call := map[string]any{
					"id":   item.Get("call_id").String(),
					"type": "function",
					"function": map[string]any{
						"name":      item.Get("name").String(),
						"arguments": item.Get("arguments").String(),
					},
				}
				toolCalls = append(toolCalls, call)
			}
			return true
		})
	}

	out, _ = sjson.Set(out, "choices.0.message.content", content.String())
	if len(toolCalls) > 0 {
		out, _ = sjson.Set(out, "choices.0.message.tool_calls", toolCalls)
	}

	if usage := root.Get("usage"); usage.Exists() {
		if inputTokens := usage.Get("input_tokens"); inputTokens.Exists() {
			out, _ = sjson.Set(out, "usage.prompt_tokens", inputTokens.Int())
		}
		if outputTokens := usage.Get("output_tokens"); outputTokens.Exists() {
			out, _ = sjson.Set(out, "usage.completion_tokens", outputTokens.Int())
		}
		if totalTokens := usage.Get("total_tokens"); totalTokens.Exists() {
			out, _ = sjson.Set(out, "usage.total_tokens", totalTokens.Int())
		}
	}

	return []byte(out)
}
