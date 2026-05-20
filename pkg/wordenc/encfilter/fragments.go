package encfilter

// fragments mirrors TS WordEncFragments (Engine-TS/src/cache/wordenc/
// WordEncFragments.ts). items is the sorted []uint16 of encoded fragment
// values; isBadFragment does binary search; filter masks long digit runs.

type fragments struct {
	items []uint16
}

// filter mirrors WordEncFragments.filter (WordEncFragments.ts:6-48). Detects
// runs of 4+ digits adjacent to "non-common" characters and masks them.
// NOTE: due to the local startIndex resetting each outer iteration the mask
// condition (startIndex == 4) can only trigger when this is called as part of
// the broader filter pipeline (after badWords/domains have already masked
// surrounding chars).
func (f *fragments) filter(chars []rune) {
	for currentIndex := 0; currentIndex < len(chars); {
		numberIndex := f.indexOfNumber(chars, currentIndex)
		if numberIndex == -1 {
			return
		}

		isSymbolOrNotLowercaseAlpha := false
		for i := currentIndex; i >= 0 && i < numberIndex && !isSymbolOrNotLowercaseAlpha; i++ {
			if !isSymbol(chars[i]) && !isNotLowercaseAlpha(chars[i]) {
				isSymbolOrNotLowercaseAlpha = true
			}
		}

		startIndex := 0

		if isSymbolOrNotLowercaseAlpha {
			startIndex = 0
		}

		if startIndex == 0 {
			startIndex = 1
			currentIndex = numberIndex
		}

		value := 0
		for i := numberIndex; i < len(chars) && i < currentIndex; i++ {
			value = value*10 + int(chars[i]-'0')
		}

		if value <= 255 && currentIndex-numberIndex <= 8 {
			startIndex++
		} else {
			startIndex = 0
		}

		if startIndex == 4 {
			maskChars(numberIndex, currentIndex, chars)
			startIndex = 0 //nolint:ineffassign
		}
		currentIndex = f.indexOfNonNumber(currentIndex, chars)
	}
}

// isBadFragment mirrors WordEncFragments.isBadFragment (WordEncFragments.ts:50-77).
// All-numerical chars always return true (TS short-circuit). Otherwise the
// encoded value is binary-searched in items.
func (f *fragments) isBadFragment(chars []rune) bool {
	if isNumericalChars(chars) {
		return true
	}
	value := uint16(getFragmentInteger(chars))
	items := f.items
	if len(items) == 0 {
		return false
	}
	if value == items[0] || value == items[len(items)-1] {
		return true
	}
	start, end := 0, len(items)-1
	for start <= end {
		mid := (start + end) / 2
		if value == items[mid] {
			return true
		} else if value < items[mid] {
			end = mid - 1
		} else {
			start = mid + 1
		}
	}
	return false
}

// getFragmentInteger mirrors WordEncFragments.getInteger (WordEncFragments.ts:79-97).
// Walks chars BACKWARDS and accumulates a base-38 value. Returns 0 for len > 6
// or for non-alpha/non-digit/non-apostrophe content (NUL contributes nothing
// but does not abort — it advances the iteration with no value added).
func getFragmentInteger(chars []rune) int {
	if len(chars) > 6 {
		return 0
	}
	value := 0
	for i := range len(chars) {
		c := chars[len(chars)-i-1]
		switch {
		case isLowercaseAlpha(c):
			value = value*38 + int(c) + 1 - 'a'
		case c == '\'':
			value = value*38 + 27
		case isNumerical(c):
			value = value*38 + int(c) + 28 - '0'
		case c != '\x00':
			return 0
		}
	}
	return value
}

// indexOfNumber mirrors WordEncFragments.ts:99-106.
func (f *fragments) indexOfNumber(chars []rune, offset int) int {
	for i := offset; i < len(chars) && i >= 0; i++ {
		if isNumerical(chars[i]) {
			return i
		}
	}
	return -1
}

// indexOfNonNumber mirrors WordEncFragments.ts:108-115.
func (f *fragments) indexOfNonNumber(offset int, chars []rune) int {
	for i := offset; i < len(chars) && i >= 0; i++ {
		if !isNumerical(chars[i]) {
			return i
		}
	}
	return len(chars)
}
