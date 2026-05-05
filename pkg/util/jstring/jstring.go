package util

import (
	"strings"
)

var base37Lookup = []uint8{
	'_', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i',
	'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's',
	't', 'u', 'v', 'w', 'x', 'y', 'z',
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
}

func ToBase37(s string) uint64 {
	s = strings.TrimSpace(s)
	var l uint64 = 0

	for i := 0; i < len(s) && i < 12; i++ {
		c := uint64(s[i])
		l *= 37

		if c >= 0x41 && c <= 0x5A { // A-Z
			l += (c + 1) - 0x41
		} else if c >= 0x61 && c <= 0x7A { // a-z
			l += (c + 1) - 0x61
		} else if c >= 0x30 && c <= 0x39 { // 0-9
			l += (c + 27) - 0x30
		}
	}

	return l
}

func FromBase37(v uint64) string {
	// >= 37 to the 12th power
	if v >= 6582952005840035281 {
		return "invalid_name"
	}

	// Mirrors TS JString.ts:42-44 — values divisible by 37 are invalid
	// (NAI-72: surfaced by social handler invalid_name gate).
	if v != 0 && v%37 == 0 {
		return "invalid_name"
	}

	l := 0
	chars := make([]uint8, 12)
	for v != 0 {
		l1 := v
		v /= 37
		chars[11-l] = base37Lookup[l1-v*37]
		l += 1
	}

	return string(chars[12-l:]) // TODO: is this right? the last 12 chars?
}

func ToTitleCase(s string) string {
	return strings.Title(s)
}

func ToSafeName(s string) string {
	return FromBase37(ToBase37(s))
}

func ToDisplayName(s string) string {
	return ToTitleCase(strings.ReplaceAll(ToSafeName(s), "_", " "))
}
