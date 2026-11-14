package adocgo

import (
	"strings"
	"unicode"
)

// Scanners for inline constructs. Each one replaces the regular expression
// quoted in its declaration and reproduces its leftmost-match and capture
// semantics, including the order in which alternatives are tried.

// findClose returns the offset k of the first ']' at or after start+1 that
// is not preceded by a backslash, i.e. the end of `.*?[^\\]` followed by
// `\]`. Unless dotall is set the `.*?` part must not cross a newline (the
// final character may still be one). It returns -1 if there is none.
func findClose(s string, start int, dotall bool) int {
	return findUnescaped(s, start, ']', dotall)
}

// findUnescaped is findClose for an arbitrary closing byte.
func findUnescaped(s string, start int, closer byte, dotall bool) int {
	limit := len(s)
	if !dotall {
		if nl := strings.IndexByte(s[start:], '\n'); nl >= 0 {
			// the lazy part may extend up to the newline, which can then
			// only be matched by the final [^\\]
			limit = start + nl + 2
			if limit > len(s) {
				limit = len(s)
			}
		}
	}
	for k := start + 1; k < limit; k++ {
		j := strings.IndexByte(s[k:limit], closer)
		if j < 0 {
			return -1
		}
		k += j
		if s[k-1] != '\\' {
			return k
		}
	}
	return -1
}

// escStart returns i-1 if s[i-1] is a backslash at or after from, else i.
func escStart(s string, i, from int) int {
	if i > from && s[i-1] == '\\' {
		return i - 1
	}
	return i
}

// escLiteralScanner builds a scanner for `\\?LIT`.
func escLiteralScanner(pat, lit string) *scanner {
	return newScanner(pat, "", 0, func(s string, from int, c []int) bool {
		j := strings.Index(s[from:], lit)
		if j < 0 {
			return false
		}
		k := from + j
		set2(c, 0, escStart(s, k, from), k+len(lit))
		return true
	})
}

var authorDelimiterRx = newScanner(`;(?: |$)`, "", 0,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.IndexByte(s[i:], ';')
			if j < 0 {
				return false
			}
			i += j
			if i+1 < len(s) && s[i+1] == ' ' {
				set2(c, 0, i, i+2)
				return true
			}
			if isEOL(s, i+1) {
				set2(c, 0, i, i+1)
				return true
			}
		}
		return false
	})

var tagDirectiveRx = newScanner(`\b(?:tag|(e)nd)::(\S+?)\[\](?=$|[ \r])`, "m", 2,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], "::")
			if j < 0 {
				return false
			}
			i += j
			st := i - 3
			if st < from || (s[st:i] != "tag" && s[st:i] != "end") || isWordBefore(s, st) {
				continue
			}
			t := i + 2
			for e := t + 1; e < len(s) && !isSpaceByte(s[e-1]); e++ {
				if s[e] == '[' && e+1 < len(s) && s[e+1] == ']' && (isEOL(s, e+2) || s[e+2] == ' ' || s[e+2] == '\r') {
					set2(c, 0, st, e+2)
					if s[st] == 'e' {
						set2(c, 1, st, st+1)
					}
					set2(c, 2, t, e)
					return true
				}
			}
		}
		return false
	})

var invalidAttributeNameCharsRx = newScanner(`[^\p{Word}-]`, "", 0,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); {
			if s[i] == '-' {
				i++
				continue
			}
			ok, sz := wordAt(s, i)
			if !ok && validStart(s, i) {
				set2(c, 0, i, i+sz)
				return true
			}
			i += sz
		}
		return false
	})

var attributeReferenceRx = newScanner(`(\\)?\{(\p{Word}[\p{Word}-]*|(set|counter2?):.+?)(\\)?\}`, "", 4,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.IndexByte(s[i:], '{')
			if j < 0 {
				return false
			}
			i += j
			p := i + 1
			st := escStart(s, i, from)
			// first alternative: a plain attribute name
			if ok, sz := wordAt(s, p); ok {
				r := p + sz
				for r < len(s) {
					if s[r] == '-' {
						r++
					} else if ok, sz := wordAt(s, r); ok {
						r += sz
					} else {
						break
					}
				}
				g4 := -1
				e := r
				if e+1 < len(s) && s[e] == '\\' && s[e+1] == '}' {
					g4 = e
					e++
				}
				if e < len(s) && s[e] == '}' {
					set2(c, 0, st, e+1)
					if st < i {
						set2(c, 1, st, i)
					}
					set2(c, 2, p, r)
					if g4 >= 0 {
						set2(c, 4, g4, g4+1)
					}
					return true
				}
			}
			// second alternative: set:, counter: or counter2: directive
			rest := s[p:]
			n := 0
			switch {
			case strings.HasPrefix(rest, "set:"):
				n = 3
			case strings.HasPrefix(rest, "counter2:"):
				n = 8
			case strings.HasPrefix(rest, "counter:"):
				n = 7
			}
			if n == 0 {
				continue
			}
			le := lineEnd(s, p)
			for e := p + n + 2; e < le; e++ {
				if s[e] == '\\' && e+1 < len(s) && s[e+1] == '}' {
					set2(c, 0, st, e+2)
					set2(c, 4, e, e+1)
				} else if s[e] == '}' {
					set2(c, 0, st, e+1)
				} else {
					continue
				}
				if st < i {
					set2(c, 1, st, i)
				}
				set2(c, 2, p, e)
				set2(c, 3, p, p+n)
				return true
			}
		}
		return false
	})

// scanEntity matches the entity alternatives used by the replacement and
// section id patterns at p (after the ampersand) and returns the offset
// after the terminating semicolon, or -1. If lower is set only lower-case
// letters and hex digits are accepted.
func scanEntity(s string, p int, lower bool) int {
	if p >= len(s) {
		return -1
	}
	isLetter := func(ch byte) bool {
		if lower {
			return ch >= 'a' && ch <= 'z'
		}
		return isASCIILetter(ch)
	}
	e := p
	if s[p] != '#' {
		for e < len(s) && isLetter(s[e]) {
			e++
		}
		if e-p < 2 {
			return -1
		}
		d := skipDigits(s, e)
		if d-e > 2 {
			return -1
		}
		e = d
	} else if p+1 < len(s) && s[p+1] == 'x' {
		e = p + 2
		for e < len(s) && (isDigit(s[e]) || (s[e] >= 'a' && s[e] <= 'f') || (!lower && s[e] >= 'A' && s[e] <= 'F')) {
			e++
		}
		if n := e - (p + 2); n < 2 || n > 5 {
			return -1
		}
	} else {
		e = skipDigits(s, p+1)
		if n := e - (p + 1); n < 2 || n > 6 {
			return -1
		}
	}
	if e < len(s) && s[e] == ';' {
		return e + 1
	}
	return -1
}

var invalidSectionIDCharsRx = newScanner(`<[^>]+>|&(?:[a-z][a-z]+\d{0,2}|#\d\d\d{0,4}|#x[\da-f][\da-f][\da-f]{0,3});|[^ \p{Word}\-.]+?`, "", 0,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); {
			ch := s[i]
			if ch == ' ' || ch == '-' || ch == '.' {
				i++
				continue
			}
			ok, sz := wordAt(s, i)
			if ok || !validStart(s, i) {
				i += sz
				continue
			}
			if ch == '<' {
				if j := strings.IndexByte(s[i+1:], '>'); j > 0 {
					set2(c, 0, i, i+1+j+1)
					return true
				}
			} else if ch == '&' {
				if e := scanEntity(s, i+1, true); e >= 0 {
					set2(c, 0, i, e)
					return true
				}
			}
			set2(c, 0, i, i+sz)
			return true
		}
		return false
	})

var hardLineBreakRx = bolScanner(`^(.*) \+$`, "", 1,
	func(s string, i int, c []int) bool {
		le := lineEnd(s, i)
		if le-2 < i || s[le-2] != ' ' || s[le-1] != '+' {
			return false
		}
		set2(c, 0, i, le)
		set2(c, 1, i, le-2)
		return true
	})

var blankLineRx = newScanner(`\n{2,}`, "", 0,
	func(s string, from int, c []int) bool {
		j := strings.Index(s[from:], "\n\n")
		if j < 0 {
			return false
		}
		i := from + j
		e := i + 2
		for e < len(s) && s[e] == '\n' {
			e++
		}
		set2(c, 0, i, e)
		return true
	})

var replaceableTextRx = newScanner(`[&']|--|\.\.\.|\([CRT]M?\)`, "", 0,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			switch s[i] {
			case '&', '\'':
				set2(c, 0, i, i+1)
				return true
			case '-':
				if i+1 < len(s) && s[i+1] == '-' {
					set2(c, 0, i, i+2)
					return true
				}
			case '.':
				if i+2 < len(s) && s[i+1] == '.' && s[i+2] == '.' {
					set2(c, 0, i, i+3)
					return true
				}
			case '(':
				if i+2 < len(s) && (s[i+1] == 'C' || s[i+1] == 'R' || s[i+1] == 'T') {
					e := i + 2
					if s[e] == 'M' {
						e++
					}
					if e < len(s) && s[e] == ')' {
						set2(c, 0, i, e+1)
						return true
					}
				}
			}
		}
		return false
	})

var xmlSanitizeRx = newScanner(`<[^>]+>`, "", 0,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				return false
			}
			i += j
			if k := strings.IndexByte(s[i+1:], '>'); k > 0 {
				set2(c, 0, i, i+1+k+1)
				return true
			} else if k < 0 {
				return false
			}
		}
		return false
	})

var passSlotRx = newScanner("\u0096(\\d+)\u0097", "", 1,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], passStart)
			if j < 0 {
				return false
			}
			i += j
			p := i + len(passStart)
			e := skipDigits(s, p)
			if e > p && strings.HasPrefix(s[e:], passEnd) {
				set2(c, 0, i, e+len(passEnd))
				set2(c, 1, p, e)
				return true
			}
		}
		return false
	})

// ---------------------------------------------------------------------------
// Replacements

func isBlankRune(r rune) bool {
	return r == '\t' || r == ' ' || (r >= 0x80 && unicode.Is(unicode.Zs, r))
}

var replacements = []replacement{
	{escLiteralScanner(`\\?\(C\)`, "(C)"), "&#169;", "none"},
	{escLiteralScanner(`\\?\(R\)`, "(R)"), "&#174;", "none"},
	{escLiteralScanner(`\\?\(TM\)`, "(TM)"), "&#8482;", "none"},
	{newScanner(`(?: |\n|^|\\)--(?: |\n|$)`, "", 0, func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], "--")
			if j < 0 {
				return false
			}
			i += j
			st := -1
			if i > from && (s[i-1] == ' ' || s[i-1] == '\n' || s[i-1] == '\\') {
				st = i - 1
			} else if isBOL(s, i) {
				st = i
			} else {
				continue
			}
			e := i + 2
			if e < len(s) && (s[e] == ' ' || s[e] == '\n') {
				e++
			} else if e != len(s) {
				continue
			}
			set2(c, 0, st, e)
			return true
		}
		return false
	}), "&#8201;&#8212;&#8201;", "none"},
	{newScanner(`(\p{Word})\\?--(?=\p{Word})`, "", 1, func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], "--")
			if j < 0 {
				return false
			}
			i += j
			if !isWordAt(s, i+2) {
				continue
			}
			p := i
			if p > from && s[p-1] == '\\' {
				p--
			}
			if p <= from {
				continue
			}
			r, sz := runeBefore(s, p)
			if !isWordRune(r) || p-sz < from {
				continue
			}
			set2(c, 0, p-sz, i+2)
			set2(c, 1, p-sz, p)
			return true
		}
		return false
	}), "&#8212;&#8203;", "leading"},
	{escLiteralScanner(`\\?\.\.\.`, "..."), "&#8230;&#8203;", "none"},
	{escLiteralScanner("\\\\?`'", "`'"), "&#8217;", "none"},
	{newScanner(`(\p{Alnum})\\?'(?=\p{Alpha})`, "", 1, func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.IndexByte(s[i:], '\'')
			if j < 0 {
				return false
			}
			i += j
			if ok, _ := alphaAt(s, i+1); !ok {
				continue
			}
			p := i
			if p > from && s[p-1] == '\\' {
				p--
			}
			if p <= from {
				continue
			}
			r, sz := runeBefore(s, p)
			if !isAlnumRune(r) || p-sz < from {
				continue
			}
			set2(c, 0, p-sz, i+1)
			set2(c, 1, p-sz, p)
			return true
		}
		return false
	}), "&#8217;", "leading"},
	{escLiteralScanner(`\\?-&gt;`, "-&gt;"), "&#8594;", "none"},
	{escLiteralScanner(`\\?=&gt;`, "=&gt;"), "&#8658;", "none"},
	{escLiteralScanner(`\\?&lt;-`, "&lt;-"), "&#8592;", "none"},
	{escLiteralScanner(`\\?&lt;=`, "&lt;="), "&#8656;", "none"},
	{newScanner(`\\?(&)amp;((?:[a-zA-Z][a-zA-Z]+\d{0,2}|#\d\d\d{0,4}|#x[\da-fA-F][\da-fA-F][\da-fA-F]{0,3});)`, "", 2, func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], "&amp;")
			if j < 0 {
				return false
			}
			i += j
			if e := scanEntity(s, i+5, false); e >= 0 {
				set2(c, 0, escStart(s, i, from), e)
				set2(c, 1, i, i+1)
				set2(c, 2, i+5, e)
				return true
			}
		}
		return false
	}), "", "bounding"},
}

// ---------------------------------------------------------------------------
// Quoted text

// scanAttrs matches (?:\[([^\]]+)\])? at p for the quoted-text patterns. If
// s[p] is '[' the attribute list is mandatory (the patterns cannot match a
// bare bracket), so ok reports whether matching can continue at q.
func scanAttrs(s string, p int) (q, as, ae int, ok bool) {
	if p >= len(s) || s[p] != '[' {
		return p, -1, -1, true
	}
	j := strings.IndexByte(s[p+1:], ']')
	if j <= 0 {
		return 0, -1, -1, false
	}
	return p + 1 + j + 1, p + 1, p + 1 + j, true
}

// unconstrainedScanner builds the scanner for
// \\?(?:\[([^\]]+)\])?MARK(.+?)MARK (dot matches newlines), or, when
// nonSpace is set, \\?(?:\[([^\]]+)\])?MARK(\S+?)MARK.
func unconstrainedScanner(pat, flags, mark string, nonSpace bool) *scanner {
	m0 := mark[0]
	return newScanner(pat, flags, 2, func(s string, from int, c []int) bool {
		first := strings.Index(s[from:], mark)
		if first < 0 {
			return false
		}
		last := strings.LastIndex(s, mark)
		for i := from; i <= last; i++ {
			ch := s[i]
			if ch != m0 && ch != '[' && ch != '\\' {
				continue
			}
			p := i
			if ch == '\\' {
				p++
			}
			q, as, ae, ok := scanAttrs(s, p)
			if !ok || !strings.HasPrefix(s[q:], mark) {
				continue
			}
			cs := q + len(mark)
			if cs >= len(s) {
				continue
			}
			_, sz := runeAt(s, cs)
			var e int
			if nonSpace {
				if isSpaceByte(s[cs]) {
					continue
				}
				e = -1
				for k := cs + sz; k < len(s) && !isSpaceByte(s[k-1]); k++ {
					if s[k] == m0 {
						e = k
						break
					}
				}
			} else {
				j := strings.Index(s[cs+sz:], mark)
				if j < 0 {
					e = -1
				} else {
					e = cs + sz + j
				}
			}
			if e < 0 {
				continue
			}
			set2(c, 0, i, e+len(mark))
			set2(c, 1, as, ae)
			set2(c, 2, cs, e)
			return true
		}
		return false
	})
}

// scanConstrainedBody matches (\S|\S.*?\S)CLOSE(?!LOOKAHEAD) at cs and
// returns the offset of the closing mark, or -1. The lookahead rejects a
// \p{Word} character and any of the bytes in laExcl.
func scanConstrainedBody(s string, cs int, closer, laExcl string) int {
	if cs >= len(s) || isSpaceByte(s[cs]) {
		return -1
	}
	_, sz := runeAt(s, cs)
	c0 := closer[0]
	for q := cs + sz; q < len(s); q++ {
		j := strings.IndexByte(s[q:], c0)
		if j < 0 {
			return -1
		}
		q += j
		if isSpaceByte(s[q-1]) || !strings.HasPrefix(s[q:], closer) {
			continue
		}
		after := q + len(closer)
		if after < len(s) {
			if isWordAt(s, after) || (laExcl != "" && strings.IndexByte(laExcl, s[after]) >= 0) {
				continue
			}
		}
		return q
	}
	return -1
}

// constrainedScanner builds the scanner for
// (^|[^\p{Word}EXCL])(?:\[([^\]]+)\])?OPEN(\S|\S.*?\S)CLOSE(?![\p{Word}LAEXCL]).
func constrainedScanner(pat, open, closer, excl, laExcl string) *scanner {
	o0 := open[0]
	try := func(s string, i, p int, c []int) bool {
		q, as, ae, ok := scanAttrs(s, p)
		if !ok || !strings.HasPrefix(s[q:], open) {
			return false
		}
		cs := q + len(open)
		e := scanConstrainedBody(s, cs, closer, laExcl)
		if e < 0 {
			return false
		}
		set2(c, 0, i, e+len(closer))
		set2(c, 1, i, p)
		set2(c, 2, as, ae)
		set2(c, 3, cs, e)
		return true
	}
	return newScanner(pat, "m", 3, func(s string, from int, c []int) bool {
		if strings.Index(s[from:], open) < 0 {
			return false
		}
		for j := from; j < len(s); j++ {
			ch := s[j]
			if ch != o0 && ch != '[' {
				continue
			}
			// a match starting at the preceding character
			if j > from {
				r, sz := runeBefore(s, j)
				if i := j - sz; i >= from && validStart(s, i) && !isWordRune(r) && (r >= 0x80 || strings.IndexByte(excl, byte(r)) < 0) {
					if try(s, i, j, c) {
						return true
					}
				}
			}
			// a match starting here, at the beginning of a line
			if isBOL(s, j) && try(s, j, j, c) {
				return true
			}
		}
		return false
	})
}

func init() {
	quoteSubs = []quoteSub{
		{"strong", false, unconstrainedScanner(`\\?(?:\[([^\]]+)\])?\*\*(.+?)\*\*`, "m", "**", false)},
		{"strong", true, constrainedScanner(`(^|[^\p{Word};:}])(?:\[([^\]]+)\])?\*(\S|\S.*?\S)\*(?!\p{Word})`, "*", "*", ";:}", "")},
		{"double", true, constrainedScanner("(^|[^\\p{Word};:}])(?:\\[([^\\]]+)\\])?\"`(\\S|\\S.*?\\S)`\"(?!\\p{Word})", "\"`", "`\"", ";:}", "")},
		{"single", true, constrainedScanner("(^|[^\\p{Word};:`}])(?:\\[([^\\]]+)\\])?'`(\\S|\\S.*?\\S)`'(?!\\p{Word})", "'`", "`'", ";:`}", "")},
		{"monospaced", false, unconstrainedScanner("\\\\?(?:\\[([^\\]]+)\\])?``(.+?)``", "m", "``", false)},
		{"monospaced", true, constrainedScanner("(^|[^\\p{Word};:\"'`}])(?:\\[([^\\]]+)\\])?`(\\S|\\S.*?\\S)`(?![\\p{Word}\"'`])", "`", "`", ";:\"'`}", "\"'`")},
		{"emphasis", false, unconstrainedScanner(`\\?(?:\[([^\]]+)\])?__(.+?)__`, "m", "__", false)},
		{"emphasis", true, constrainedScanner(`(^|[^\p{Word};:}])(?:\[([^\]]+)\])?_(\S|\S.*?\S)_(?!\p{Word})`, "_", "_", ";:}", "")},
		{"mark", false, unconstrainedScanner(`\\?(?:\[([^\]]+)\])?##(.+?)##`, "m", "##", false)},
		{"mark", true, constrainedScanner(`(^|[^\p{Word}&;:}])(?:\[([^\]]+)\])?#(\S|\S.*?\S)#(?!\p{Word})`, "#", "#", "&;:}", "")},
		{"superscript", false, unconstrainedScanner(`\\?(?:\[([^\]]+)\])?\^(\S+?)\^`, "", "^", true)},
		{"subscript", false, unconstrainedScanner(`\\?(?:\[([^\]]+)\])?~(\S+?)~`, "", "~", true)},
	}
	c := append([]quoteSub(nil), quoteSubs...)
	c[2] = quoteSub{"double", true, constrainedScanner("(^|[^\\p{Word};:}])(?:\\[([^\\]]+)\\])?``(\\S|\\S.*?\\S)''(?!\\p{Word})", "``", "''", ";:}", "")}
	c[3] = quoteSub{"single", true, constrainedScanner("(^|[^\\p{Word};:}])(?:\\[([^\\]]+)\\])?`(\\S|\\S.*?\\S)'(?!\\p{Word})", "`", "'", ";:}", "")}
	c[4] = quoteSub{"monospaced", false, unconstrainedScanner(`\\?(?:\[([^\]]+)\])?\+\+(.+?)\+\+`, "m", "++", false)}
	c[5] = quoteSub{"monospaced", true, constrainedScanner(`(^|[^\p{Word};:}])(?:\[([^\]]+)\])?\+(\S|\S.*?\S)\+(?!\p{Word})`, "+", "+", ";:}", "")}
	emph := quoteSub{"emphasis", true, constrainedScanner(`(^|[^\p{Word};:}])(?:\[([^\]]+)\])?'(\S|\S.*?\S)'(?!\p{Word})`, "'", "'", ";:}", "")}
	quoteSubsCompat = append(append(append([]quoteSub(nil), c[:3]...), emph), c[3:]...)
}
