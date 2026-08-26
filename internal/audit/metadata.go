package audit

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	maxMetadataDepth  = 4
	maxMetadataKeys   = 64
	maxMetadataArray  = 32
	maxMetadataString = 512
	maxMetadataBudget = 8192
)

var sensitiveMetadataKeys = map[string]struct{}{
	"authorization": {}, "certificatebody": {}, "certificatepem": {}, "clientidentifier": {},
	"clientidentity": {}, "credential": {}, "credentialencryptionmaterial": {}, "credentialnonce": {},
	"credentials": {}, "csrftoken": {}, "customcapem": {}, "destination": {}, "encryptedcredentials": {},
	"nodecredentials": {}, "password": {}, "passwordhash": {}, "privatekey": {}, "query": {},
	"querycontents": {}, "queryname": {}, "rawerror": {}, "responsebody": {}, "rule": {}, "rules": {},
	"secret": {}, "secrets": {}, "sessiontoken": {}, "token": {}, "username": {}, "webhooksecret": {},
	"totp": {}, "totpcode": {}, "otpcode": {}, "recoverycode": {}, "recoverycodehash": {},
	"provisioninguri": {}, "qrcode": {}, "qrcodepayload": {},
}

// SafeMetadata is the common persistence and representation boundary for audit
// evidence. Known safe summaries remain available, while suspicious values and
// unbounded unknown structures cannot enter or leave the durable audit stream.
func SafeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	budget := maxMetadataBudget
	value, ok := safeObject(metadata, 0, &budget)
	if !ok {
		return map[string]any{}
	}
	return value
}

func safeObject(input map[string]any, depth int, budget *int) (map[string]any, bool) {
	if depth >= maxMetadataDepth || *budget <= 0 {
		return map[string]any{"truncated": true}, true
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maxMetadataKeys {
		keys = keys[:maxMetadataKeys]
	}
	result := make(map[string]any, len(keys)+1)
	for _, key := range keys {
		if *budget <= 0 {
			result["truncated"] = true
			break
		}
		*budget -= len(key)
		if sensitiveMetadataKey(key) {
			result[key] = "[REDACTED]"
			continue
		}
		if value, ok := safeValue(input[key], depth+1, budget); ok {
			result[key] = value
		}
	}
	if len(input) > len(keys) {
		result["truncated"] = true
	}
	return result, true
}

func safeValue(input any, depth int, budget *int) (any, bool) {
	if *budget <= 0 {
		return "[TRUNCATED]", true
	}
	switch value := input.(type) {
	case nil, bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return value, true
	case string:
		return boundedString(value, budget), true
	case fmt.Stringer:
		return boundedString(value.String(), budget), true
	case map[string]any:
		return safeObject(value, depth, budget)
	case []any:
		if depth >= maxMetadataDepth {
			return []any{"[TRUNCATED]"}, true
		}
		limit := len(value)
		if limit > maxMetadataArray {
			limit = maxMetadataArray
		}
		result := make([]any, 0, limit+1)
		for _, item := range value[:limit] {
			safe, ok := safeValue(item, depth+1, budget)
			if ok {
				result = append(result, safe)
			}
		}
		if len(value) > limit {
			result = append(result, "[TRUNCATED]")
		}
		return result, true
	case []string:
		items := make([]any, len(value))
		for index := range value {
			items[index] = value[index]
		}
		return safeValue(items, depth, budget)
	default:
		return boundedString(fmt.Sprint(value), budget), true
	}
}

func boundedString(value string, budget *int) string {
	limit := maxMetadataString
	if *budget < limit {
		limit = *budget
	}
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:limit]) + "…"
	}
	*budget -= len([]rune(value))
	return value
}

func sensitiveMetadataKey(key string) bool {
	normalized := strings.Map(func(value rune) rune {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			return unicode.ToLower(value)
		}
		return -1
	}, key)
	if _, sensitive := sensitiveMetadataKeys[normalized]; sensitive {
		return true
	}
	if normalized == "destinationsummary" || normalized == "destinationreplaced" || strings.HasPrefix(normalized, "querylog") {
		return false
	}
	for _, fragment := range []string{
		"password", "passwd", "token", "secret", "credential", "authorization",
		"privatekey", "certificatebody", "certificatepem", "customcapem", "rawerror",
		"rawnodeerror", "responsebody", "querycontents", "queryname", "querytext",
		"clientidentity", "clientidentifier", "destination", "webhookurl",
		"totp", "otpcode", "recoverycode", "provisioninguri", "qrcode",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return strings.HasSuffix(normalized, "rule") || strings.HasSuffix(normalized, "rules")
}
