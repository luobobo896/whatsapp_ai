package audit

import (
	"encoding/json"
	"strings"
)

const redacted = "[REDACTED]"

func Redact(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return redacted
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return redacted
	}
	return redactValue(normalized)
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if secretKey(key) {
				result[key] = redacted
			} else {
				result[key] = redactValue(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = redactValue(item)
		}
		return result
	default:
		return value
	}
}

func secretKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(key))
	return strings.Contains(normalized, "password") ||
		strings.HasSuffix(normalized, "secret") ||
		strings.HasSuffix(normalized, "token") ||
		strings.Contains(normalized, "apikey") ||
		strings.HasSuffix(normalized, "authorization")
}
