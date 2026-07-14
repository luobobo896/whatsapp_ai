package audit

import (
	"reflect"
	"strings"
)

const redacted = "[REDACTED]"

func Redact(value any) any {
	return redactValue(reflect.ValueOf(value))
}

func redactValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return value.Interface()
		}
		result := make(map[string]any, value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			key := iterator.Key().String()
			if secretKey(key) {
				result[key] = redacted
			} else {
				result[key] = redactValue(iterator.Value())
			}
		}
		return result
	case reflect.Slice, reflect.Array:
		result := make([]any, value.Len())
		for i := range value.Len() {
			result[i] = redactValue(value.Index(i))
		}
		return result
	default:
		return value.Interface()
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
