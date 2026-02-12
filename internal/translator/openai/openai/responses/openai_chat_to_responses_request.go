package responses

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertOpenAIChatCompletionsRequestToOpenAIResponses converts OpenAI chat completions
// requests to OpenAI responses format.
func ConvertOpenAIChatCompletionsRequestToOpenAIResponses(modelName string, inputRawJSON []byte, stream bool) []byte {
	root := gjson.ParseBytes(inputRawJSON)
	out := `{"model":"","input":[],"stream":false}`
	out, _ = sjson.Set(out, "model", modelName)
	out, _ = sjson.Set(out, "stream", stream)

	if v := root.Get("max_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "max_output_tokens", v.Int())
	}
	if v := root.Get("max_completion_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "max_output_tokens", v.Int())
	}

	for _, key := range []string{
		"temperature", "top_p", "presence_penalty", "frequency_penalty",
		"logit_bias", "seed", "user", "metadata", "parallel_tool_calls",
		"tool_choice", "response_format", "stream_options", "stop", "n",
		"top_logprobs", "logprobs",
	} {
		if v := root.Get(key); v.Exists() {
			out, _ = sjson.SetRaw(out, key, v.Raw)
		}
	}

	if v := root.Get("reasoning"); v.Exists() {
		out, _ = sjson.SetRaw(out, "reasoning", v.Raw)
	} else if v := root.Get("reasoning_effort"); v.Exists() {
		out, _ = sjson.Set(out, "reasoning.effort", v.String())
	}

	if tools := root.Get("tools"); tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			if tool.Get("type").String() != "function" {
				return true
			}
			item := `{"type":"function","name":"","description":"","parameters":{}}`
			item, _ = sjson.Set(item, "name", tool.Get("function.name").String())
			if v := tool.Get("function.description"); v.Exists() {
				item, _ = sjson.Set(item, "description", v.String())
			}
			if v := tool.Get("function.parameters"); v.Exists() {
				item, _ = sjson.SetRaw(item, "parameters", v.Raw)
			}
			out, _ = sjson.SetRaw(out, "tools.-1", item)
			return true
		})
	}

	if msgs := root.Get("messages"); msgs.IsArray() {
		msgs.ForEach(func(_, m gjson.Result) bool {
			role := m.Get("role").String()
			if role == "system" {
				role = "developer"
			}
			if role == "tool" {
				item := `{"type":"function_call_output","call_id":"","output":""}`
				item, _ = sjson.Set(item, "call_id", m.Get("tool_call_id").String())
				item, _ = sjson.Set(item, "output", messageTextFromContent(m.Get("content")))
				out, _ = sjson.SetRaw(out, "input.-1", item)
				return true
			}

			msg := `{"type":"message","role":"","content":[]}`
			msg, _ = sjson.Set(msg, "role", role)
			if content := m.Get("content"); content.Exists() {
				msg = appendContentParts(msg, role, content)
			}
			out, _ = sjson.SetRaw(out, "input.-1", msg)

			if role == "assistant" {
				if tcs := m.Get("tool_calls"); tcs.IsArray() {
					callIndex := 0
					tcs.ForEach(func(_, tc gjson.Result) bool {
						call := `{"type":"function_call","call_id":"","name":"","arguments":""}`
						callID := tc.Get("id").String()
						if strings.TrimSpace(callID) == "" {
							callID = fmt.Sprintf("call_%d", callIndex)
						}
						callIndex++
						call, _ = sjson.Set(call, "call_id", callID)
						call, _ = sjson.Set(call, "name", tc.Get("function.name").String())
						call, _ = sjson.Set(call, "arguments", tc.Get("function.arguments").String())
						out, _ = sjson.SetRaw(out, "input.-1", call)
						return true
					})
				} else if fc := m.Get("function_call"); fc.Exists() {
					call := `{"type":"function_call","call_id":"","name":"","arguments":""}`
					call, _ = sjson.Set(call, "call_id", "call_0")
					call, _ = sjson.Set(call, "name", fc.Get("name").String())
					call, _ = sjson.Set(call, "arguments", fc.Get("arguments").String())
					out, _ = sjson.SetRaw(out, "input.-1", call)
				}
			}
			return true
		})
	}

	return []byte(out)
}

func messageTextFromContent(content gjson.Result) string {
	if content.Type == gjson.String {
		return content.String()
	}
	var b strings.Builder
	if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "text" {
				b.WriteString(part.Get("text").String())
			}
			return true
		})
	}
	return b.String()
}

func appendContentParts(msg string, role string, content gjson.Result) string {
	partType := "input_text"
	if role == "assistant" {
		partType = "output_text"
	}
	if content.Type == gjson.String {
		part := `{"type":"","text":""}`
		part, _ = sjson.Set(part, "type", partType)
		part, _ = sjson.Set(part, "text", content.String())
		msg, _ = sjson.SetRaw(msg, "content.-1", part)
		return msg
	}
	if content.IsArray() {
		content.ForEach(func(_, part gjson.Result) bool {
			switch part.Get("type").String() {
			case "text":
				item := `{"type":"","text":""}`
				item, _ = sjson.Set(item, "type", partType)
				item, _ = sjson.Set(item, "text", part.Get("text").String())
				msg, _ = sjson.SetRaw(msg, "content.-1", item)
			case "image_url":
				if role == "developer" {
					return true
				}
				item := `{"type":"input_image","image_url":""}`
				item, _ = sjson.Set(item, "image_url", part.Get("image_url.url").String())
				msg, _ = sjson.SetRaw(msg, "content.-1", item)
			}
			return true
		})
	}
	return msg
}
