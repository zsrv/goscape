package encfilter

// helpers.go ports the static methods on TS WordEnc (Engine-TS/src/cache/
// wordenc/WordEnc.ts:97-188). All operate on []rune to match TS character
// semantics over the ASCII + £/€ charset.

func isLowercaseAlpha(c rune) bool { return c >= 'a' && c <= 'z' }
func isUppercaseAlpha(c rune) bool { return c >= 'A' && c <= 'Z' }
func isNumerical(c rune) bool      { return c >= '0' && c <= '9' }
func isAlpha(c rune) bool          { return isLowercaseAlpha(c) || isUppercaseAlpha(c) }
func isSymbol(c rune) bool         { return !isAlpha(c) && !isNumerical(c) }

// isNotLowercaseAlpha mirrors WordEnc.ts:101-103.
//
//	return this.isLowercaseAlpha(char)
//	  ? char == 'v' || char == 'x' || char == 'j' || char == 'q' || char == 'z'
//	  : true
//
// I.e. uncommon-lowercase OR not-lowercase-at-all.
func isNotLowercaseAlpha(c rune) bool {
	if isLowercaseAlpha(c) {
		return c == 'v' || c == 'x' || c == 'j' || c == 'q' || c == 'z'
	}
	return true
}

// isNumericalChars mirrors WordEnc.ts:121-128. NUL ('\x00') counts as
// wildcard (returns true), any other non-digit returns false.
func isNumericalChars(chars []rune) bool {
	for _, c := range chars {
		if !isNumerical(c) && c != '\x00' {
			return false
		}
	}
	return true
}

// maskChars replaces chars[offset:length] with '*'. Mirrors WordEnc.ts:130-134.
// NOTE: TS uses `for index = offset; index < length; index++` — length is the
// EXCLUSIVE upper bound, NOT a count.
func maskChars(offset, end int, chars []rune) {
	for i := offset; i < end; i++ {
		chars[i] = '*'
	}
}

// maskedCountBackwards mirrors WordEnc.ts:136-144.
func maskedCountBackwards(chars []rune, offset int) int {
	count := 0
	for i := offset - 1; i >= 0 && isSymbol(chars[i]); i-- {
		if chars[i] == '*' {
			count++
		}
	}
	return count
}

// maskedCountForwards mirrors WordEnc.ts:146-154.
func maskedCountForwards(chars []rune, offset int) int {
	count := 0
	for i := offset + 1; i < len(chars) && isSymbol(chars[i]); i++ {
		if chars[i] == '*' {
			count++
		}
	}
	return count
}

// maskedCharsStatus mirrors WordEnc.ts:156-164. Returns 0/1/4.
func maskedCharsStatus(chars, filtered []rune, offset, length int, prefix bool) int {
	var count int
	if prefix {
		count = maskedCountBackwards(filtered, offset)
	} else {
		count = maskedCountForwards(filtered, offset)
	}
	if count >= length {
		return 4
	}
	var adj rune
	if prefix {
		adj = chars[offset-1]
	} else {
		adj = chars[offset+1]
	}
	if isSymbol(adj) {
		return 1
	}
	return 0
}

// prefixSymbolStatus mirrors WordEnc.ts:166-176.
func prefixSymbolStatus(offset int, chars []rune, length int, symbolChars []rune, symbols []rune) int {
	if offset == 0 {
		return 2
	}
	for i := offset - 1; i >= 0 && isSymbol(chars[i]); i-- {
		for _, s := range symbols {
			if chars[i] == s {
				return 3
			}
		}
	}
	return maskedCharsStatus(chars, symbolChars, offset, length, true)
}

// suffixSymbolStatus mirrors WordEnc.ts:178-188.
func suffixSymbolStatus(offset int, chars []rune, length int, symbolChars []rune, symbols []rune) int {
	if offset+1 == len(chars) {
		return 2
	}
	for i := offset + 1; i < len(chars) && isSymbol(chars[i]); i++ {
		for _, s := range symbols {
			if chars[i] == s {
				return 3
			}
		}
	}
	return maskedCharsStatus(chars, symbolChars, offset, length, false)
}

// isCharacterAllowed mirrors WordEnc.ts:240-242.
// Accepts ASCII printable ' '..'\x7f', plus '\n', '\t', '£', '€'.
func isCharacterAllowed(c rune) bool {
	if c >= ' ' && c <= '\x7f' {
		return true
	}
	return c == ' ' || c == '\n' || c == '\t' || c == '£' || c == '€'
}

// format mirrors WordEnc.ts:223-238. In-place: replaces disallowed chars with
// space, collapses consecutive spaces, pads tail with spaces.
func format(chars []rune) {
	pos := 0
	for i := range len(chars) {
		if isCharacterAllowed(chars[i]) {
			chars[pos] = chars[i]
		} else {
			chars[pos] = ' '
		}
		if pos == 0 || chars[pos] != ' ' || chars[pos-1] != ' ' {
			pos++
		}
	}
	for i := pos; i < len(chars); i++ {
		chars[i] = ' '
	}
}

// replaceUppercases mirrors WordEnc.ts:244-250. For each i in [0, len(comparison)):
// if comparison[i] is uppercase AND chars[i] != '*', copy comparison[i] over chars[i].
func replaceUppercases(chars, comparison []rune) {
	for i := range len(comparison) {
		if chars[i] != '*' && isUppercaseAlpha(comparison[i]) {
			chars[i] = comparison[i]
		}
	}
}

// formatUppercases mirrors WordEnc.ts:252-266. First letter of each alphabetic
// run keeps its case; subsequent uppercase letters in the same run get
// lowercased (to canonicalize "HELLO world" — but only after the run starts
// with a lowercase).
func formatUppercases(chars []rune) {
	flagged := true
	for i, c := range chars {
		if !isAlpha(c) {
			flagged = true
		} else if flagged {
			if isLowercaseAlpha(c) {
				flagged = false
			}
		} else if isUppercaseAlpha(c) {
			chars[i] = c + ('a' - 'A')
		}
	}
}
