package proxy

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "empty", in: "", want: "", ok: true},
		{name: "trim and lowercase", in: " High ", want: "high", ok: true},
		{name: "xhigh", in: "xhigh", want: "xhigh", ok: true},
		{name: "invalid", in: "ultra", want: "", ok: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := NormalizeReasoningEffort(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NormalizeReasoningEffort(%q)=(%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestApplyProfileThinking(t *testing.T) {
	t.Parallel()

	t.Run("inject enabled with budget", func(t *testing.T) {
		in := []byte(`{"model":"x","messages":[]}`)
		out, err := applyProfileThinking(in, "high")
		if err != nil {
			t.Fatalf("applyProfileThinking returned error: %v", err)
		}
		root := gjson.ParseBytes(out)
		if root.Get("thinking.type").String() != "enabled" {
			t.Fatalf("thinking.type=%q, want enabled", root.Get("thinking.type").String())
		}
		if root.Get("thinking.budget_tokens").Int() != 24576 {
			t.Fatalf("thinking.budget_tokens=%d, want 24576", root.Get("thinking.budget_tokens").Int())
		}
	})

	t.Run("inject disabled for none", func(t *testing.T) {
		in := []byte(`{"model":"x","messages":[]}`)
		out, err := applyProfileThinking(in, "none")
		if err != nil {
			t.Fatalf("applyProfileThinking returned error: %v", err)
		}
		root := gjson.ParseBytes(out)
		if root.Get("thinking.type").String() != "disabled" {
			t.Fatalf("thinking.type=%q, want disabled", root.Get("thinking.type").String())
		}
		if root.Get("thinking.budget_tokens").Exists() {
			t.Fatalf("thinking.budget_tokens should not exist for disabled thinking")
		}
	})

	t.Run("do not override existing thinking", func(t *testing.T) {
		in := []byte(`{"model":"x","thinking":{"type":"enabled","budget_tokens":1},"messages":[]}`)
		out, err := applyProfileThinking(in, "high")
		if err != nil {
			t.Fatalf("applyProfileThinking returned error: %v", err)
		}
		root := gjson.ParseBytes(out)
		if root.Get("thinking.budget_tokens").Int() != 1 {
			t.Fatalf("existing thinking should be preserved, got %d", root.Get("thinking.budget_tokens").Int())
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		in := []byte(`{"model":"x","messages":[]}`)
		_, err := applyProfileThinking(in, "ultra")
		if err == nil {
			t.Fatalf("expected error for invalid level")
		}
	})
}

func TestStartProxyFailFastOnInvalidReasoningEffort(t *testing.T) {
	t.Parallel()

	_, _, err := StartProxy("https://api.example.com/v1", "token", ProxyOptions{
		ReasoningEffort: "ultra",
	})
	if err == nil {
		t.Fatalf("expected StartProxy to fail on invalid OPENAI_REASONING_EFFORT")
	}
	if !strings.Contains(err.Error(), "OPENAI_REASONING_EFFORT") {
		t.Fatalf("error should mention OPENAI_REASONING_EFFORT: %v", err)
	}
}
