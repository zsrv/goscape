// Package wordpack ports the TS WordPack codec (Engine-TS
// src/wordenc/WordPack.ts) used by the MessagePrivate handler to decode
// the word-packed chat payload. NAI-158.
//
// The codec uses a 60-entry character table indexed by 4-bit (indices
// 0-12) or 12-bit (indices 13-59, encoded as a carry nibble + 8 bits)
// nibble groups. Two nibbles fit in each byte.
package wordpack

import (
	"strings"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// charLookup mirrors TS WordPack.CHAR_LOOKUP (WordPack.ts:5-12).
// Stored as []string instead of []byte because entry 56 ('£') is a
// multi-byte UTF-8 codepoint — preserving it as a length-1 substring
// keeps the TS semantics of "one table slot per character".
var charLookup = []string{
	" ",
	"e", "t", "a", "o", "i", "h", "n", "s", "r", "d", "l", "u", "m",
	"w", "c", "y", "f", "g", "p", "b", "v", "k", "x", "j", "q", "z",
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
	" ", "!", "?", ".", ",", ":", ";", "(", ")", "-",
	"&", "*", "\\", "'", "@", "#", "+", "=", "£", "$", "%", "\"", "[", "]",
}

// Unpack decodes length bytes of word-packed input from pk starting at
// pk.Pos, returning the sentence-cased plain text. Mirrors TS
// WordPack.unpack (WordPack.ts:14-41).
//
// Output is capped at 80 characters per TS line 19 (`pos < 80`).
func Unpack(pk *packet.Packet, length int) string {
	var parts []string
	pos := 0
	carry := -1
	for i := 0; i < length && pos < 80; i++ {
		data := int(pk.G1())
		nibble := (data >> 4) & 0xf
		if carry != -1 {
			parts = append(parts, charLookup[(carry<<4)+nibble-195])
			pos++
			carry = -1
		} else if nibble < 13 {
			parts = append(parts, charLookup[nibble])
			pos++
		} else {
			carry = nibble
		}
		nibble = data & 0xf
		if carry != -1 {
			parts = append(parts, charLookup[(carry<<4)+nibble-195])
			pos++
			carry = -1
		} else if nibble < 13 {
			parts = append(parts, charLookup[nibble])
			pos++
		} else {
			carry = nibble
		}
	}
	return toSentenceCase(strings.Join(parts, ""))
}

// Pack encodes input as word-packed bytes appended to pk. Input is
// lowercased and truncated to 80 characters first. Mirrors TS
// WordPack.pack (WordPack.ts:43-78).
func Pack(pk *packet.Packet, input string) {
	// Truncate to 80 runes (TS line 44-46 uses substring(0, 80) which
	// is UTF-16-code-unit-based; for the limited charLookup alphabet
	// all chars are single-rune so rune-count truncation matches).
	runes := []rune(strings.ToLower(input))
	if len(runes) > 80 {
		runes = runes[:80]
	}
	carry := -1
	for _, r := range runes {
		ch := string(r)
		index := 0
		for j := range len(charLookup) {
			if ch == charLookup[j] {
				index = j
				break
			}
		}
		if index > 12 {
			index += 195
		}
		if carry == -1 {
			if index < 13 {
				carry = index
			} else {
				pk.P1(uint8(index))
			}
		} else if index < 13 {
			pk.P1(uint8((carry << 4) + index))
			carry = -1
		} else {
			pk.P1(uint8((carry << 4) + (index >> 4)))
			carry = index & 0xf
		}
	}
	if carry != -1 {
		pk.P1(uint8(carry << 4))
	}
}

// toSentenceCase mirrors TS WordPack.toSentenceCase (WordPack.ts:80-94):
// capitalize the first lowercase letter at the start of the string and
// after any '.' or '!'.
func toSentenceCase(input string) string {
	chars := []rune(strings.ToLower(input))
	punctuation := true
	for i, c := range chars {
		if punctuation && c >= 'a' && c <= 'z' {
			chars[i] = c - 'a' + 'A'
			punctuation = false
		}
		if c == '.' || c == '!' {
			punctuation = true
		}
	}
	return string(chars)
}
