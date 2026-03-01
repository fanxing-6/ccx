package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ResponsesStreamConverter converts OpenAI Responses SSE events to Anthropic SSE events.
// Implementation is aligned with CLIProxyAPI codex->claude stream translator behavior.
type ResponsesStreamConverter struct {
	hasToolCall               bool
	blockIndex                int
	hasReceivedArgumentsDelta bool
	done                      bool

	messageStarted bool
	messageID      string
	model          string
}

func NewResponsesStreamConverter() *ResponsesStreamConverter {
	return &ResponsesStreamConverter{}
}

func (c *ResponsesStreamConverter) IsDone() bool {
	return c.done
}

func (c *ResponsesStreamConverter) ProcessEvent(eventType string, dataJSON string) []string {
	if c.done {
		return nil
	}

	root := gjson.Parse(dataJSON)
	var out []string

	switch eventType {
	case "response.created":
		c.messageID = root.Get("response.id").String()
		c.model = root.Get("response.model").String()

		payload := `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","model":"","stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0},"content":[],"stop_reason":null}}`
		payload, _ = sjson.Set(payload, "message.id", c.messageID)
		payload, _ = sjson.Set(payload, "message.model", c.model)
		out = append(out, "event: message_start\ndata: "+payload+"\n\n")
		c.messageStarted = true

	case "response.reasoning_summary_part.added":
		payload := `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		out = append(out, "event: content_block_start\ndata: "+payload+"\n\n")

	case "response.reasoning_summary_text.delta":
		payload := `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":""}}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		payload, _ = sjson.Set(payload, "delta.thinking", root.Get("delta").String())
		out = append(out, "event: content_block_delta\ndata: "+payload+"\n\n")

	case "response.reasoning_summary_part.done":
		payload := `{"type":"content_block_stop","index":0}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		out = append(out, "event: content_block_stop\ndata: "+payload+"\n\n")
		c.blockIndex++

	case "response.content_part.added":
		payload := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		out = append(out, "event: content_block_start\ndata: "+payload+"\n\n")

	case "response.output_text.delta":
		payload := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		payload, _ = sjson.Set(payload, "delta.text", root.Get("delta").String())
		out = append(out, "event: content_block_delta\ndata: "+payload+"\n\n")

	case "response.content_part.done":
		payload := `{"type":"content_block_stop","index":0}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		out = append(out, "event: content_block_stop\ndata: "+payload+"\n\n")
		c.blockIndex++

	case "response.output_item.added":
		item := root.Get("item")
		if item.Get("type").String() == "function_call" {
			c.hasToolCall = true
			c.hasReceivedArgumentsDelta = false

			start := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"","name":"","input":{}}}`
			start, _ = sjson.Set(start, "index", c.blockIndex)
			start, _ = sjson.Set(start, "content_block.id", item.Get("call_id").String())
			start, _ = sjson.Set(start, "content_block.name", item.Get("name").String())
			out = append(out, "event: content_block_start\ndata: "+start+"\n\n")

			delta := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`
			delta, _ = sjson.Set(delta, "index", c.blockIndex)
			out = append(out, "event: content_block_delta\ndata: "+delta+"\n\n")
		}

	case "response.function_call_arguments.delta":
		c.hasReceivedArgumentsDelta = true
		payload := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`
		payload, _ = sjson.Set(payload, "index", c.blockIndex)
		payload, _ = sjson.Set(payload, "delta.partial_json", root.Get("delta").String())
		out = append(out, "event: content_block_delta\ndata: "+payload+"\n\n")

	case "response.function_call_arguments.done":
		// Some models emit arguments only in *.done without delta.
		if !c.hasReceivedArgumentsDelta {
			if args := root.Get("arguments").String(); args != "" {
				payload := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`
				payload, _ = sjson.Set(payload, "index", c.blockIndex)
				payload, _ = sjson.Set(payload, "delta.partial_json", args)
				out = append(out, "event: content_block_delta\ndata: "+payload+"\n\n")
			}
		}

	case "response.output_item.done":
		item := root.Get("item")
		if item.Get("type").String() == "function_call" {
			payload := `{"type":"content_block_stop","index":0}`
			payload, _ = sjson.Set(payload, "index", c.blockIndex)
			out = append(out, "event: content_block_stop\ndata: "+payload+"\n\n")
			c.blockIndex++
		}

	case "response.completed":
		resp := root.Get("response")
		inputTokens, outputTokens, cachedTokens := extractResponsesUsage(resp.Get("usage"))

		payload := `{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`
		stopReason := "end_turn"
		if c.hasToolCall {
			stopReason = "tool_use"
		} else {
			r := resp.Get("stop_reason").String()
			if r == "max_tokens" || r == "stop" {
				stopReason = r
			}
		}

		payload, _ = sjson.Set(payload, "delta.stop_reason", stopReason)
		payload, _ = sjson.Set(payload, "usage.input_tokens", inputTokens)
		payload, _ = sjson.Set(payload, "usage.output_tokens", outputTokens)
		if cachedTokens > 0 {
			payload, _ = sjson.Set(payload, "usage.cache_read_input_tokens", cachedTokens)
		}

		out = append(out, "event: message_delta\ndata: "+payload+"\n\n")
		out = append(out, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		c.done = true

	case "response.failed", "error":
		// Keep an explicit terminal event on failures.
		if !c.messageStarted {
			start := `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","model":"","stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0},"content":[],"stop_reason":null}}`
			start, _ = sjson.Set(start, "message.id", c.messageID)
			start, _ = sjson.Set(start, "message.model", c.model)
			out = append(out, "event: message_start\ndata: "+start+"\n\n")
			c.messageStarted = true
		}
		msg := root.Get("response.error.message").String()
		if msg == "" {
			msg = root.Get("message").String()
		}
		if msg != "" {
			idx := c.blockIndex
			start := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
			start, _ = sjson.Set(start, "index", idx)
			out = append(out, "event: content_block_start\ndata: "+start+"\n\n")

			delta := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`
			delta, _ = sjson.Set(delta, "index", idx)
			delta, _ = sjson.Set(delta, "delta.text", "[ERROR] "+msg)
			out = append(out, "event: content_block_delta\ndata: "+delta+"\n\n")

			stop := `{"type":"content_block_stop","index":0}`
			stop, _ = sjson.Set(stop, "index", idx)
			out = append(out, "event: content_block_stop\ndata: "+stop+"\n\n")
			c.blockIndex++
		}

		final := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`
		out = append(out, "event: message_delta\ndata: "+final+"\n\n")
		out = append(out, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		c.done = true
	}

	return out
}

// Finish ensures downstream gets terminal events even when upstream closes unexpectedly.
func (c *ResponsesStreamConverter) Finish() []string {
	if c.done {
		return nil
	}

	var out []string
	if !c.messageStarted {
		start := `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","model":"","stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0},"content":[],"stop_reason":null}}`
		start, _ = sjson.Set(start, "message.id", c.messageID)
		start, _ = sjson.Set(start, "message.model", c.model)
		out = append(out, "event: message_start\ndata: "+start+"\n\n")
	}

	stopReason := "end_turn"
	if c.hasToolCall {
		stopReason = "tool_use"
	}
	delta := `{"type":"message_delta","delta":{"stop_reason":"","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`
	delta, _ = sjson.Set(delta, "delta.stop_reason", stopReason)
	out = append(out, "event: message_delta\ndata: "+delta+"\n\n")
	out = append(out, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	c.done = true
	return out
}

// ConvertResponsesToClaudeNonStream converts OpenAI /v1/responses JSON to Anthropic non-stream response.
// Implementation is aligned with CLIProxyAPI codex->claude non-stream behavior.
func ConvertResponsesToClaudeNonStream(rawJSON []byte) string {
	root := gjson.ParseBytes(rawJSON)

	out := `{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`
	out, _ = sjson.Set(out, "id", root.Get("id").String())
	out, _ = sjson.Set(out, "model", root.Get("model").String())

	inputTokens, outputTokens, cachedTokens := extractResponsesUsage(root.Get("usage"))
	out, _ = sjson.Set(out, "usage.input_tokens", inputTokens)
	out, _ = sjson.Set(out, "usage.output_tokens", outputTokens)
	if cachedTokens > 0 {
		out, _ = sjson.Set(out, "usage.cache_read_input_tokens", cachedTokens)
	}

	hasToolCall := false
	if output := root.Get("output"); output.Exists() && output.IsArray() {
		output.ForEach(func(_, item gjson.Result) bool {
			switch item.Get("type").String() {
			case "reasoning":
				thinking := strings.Builder{}
				if summary := item.Get("summary"); summary.Exists() {
					if summary.IsArray() {
						summary.ForEach(func(_, part gjson.Result) bool {
							if txt := part.Get("text"); txt.Exists() {
								thinking.WriteString(txt.String())
							} else {
								thinking.WriteString(part.String())
							}
							return true
						})
					} else {
						thinking.WriteString(summary.String())
					}
				}
				if thinking.Len() == 0 {
					if content := item.Get("content"); content.Exists() {
						if content.IsArray() {
							content.ForEach(func(_, part gjson.Result) bool {
								if txt := part.Get("text"); txt.Exists() {
									thinking.WriteString(txt.String())
								} else {
									thinking.WriteString(part.String())
								}
								return true
							})
						} else {
							thinking.WriteString(content.String())
						}
					}
				}
				if thinking.Len() > 0 {
					block := `{"type":"thinking","thinking":""}`
					block, _ = sjson.Set(block, "thinking", thinking.String())
					out, _ = sjson.SetRaw(out, "content.-1", block)
				}

			case "message":
				if content := item.Get("content"); content.Exists() {
					if content.IsArray() {
						content.ForEach(func(_, part gjson.Result) bool {
							if part.Get("type").String() == "output_text" {
								text := part.Get("text").String()
								if text != "" {
									block := `{"type":"text","text":""}`
									block, _ = sjson.Set(block, "text", text)
									out, _ = sjson.SetRaw(out, "content.-1", block)
								}
							}
							return true
						})
					} else {
						text := content.String()
						if text != "" {
							block := `{"type":"text","text":""}`
							block, _ = sjson.Set(block, "text", text)
							out, _ = sjson.SetRaw(out, "content.-1", block)
						}
					}
				}

			case "function_call":
				hasToolCall = true
				toolBlock := `{"type":"tool_use","id":"","name":"","input":{}}`
				toolBlock, _ = sjson.Set(toolBlock, "id", item.Get("call_id").String())
				toolBlock, _ = sjson.Set(toolBlock, "name", item.Get("name").String())

				inputRaw := "{}"
				if argsStr := item.Get("arguments").String(); argsStr != "" && gjson.Valid(argsStr) {
					argsJSON := gjson.Parse(argsStr)
					if argsJSON.IsObject() {
						inputRaw = argsJSON.Raw
					}
				}
				toolBlock, _ = sjson.SetRaw(toolBlock, "input", inputRaw)
				out, _ = sjson.SetRaw(out, "content.-1", toolBlock)
			}
			return true
		})
	}

	if stopReason := root.Get("stop_reason"); stopReason.Exists() && stopReason.String() != "" {
		out, _ = sjson.Set(out, "stop_reason", stopReason.String())
	} else if hasToolCall {
		out, _ = sjson.Set(out, "stop_reason", "tool_use")
	} else {
		out, _ = sjson.Set(out, "stop_reason", "end_turn")
	}

	if stopSequence := root.Get("stop_sequence"); stopSequence.Exists() && stopSequence.String() != "" {
		out, _ = sjson.SetRaw(out, "stop_sequence", stopSequence.Raw)
	}

	return out
}

func extractResponsesUsage(usage gjson.Result) (int64, int64, int64) {
	if !usage.Exists() || usage.Type == gjson.Null {
		return 0, 0, 0
	}

	inputTokens := usage.Get("input_tokens").Int()
	outputTokens := usage.Get("output_tokens").Int()
	cachedTokens := usage.Get("input_tokens_details.cached_tokens").Int()

	if cachedTokens > 0 {
		if inputTokens >= cachedTokens {
			inputTokens -= cachedTokens
		} else {
			inputTokens = 0
		}
	}

	return inputTokens, outputTokens, cachedTokens
}

// ClaudeTokenCount returns Anthropic-format count_tokens response body.
func ClaudeTokenCount(_ context.Context, count int64) string {
	return fmt.Sprintf(`{"input_tokens":%d}`, count)
}
