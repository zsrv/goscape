package encfilter

// tlds.go mirrors TS WordEncTlds (Engine-TS/src/cache/wordenc/WordEncTlds.ts).

// tlds mirrors TS WordEncTlds (WordEncTlds.ts:5-15). It holds the TLD list,
// parallel type array, and back-references to badWords and domains for
// filterBadCombinations and getEmulatedDomainCharLen.
type tlds struct {
	bads    *badWords
	domains *domains // TS-faithful dead back-ref: WordEncTlds.ts:7,15 stores wordEncDomains but never reads it; preserved for parity.
	tlds    [][]rune
	types   []int
}

// filter copies chars twice (period-mask and slash-mask), runs
// filterBadCombinations against PERIOD and SLASH, then walks the TLD list
// calling filterTld. Mirrors WordEncTlds.ts:17-25.
func (t *tlds) filter(chars []rune) {
	period := append([]rune(nil), chars...)
	slash := append([]rune(nil), chars...)
	t.bads.filterBadCombinations(nil, period, constPeriod)
	t.bads.filterBadCombinations(nil, slash, constSlash)
	for i := range len(t.tlds) {
		t.filterTld(slash, t.types[i], chars, t.tlds[i], period)
	}
}

// filterTld scans chars for occurrences of tld and masks them (plus surrounding
// domain/path context) when prefix/suffix symbol status meets the tldType
// threshold. Mirrors WordEncTlds.ts:27-113.
func (t *tlds) filterTld(slash []rune, tldType int, chars []rune, tld []rune, period []rune) {
	if len(tld) > len(chars) {
		return
	}
	for index := 0; index <= len(chars)-len(tld); index++ {
		currentIndex, tldIndex := t.processTlds(chars, tld, index)
		if tldIndex < len(tld) {
			continue
		}
		shouldFilter := false
		periodFilterStatus := prefixSymbolStatus(index, chars, 3, period, []rune{',', '.'})
		slashFilterStatus := suffixSymbolStatus(currentIndex-1, chars, 5, slash, []rune{'\\', '/'})
		if tldType == 1 && periodFilterStatus > 0 && slashFilterStatus > 0 {
			shouldFilter = true
		}
		if tldType == 2 && ((periodFilterStatus > 2 && slashFilterStatus > 0) || (periodFilterStatus > 0 && slashFilterStatus > 2)) {
			shouldFilter = true
		}
		if tldType == 3 && periodFilterStatus > 0 && slashFilterStatus > 2 {
			shouldFilter = true
		}
		if !shouldFilter {
			continue
		}
		startFilterIndex := index
		endFilterIndex := currentIndex - 1
		if periodFilterStatus > 2 {
			if periodFilterStatus == 4 {
				foundPeriod := false
				for pi := index - 1; pi >= 0; pi-- {
					if foundPeriod {
						if period[pi] != '*' {
							break
						}
						startFilterIndex = pi
					} else if period[pi] == '*' {
						startFilterIndex = pi
						foundPeriod = true
					}
				}
			}
			foundPeriod := false
			for pi := startFilterIndex - 1; pi >= 0; pi-- {
				if foundPeriod {
					if isSymbol(chars[pi]) {
						break
					}
					startFilterIndex = pi
				} else if !isSymbol(chars[pi]) {
					foundPeriod = true
					startFilterIndex = pi
				}
			}
		}
		if slashFilterStatus > 2 {
			if slashFilterStatus == 4 {
				foundPeriod := false
				for pi := endFilterIndex + 1; pi < len(chars); pi++ {
					if foundPeriod {
						if slash[pi] != '*' {
							break
						}
						endFilterIndex = pi
					} else if slash[pi] == '*' {
						endFilterIndex = pi
						foundPeriod = true
					}
				}
			}
			foundPeriod := false
			for pi := endFilterIndex + 1; pi < len(chars); pi++ {
				if foundPeriod {
					if isSymbol(chars[pi]) {
						break
					}
					endFilterIndex = pi
				} else if !isSymbol(chars[pi]) {
					foundPeriod = true
					endFilterIndex = pi
				}
			}
		}
		maskChars(startFilterIndex, endFilterIndex+1, chars)
	}
}

// processTlds attempts to match tld in chars starting at startIndex, consuming
// leet-speak variants via getEmulatedDomainCharLen. Returns the final
// currentIndex (one past the last consumed char) and the number of tld
// characters matched (tldIndex). Mirrors WordEncTlds.ts:115-141.
func (t *tlds) processTlds(chars, tld []rune, startIndex int) (currentIndex, tldIndex int) {
	currentIndex = startIndex
	for currentIndex < len(chars) && tldIndex < len(tld) {
		currentChar := chars[currentIndex]
		nextChar := rune('\x00')
		if currentIndex+1 < len(chars) {
			nextChar = chars[currentIndex+1]
		}
		currentLength := getEmulatedDomainCharLen(nextChar, tld[tldIndex], currentChar)
		if currentLength > 0 {
			currentIndex += currentLength
			tldIndex++
		} else {
			if tldIndex == 0 {
				break
			}
			previousLength := getEmulatedDomainCharLen(nextChar, tld[tldIndex-1], currentChar)
			if previousLength > 0 {
				currentIndex += previousLength
			} else {
				if !isSymbol(currentChar) {
					break
				}
				currentIndex++
			}
		}
	}
	return
}
