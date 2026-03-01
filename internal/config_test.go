package internal

import (
	"encoding/json"
	"testing"
)

func TestExtractProfileInfoReasoningEffort(t *testing.T) {
	t.Parallel()

	settings := json.RawMessage(`{
	  "api_format":"openai",
	  "env":{
	    "ANTHROPIC_BASE_URL":"https://api.linkflow.run/v1",
	    "ANTHROPIC_MODEL":"gpt-5.3-codex",
	    "ANTHROPIC_API_KEY":"k",
	    "ANTHROPIC_AUTH_TOKEN":"k",
	    "OPENAI_REASONING_EFFORT":"high"
	  }
	}`)

	info := ExtractProfileInfo(settings)
	if info.APIFormat != "openai" {
		t.Fatalf("APIFormat=%q, want openai", info.APIFormat)
	}
	if info.ReasoningEffort != "high" {
		t.Fatalf("ReasoningEffort=%q, want high", info.ReasoningEffort)
	}
}
