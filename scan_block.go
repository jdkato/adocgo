package adocgo

import "strings"

// Scanners for the line-oriented constructs recognised by the block parser.
// Each one replaces the regular expression quoted in its declaration.

// scanNameToken matches \p{Word}[\p{Word}\-'.]* at i.
func scanNameToken(s string, i int) int {
	ok, sz := wordAt(s, i)
	if !ok {
		return -1
	}
	i += sz
	for i < len(s) {
		if c := s[i]; c == '-' || c == '\'' || c == '.' {
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

var authorInfoLineRx = bolScanner(`^(\p{Word}[\p{Word}\-'.]*)(?: +(\p{Word}[\p{Word}\-'.]*))?(?: +(\p{Word}[\p{Word}\-'.]*))?(?: +<([^>]+)>)?$`, "", 4,
	func(s string, i int, c []int) bool {
		var g [8]int
		for k := range g {
			g[k] = -1
		}
		p := scanNameToken(s, i)
		if p < 0 {
			return false
		}
		g[0], g[1] = i, p
		for k := 1; k <= 2; k++ {
			q := skipSpaces(s, p)
			if q == p {
				break
			}
			e := scanNameToken(s, q)
			if e < 0 {
				break
			}
			g[2*k], g[2*k+1] = q, e
			p = e
		}
		if q := skipSpaces(s, p); q > p && q < len(s) && s[q] == '<' {
			if j := strings.IndexByte(s[q+1:], '>'); j > 0 && isEOL(s, q+1+j+1) {
				g[6], g[7] = q+1, q+1+j
				p = q + 1 + j + 1
			}
		}
		if !isEOL(s, p) {
			return false
		}
		set2(c, 0, i, p)
		copy(c[2:], g[:])
		return true
	})

var revisionInfoLineRx = bolScanner(`^(?:[^\d{]*(.*?),)? *(?!:)(.*?)(?: *(?!^),?: *(.*))?$`, "", 3,
	func(s string, i int, c []int) bool {
		// rest matches ` *(?!:)(.*?)(?: *(?!^),?: *(.*))?$` at q
		rest := func(q int) bool {
			sp := skipSpaces(s, q)
			for ; sp >= q; sp-- {
				if sp < len(s) && s[sp] == ':' {
					continue
				}
				le := lineEnd(s, sp)
				for e := sp; e <= le; e++ {
					// optional remark group
					k := skipSpaces(s, e)
					for ; k >= e; k-- {
						if isBOL(s, k) {
							continue
						}
						for _, comma := range [2]bool{true, false} {
							t := k
							if comma {
								if t >= len(s) || s[t] != ',' {
									continue
								}
								t++
							}
							if t < len(s) && s[t] == ':' {
								r := skipSpaces(s, t+1)
								re := lineEnd(s, r)
								set2(c, 0, i, re)
								set2(c, 2, sp, e)
								set2(c, 3, r, re)
								return true
							}
						}
					}
					if isEOL(s, e) {
						set2(c, 0, i, e)
						set2(c, 2, sp, e)
						return true
					}
				}
			}
			return false
		}
		r := i
		for r < len(s) && !isDigit(s[r]) && s[r] != '{' {
			r++
		}
		for p := r; p >= i; p-- {
			le := lineEnd(s, p)
			for k := p; k < le; k++ {
				if s[k] == ',' {
					if rest(k + 1) {
						set2(c, 1, p, k)
						return true
					}
				}
			}
		}
		return rest(i)
	})

var manpageTitleVolnumRx = bolScanner(`^(.+?) *\( *(.+?) *\)$`, "", 2,
	func(s string, i int, c []int) bool {
		le := lineEnd(s, i)
		for e := i + 1; e < le; e++ {
			j := skipSpaces(s, e)
			if j >= le || s[j] != '(' {
				continue
			}
			m1 := skipSpaces(s, j+1)
			for m := m1; m > j; m-- {
				for f := m + 1; f < le; f++ {
					k := skipSpaces(s, f)
					if k < le && s[k] == ')' && isEOL(s, k+1) {
						set2(c, 0, i, k+1)
						set2(c, 1, i, e)
						set2(c, 2, m, f)
						return true
					}
				}
			}
		}
		return false
	})

var manpageNamePurposeRx = bolScanner(`^(.+?) +- +(.+)$`, "", 2,
	func(s string, i int, c []int) bool {
		le := lineEnd(s, i)
		for e := i + 1; e < le; e++ {
			if s[e] != ' ' {
				continue
			}
			j := skipSpaces(s, e)
			if j >= le || s[j] != '-' {
				continue
			}
			k := skipSpaces(s, j+1)
			if k == j+1 {
				continue
			}
			if k >= le {
				if k-(j+1) < 2 {
					continue
				}
				k--
			}
			set2(c, 0, i, le)
			set2(c, 1, i, e)
			set2(c, 2, k, le)
			return true
		}
		return false
	})

var conditionalDirectiveRx = bolScanner(`^(\\)?(ifdef|ifndef|ifeval|endif)::(\S*?(?:([,+])\S*?)?)\[(.+)?\]$`, "", 5,
	func(s string, i int, c []int) bool {
		p := i
		if p < len(s) && s[p] == '\\' {
			p++
		}
		rest := s[p:]
		var kw int
		switch {
		case strings.HasPrefix(rest, "ifdef::"), strings.HasPrefix(rest, "endif::"):
			kw = 5
		case strings.HasPrefix(rest, "ifndef::"), strings.HasPrefix(rest, "ifeval::"):
			kw = 6
		default:
			return false
		}
		t := p + kw + 2
		le := lineEnd(s, t)
		if le-1 <= t-1 || s[le-1] != ']' {
			return false
		}
		delim := -1
		b := t
		for ; b < le-1 && s[b] != '['; b++ {
			if isSpaceByte(s[b]) {
				return false
			}
			if delim < 0 && (s[b] == ',' || s[b] == '+') {
				delim = b
			}
		}
		if b >= le-1 {
			return false
		}
		set2(c, 0, i, le)
		if p > i {
			set2(c, 1, i, p)
		}
		set2(c, 2, p, p+kw)
		set2(c, 3, t, b)
		if delim >= 0 {
			set2(c, 4, delim, delim+1)
		}
		if b+1 < le-1 {
			set2(c, 5, b+1, le-1)
		}
		return true
	})

var evalExpressionRx = bolScanner(`^(.+?) *([=!><]=|[><]) *(.+)$`, "", 3,
	func(s string, i int, c []int) bool {
		le := lineEnd(s, i)
		for e := i + 1; e < le; e++ {
			j := skipSpaces(s, e)
			if j >= le {
				return false
			}
			for _, n := range [2]int{2, 1} {
				if n == 2 {
					if !(j+1 < le && s[j+1] == '=' && (s[j] == '=' || s[j] == '!' || s[j] == '>' || s[j] == '<')) {
						continue
					}
				} else if s[j] != '>' && s[j] != '<' {
					continue
				}
				k := skipSpaces(s, j+n)
				if k >= le {
					if k == j+n {
						continue
					}
					k--
				}
				set2(c, 0, i, le)
				set2(c, 1, i, e)
				set2(c, 2, j, j+n)
				set2(c, 3, k, le)
				return true
			}
		}
		return false
	})

var includeDirectiveRx = bolScanner(`^(\\)?include::([^\s\[](?:[^\[]*[^\s\[])?)\[(.+)?\]$`, "", 3,
	func(s string, i int, c []int) bool {
		p := i
		if p < len(s) && s[p] == '\\' {
			p++
		}
		if !strings.HasPrefix(s[p:], "include::") {
			return false
		}
		t := p + 9
		if t >= len(s) || isSpaceByte(s[t]) || s[t] == '[' {
			return false
		}
		j := strings.IndexByte(s[t:], '[')
		if j < 0 {
			return false
		}
		b := t + j
		_, sz := runeAt(s, t)
		if b > t+sz && isSpaceByte(s[b-1]) {
			return false
		}
		le := lineEnd(s, b)
		if le-1 <= b || s[le-1] != ']' {
			return false
		}
		set2(c, 0, i, le)
		if p > i {
			set2(c, 1, i, p)
		}
		set2(c, 2, t, b)
		if b+1 < le-1 {
			set2(c, 3, b+1, le-1)
		}
		return true
	})

var attributeEntryRx = bolScanner(`^:(!?\p{Word}[^:]*):(?:[ \t]+(.*))?$`, "", 2,
	func(s string, i int, c []int) bool {
		if i >= len(s) || s[i] != ':' {
			return false
		}
		p := i + 1
		if p < len(s) && s[p] == '!' {
			p++
		}
		ok, sz := wordAt(s, p)
		if !ok {
			return false
		}
		j := strings.IndexByte(s[p+sz:], ':')
		if j < 0 {
			return false
		}
		e := p + sz + j
		if k := skipBlanks(s, e+1); k > e+1 {
			le := lineEnd(s, k)
			set2(c, 0, i, le)
			set2(c, 1, i+1, e)
			set2(c, 2, k, le)
			return true
		}
		if !isEOL(s, e+1) {
			return false
		}
		set2(c, 0, i, e+1)
		set2(c, 1, i+1, e)
		return true
	})

// scanAnchorBody matches (?:|ID(?:, *(.+))?)CLOSE$ at p, where the opening
// brackets have already been consumed. It returns the end of the match and
// the id and reftext spans.
func scanAnchorBody(s string, p int, closer string) (end, ids, ide, rs, re int, ok bool) {
	ids, ide, rs, re = -1, -1, -1, -1
	if strings.HasPrefix(s[p:], closer) && isEOL(s, p+len(closer)) {
		return p + len(closer), -1, -1, -1, -1, true
	}
	e := scanID(s, p)
	if e < 0 {
		return 0, -1, -1, -1, -1, false
	}
	ids, ide = p, e
	if e < len(s) && s[e] == ',' {
		le := lineEnd(s, e)
		cs := le - len(closer)
		if cs > e && s[cs:le] == closer {
			q := skipSpaces(s, e+1)
			if q >= cs && q > e+1 {
				q = cs - 1
			}
			if q < cs {
				return le, ids, ide, q, cs, true
			}
		}
	}
	if strings.HasPrefix(s[e:], closer) && isEOL(s, e+len(closer)) {
		return e + len(closer), ids, ide, -1, -1, true
	}
	return 0, -1, -1, -1, -1, false
}

var blockAnchorRx = bolScanner(`^\[\[(?:|([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+))?)\]\]$`, "", 2,
	func(s string, i int, c []int) bool {
		if !strings.HasPrefix(s[i:], "[[") {
			return false
		}
		end, ids, ide, rs, re, ok := scanAnchorBody(s, i+2, "]]")
		if !ok {
			return false
		}
		set2(c, 0, i, end)
		set2(c, 1, ids, ide)
		set2(c, 2, rs, re)
		return true
	})

// isAttrListStart reports whether [\p{Word}.#%{,"'] matches at i.
func isAttrListStart(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	switch s[i] {
	case '.', '#', '%', '{', ',', '"', '\'':
		return true
	}
	return isWordAt(s, i)
}

// tryBlockAttributeList matches ^\[(|[\p{Word}.#%{,"'].*)\]$ at i and
// returns the span of the attribute list.
func tryBlockAttributeList(s string, i int) (as, ae, end int, ok bool) {
	if i >= len(s) || s[i] != '[' {
		return 0, 0, 0, false
	}
	if i+1 < len(s) && s[i+1] == ']' && isEOL(s, i+2) {
		return i + 1, i + 1, i + 2, true
	}
	if !isAttrListStart(s, i+1) {
		return 0, 0, 0, false
	}
	_, sz := runeAt(s, i+1)
	le := lineEnd(s, i+1+sz)
	if le-1 < i+1+sz || s[le-1] != ']' {
		return 0, 0, 0, false
	}
	return i + 1, le - 1, le, true
}

var blockAttributeListRx = bolScanner(`^\[(|[\p{Word}.#%{,"'].*)\]$`, "", 1,
	func(s string, i int, c []int) bool {
		as, ae, end, ok := tryBlockAttributeList(s, i)
		if !ok {
			return false
		}
		set2(c, 0, i, end)
		set2(c, 1, as, ae)
		return true
	})

var blockAttributeLineRx = bolScanner(`^\[(?:|[\p{Word}.#%{,"'].*|\[(?:|[\p{Alpha}_:][\p{Word}\-:.]*(?:, *.+)?)\])\]$`, "", 0,
	func(s string, i int, c []int) bool {
		if _, _, end, ok := tryBlockAttributeList(s, i); ok {
			set2(c, 0, i, end)
			return true
		}
		if strings.HasPrefix(s[i:], "[[") {
			if end, _, _, _, _, ok := scanAnchorBody(s, i+2, "]]"); ok {
				set2(c, 0, i, end)
				return true
			}
		}
		return false
	})

var blockTitleRx = bolScanner(`^\.(\.?[^ \t.].*)$`, "", 1,
	func(s string, i int, c []int) bool {
		if i >= len(s) || s[i] != '.' {
			return false
		}
		p := i + 1
		if p < len(s) && s[p] == '.' {
			p++
		}
		if p >= len(s) || s[p] == ' ' || s[p] == '\t' || s[p] == '.' {
			return false
		}
		_, sz := runeAt(s, p)
		le := lineEnd(s, p+sz)
		set2(c, 0, i, le)
		set2(c, 1, i+1, le)
		return true
	})

var admonitionParagraphRx = bolScanner(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION):[ \t]+`, "", 1,
	func(s string, i int, c []int) bool {
		for _, name := range [...]string{"NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION"} {
			if strings.HasPrefix(s[i:], name) {
				p := i + len(name)
				if p < len(s) && s[p] == ':' {
					if k := skipBlanks(s, p+1); k > p+1 {
						set2(c, 0, i, k)
						set2(c, 1, i, p)
						return true
					}
				}
				return false
			}
		}
		return false
	})

var literalParagraphRx = bolScanner(`^([ \t]+.*)$`, "", 1,
	func(s string, i int, c []int) bool {
		if i >= len(s) || !isBlankByte(s[i]) {
			return false
		}
		le := lineEnd(s, i)
		set2(c, 0, i, le)
		set2(c, 1, i, le)
		return true
	})

var extAtxSectionTitleRx = bolScanner(`^(=={0,5}|##{0,5})[ \t]+(.+?)(?:[ \t]+\1)?$`, "", 2,
	func(s string, i int, c []int) bool {
		if i >= len(s) || (s[i] != '=' && s[i] != '#') {
			return false
		}
		ch := s[i]
		p := i
		for p < len(s) && s[p] == ch {
			p++
		}
		if p-i > 6 {
			return false
		}
		marker := s[i:p]
		gs := skipBlanks(s, p)
		if gs == p {
			return false
		}
		le := lineEnd(s, p)
		if gs >= le {
			if gs-p < 2 {
				return false
			}
			gs--
		}
		for e := gs + 1; e < le; e++ {
			if !isBlankByte(s[e]) {
				continue
			}
			k := skipBlanks(s, e)
			if strings.HasPrefix(s[k:], marker) && k+len(marker) == le {
				set2(c, 0, i, le)
				set2(c, 1, i, p)
				set2(c, 2, gs, e)
				return true
			}
		}
		set2(c, 0, i, le)
		set2(c, 1, i, p)
		set2(c, 2, gs, le)
		return true
	})

var setextSectionTitleRx = bolScanner(`^((?!\.).*?\p{Alnum}.*)$`, "", 1,
	func(s string, i int, c []int) bool {
		if i < len(s) && s[i] == '.' {
			return false
		}
		le := lineEnd(s, i)
		for p := i; p < le; {
			ok, sz := alnumAt(s, p)
			if ok {
				set2(c, 0, i, le)
				set2(c, 1, i, le)
				return true
			}
			p += sz
		}
		return false
	})

var inlineSectionAnchorRx = newScanner(` (\\)?\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+))?\]\]$`, "", 3,
	func(s string, from int, c []int) bool {
		for i := from; ; i++ {
			j := strings.IndexByte(s[i:], ' ')
			if j < 0 {
				return false
			}
			i += j
			p := i + 1
			esc := false
			if p < len(s) && s[p] == '\\' {
				esc = true
				p++
			}
			if !strings.HasPrefix(s[p:], "[[") {
				continue
			}
			e := scanID(s, p+2)
			if e < 0 {
				continue
			}
			end, _, _, rs, re, ok := scanAnchorBody(s, p+2, "]]")
			if !ok {
				continue
			}
			set2(c, 0, i, end)
			if esc {
				set2(c, 1, i+1, i+2)
			}
			set2(c, 2, p+2, e)
			set2(c, 3, rs, re)
			return true
		}
	})

var sectionLevelStyleRx = bolScanner(`^sect\d$`, "", 0,
	func(s string, i int, c []int) bool {
		if strings.HasPrefix(s[i:], "sect") && i+4 < len(s) && isDigit(s[i+4]) && isEOL(s, i+5) {
			set2(c, 0, i, i+5)
			return true
		}
		return false
	})

const bullet = "•"

// scanListMarkerEnd matches the marker alternatives shared by the list
// patterns at p and returns the end of the marker, or -1.
func scanUlistMarker(s string, p int) int {
	if p >= len(s) {
		return -1
	}
	switch s[p] {
	case '-':
		return p + 1
	case '*':
		for p < len(s) && s[p] == '*' {
			p++
		}
		return p
	}
	if strings.HasPrefix(s[p:], bullet) {
		return p + len(bullet)
	}
	return -1
}

func isRomanByte(c byte) bool {
	switch c {
	case 'I', 'V', 'X', 'i', 'v', 'x':
		return true
	}
	return false
}

func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func scanOlistMarker(s string, p int) int {
	if p >= len(s) {
		return -1
	}
	ch := s[p]
	if ch == '.' {
		for p < len(s) && s[p] == '.' {
			p++
		}
		return p
	}
	if isDigit(ch) {
		if e := skipDigits(s, p); e < len(s) && s[e] == '.' {
			return e + 1
		}
		return -1
	}
	if isASCIILetter(ch) && p+1 < len(s) && s[p+1] == '.' {
		return p + 2
	}
	if isRomanByte(ch) {
		e := p
		for e < len(s) && isRomanByte(s[e]) {
			e++
		}
		if e < len(s) && s[e] == ')' {
			return e + 1
		}
	}
	return -1
}

// dlistBlocked implements the (?!//[^/]) guard of the description list
// patterns.
func dlistBlocked(s string, i int) bool {
	return i+2 < len(s) && s[i] == '/' && s[i+1] == '/' && s[i+2] != '/'
}

// dlistTail matches (?:$|[ \t]+(.*)$) at p.
func dlistTail(s string, p int) (end, ts, te int, ok bool) {
	if isEOL(s, p) {
		return p, -1, -1, true
	}
	if isBlankByte(s[p]) {
		k := skipBlanks(s, p)
		le := lineEnd(s, k)
		return le, k, le, true
	}
	return 0, -1, -1, false
}

var anyListRx = bolScanner(`^(?:[ \t]*(?:-|\*\**|\.\.*|•|\d+\.|[a-zA-Z]\.|[IVXivx]+\))[ \t]|(?!//[^/])[ \t]*[^ \t].*?(?::::{0,2}|;;)(?:$|[ \t])|<(?:\d+|\.)>[ \t])`, "", 0,
	func(s string, i int, c []int) bool {
		f := skipBlanks(s, i)
		for _, e := range [2]int{scanUlistMarker(s, f), scanOlistMarker(s, f)} {
			if e >= 0 && e < len(s) && isBlankByte(s[e]) {
				set2(c, 0, i, e+1)
				return true
			}
		}
		if f < len(s) && !dlistBlocked(s, i) {
			_, sz := runeAt(s, f)
			le := lineEnd(s, f+sz)
			for e := f + sz; e+1 < le; e++ {
				ch := s[e]
				if ch == ':' && s[e+1] == ':' {
					n := 2
					for n < 4 && e+n < le && s[e+n] == ':' {
						n++
					}
					for ; n >= 2; n-- {
						if e+n == le {
							set2(c, 0, i, e+n)
							return true
						}
						if isBlankByte(s[e+n]) {
							set2(c, 0, i, e+n+1)
							return true
						}
					}
				} else if ch == ';' && s[e+1] == ';' {
					if e+2 == le {
						set2(c, 0, i, e+2)
						return true
					}
					if isBlankByte(s[e+2]) {
						set2(c, 0, i, e+3)
						return true
					}
				}
			}
		}
		if i < len(s) && s[i] == '<' {
			p := i + 1
			if p < len(s) && s[p] == '.' {
				p++
			} else if e := skipDigits(s, p); e > p {
				p = e
			} else {
				return false
			}
			if p+1 < len(s) && s[p] == '>' && isBlankByte(s[p+1]) {
				set2(c, 0, i, p+2)
				return true
			}
		}
		return false
	})

func listItemScanner(pat string, marker func(s string, p int) int) *scanner {
	return bolScanner(pat, "", 2, func(s string, i int, c []int) bool {
		f := skipBlanks(s, i)
		e := marker(s, f)
		if e < 0 {
			return false
		}
		k := skipBlanks(s, e)
		if k == e {
			return false
		}
		le := lineEnd(s, k)
		set2(c, 0, i, le)
		set2(c, 1, f, e)
		set2(c, 2, k, le)
		return true
	})
}

var (
	unorderedListRx = listItemScanner(`^[ \t]*(-|\*\**|•)[ \t]+(.*)$`, scanUlistMarker)
	orderedListRx   = listItemScanner(`^[ \t]*(\.\.*|\d+\.|[a-zA-Z]\.|[IVXivx]+\))[ \t]+(.*)$`, scanOlistMarker)
)

var descriptionListRx = bolScanner(`^(?!//[^/])[ \t]*([^ \t].*?)(:::{0,2}|;;)(?:$|[ \t]+(.*)$)`, "", 3,
	func(s string, i int, c []int) bool {
		f := skipBlanks(s, i)
		if f >= len(s) || dlistBlocked(s, i) {
			return false
		}
		_, sz := runeAt(s, f)
		le := lineEnd(s, f+sz)
		for e := f + sz; e+1 < le; e++ {
			ch := s[e]
			if ch == ':' && s[e+1] == ':' {
				n := 2
				for n < 4 && e+n < le && s[e+n] == ':' {
					n++
				}
				for ; n >= 2; n-- {
					if end, ts, te, ok := dlistTail(s, e+n); ok {
						set2(c, 0, i, end)
						set2(c, 1, f, e)
						set2(c, 2, e, e+n)
						set2(c, 3, ts, te)
						return true
					}
				}
			} else if ch == ';' && s[e+1] == ';' {
				if end, ts, te, ok := dlistTail(s, e+2); ok {
					set2(c, 0, i, end)
					set2(c, 1, f, e)
					set2(c, 2, e, e+2)
					set2(c, 3, ts, te)
					return true
				}
			}
		}
		return false
	})

// dlistSiblingScanner builds the scanner for a description list sibling
// with the given marker ("::", ":::", "::::" or ";;").
func dlistSiblingScanner(pat, marker string) *scanner {
	colon := marker[0] == ':'
	return bolScanner(pat, "", 3, func(s string, i int, c []int) bool {
		f := skipBlanks(s, i)
		if f >= len(s) || dlistBlocked(s, i) {
			return false
		}
		_, sz := runeAt(s, f)
		le := lineEnd(s, f+sz)
		start := f + sz
		if colon {
			// the term is [^ \t].*?[^:], i.e. at least two characters
			if start >= le {
				start = le
			} else {
				_, sz2 := runeAt(s, start)
				start += sz2
			}
		}
		for e := start; e+len(marker) <= le; e++ {
			if s[e] != marker[0] || !strings.HasPrefix(s[e:], marker) {
				continue
			}
			if colon && s[e-1] == ':' {
				continue
			}
			if end, ts, te, ok := dlistTail(s, e+len(marker)); ok {
				set2(c, 0, i, end)
				set2(c, 1, f, e)
				set2(c, 2, e, e+len(marker))
				set2(c, 3, ts, te)
				return true
			}
		}
		if colon && le < len(s) && le >= f+sz && strings.HasPrefix(s[le+1:], marker) {
			// the [^:] before the marker may be the newline itself
			e := le + 1
			if end, ts, te, ok := dlistTail(s, e+len(marker)); ok {
				set2(c, 0, i, end)
				set2(c, 1, f, e)
				set2(c, 2, e, e+len(marker))
				set2(c, 3, ts, te)
				return true
			}
		}
		if colon && s[f] != ':' && strings.HasPrefix(s[f+sz:], marker) {
			// second alternative: a single-character term
			e := f + sz
			if end, ts, te, ok := dlistTail(s, e+len(marker)); ok {
				set2(c, 0, i, end)
				set2(c, 1, f, e)
				set2(c, 2, e, e+len(marker))
				set2(c, 3, ts, te)
				return true
			}
		}
		return false
	})
}

var descriptionListSiblingRx = map[string]*scanner{
	"::":   dlistSiblingScanner(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(::)(?:$|[ \t]+(.*)$)`, "::"),
	":::":  dlistSiblingScanner(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(:::)(?:$|[ \t]+(.*)$)`, ":::"),
	"::::": dlistSiblingScanner(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(::::)(?:$|[ \t]+(.*)$)`, "::::"),
	";;":   dlistSiblingScanner(`^(?!//[^/])[ \t]*([^ \t].*?)(;;)(?:$|[ \t]+(.*)$)`, ";;"),
}

var calloutListRx = bolScanner(`^<(\d+|\.)>[ \t]+(.*)$`, "", 2,
	func(s string, i int, c []int) bool {
		if i >= len(s) || s[i] != '<' {
			return false
		}
		p := i + 1
		if p < len(s) && s[p] == '.' {
			p++
		} else if e := skipDigits(s, p); e > p {
			p = e
		} else {
			return false
		}
		if p >= len(s) || s[p] != '>' {
			return false
		}
		k := skipBlanks(s, p+1)
		if k == p+1 {
			return false
		}
		le := lineEnd(s, k)
		set2(c, 0, i, le)
		set2(c, 1, i+1, p)
		set2(c, 2, k, le)
		return true
	})

func isAlignByte(c byte) bool { return c == '<' || c == '^' || c == '>' }

// scanAlign matches ([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])? at p and returns
// the end offset (p if nothing matched).
func scanAlign(s string, p int) int {
	if p >= len(s) {
		return p
	}
	if isAlignByte(s[p]) {
		e := p + 1
		if e < len(s) && s[e] == '.' {
			e++
			if e < len(s) && isAlignByte(s[e]) {
				e++
			}
		}
		return e
	}
	if s[p] == '.' && p+1 < len(s) && isAlignByte(s[p+1]) {
		return p + 2
	}
	return p
}

var columnSpecRx = bolScanner(`^(?:(\d+)\*)?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?(\d+%?|~)?([a-z])?$`, "", 4,
	func(s string, i int, c []int) bool {
		var g [8]int
		for k := range g {
			g[k] = -1
		}
		p := i
		if e := skipDigits(s, p); e > p && e < len(s) && s[e] == '*' {
			g[0], g[1] = p, e
			p = e + 1
		}
		if e := scanAlign(s, p); e > p {
			g[2], g[3] = p, e
			p = e
		}
		if e := skipDigits(s, p); e > p {
			if e < len(s) && s[e] == '%' {
				e++
			}
			g[4], g[5] = p, e
			p = e
		} else if p < len(s) && s[p] == '~' {
			g[4], g[5] = p, p+1
			p++
		}
		if p < len(s) && s[p] >= 'a' && s[p] <= 'z' {
			g[6], g[7] = p, p+1
			p++
		}
		if !isEOL(s, p) {
			return false
		}
		set2(c, 0, i, p)
		copy(c[2:], g[:])
		return true
	})

// scanCellSpecTail matches
// (?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?(ALIGN)?([a-z])?$ at p.
func scanCellSpecTail(s string, p int, g *[8]int) (int, bool) {
	for k := range g {
		g[k] = -1
	}
	e := -1
	if p < len(s) && isDigit(s[p]) {
		e = skipDigits(s, p)
		if e < len(s) && s[e] == '.' {
			e = skipDigits(s, e+1)
		}
	} else if p < len(s) && s[p] == '.' {
		if d := skipDigits(s, p+1); d > p+1 {
			e = d
		}
	}
	if e >= 0 && e < len(s) && (s[e] == '*' || s[e] == '+') {
		g[0], g[1] = p, e
		g[2], g[3] = e, e+1
		p = e + 1
	}
	if e := scanAlign(s, p); e > p {
		g[4], g[5] = p, e
		p = e
	}
	if p < len(s) && s[p] >= 'a' && s[p] <= 'z' {
		g[6], g[7] = p, p+1
		p++
	}
	return p, isEOL(s, p)
}

var cellSpecStartRx = bolScanner(`^[ \t]*(?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?([a-z])?$`, "", 4,
	func(s string, i int, c []int) bool {
		var g [8]int
		end, ok := scanCellSpecTail(s, skipBlanks(s, i), &g)
		if !ok {
			return false
		}
		set2(c, 0, i, end)
		copy(c[2:], g[:])
		return true
	})

var cellSpecEndRx = newScanner(`[ \t]+(?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?([a-z])?$`, "", 4,
	func(s string, from int, c []int) bool {
		var g [8]int
		for i := from; i < len(s); i++ {
			if !isBlankByte(s[i]) {
				continue
			}
			b := skipBlanks(s, i)
			if end, ok := scanCellSpecTail(s, b, &g); ok {
				set2(c, 0, i, end)
				copy(c[2:], g[:])
				return true
			}
			i = b
		}
		return false
	})

// scanMacroTail matches \[(.+)?\]$ at b and returns the line end.
func scanMacroTail(s string, b int) (int, bool) {
	if b >= len(s) || s[b] != '[' {
		return 0, false
	}
	le := lineEnd(s, b)
	if le-1 <= b || s[le-1] != ']' {
		return 0, false
	}
	return le, true
}

var blockMediaMacroRx = bolScanner(`^(image|video|audio)::(\S|\S.*?\S)\[(.+)?\]$`, "", 3,
	func(s string, i int, c []int) bool {
		rest := s[i:]
		if !(strings.HasPrefix(rest, "image::") || strings.HasPrefix(rest, "video::") || strings.HasPrefix(rest, "audio::")) {
			return false
		}
		t := i + 7
		if t >= len(s) || isSpaceByte(s[t]) {
			return false
		}
		_, sz := runeAt(s, t)
		b := t + sz
		le, ok := scanMacroTail(s, b)
		if !ok {
			ok = false
			tl := lineEnd(s, b)
			for b = t + sz + 1; b < tl; b++ {
				if s[b] == '[' && !isSpaceByte(s[b-1]) {
					if le, ok = scanMacroTail(s, b); ok {
						break
					}
				}
			}
			if !ok {
				return false
			}
		}
		set2(c, 0, i, le)
		set2(c, 1, i, i+5)
		set2(c, 2, t, b)
		if b+1 < le-1 {
			set2(c, 3, b+1, le-1)
		}
		return true
	})

var blockTocMacroRx = bolScanner(`^toc::\[(.+)?\]$`, "", 1,
	func(s string, i int, c []int) bool {
		if !strings.HasPrefix(s[i:], "toc::") {
			return false
		}
		le, ok := scanMacroTail(s, i+5)
		if !ok {
			return false
		}
		set2(c, 0, i, le)
		if i+6 < le-1 {
			set2(c, 1, i+6, le-1)
		}
		return true
	})

// scanMdBreak matches ([-*_])( *)\1\2\1$ at p.
func scanMdBreak(s string, p int, c []int) (int, bool) {
	if p >= len(s) || (s[p] != '-' && s[p] != '*' && s[p] != '_') {
		return 0, false
	}
	ch := s[p]
	q := skipSpaces(s, p+1)
	n := q - (p + 1)
	if q >= len(s) || s[q] != ch {
		return 0, false
	}
	r := q + 1
	if r+n >= len(s) || skipSpaces(s, r) < r+n || s[r+n] != ch || !isEOL(s, r+n+1) {
		return 0, false
	}
	if skipSpaces(s, r) != r+n && n > 0 {
		// more spaces than in the first gap
		return 0, false
	}
	set2(c, 1, p, p+1)
	set2(c, 2, p+1, q)
	return r + n + 1, true
}

var markdownThematicBreakRx = bolScanner(`^ {0,3}([-*_])( *)\1\2\1$`, "", 2,
	func(s string, i int, c []int) bool {
		p := skipSpaces(s, i)
		if p-i > 3 {
			return false
		}
		end, ok := scanMdBreak(s, p, c)
		if !ok {
			return false
		}
		set2(c, 0, i, end)
		return true
	})

var extLayoutBreakRx = bolScanner(`^(?:'{3,}|<{3,}|([-*_])( *)\1\2\1)$`, "", 2,
	func(s string, i int, c []int) bool {
		if i < len(s) && (s[i] == '\'' || s[i] == '<') {
			p := i
			for p < len(s) && s[p] == s[i] {
				p++
			}
			if p-i >= 3 && isEOL(s, p) {
				set2(c, 0, i, p)
				return true
			}
			return false
		}
		end, ok := scanMdBreak(s, i, c)
		if !ok {
			return false
		}
		set2(c, 0, i, end)
		return true
	})

// scanInlineAnchorRest matches ID(?:, *(.+?))?CLOSE at p (after the opening
// brackets) for the leading anchor patterns.
func scanLeadingAnchor(s string, i int, open, closer string, c []int) bool {
	if !strings.HasPrefix(s[i:], open) {
		return false
	}
	p := i + len(open)
	e := scanID(s, p)
	if e < 0 {
		return false
	}
	if e < len(s) && s[e] == ',' {
		q := skipSpaces(s, e+1)
		le := lineEnd(s, e)
		if q < le {
			_, sz := runeAt(s, q)
			if j := strings.Index(s[q+sz:le], closer); j >= 0 {
				k := q + sz + j
				set2(c, 0, i, k+len(closer))
				set2(c, 1, p, e)
				set2(c, 2, q, k)
				return true
			}
		}
		if q > e+1 && strings.HasPrefix(s[q:], closer) {
			set2(c, 0, i, q+len(closer))
			set2(c, 1, p, e)
			set2(c, 2, q-1, q)
			return true
		}
	}
	if strings.HasPrefix(s[e:], closer) {
		set2(c, 0, i, e+len(closer))
		set2(c, 1, p, e)
		return true
	}
	return false
}

var leadingInlineAnchorRx = bolScanner(`^\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))?\]\]`, "", 2,
	func(s string, i int, c []int) bool { return scanLeadingAnchor(s, i, "[[", "]]", c) })

var inlineBiblioAnchorRx = bolScanner(`^\[\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))?\]\]\]`, "", 2,
	func(s string, i int, c []int) bool { return scanLeadingAnchor(s, i, "[[[", "]]]", c) })

var trailingDigitsRx = newScanner(`\d+$`, "", 0,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			if !isDigit(s[i]) {
				continue
			}
			e := skipDigits(s, i)
			if isEOL(s, e) {
				set2(c, 0, i, e)
				return true
			}
			i = e
		}
		return false
	})

var uriSniffRx = newScanner(`\A\p{Alpha}[\p{Alnum}.+-]+:/{0,2}`, "", 0,
	func(s string, from int, c []int) bool {
		if from != 0 {
			return false
		}
		ok, p := alphaAt(s, 0)
		if !ok {
			return false
		}
		n := 0
		for p < len(s) {
			if ch := s[p]; ch == '.' || ch == '+' || ch == '-' {
				p++
			} else if ok, sz := alnumAt(s, p); ok {
				p += sz
			} else {
				break
			}
			n++
		}
		if n == 0 || p >= len(s) || s[p] != ':' {
			return false
		}
		p++
		for k := 0; k < 2 && p < len(s) && s[p] == '/'; k++ {
			p++
		}
		set2(c, 0, 0, p)
		return true
	})

var attributeEntryPassMacroRx = newScanner(`\Apass:([a-z]+(?:,[a-z-]+)*)?\[(.*)\]\Z`, "m", 2,
	func(s string, from int, c []int) bool {
		if from != 0 || !strings.HasPrefix(s, "pass:") {
			return false
		}
		p := 5
		for p < len(s) && s[p] >= 'a' && s[p] <= 'z' {
			p++
		}
		if p > 5 {
			for p < len(s) && s[p] == ',' {
				q := p + 1
				for q < len(s) && ((s[q] >= 'a' && s[q] <= 'z') || s[q] == '-') {
					q++
				}
				if q == p+1 {
					break
				}
				p = q
			}
		}
		if p >= len(s) || s[p] != '[' {
			return false
		}
		end := len(s)
		if s[end-1] == '\n' {
			end--
		}
		if end-1 <= p || s[end-1] != ']' {
			if end == len(s) || s[len(s)-1] != ']' {
				return false
			}
			end = len(s)
		}
		if s[len(s)-1] == ']' {
			end = len(s)
		}
		set2(c, 0, 0, end)
		if p > 5 {
			set2(c, 1, 5, p)
		}
		set2(c, 2, p+1, end-1)
		return true
	})

func orderedMarkerScanner(pat string, isLead func(byte) bool, multi bool, closer byte) *scanner {
	return newScanner(pat, "", 0, func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			if !isLead(s[i]) {
				continue
			}
			e := i + 1
			if multi {
				for e < len(s) && isLead(s[e]) {
					e++
				}
			}
			if e < len(s) && s[e] == closer {
				set2(c, 0, i, e+1)
				return true
			}
			if !multi {
				continue
			}
			i = e - 1
		}
		return false
	})
}

var orderedListMarkerRx = map[string]*scanner{
	"arabic":     orderedMarkerScanner(`\d+\.`, isDigit, true, '.'),
	"loweralpha": orderedMarkerScanner(`[a-z]\.`, func(c byte) bool { return c >= 'a' && c <= 'z' }, false, '.'),
	"lowerroman": orderedMarkerScanner(`[ivx]+\)`, func(c byte) bool { return c == 'i' || c == 'v' || c == 'x' }, true, ')'),
	"upperalpha": orderedMarkerScanner(`[A-Z]\.`, func(c byte) bool { return c >= 'A' && c <= 'Z' }, false, '.'),
	"upperroman": orderedMarkerScanner(`[IVX]+\)`, func(c byte) bool { return c == 'I' || c == 'V' || c == 'X' }, true, ')'),
}
