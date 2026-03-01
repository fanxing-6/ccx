package proxy

import (
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// applyProfileThinking injects a default Claude thinking config when:
// 1) profile has OPENAI_REASONING_EFFORT configured, and
// 2) request body has no explicit "thinking".
func applyProfileThinking(body []byte, reasoningEffort string) ([]byte, error) {
	if len(body) == 0 || reasoningEffort == "" {
		return body, nil
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("请求 JSON 无效")
	}
	if gjson.GetBytes(body, "thinking").Exists() {
		return body, nil
	}

	normalized, ok := NormalizeReasoningEffort(reasoningEffort)
	if !ok {
		return nil, fmt.Errorf("无效思考档位: %q", reasoningEffort)
	}
	if normalized == "" {
		return body, nil
	}

	budget, ok := convertLevelToBudget(normalized)
	if !ok {
		return nil, fmt.Errorf("无效思考档位: %q", normalized)
	}

	out := string(body)
	if normalized == thinkingLevelNone {
		out, _ = sjson.Set(out, "thinking.type", "disabled")
		return []byte(out), nil
	}

	out, _ = sjson.Set(out, "thinking.type", "enabled")
	out, _ = sjson.Set(out, "thinking.budget_tokens", budget)
	return []byte(out), nil
}
