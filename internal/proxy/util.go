package proxy

import (
	"bytes"
	"strings"

	"github.com/tidwall/gjson"
)

// Thinking levels used by reasoning_effort and Claude thinking budget mapping.
const (
	thinkingLevelNone    = "none"
	thinkingLevelAuto    = "auto"
	thinkingLevelMinimal = "minimal"
	thinkingLevelLow     = "low"
	thinkingLevelMedium  = "medium"
	thinkingLevelHigh    = "high"
	thinkingLevelXHigh   = "xhigh"
)

var levelToBudgetMap = map[string]int{
	thinkingLevelNone:    0,
	thinkingLevelAuto:    -1,
	thinkingLevelMinimal: 512,
	thinkingLevelLow:     1024,
	thinkingLevelMedium:  8192,
	thinkingLevelHigh:    24576,
	thinkingLevelXHigh:   32768,
}

func convertBudgetToLevel(budget int) (string, bool) {
	switch {
	case budget < -1:
		return "", false
	case budget == -1:
		return thinkingLevelAuto, true
	case budget == 0:
		return thinkingLevelNone, true
	case budget <= 512:
		return thinkingLevelMinimal, true
	case budget <= 1024:
		return thinkingLevelLow, true
	case budget <= 8192:
		return thinkingLevelMedium, true
	case budget <= 24576:
		return thinkingLevelHigh, true
	default:
		return thinkingLevelXHigh, true
	}
}

func convertLevelToBudget(level string) (int, bool) {
	budget, ok := levelToBudgetMap[strings.ToLower(strings.TrimSpace(level))]
	return budget, ok
}

// NormalizeReasoningEffort validates and normalizes effort level to lowercase.
// Allowed values follow CLIProxyAPI level semantics:
// none/auto/minimal/low/medium/high/xhigh.
func NormalizeReasoningEffort(level string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(level))
	if normalized == "" {
		return "", true
	}
	if _, ok := convertLevelToBudget(normalized); !ok {
		return "", false
	}
	return normalized, true
}

// getThinkingText extracts text from thinking content blocks with several provider variants.
func getThinkingText(part gjson.Result) string {
	if text := part.Get("text"); text.Exists() && text.Type == gjson.String {
		return text.String()
	}

	thinkingField := part.Get("thinking")
	if !thinkingField.Exists() {
		return ""
	}

	if thinkingField.Type == gjson.String {
		return thinkingField.String()
	}

	if thinkingField.IsObject() {
		if inner := thinkingField.Get("text"); inner.Exists() && inner.Type == gjson.String {
			return inner.String()
		}
		if inner := thinkingField.Get("thinking"); inner.Exists() && inner.Type == gjson.String {
			return inner.String()
		}
	}

	return ""
}

// FixJSON converts non-standard JSON that uses single-quoted strings to valid JSON.
func FixJSON(input string) string {
	var out bytes.Buffer

	inDouble := false
	inSingle := false
	escaped := false

	writeConverted := func(r rune) {
		if r == '"' {
			out.WriteByte('\\')
			out.WriteByte('"')
			return
		}
		out.WriteRune(r)
	}

	runes := []rune(input)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inDouble {
			out.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inDouble = false
			}
			continue
		}

		if inSingle {
			if escaped {
				escaped = false
				switch r {
				case 'n', 'r', 't', 'b', 'f', '/', '"':
					out.WriteByte('\\')
					out.WriteRune(r)
				case '\\':
					out.WriteByte('\\')
					out.WriteByte('\\')
				case '\'':
					out.WriteRune('\'')
				case 'u':
					out.WriteByte('\\')
					out.WriteByte('u')
					for k := 0; k < 4 && i+1 < len(runes); k++ {
						peek := runes[i+1]
						if (peek >= '0' && peek <= '9') || (peek >= 'a' && peek <= 'f') || (peek >= 'A' && peek <= 'F') {
							out.WriteRune(peek)
							i++
						} else {
							break
						}
					}
				default:
					out.WriteByte('\\')
					out.WriteRune(r)
				}
				continue
			}

			if r == '\\' {
				escaped = true
				continue
			}
			if r == '\'' {
				out.WriteByte('"')
				inSingle = false
				continue
			}
			writeConverted(r)
			continue
		}

		if r == '"' {
			inDouble = true
			out.WriteRune(r)
			continue
		}
		if r == '\'' {
			inSingle = true
			out.WriteByte('"')
			continue
		}
		out.WriteRune(r)
	}

	if inSingle {
		out.WriteByte('"')
	}

	return out.String()
}
