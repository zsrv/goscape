package encfilter

// badwords.go mirrors TS WordEncBadWords (Engine-TS/src/cache/wordenc/
// WordEncBadWords.ts).

// badWords holds the bad word list, per-word combo lists, and a back-reference
// to fragments for the substring-validity check.
type badWords struct {
	bads       [][]rune
	combos     [][][2]int // parallel array; nil entry means "no combos for this word"
	fragments_ *fragments // back-ref for isBadFragment check (substring validity)
}

// filter runs filterBadCombinations for each bad word TWICE (the comboIndex
// loop 0..1 in WordEncBadWords.ts:14-19). Walks bad words from len-1 down to 0.
func (b *badWords) filter(chars []rune) {
	for range 2 {
		for i := len(b.bads) - 1; i >= 0; i-- {
			b.filterBadCombinations(b.combos[i], chars, b.bads[i])
		}
	}
}

// filterBadCombinations scans chars for occurrences of bad starting at every
// position; if a match is found, the combo / symbol / substring-validity
// conditions are checked and the match is masked when the masking threshold
// (numeralCount <= alphaCount) holds. Mirrors WordEncBadWords.ts:22-111.
func (b *badWords) filterBadCombinations(combos [][2]int, chars []rune, bad []rune) {
	if len(bad) > len(chars) {
		return
	}
	for startIndex := 0; startIndex <= len(chars)-len(bad); startIndex++ {
		currentIndex := startIndex
		updated, badIndex, hasSymbol, hasNumber, hasDigit := b.processBadCharacters(chars, bad, currentIndex)
		currentIndex = updated
		if !(badIndex >= len(bad) && (!hasNumber || !hasDigit)) {
			continue
		}
		shouldFilter := true
		if hasSymbol {
			isBeforeSymbol := false
			isAfterSymbol := false
			if startIndex-1 < 0 || (isSymbol(chars[startIndex-1]) && chars[startIndex-1] != '\'') {
				isBeforeSymbol = true
			}
			if currentIndex >= len(chars) || (isSymbol(chars[currentIndex]) && chars[currentIndex] != '\'') {
				isAfterSymbol = true
			}
			if !isBeforeSymbol || !isAfterSymbol {
				isSubstringValid := false
				localIndex := startIndex - 2
				if isBeforeSymbol {
					localIndex = startIndex
				}
				for !isSubstringValid && localIndex < currentIndex {
					if localIndex >= 0 && (!isSymbol(chars[localIndex]) || chars[localIndex] == '\'') {
						localSub := []rune{}
						localSubIndex := 0
						for localSubIndex < 3 && localIndex+localSubIndex < len(chars) &&
							(!isSymbol(chars[localIndex+localSubIndex]) || chars[localIndex+localSubIndex] == '\'') {
							localSub = append(localSub, chars[localIndex+localSubIndex])
							localSubIndex++
						}
						isSubStringValidCondition := true
						if localSubIndex == 0 {
							isSubStringValidCondition = false
						}
						if localSubIndex < 3 && localIndex-1 >= 0 &&
							(!isSymbol(chars[localIndex-1]) || chars[localIndex-1] == '\'') {
							isSubStringValidCondition = false
						}
						if isSubStringValidCondition && !b.fragments_.isBadFragment(localSub) {
							isSubstringValid = true
						}
					}
					localIndex++
				}
				if !isSubstringValid {
					shouldFilter = false
				}
			}
		} else {
			currentChar := ' '
			if startIndex-1 >= 0 {
				currentChar = chars[startIndex-1]
			}
			nextChar := ' '
			if currentIndex < len(chars) {
				nextChar = chars[currentIndex]
			}
			current := badGetIndex(currentChar)
			next := badGetIndex(nextChar)
			if combos != nil && badComboMatches(current, combos, next) {
				shouldFilter = false
			}
		}
		if !shouldFilter {
			continue
		}
		numeralCount := 0
		alphaCount := 0
		for i := startIndex; i < currentIndex; i++ {
			if isNumerical(chars[i]) {
				numeralCount++
			} else if isAlpha(chars[i]) {
				alphaCount++
			}
		}
		if numeralCount <= alphaCount {
			maskChars(startIndex, currentIndex, chars)
		}
	}
}

// processBadCharacters mirrors WordEncBadWords.ts:113-180.
func (b *badWords) processBadCharacters(chars, bad []rune, startIndex int) (currentIndex, badIndex int, hasSymbol, hasNumber, hasDigit bool) {
	index := startIndex
	badIndex = 0
	count := 0
	for index < len(chars) && !(hasNumber && hasDigit) {
		// TS-faithful: redundant given outer-loop predicate. Preserved for byte-equivalent
		// trace-debug parity with WordEncBadWords.ts:131-134. Do not "clean up".
		if index >= len(chars) || (hasNumber && hasDigit) {
			break
		}
		currentChar := chars[index]
		nextChar := rune('\x00')
		if index+1 < len(chars) {
			nextChar = chars[index+1]
		}

		var currentLength int
		if badIndex < len(bad) {
			currentLength = getEmulatedBadCharLen(nextChar, bad[badIndex], currentChar)
		}
		if badIndex < len(bad) && currentLength > 0 {
			if currentLength == 1 && isNumerical(currentChar) {
				hasNumber = true
			}
			if currentLength == 2 && (isNumerical(currentChar) || isNumerical(nextChar)) {
				hasNumber = true
			}
			index += currentLength
			badIndex++
		} else {
			if badIndex == 0 {
				break
			}
			previousLength := getEmulatedBadCharLen(nextChar, bad[badIndex-1], currentChar)
			if previousLength > 0 {
				index += previousLength
			} else {
				if badIndex >= len(bad) || !isNotLowercaseAlpha(currentChar) {
					break
				}
				if isSymbol(currentChar) && currentChar != '\'' {
					hasSymbol = true
				}
				if isNumerical(currentChar) {
					hasDigit = true
				}
				index++
				count++
				// index > startIndex here (index was incremented above before this check).
				if (count*100)/(index-startIndex) > 90 {
					break
				}
			}
		}
	}
	currentIndex = index
	return
}

// getEmulatedBadCharLen ports the entire TS leetspeak switch
// (WordEncBadWords.ts:182-356).
func getEmulatedBadCharLen(nextChar, badChar, currentChar rune) int {
	if badChar == currentChar {
		return 1
	}
	if badChar >= 'a' && badChar <= 'm' {
		switch badChar {
		case 'a':
			if currentChar != '4' && currentChar != '@' && currentChar != '^' {
				if currentChar == '/' && nextChar == '\\' {
					return 2
				}
				return 0
			}
			return 1
		case 'b':
			if currentChar != '6' && currentChar != '8' {
				if currentChar == '1' && nextChar == '3' {
					return 2
				}
				return 0
			}
			return 1
		case 'c':
			if currentChar != '(' && currentChar != '<' && currentChar != '{' && currentChar != '[' {
				return 0
			}
			return 1
		case 'd':
			if currentChar == '[' && nextChar == ')' {
				return 2
			}
			return 0
		case 'e':
			if currentChar != '3' && currentChar != '€' {
				return 0
			}
			return 1
		case 'f':
			if currentChar == 'p' && nextChar == 'h' {
				return 2
			}
			if currentChar == '£' {
				return 1
			}
			return 0
		case 'g':
			if currentChar != '9' && currentChar != '6' {
				return 0
			}
			return 1
		case 'h':
			if currentChar == '#' {
				return 1
			}
			return 0
		case 'i':
			if currentChar != 'y' && currentChar != 'l' && currentChar != 'j' && currentChar != '1' && currentChar != '!' && currentChar != ':' && currentChar != ';' && currentChar != '|' {
				return 0
			}
			return 1
		case 'j', 'k':
			return 0
		case 'l':
			if currentChar != '1' && currentChar != '|' && currentChar != 'i' {
				return 0
			}
			return 1
		case 'm':
			return 0
		}
	}
	if badChar >= 'n' && badChar <= 'z' {
		switch badChar {
		case 'n':
			return 0
		case 'o':
			if currentChar != '0' && currentChar != '*' {
				if (currentChar != '(' || nextChar != ')') && (currentChar != '[' || nextChar != ']') && (currentChar != '{' || nextChar != '}') && (currentChar != '<' || nextChar != '>') {
					return 0
				}
				return 2
			}
			return 1
		case 'p', 'q', 'r':
			return 0
		case 's':
			if currentChar != '5' && currentChar != 'z' && currentChar != '$' && currentChar != '2' {
				return 0
			}
			return 1
		case 't':
			if currentChar != '7' && currentChar != '+' {
				return 0
			}
			return 1
		case 'u':
			if currentChar == 'v' {
				return 1
			}
			if (currentChar != '\\' || nextChar != '/') && (currentChar != '\\' || nextChar != '|') && (currentChar != '|' || nextChar != '/') {
				return 0
			}
			return 2
		case 'v':
			if (currentChar != '\\' || nextChar != '/') && (currentChar != '\\' || nextChar != '|') && (currentChar != '|' || nextChar != '/') {
				return 0
			}
			return 2
		case 'w':
			if currentChar == 'v' && nextChar == 'v' {
				return 2
			}
			return 0
		case 'x':
			if (currentChar != ')' || nextChar != '(') && (currentChar != '}' || nextChar != '{') && (currentChar != ']' || nextChar != '[') && (currentChar != '>' || nextChar != '<') {
				return 0
			}
			return 2
		case 'y', 'z':
			return 0
		}
	}
	if badChar >= '0' && badChar <= '9' {
		switch badChar {
		case '0':
			if currentChar == 'o' || currentChar == 'O' {
				return 1
			} else if (currentChar != '(' || nextChar != ')') && (currentChar != '{' || nextChar != '}') && (currentChar != '[' || nextChar != ']') {
				return 0
			} else {
				return 2
			}
		case '1':
			if currentChar == 'l' {
				return 1
			}
			return 0
		default:
			return 0
		}
	}
	switch badChar {
	case ',':
		if currentChar == '.' {
			return 1
		}
		return 0
	case '.':
		if currentChar == ',' {
			return 1
		}
		return 0
	case '!':
		if currentChar == 'i' {
			return 1
		}
		return 0
	}
	return 0
}

// badComboMatches binary-searches combos (sorted by [a, b] ascending).
// Mirrors WordEncBadWords.ts:358-373.
func badComboMatches(currentIndex int, combos [][2]int, nextIndex int) bool {
	start, end := 0, len(combos)-1
	for start <= end {
		mid := (start + end) / 2
		if combos[mid][0] == currentIndex && combos[mid][1] == nextIndex {
			return true
		} else if currentIndex < combos[mid][0] || (currentIndex == combos[mid][0] && nextIndex < combos[mid][1]) {
			end = mid - 1
		} else {
			start = mid + 1
		}
	}
	return false
}

// badGetIndex mirrors WordEncBadWords.ts:375-384.
func badGetIndex(c rune) int {
	if isLowercaseAlpha(c) {
		return int(c) + 1 - 'a'
	}
	if c == '\'' {
		return 28
	}
	if isNumerical(c) {
		return int(c) + 29 - '0'
	}
	return 27
}
