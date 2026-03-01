package proxy

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertClaudeRequestToResponses converts Anthropic /v1/messages request to OpenAI /v1/responses format.
// The implementation is aligned with CLIProxyAPI codex->claude request translator behavior.
func ConvertClaudeRequestToResponses(inputRawJSON []byte, stream bool) []byte {
	root := gjson.ParseBytes(inputRawJSON)
	modelName := root.Get("model").String()

	out := `{"model":"","instructions":"","input":[]}`
	out, _ = sjson.Set(out, "model", modelName)

	// System message -> developer input message.
	system := root.Get("system")
	if system.IsArray() {
		message := `{"type":"message","role":"developer","content":[]}`
		contentIndex := 0
		system.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() != "text" {
				return true
			}
			text := item.Get("text").String()
			if strings.HasPrefix(text, "x-anthropic-billing-header: ") {
				return true
			}
			message, _ = sjson.Set(message, fmt.Sprintf("content.%d.type", contentIndex), "input_text")
			message, _ = sjson.Set(message, fmt.Sprintf("content.%d.text", contentIndex), text)
			contentIndex++
			return true
		})
		if contentIndex > 0 {
			out, _ = sjson.SetRaw(out, "input.-1", message)
		}
	} else if system.Exists() && system.Type == gjson.String && strings.TrimSpace(system.String()) != "" {
		message := `{"type":"message","role":"developer","content":[{"type":"input_text","text":""}]}`
		message, _ = sjson.Set(message, "content.0.text", system.String())
		out, _ = sjson.SetRaw(out, "input.-1", message)
	}

	// Messages conversion.
	messages := root.Get("messages")
	if messages.IsArray() {
		messages.ForEach(func(_, message gjson.Result) bool {
			role := message.Get("role").String()
			content := message.Get("content")

			newMessage := func() string {
				msg := `{"type":"message","role":"","content":[]}`
				msg, _ = sjson.Set(msg, "role", role)
				return msg
			}

			msg := newMessage()
			contentIndex := 0
			hasContent := false

			flushMessage := func() {
				if hasContent {
					out, _ = sjson.SetRaw(out, "input.-1", msg)
					msg = newMessage()
					contentIndex = 0
					hasContent = false
				}
			}

			appendText := func(text string) {
				partType := "input_text"
				if role == "assistant" {
					partType = "output_text"
				}
				msg, _ = sjson.Set(msg, fmt.Sprintf("content.%d.type", contentIndex), partType)
				msg, _ = sjson.Set(msg, fmt.Sprintf("content.%d.text", contentIndex), text)
				contentIndex++
				hasContent = true
			}

			appendImage := func(dataURL string) {
				msg, _ = sjson.Set(msg, fmt.Sprintf("content.%d.type", contentIndex), "input_image")
				msg, _ = sjson.Set(msg, fmt.Sprintf("content.%d.image_url", contentIndex), dataURL)
				contentIndex++
				hasContent = true
			}

			if content.IsArray() {
				content.ForEach(func(_, part gjson.Result) bool {
					switch part.Get("type").String() {
					case "text":
						appendText(part.Get("text").String())

					case "image":
						source := part.Get("source")
						if source.Exists() {
							data := source.Get("data").String()
							if data == "" {
								data = source.Get("base64").String()
							}
							if data != "" {
								mediaType := source.Get("media_type").String()
								if mediaType == "" {
									mediaType = source.Get("mime_type").String()
								}
								if mediaType == "" {
									mediaType = "application/octet-stream"
								}
								appendImage(fmt.Sprintf("data:%s;base64,%s", mediaType, data))
							}
						}

					case "tool_use":
						flushMessage()
						functionCall := `{"type":"function_call"}`
						functionCall, _ = sjson.Set(functionCall, "call_id", part.Get("id").String())
						name := part.Get("name").String()
						toolMap := buildReverseMapFromClaudeOriginalToShort(inputRawJSON)
						if short, ok := toolMap[name]; ok {
							name = short
						} else {
							name = shortenNameIfNeeded(name)
						}
						functionCall, _ = sjson.Set(functionCall, "name", name)
						functionCall, _ = sjson.Set(functionCall, "arguments", part.Get("input").Raw)
						out, _ = sjson.SetRaw(out, "input.-1", functionCall)

					case "tool_result":
						flushMessage()
						functionCallOutput := `{"type":"function_call_output"}`
						functionCallOutput, _ = sjson.Set(functionCallOutput, "call_id", part.Get("tool_use_id").String())
						functionCallOutput, _ = sjson.Set(functionCallOutput, "output", part.Get("content").String())
						out, _ = sjson.SetRaw(out, "input.-1", functionCallOutput)
					}
					return true
				})
				flushMessage()
			} else if content.Type == gjson.String {
				appendText(content.String())
				flushMessage()
			}

			return true
		})
	}

	// Tool declarations.
	tools := root.Get("tools")
	if tools.IsArray() {
		out, _ = sjson.SetRaw(out, "tools", `[]`)
		out, _ = sjson.Set(out, "tool_choice", "auto")

		var names []string
		tools.ForEach(func(_, tool gjson.Result) bool {
			if name := tool.Get("name").String(); name != "" {
				names = append(names, name)
			}
			return true
		})
		shortMap := buildShortNameMap(names)

		tools.ForEach(func(_, tool gjson.Result) bool {
			if tool.Get("type").String() == "web_search_20250305" {
				out, _ = sjson.SetRaw(out, "tools.-1", `{"type":"web_search"}`)
				return true
			}

			converted := tool.Raw
			converted, _ = sjson.Set(converted, "type", "function")
			if v := tool.Get("name"); v.Exists() {
				name := v.String()
				if short, ok := shortMap[name]; ok {
					name = short
				} else {
					name = shortenNameIfNeeded(name)
				}
				converted, _ = sjson.Set(converted, "name", name)
			}
			converted, _ = sjson.SetRaw(converted, "parameters", normalizeToolParameters(tool.Get("input_schema").Raw))
			converted, _ = sjson.Delete(converted, "input_schema")
			converted, _ = sjson.Delete(converted, "parameters.$schema")
			converted, _ = sjson.Set(converted, "strict", false)
			out, _ = sjson.SetRaw(out, "tools.-1", converted)
			return true
		})
	}

	// Additional Codex-compatible parameters.
	out, _ = sjson.Set(out, "parallel_tool_calls", true)

	reasoningEffort := thinkingLevelMedium
	if thinkingConfig := root.Get("thinking"); thinkingConfig.Exists() && thinkingConfig.IsObject() {
		switch thinkingConfig.Get("type").String() {
		case "enabled":
			if budgetTokens := thinkingConfig.Get("budget_tokens"); budgetTokens.Exists() {
				if effort, ok := convertBudgetToLevel(int(budgetTokens.Int())); ok && effort != "" {
					reasoningEffort = effort
				}
			}
		case "adaptive":
			reasoningEffort = thinkingLevelXHigh
		case "disabled":
			if effort, ok := convertBudgetToLevel(0); ok && effort != "" {
				reasoningEffort = effort
			}
		}
	}
	out, _ = sjson.Set(out, "reasoning.effort", reasoningEffort)
	out, _ = sjson.Set(out, "reasoning.summary", "auto")
	out, _ = sjson.Set(out, "stream", stream)
	out, _ = sjson.Set(out, "store", false)
	out, _ = sjson.Set(out, "include", []string{"reasoning.encrypted_content"})

	return []byte(out)
}

func shortenNameIfNeeded(name string) string {
	const limit = 64
	if len(name) <= limit {
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		idx := strings.LastIndex(name, "__")
		if idx > 0 {
			cand := "mcp__" + name[idx+2:]
			if len(cand) > limit {
				return cand[:limit]
			}
			return cand
		}
	}
	return name[:limit]
}

func buildShortNameMap(names []string) map[string]string {
	const limit = 64
	used := map[string]struct{}{}
	result := map[string]string{}

	baseCandidate := func(n string) string {
		if len(n) <= limit {
			return n
		}
		if strings.HasPrefix(n, "mcp__") {
			idx := strings.LastIndex(n, "__")
			if idx > 0 {
				cand := "mcp__" + n[idx+2:]
				if len(cand) > limit {
					cand = cand[:limit]
				}
				return cand
			}
		}
		return n[:limit]
	}

	makeUnique := func(cand string) string {
		if _, ok := used[cand]; !ok {
			return cand
		}
		base := cand
		for i := 1; ; i++ {
			suffix := "_" + strconv.Itoa(i)
			allowed := limit - len(suffix)
			if allowed < 0 {
				allowed = 0
			}
			tmp := base
			if len(tmp) > allowed {
				tmp = tmp[:allowed]
			}
			tmp += suffix
			if _, ok := used[tmp]; !ok {
				return tmp
			}
		}
	}

	for _, n := range names {
		cand := baseCandidate(n)
		uniq := makeUnique(cand)
		used[uniq] = struct{}{}
		result[n] = uniq
	}

	return result
}

func buildReverseMapFromClaudeOriginalToShort(original []byte) map[string]string {
	tools := gjson.GetBytes(original, "tools")
	result := map[string]string{}
	if !tools.IsArray() {
		return result
	}

	var names []string
	tools.ForEach(func(_, tool gjson.Result) bool {
		if n := tool.Get("name").String(); n != "" {
			names = append(names, n)
		}
		return true
	})

	if len(names) > 0 {
		result = buildShortNameMap(names)
	}
	return result
}

func normalizeToolParameters(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || !gjson.Valid(raw) {
		return `{"type":"object","properties":{}}`
	}

	schema := raw
	parsed := gjson.Parse(raw)
	schemaType := parsed.Get("type").String()
	if schemaType == "" {
		schema, _ = sjson.Set(schema, "type", "object")
		schemaType = "object"
	}
	if schemaType == "object" && !parsed.Get("properties").Exists() {
		schema, _ = sjson.SetRaw(schema, "properties", `{}`)
	}
	return schema
}
