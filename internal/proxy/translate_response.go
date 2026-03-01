package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ResponsesStreamConverter 将 OpenAI Responses API SSE 事件逐行转换为 Anthropic SSE 事件。
// 每次收到上游的一行 SSE 数据时调用 ProcessLine，返回需要写给下游的 Anthropic SSE 事件。
type ResponsesStreamConverter struct {
	messageID string
	model     string

	// content block 追踪
	nextBlockIndex int
	textBlockIdx   int  // 当前文本块的索引，-1 表示没有活跃的文本块
	textBlockOpen  bool // 文本 content_block 是否已 start

	thinkingBlockIdx  int  // 当前 thinking 块的索引
	thinkingBlockOpen bool // thinking content_block 是否已 start

	// tool call 追踪：call_id -> blockIndex
	toolBlockIndexes map[string]int
	toolBlockOpen    map[string]bool // 某个 tool 块是否已 start

	messageStarted bool
	done           bool

	// usage 累积
	inputTokens  int64
	outputTokens int64
}

// NewResponsesStreamConverter 创建新的流式转换器。
func NewResponsesStreamConverter() *ResponsesStreamConverter {
	return &ResponsesStreamConverter{
		textBlockIdx:     -1,
		thinkingBlockIdx: -1,
		toolBlockIndexes: make(map[string]int),
		toolBlockOpen:    make(map[string]bool),
	}
}

// IsDone 返回是否已完成转换（收到 response.completed / response.failed / response.incomplete）。
func (c *ResponsesStreamConverter) IsDone() bool {
	return c.done
}

// ProcessLine 处理一行上游 SSE。输入格式为 "event: xxx" 或 "data: {...}"。
// 调用者应按行读取上游 SSE，每次遇到空行（事件分隔符）时，将累积的 event 和 data 传入。
// 此方法的简化用法：传入 eventType 和 dataJSON，返回 Anthropic SSE 事件字符串列表。
func (c *ResponsesStreamConverter) ProcessEvent(eventType string, dataJSON string) []string {
	if c.done {
		return nil
	}

	root := gjson.Parse(dataJSON)

	switch eventType {
	case "response.created":
		c.messageID = root.Get("response.id").String()
		c.model = root.Get("response.model").String()
		return c.emitMessageStart()

	case "response.in_progress":
		// 忽略，不需要转换
		return nil

	case "response.output_item.added":
		itemType := root.Get("item.type").String()
		switch itemType {
		case "function_call":
			callID := root.Get("item.call_id").String()
			if callID == "" {
				callID = root.Get("item.id").String()
			}
			name := root.Get("item.name").String()
			return c.startToolBlock(callID, name)
		case "message":
			// message item added，内容通过后续 delta 事件到达
			return nil
		}
		return nil

	case "response.output_item.done":
		// output item 完成，不需要额外动作（块通过 .done 事件关闭）
		return nil

	case "response.content_part.added":
		// content part added，内容通过 delta 到达
		return nil

	case "response.content_part.done":
		// content part 完成
		return nil

	case "response.output_text.delta":
		delta := root.Get("delta").String()
		if delta == "" {
			return nil
		}
		return c.emitTextDelta(delta)

	case "response.output_text.done":
		return c.closeTextBlock()

	case "response.function_call_arguments.delta":
		delta := root.Get("delta").String()
		callID := root.Get("call_id").String()
		if callID == "" {
			callID = root.Get("item_id").String()
		}
		if delta == "" {
			return nil
		}
		return c.emitToolInputDelta(callID, delta)

	case "response.function_call_arguments.done":
		callID := root.Get("call_id").String()
		if callID == "" {
			callID = root.Get("item_id").String()
		}
		return c.closeToolBlock(callID)

	case "response.reasoning_summary_text.delta":
		delta := root.Get("delta").String()
		if delta == "" {
			return nil
		}
		return c.emitThinkingDelta(delta)

	case "response.reasoning_summary_text.done":
		return c.closeThinkingBlock()

	case "response.completed":
		resp := root.Get("response")
		c.extractUsage(resp)
		return c.finish("end_turn")

	case "response.failed":
		// 发送错误文本块后结束
		errMsg := root.Get("response.error.message").String()
		if errMsg == "" {
			errMsg = root.Get("response.status_details.error.message").String()
		}
		if errMsg == "" {
			errMsg = "upstream error"
		}
		var results []string
		results = append(results, c.ensureMessageStart()...)
		c.closeAllOpenBlocks(&results)
		// 发送错误文本块
		idx := c.nextBlockIndex
		c.nextBlockIndex++
		start := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
		start, _ = sjson.Set(start, "index", idx)
		results = append(results, "event: content_block_start\ndata: "+start+"\n\n")

		delta := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`
		delta, _ = sjson.Set(delta, "index", idx)
		delta, _ = sjson.Set(delta, "delta.text", "[ERROR] "+errMsg)
		results = append(results, "event: content_block_delta\ndata: "+delta+"\n\n")

		stop := `{"type":"content_block_stop","index":0}`
		stop, _ = sjson.Set(stop, "index", idx)
		results = append(results, "event: content_block_stop\ndata: "+stop+"\n\n")

		results = append(results, c.emitMessageDeltaAndStop("end_turn")...)
		c.done = true
		return results

	case "response.incomplete":
		c.extractUsage(root.Get("response"))
		return c.finish("max_tokens")

	case "error":
		// 顶层 error 事件
		errMsg := root.Get("message").String()
		if errMsg == "" {
			errMsg = "stream error"
		}
		var results []string
		results = append(results, c.ensureMessageStart()...)
		c.closeAllOpenBlocks(&results)
		idx := c.nextBlockIndex
		c.nextBlockIndex++
		start := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
		start, _ = sjson.Set(start, "index", idx)
		results = append(results, "event: content_block_start\ndata: "+start+"\n\n")

		delta := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`
		delta, _ = sjson.Set(delta, "index", idx)
		delta, _ = sjson.Set(delta, "delta.text", "[ERROR] "+errMsg)
		results = append(results, "event: content_block_delta\ndata: "+delta+"\n\n")

		stop := `{"type":"content_block_stop","index":0}`
		stop, _ = sjson.Set(stop, "index", idx)
		results = append(results, "event: content_block_stop\ndata: "+stop+"\n\n")

		results = append(results, c.emitMessageDeltaAndStop("end_turn")...)
		c.done = true
		return results
	}

	// 未知事件类型，忽略
	return nil
}

// Finish 在上游断开时调用，确保所有块已关闭并发送 message_stop。
func (c *ResponsesStreamConverter) Finish() []string {
	if c.done {
		return nil
	}
	return c.finish("end_turn")
}

// --- 内部方法 ---

func (c *ResponsesStreamConverter) ensureMessageStart() []string {
	if c.messageStarted {
		return nil
	}
	return c.emitMessageStart()
}

func (c *ResponsesStreamConverter) emitMessageStart() []string {
	if c.messageStarted {
		return nil
	}
	c.messageStarted = true
	msgStart := `{"type":"message_start","message":{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`
	msgStart, _ = sjson.Set(msgStart, "message.id", c.messageID)
	msgStart, _ = sjson.Set(msgStart, "message.model", c.model)
	return []string{"event: message_start\ndata: " + msgStart + "\n\n"}
}

func (c *ResponsesStreamConverter) emitTextDelta(text string) []string {
	var results []string
	results = append(results, c.ensureMessageStart()...)

	// 先关闭 thinking 块（文本和 thinking 不会同时打开）
	c.closeThinkingBlockInto(&results)

	if !c.textBlockOpen {
		if c.textBlockIdx == -1 {
			c.textBlockIdx = c.nextBlockIndex
			c.nextBlockIndex++
		}
		start := `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`
		start, _ = sjson.Set(start, "index", c.textBlockIdx)
		results = append(results, "event: content_block_start\ndata: "+start+"\n\n")
		c.textBlockOpen = true
	}

	delta := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`
	delta, _ = sjson.Set(delta, "index", c.textBlockIdx)
	delta, _ = sjson.Set(delta, "delta.text", text)
	results = append(results, "event: content_block_delta\ndata: "+delta+"\n\n")
	return results
}

func (c *ResponsesStreamConverter) closeTextBlock() []string {
	if !c.textBlockOpen {
		return nil
	}
	stop := `{"type":"content_block_stop","index":0}`
	stop, _ = sjson.Set(stop, "index", c.textBlockIdx)
	c.textBlockOpen = false
	c.textBlockIdx = -1
	return []string{"event: content_block_stop\ndata: " + stop + "\n\n"}
}

func (c *ResponsesStreamConverter) emitThinkingDelta(text string) []string {
	var results []string
	results = append(results, c.ensureMessageStart()...)

	// 先关闭文本块
	c.closeTextBlockInto(&results)

	if !c.thinkingBlockOpen {
		if c.thinkingBlockIdx == -1 {
			c.thinkingBlockIdx = c.nextBlockIndex
			c.nextBlockIndex++
		}
		start := `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`
		start, _ = sjson.Set(start, "index", c.thinkingBlockIdx)
		results = append(results, "event: content_block_start\ndata: "+start+"\n\n")
		c.thinkingBlockOpen = true
	}

	delta := `{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":""}}`
	delta, _ = sjson.Set(delta, "index", c.thinkingBlockIdx)
	delta, _ = sjson.Set(delta, "delta.thinking", text)
	results = append(results, "event: content_block_delta\ndata: "+delta+"\n\n")
	return results
}

func (c *ResponsesStreamConverter) closeThinkingBlock() []string {
	if !c.thinkingBlockOpen {
		return nil
	}
	stop := `{"type":"content_block_stop","index":0}`
	stop, _ = sjson.Set(stop, "index", c.thinkingBlockIdx)
	c.thinkingBlockOpen = false
	c.thinkingBlockIdx = -1
	return []string{"event: content_block_stop\ndata: " + stop + "\n\n"}
}

func (c *ResponsesStreamConverter) startToolBlock(callID, name string) []string {
	var results []string
	results = append(results, c.ensureMessageStart()...)

	// 关闭文本块和 thinking 块
	c.closeThinkingBlockInto(&results)
	c.closeTextBlockInto(&results)

	idx := c.nextBlockIndex
	c.nextBlockIndex++
	c.toolBlockIndexes[callID] = idx
	c.toolBlockOpen[callID] = true

	start := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"","name":"","input":{}}}`
	start, _ = sjson.Set(start, "index", idx)
	start, _ = sjson.Set(start, "content_block.id", callID)
	start, _ = sjson.Set(start, "content_block.name", name)
	results = append(results, "event: content_block_start\ndata: "+start+"\n\n")
	return results
}

func (c *ResponsesStreamConverter) emitToolInputDelta(callID, partialJSON string) []string {
	idx, ok := c.toolBlockIndexes[callID]
	if !ok {
		return nil
	}
	delta := `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":""}}`
	delta, _ = sjson.Set(delta, "index", idx)
	delta, _ = sjson.Set(delta, "delta.partial_json", partialJSON)
	return []string{"event: content_block_delta\ndata: " + delta + "\n\n"}
}

func (c *ResponsesStreamConverter) closeToolBlock(callID string) []string {
	idx, ok := c.toolBlockIndexes[callID]
	if !ok {
		return nil
	}
	if !c.toolBlockOpen[callID] {
		return nil
	}
	c.toolBlockOpen[callID] = false
	stop := `{"type":"content_block_stop","index":0}`
	stop, _ = sjson.Set(stop, "index", idx)
	return []string{"event: content_block_stop\ndata: " + stop + "\n\n"}
}

func (c *ResponsesStreamConverter) closeTextBlockInto(results *[]string) {
	if !c.textBlockOpen {
		return
	}
	stop := `{"type":"content_block_stop","index":0}`
	stop, _ = sjson.Set(stop, "index", c.textBlockIdx)
	*results = append(*results, "event: content_block_stop\ndata: "+stop+"\n\n")
	c.textBlockOpen = false
	c.textBlockIdx = -1
}

func (c *ResponsesStreamConverter) closeThinkingBlockInto(results *[]string) {
	if !c.thinkingBlockOpen {
		return
	}
	stop := `{"type":"content_block_stop","index":0}`
	stop, _ = sjson.Set(stop, "index", c.thinkingBlockIdx)
	*results = append(*results, "event: content_block_stop\ndata: "+stop+"\n\n")
	c.thinkingBlockOpen = false
	c.thinkingBlockIdx = -1
}

func (c *ResponsesStreamConverter) closeAllOpenBlocks(results *[]string) {
	c.closeThinkingBlockInto(results)
	c.closeTextBlockInto(results)
	for callID, open := range c.toolBlockOpen {
		if open {
			idx := c.toolBlockIndexes[callID]
			stop := `{"type":"content_block_stop","index":0}`
			stop, _ = sjson.Set(stop, "index", idx)
			*results = append(*results, "event: content_block_stop\ndata: "+stop+"\n\n")
			c.toolBlockOpen[callID] = false
		}
	}
}

func (c *ResponsesStreamConverter) emitMessageDeltaAndStop(stopReason string) []string {
	msgDelta := `{"type":"message_delta","delta":{"stop_reason":"","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`
	msgDelta, _ = sjson.Set(msgDelta, "delta.stop_reason", stopReason)
	msgDelta, _ = sjson.Set(msgDelta, "usage.input_tokens", c.inputTokens)
	msgDelta, _ = sjson.Set(msgDelta, "usage.output_tokens", c.outputTokens)
	return []string{
		"event: message_delta\ndata: " + msgDelta + "\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	}
}

func (c *ResponsesStreamConverter) finish(stopReason string) []string {
	var results []string
	results = append(results, c.ensureMessageStart()...)
	c.closeAllOpenBlocks(&results)
	results = append(results, c.emitMessageDeltaAndStop(stopReason)...)
	c.done = true
	return results
}

func (c *ResponsesStreamConverter) extractUsage(resp gjson.Result) {
	usage := resp.Get("usage")
	if !usage.Exists() {
		return
	}
	c.inputTokens = usage.Get("input_tokens").Int()
	if c.inputTokens == 0 {
		c.inputTokens = usage.Get("prompt_tokens").Int()
	}
	c.outputTokens = usage.Get("output_tokens").Int()
	if c.outputTokens == 0 {
		c.outputTokens = usage.Get("completion_tokens").Int()
	}
}

// ConvertResponsesToClaudeNonStream 将 OpenAI /v1/responses JSON 转换为 Anthropic 非流式消息 JSON。
func ConvertResponsesToClaudeNonStream(rawJSON []byte) string {
	root := gjson.ParseBytes(rawJSON)
	out := `{"id":"","type":"message","role":"assistant","model":"","content":[],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}`
	out, _ = sjson.Set(out, "id", root.Get("id").String())
	out, _ = sjson.Set(out, "model", root.Get("model").String())

	hasToolUse := false
	hasContent := false

	if output := root.Get("output"); output.Exists() && output.IsArray() {
		output.ForEach(func(_, item gjson.Result) bool {
			itemType := item.Get("type").String()
			switch itemType {
			case "message":
				if item.Get("role").String() != "assistant" {
					return true
				}
				content := item.Get("content")
				if !content.Exists() || !content.IsArray() {
					return true
				}
				content.ForEach(func(_, part gjson.Result) bool {
					switch part.Get("type").String() {
					case "output_text", "text":
						text := part.Get("text").String()
						if strings.TrimSpace(text) == "" {
							return true
						}
						block := `{"type":"text","text":""}`
						block, _ = sjson.Set(block, "text", text)
						out, _ = sjson.SetRaw(out, "content.-1", block)
						hasContent = true
					case "tool_call", "function_call":
						tool := `{"type":"tool_use","id":"","name":"","input":{}}`
						callID := part.Get("call_id").String()
						if callID == "" {
							callID = part.Get("id").String()
						}
						tool, _ = sjson.Set(tool, "id", callID)
						tool, _ = sjson.Set(tool, "name", part.Get("name").String())

						args := part.Get("arguments")
						if args.Exists() && args.Type == gjson.String {
							fixed := FixJSON(args.String())
							if gjson.Valid(fixed) && gjson.Parse(fixed).IsObject() {
								tool, _ = sjson.SetRaw(tool, "input", fixed)
							}
						} else if args.Exists() && args.IsObject() {
							tool, _ = sjson.SetRaw(tool, "input", args.Raw)
						}

						out, _ = sjson.SetRaw(out, "content.-1", tool)
						hasToolUse = true
						hasContent = true
					}
					return true
				})

			case "function_call":
				// 顶层 function_call output item
				tool := `{"type":"tool_use","id":"","name":"","input":{}}`
				callID := item.Get("call_id").String()
				if callID == "" {
					callID = item.Get("id").String()
				}
				tool, _ = sjson.Set(tool, "id", callID)
				tool, _ = sjson.Set(tool, "name", item.Get("name").String())

				args := item.Get("arguments")
				if args.Exists() && args.Type == gjson.String {
					fixed := FixJSON(args.String())
					if gjson.Valid(fixed) && gjson.Parse(fixed).IsObject() {
						tool, _ = sjson.SetRaw(tool, "input", fixed)
					}
				} else if args.Exists() && args.IsObject() {
					tool, _ = sjson.SetRaw(tool, "input", args.Raw)
				}

				out, _ = sjson.SetRaw(out, "content.-1", tool)
				hasToolUse = true
				hasContent = true
			}
			return true
		})
	}

	if !hasContent {
		if outputText := root.Get("output_text"); outputText.Exists() && strings.TrimSpace(outputText.String()) != "" {
			block := `{"type":"text","text":""}`
			block, _ = sjson.Set(block, "text", outputText.String())
			out, _ = sjson.SetRaw(out, "content.-1", block)
		}
	}

	if usage := root.Get("usage"); usage.Exists() {
		inputTokens := usage.Get("input_tokens").Int()
		if inputTokens == 0 {
			inputTokens = usage.Get("prompt_tokens").Int()
		}
		outputTokens := usage.Get("output_tokens").Int()
		if outputTokens == 0 {
			outputTokens = usage.Get("completion_tokens").Int()
		}
		out, _ = sjson.Set(out, "usage.input_tokens", inputTokens)
		out, _ = sjson.Set(out, "usage.output_tokens", outputTokens)
	}

	if hasToolUse {
		out, _ = sjson.Set(out, "stop_reason", "tool_use")
	}

	return out
}

// ClaudeTokenCount 返回 Anthropic 格式的 count_tokens 响应体。
func ClaudeTokenCount(_ context.Context, count int64) string {
	return fmt.Sprintf(`{"input_tokens":%d}`, count)
}
