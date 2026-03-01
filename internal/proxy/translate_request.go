package proxy

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertClaudeRequestToResponses converts Anthropic /v1/messages request to OpenAI /v1/responses format.
func ConvertClaudeRequestToResponses(inputRawJSON []byte, stream bool) []byte {
	root := gjson.ParseBytes(inputRawJSON)
	out := `{"model":"","input":[],"stream":false}`

	out, _ = sjson.Set(out, "model", root.Get("model").String())
	out, _ = sjson.Set(out, "stream", stream)

	if maxTokens := root.Get("max_tokens"); maxTokens.Exists() {
		out, _ = sjson.Set(out, "max_output_tokens", maxTokens.Int())
	}

	if temp := root.Get("temperature"); temp.Exists() {
		out, _ = sjson.Set(out, "temperature", temp.Float())
	}
	if topP := root.Get("top_p"); topP.Exists() {
		out, _ = sjson.Set(out, "top_p", topP.Float())
	}

	if stopSequences := root.Get("stop_sequences"); stopSequences.Exists() && stopSequences.IsArray() {
		var stops []string
		stopSequences.ForEach(func(_, value gjson.Result) bool {
			stops = append(stops, value.String())
			return true
		})
		if len(stops) > 0 {
			out, _ = sjson.Set(out, "stop", stops)
		}
	}

	if system := root.Get("system"); system.Exists() {
		switch {
		case system.Type == gjson.String:
			if system.String() != "" {
				out, _ = sjson.Set(out, "instructions", system.String())
			}
		case system.IsArray():
			var parts []string
			system.ForEach(func(_, item gjson.Result) bool {
				if text := item.Get("text"); text.Exists() {
					parts = append(parts, text.String())
				}
				return true
			})
			if len(parts) > 0 {
				out, _ = sjson.Set(out, "instructions", strings.Join(parts, "\n\n"))
			}
		}
	}

	// 按 CLIProxyAPI 的逻辑：默认 medium，始终设置 reasoning.summary = "auto"
	reasoningEffort := thinkingLevelMedium
	if thinkingConfig := root.Get("thinking"); thinkingConfig.Exists() && thinkingConfig.IsObject() {
		if thinkingType := thinkingConfig.Get("type"); thinkingType.Exists() {
			switch thinkingType.String() {
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
	}
	out, _ = sjson.Set(out, "reasoning.effort", reasoningEffort)
	out, _ = sjson.Set(out, "reasoning.summary", "auto")
	out, _ = sjson.Set(out, "store", false)

	if messages := root.Get("messages"); messages.Exists() && messages.IsArray() {
		messages.ForEach(func(_, message gjson.Result) bool {
			role := message.Get("role").String()
			content := message.Get("content")

			msg := `{"role":"","content":[]}`
			msg, _ = sjson.Set(msg, "role", role)
			hasContent := false

			switch {
			case content.Type == gjson.String:
				text := strings.TrimSpace(content.String())
				if text != "" {
					item := `{"type":"input_text","text":""}`
					item, _ = sjson.Set(item, "text", text)
					msg, _ = sjson.SetRaw(msg, "content.-1", item)
					hasContent = true
				}
			case content.IsArray():
				content.ForEach(func(_, part gjson.Result) bool {
					partType := part.Get("type").String()
					switch partType {
					case "text", "thinking":
						text := part.Get("text").String()
						if partType == "thinking" {
							text = getThinkingText(part)
						}
						if strings.TrimSpace(text) == "" {
							return true
						}
						item := `{"type":"input_text","text":""}`
						item, _ = sjson.Set(item, "text", text)
						msg, _ = sjson.SetRaw(msg, "content.-1", item)
						hasContent = true
					case "image":
						imageURL := ""
						if source := part.Get("source"); source.Exists() {
							switch source.Get("type").String() {
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
						if imageURL == "" {
							return true
						}
						item := `{"type":"input_image","image_url":""}`
						item, _ = sjson.Set(item, "image_url", imageURL)
						msg, _ = sjson.SetRaw(msg, "content.-1", item)
						hasContent = true
					case "tool_result":
						toolText := convertClaudeToolResultContentToString(part.Get("content"))
						if strings.TrimSpace(toolText) == "" {
							return true
						}
						item := `{"type":"input_text","text":""}`
						item, _ = sjson.Set(item, "text", toolText)
						msg, _ = sjson.SetRaw(msg, "content.-1", item)
						hasContent = true
					}
					return true
				})
			}

			if hasContent {
				out, _ = sjson.Set(out, "input.-1", gjson.Parse(msg).Value())
			}
			return true
		})
	}

	if tools := root.Get("tools"); tools.Exists() && tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			openAITool := `{"type":"function","name":"","description":"","parameters":{}}`
			openAITool, _ = sjson.Set(openAITool, "name", tool.Get("name").String())
			openAITool, _ = sjson.Set(openAITool, "description", tool.Get("description").String())
			if inputSchema := tool.Get("input_schema"); inputSchema.Exists() {
				openAITool, _ = sjson.Set(openAITool, "parameters", inputSchema.Value())
			}
			out, _ = sjson.Set(out, "tools.-1", gjson.Parse(openAITool).Value())
			return true
		})
	}

	if toolChoice := root.Get("tool_choice"); toolChoice.Exists() {
		switch toolChoice.Get("type").String() {
		case "auto":
			out, _ = sjson.Set(out, "tool_choice", "auto")
		case "any":
			out, _ = sjson.Set(out, "tool_choice", "required")
		case "tool":
			name := toolChoice.Get("name").String()
			out, _ = sjson.Set(out, "tool_choice", map[string]any{
				"type": "function",
				"name": name,
			})
		}
	}

	return []byte(out)
}

func convertClaudeToolResultContentToString(content gjson.Result) string {
	if !content.Exists() {
		return ""
	}

	if content.Type == gjson.String {
		return content.String()
	}

	if content.IsArray() {
		var parts []string
		content.ForEach(func(_, item gjson.Result) bool {
			switch {
			case item.Type == gjson.String:
				parts = append(parts, item.String())
			case item.IsObject() && item.Get("text").Exists() && item.Get("text").Type == gjson.String:
				parts = append(parts, item.Get("text").String())
			default:
				parts = append(parts, item.Raw)
			}
			return true
		})
		joined := strings.Join(parts, "\n\n")
		if strings.TrimSpace(joined) != "" {
			return joined
		}
		return content.Raw
	}

	if content.IsObject() {
		if text := content.Get("text"); text.Exists() && text.Type == gjson.String {
			return text.String()
		}
		return content.Raw
	}

	return content.Raw
}
