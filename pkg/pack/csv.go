package pack

import "fmt"

// parseCsv splits s on commas, respecting quoted fields. Mirrors the
// identical helpers duplicated in TS DbTableConfig.ts:6-26 and
// DbRowConfig.ts:7-27. Quotes are stripped from the output (TS toggles
// inQuotes on each '"' but does not emit the quote char).
//
// Always returns at least one element (the suffix after the last
// unquoted comma).
//
// TS source: tools/pack/config/DbTableConfig.ts:6-26.
func parseCsv(s string) []string {
	result := []string{}
	current := []byte{}
	inQuotes := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ',' && !inQuotes:
			result = append(result, string(current))
			current = current[:0]
		case c == '"':
			inQuotes = !inQuotes
		default:
			current = append(current, c)
		}
	}
	result = append(result, string(current))
	return result
}

// packStepError formats a typed error for per-config pack failures.
// Mirrors TS packStepError(debugname, message) — debugname appears in
// brackets prefix, followed by the format-applied message.
//
// TS source: tools/pack/config/PackShared.ts (packStepError export).
func packStepError(debugname, format string, args ...any) error {
	return fmt.Errorf("["+debugname+"] "+format, args...)
}
