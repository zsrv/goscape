package encfilter

// domains.go mirrors TS WordEncDomains (Engine-TS/src/cache/wordenc/
// WordEncDomains.ts). filter is called by Filter.Filter after badWords.filter.

import "slices"

// domains mirrors TS WordEncDomains (WordEncDomains.ts:4-11). It holds the
// domain list and a back-reference to badWords for filterBadCombinations.
type domains struct {
	bads    *badWords // for filterBadCombinations on ampersat/period copies
	domains [][]rune
}

// filter copies chars twice (ampersat-mask and period-mask), runs
// filterBadCombinations(nil, ampersat, AMPERSAT) and (nil, period, PERIOD),
// then walks domain list backwards calling filterDomain.
// Mirrors WordEncDomains.ts:13-21.
func (d *domains) filter(chars []rune) {
	ampersat := append([]rune(nil), chars...)
	period := append([]rune(nil), chars...)
	d.bads.filterBadCombinations(nil, ampersat, constAmpersat)
	d.bads.filterBadCombinations(nil, period, constPeriod)
	for _, v := range slices.Backward(d.domains) {
		d.filterDomain(period, ampersat, v, chars)
	}
}

// filterDomain scans chars for occurrences of domain and masks them when a
// surrounding ampersat (@) prefix or period/comma suffix is detected.
// Mirrors WordEncDomains.ts:42-58.
func (d *domains) filterDomain(period, ampersat, domain, chars []rune) {
	domainLength := len(domain)
	charsLength := len(chars)
	for index := 0; index <= charsLength-domainLength; index++ {
		matched, currentIndex := d.findMatchingDomain(index, domain, chars)
		if !matched {
			continue
		}
		ampersatStatus := prefixSymbolStatus(index, chars, 3, ampersat, []rune{'@'})
		periodStatus := suffixSymbolStatus(currentIndex-1, chars, 3, period, []rune{'.', ','})
		shouldFilter := ampersatStatus > 2 || periodStatus > 2
		if !shouldFilter {
			continue
		}
		maskChars(index, currentIndex, chars)
	}
}

// findMatchingDomain attempts to match domain starting at startIndex in chars,
// advancing currentIndex as characters are consumed (including leet variants).
// Returns whether the full domain was matched and the final currentIndex.
// Mirrors WordEncDomains.ts:60-88.
func (d *domains) findMatchingDomain(startIndex int, domain, chars []rune) (matched bool, currentIndex int) {
	domainLength := len(domain)
	currentIndex = startIndex
	domainIndex := 0

	for currentIndex < len(chars) && domainIndex < domainLength {
		currentChar := chars[currentIndex]
		nextChar := rune('\x00')
		if currentIndex+1 < len(chars) {
			nextChar = chars[currentIndex+1]
		}
		currentLength := getEmulatedDomainCharLen(nextChar, domain[domainIndex], currentChar)

		if currentLength > 0 {
			currentIndex += currentLength
			domainIndex++
		} else {
			if domainIndex == 0 {
				break
			}
			previousLength := getEmulatedDomainCharLen(nextChar, domain[domainIndex-1], currentChar)
			if previousLength > 0 {
				currentIndex += previousLength
				if domainIndex == 1 {
					startIndex++ // TS:79 dead store preserved for behavior parity (never read after)
				}
			} else {
				if domainIndex >= domainLength || !isSymbol(currentChar) {
					break
				}
				currentIndex++
			}
		}
	}
	matched = domainIndex >= domainLength
	return
}

// getEmulatedDomainCharLen returns the number of chars in currentChar (1 or 2)
// that match domainChar via leet-speak substitution, or 0 if no match.
// Smaller switch than the bad-words one — only handles o/c/e/s/l common
// substitutions. Mirrors WordEncDomains.ts:23-40.
func getEmulatedDomainCharLen(nextChar, domainChar, currentChar rune) int {
	if domainChar == currentChar {
		return 1
	}
	if domainChar == 'o' && currentChar == '0' {
		return 1
	}
	if domainChar == 'o' && currentChar == '(' && nextChar == ')' {
		return 2
	}
	if domainChar == 'c' && (currentChar == '(' || currentChar == '<' || currentChar == '[') {
		return 1
	}
	if domainChar == 'e' && currentChar == '€' {
		return 1
	}
	if domainChar == 's' && currentChar == '$' {
		return 1
	}
	if domainChar == 'l' && currentChar == 'i' {
		return 1
	}
	return 0
}
