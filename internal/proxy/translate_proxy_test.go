package proxy

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestConvertClaudeRequestToResponses_AlignedShape(t *testing.T) {
	in := []byte(`{
	  "model":"gpt-5.3-codex",
	  "system":[
	    {"type":"text","text":"x-anthropic-billing-header: ignore"},
	    {"type":"text","text":"system prompt"}
	  ],
	  "messages":[
	    {"role":"assistant","content":[{"type":"text","text":"hello"}]},
	    {"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"foo","input":{"x":1}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":"done"}]}
	  ],
	  "tools":[
	    {"name":"foo","description":"d","input_schema":{"type":"object","properties":{"x":{"type":"number"}}}}
	  ]
	}`)

	out := ConvertClaudeRequestToResponses(in, false)
	root := gjson.ParseBytes(out)

	if root.Get("model").String() != "gpt-5.3-codex" {
		t.Fatalf("unexpected model: %s", root.Get("model").String())
	}
	if !root.Get("parallel_tool_calls").Bool() {
		t.Fatalf("parallel_tool_calls should be true")
	}
	if root.Get("reasoning.summary").String() != "auto" {
		t.Fatalf("reasoning.summary should be auto")
	}
	if !root.Get("include").IsArray() || root.Get("include.0").String() != "reasoning.encrypted_content" {
		t.Fatalf("include should contain reasoning.encrypted_content")
	}
	if root.Get("stream").Bool() {
		t.Fatalf("stream should follow function argument")
	}

	// system -> developer message
	if root.Get("input.0.type").String() != "message" || root.Get("input.0.role").String() != "developer" {
		t.Fatalf("input.0 should be developer message: %s", root.Get("input.0").Raw)
	}
	if root.Get("input.0.content.0.type").String() != "input_text" || root.Get("input.0.content.0.text").String() != "system prompt" {
		t.Fatalf("unexpected system mapping: %s", root.Get("input.0.content").Raw)
	}

	// assistant text -> output_text
	if root.Get("input.1.type").String() != "message" || root.Get("input.1.role").String() != "assistant" {
		t.Fatalf("input.1 should be assistant message: %s", root.Get("input.1").Raw)
	}
	if root.Get("input.1.content.0.type").String() != "output_text" || root.Get("input.1.content.0.text").String() != "hello" {
		t.Fatalf("assistant text should map to output_text: %s", root.Get("input.1.content").Raw)
	}

	// tool_use -> function_call
	if root.Get("input.2.type").String() != "function_call" || root.Get("input.2.call_id").String() != "call_1" {
		t.Fatalf("tool_use should map to function_call: %s", root.Get("input.2").Raw)
	}
	if !strings.Contains(root.Get("input.2.arguments").String(), `"x":1`) {
		t.Fatalf("function_call arguments missing: %s", root.Get("input.2").Raw)
	}

	// tool_result -> function_call_output
	if root.Get("input.3.type").String() != "function_call_output" || root.Get("input.3.call_id").String() != "call_1" {
		t.Fatalf("tool_result should map to function_call_output: %s", root.Get("input.3").Raw)
	}
	if root.Get("input.3.output").String() != "done" {
		t.Fatalf("unexpected tool_result output: %s", root.Get("input.3").Raw)
	}
}

func TestConvertResponsesToClaudeNonStream_AlignedUsageAndStopReason(t *testing.T) {
	in := []byte(`{
	  "id":"resp_1",
	  "model":"gpt-5.3-codex",
	  "usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":10}},
	  "output":[
	    {"type":"reasoning","summary":[{"text":"think"}]},
	    {"type":"message","content":[{"type":"output_text","text":"answer"}]},
	    {"type":"function_call","call_id":"call_1","name":"foo","arguments":"{\"a\":1}"}
	  ]
	}`)

	out := ConvertResponsesToClaudeNonStream(in)
	root := gjson.Parse(out)

	if root.Get("id").String() != "resp_1" || root.Get("model").String() != "gpt-5.3-codex" {
		t.Fatalf("unexpected id/model: %s", out)
	}
	if root.Get("usage.input_tokens").Int() != 90 || root.Get("usage.output_tokens").Int() != 20 {
		t.Fatalf("usage should subtract cached tokens: %s", root.Get("usage").Raw)
	}
	if root.Get("usage.cache_read_input_tokens").Int() != 10 {
		t.Fatalf("missing cache_read_input_tokens: %s", root.Get("usage").Raw)
	}
	if root.Get("stop_reason").String() != "tool_use" {
		t.Fatalf("stop_reason should be tool_use: %s", out)
	}
	if root.Get("content.0.type").String() != "thinking" || root.Get("content.0.thinking").String() != "think" {
		t.Fatalf("unexpected reasoning mapping: %s", root.Get("content.0").Raw)
	}
	if root.Get("content.1.type").String() != "text" || root.Get("content.1.text").String() != "answer" {
		t.Fatalf("unexpected text mapping: %s", root.Get("content.1").Raw)
	}
	if root.Get("content.2.type").String() != "tool_use" || root.Get("content.2.id").String() != "call_1" {
		t.Fatalf("unexpected tool_use mapping: %s", root.Get("content.2").Raw)
	}
	if root.Get("content.2.input.a").Int() != 1 {
		t.Fatalf("tool_use input should parse JSON object: %s", root.Get("content.2").Raw)
	}
}

func TestResponsesStreamConverter_ToolCallDoneWithoutDelta(t *testing.T) {
	c := NewResponsesStreamConverter()

	var all []string
	all = append(all, c.ProcessEvent("response.created", `{"response":{"id":"resp_1","model":"gpt-5.3-codex"}}`)...)
	all = append(all, c.ProcessEvent("response.output_item.added", `{"item":{"type":"function_call","call_id":"call_1","name":"foo"}}`)...)
	all = append(all, c.ProcessEvent("response.function_call_arguments.done", `{"arguments":"{\"a\":1}"}`)...)
	all = append(all, c.ProcessEvent("response.output_item.done", `{"item":{"type":"function_call"}}`)...)
	all = append(all, c.ProcessEvent("response.completed", `{"response":{"stop_reason":"stop","usage":{"input_tokens":10,"output_tokens":2}}}`)...)

	out := strings.Join(all, "")
	if !strings.Contains(out, `"type":"message_start"`) {
		t.Fatalf("missing message_start: %s", out)
	}
	if !strings.Contains(out, `"type":"input_json_delta","partial_json":"{\"a\":1}"`) {
		t.Fatalf("should emit arguments from done event when no delta seen: %s", out)
	}
	if !strings.Contains(out, `"stop_reason":"tool_use"`) {
		t.Fatalf("tool call stream should end with tool_use stop_reason: %s", out)
	}
}
