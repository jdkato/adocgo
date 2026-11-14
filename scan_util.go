package adocgo

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Character classes with the same definitions the regular expressions use:
// \p{Word}, \p{Alpha}, \p{Alnum} and \p{Blank} are Unicode-aware, whereas
// \w, \d and \s are ASCII-only (as in Ruby).

func isWordRune(r rune) bool {
	if r < 128 {
		return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.Is(unicode.Nd, r) || unicode.Is(unicode.Pc, r)
}

func isAlphaRune(r rune) bool {
	if r < 128 {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Other_Alphabetic, r)
}

func isAlnumRune(r rune) bool {
	if r < 128 {
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return isAlphaRune(r) || unicode.Is(unicode.Nd, r)
}

var asciiWord, asciiAlnum, asciiSpace [256]bool

func init() {
	for c := 0; c < 128; c++ {
		asciiWord[c] = isWordRune(rune(c))
		asciiAlnum[c] = isAlnumRune(rune(c))
	}
	for _, c := range " \t\r\n\f\v" {
		asciiSpace[c] = true
	}
}

// runeAt decodes the character at s[i:] (i < len(s)).
func runeAt(s string, i int) (rune, int) {
	if c := s[i]; c < utf8.RuneSelf {
		return rune(c), 1
	}
	return utf8.DecodeRuneInString(s[i:])
}

// runeBefore decodes the character ending at s[:i] (i > 0).
func runeBefore(s string, i int) (rune, int) {
	if c := s[i-1]; c < utf8.RuneSelf {
		return rune(c), 1
	}
	return utf8.DecodeLastRuneInString(s[:i])
}

// wordAt reports whether a \p{Word} character starts at s[i:] and its size.
func wordAt(s string, i int) (bool, int) {
	if i >= len(s) {
		return false, 0
	}
	if c := s[i]; c < utf8.RuneSelf {
		return asciiWord[c], 1
	}
	r, sz := utf8.DecodeRuneInString(s[i:])
	return isWordRune(r), sz
}

func isWordAt(s string, i int) bool {
	ok, _ := wordAt(s, i)
	return ok
}

func alphaAt(s string, i int) (bool, int) {
	if i >= len(s) {
		return false, 0
	}
	r, sz := runeAt(s, i)
	return isAlphaRune(r), sz
}

func alnumAt(s string, i int) (bool, int) {
	if i >= len(s) {
		return false, 0
	}
	if c := s[i]; c < utf8.RuneSelf {
		return asciiAlnum[c], 1
	}
	r, sz := utf8.DecodeRuneInString(s[i:])
	return isAlnumRune(r), sz
}

func isWordBefore(s string, i int) bool {
	if i <= 0 {
		return false
	}
	r, _ := runeBefore(s, i)
	return isWordRune(r)
}

// isSpaceByte reports whether c matches \s.
func isSpaceByte(c byte) bool { return asciiSpace[c] }

func isBlankByte(c byte) bool { return c == ' ' || c == '\t' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isBOL reports whether ^ matches at i. Like Onigmo, ^ does not match at the
// very end of the text after a trailing newline.
func isBOL(s string, i int) bool { return i == 0 || (s[i-1] == '\n' && i < len(s)) }

// isEOL reports whether $ matches at i.
func isEOL(s string, i int) bool { return i == len(s) || s[i] == '\n' }

// lineEnd returns the offset of the end of the line containing offset i.
func lineEnd(s string, i int) int {
	if j := strings.IndexByte(s[i:], '\n'); j >= 0 {
		return i + j
	}
	return len(s)
}

// nextBOL returns the first offset >= from at which ^ matches, or -1.
func nextBOL(s string, from int) int {
	if from > len(s) {
		return -1
	}
	if isBOL(s, from) {
		return from
	}
	j := strings.IndexByte(s[from:], '\n')
	if j < 0 || from+j+1 >= len(s) {
		return -1
	}
	return from + j + 1
}

func skipBlanks(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func skipSpaces(s string, i int) int {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

func skipDigits(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}

// bolScanner declares a scanner for a pattern that starts with ^: try is
// attempted at every line start and must not modify c unless it succeeds.
func bolScanner(pat, flags string, n int, try func(s string, i int, c []int) bool) *scanner {
	return newScanner(pat, flags, n, func(s string, from int, c []int) bool {
		for i := nextBOL(s, from); i >= 0; i = nextBOL(s, i+1) {
			if !validStart(s, i) {
				continue
			}
			if try(s, i, c) {
				return true
			}
		}
		return false
	})
}

// scanID matches [\p{Alpha}_:][\p{Word}\-:.]* at i and returns the end
// offset, or -1.
func scanID(s string, i int) int {
	if i >= len(s) {
		return -1
	}
	if c := s[i]; c != '_' && c != ':' {
		ok, sz := alphaAt(s, i)
		if !ok {
			return -1
		}
		i += sz
	} else {
		i++
	}
	for i < len(s) {
		c := s[i]
		if c == '-' || c == ':' || c == '.' {
			i++
			continue
		}
		ok, sz := wordAt(s, i)
		if !ok {
			break
		}
		i += sz
	}
	return i
}

func set2(c []int, g, a, b int) { c[2*g], c[2*g+1] = a, b }

// validStart reports whether a match may start at offset i: like the
// regular expression engine, scanners never start a match in the middle of
// a UTF-8 sequence (which only matters for malformed input).
func validStart(s string, i int) bool { return i == 0 || i >= len(s) || s[i]&0xC0 != 0x80 }
