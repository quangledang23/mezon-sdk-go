package mezon

import "unicode/utf16"

// UTF16Len returns the number of UTF-16 code units in s, matching the
// semantics of JavaScript's String.length.
//
// This matters for pushing messages: the Mezon web/JS clients and the server
// treat message-content length and mention start/end (s/e) offsets as UTF-16
// code-unit indices (because JS strings are UTF-16). A naive Go port that used
// byte length (len(s)) or rune count (utf8.RuneCountInString) would misplace
// mention highlights and markdown spans for any text containing non-ASCII or
// astral-plane characters (emoji, CJK, etc.). Always measure message offsets
// with the helpers in this file.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2 // surrogate pair
		} else {
			n++
		}
	}
	return n
}

// UTF16Encode returns the UTF-16 code units of s (like JS string indexing).
func UTF16Encode(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

// RuneIndexToUTF16 converts a rune index into s to the corresponding UTF-16
// code-unit offset (the value JS would report for that position).
func RuneIndexToUTF16(s string, runeIdx int) int {
	off := 0
	i := 0
	for _, r := range s {
		if i >= runeIdx {
			break
		}
		if r > 0xFFFF {
			off += 2
		} else {
			off++
		}
		i++
	}
	return off
}

// MentionSpan returns the UTF-16 [start, end) offsets of the first occurrence
// of sub within text, suitable for an ApiMessageMention's S/E fields. ok is
// false when sub is not found. end is exclusive: it is the offset one past the
// last code unit of sub (i.e. start + UTF16Len(sub)), so for "👋 @bob" the span
// of "@bob" is start=3, end=7.
func MentionSpan(text, sub string) (start, end int, ok bool) {
	units := UTF16Encode(text)
	subUnits := UTF16Encode(sub)
	if len(subUnits) == 0 {
		return 0, 0, false
	}
	for i := 0; i+len(subUnits) <= len(units); i++ {
		match := true
		for j := range subUnits {
			if units[i+j] != subUnits[j] {
				match = false
				break
			}
		}
		if match {
			return i, i + len(subUnits), true
		}
	}
	return 0, 0, false
}
