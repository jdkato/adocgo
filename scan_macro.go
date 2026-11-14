package adocgo

import "strings"

// Scanners for inline macros, anchors, links and callouts.

// scanCalloutCore matches
//
//	OPEN!?(|--)(\d+|\.)\1CLOSE(?=(?: ?\\?OPEN!?\1(?:\d+|\.)\1CLOSE)*$)
//
// at b, where OPEN/CLOSE are "<"/">" or "&lt;"/"&gt;". When xml is false the
// `!?` and `--` parts are not accepted. It returns the spans of the guard
// group and the number and the end of the match.
func scanCalloutCore(s string, b int, open, closer string, xml bool) (gs, ge, ns, ne, end int, ok bool) {
	unit := func(p int, dashes int) (int, int, int, bool) {
		if !strings.HasPrefix(s[p:], open) {
			return 0, 0, 0, false
		}
		p += len(open)
		if xml && p < len(s) && s[p] == '!' {
			p++
		}
		if dashes > 0 {
			if !strings.HasPrefix(s[p:], "--") {
				return 0, 0, 0, false
			}
			p += 2
		}
		ns := p
		if p < len(s) && s[p] == '.' {
			p++
		} else if e := skipDigits(s, p); e > p {
			p = e
		} else {
			return 0, 0, 0, false
		}
		ne := p
		if dashes > 0 {
			if !strings.HasPrefix(s[p:], "--") {
				return 0, 0, 0, false
			}
			p += 2
		}
		if !strings.HasPrefix(s[p:], closer) {
			return 0, 0, 0, false
		}
		return ns, ne, p + len(closer), true
	}
	maxDashes := 0
	if xml {
		maxDashes = 2
	}
	for dashes := 0; dashes <= maxDashes; dashes += 2 {
		ns, ne, end, ok := unit(b, dashes)
		if !ok {
			continue
		}
		// lookahead: only further callouts may follow on the line
		p := end
		for !isEOL(s, p) {
			q := p
			if s[q] == ' ' {
				q++
			}
			if q < len(s) && s[q] == '\\' {
				q++
			}
			_, _, e, ok := unit(q, dashes)
			if !ok {
				break
			}
			p = e
		}
		if !isEOL(s, p) {
			continue
		}
		gs := ns - dashes
		return gs, gs + dashes, ns, ne, end, true
	}
	return 0, 0, 0, 0, 0, false
}

var calloutScanRx = newScanner(`\\?<!?(|--)(\d+|\.)\1>(?=(?: ?\\?<!?\1(?:\d+|\.)\1>)*$)`, "", 2,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				return false
			}
			i += j
			gs, ge, ns, ne, end, ok := scanCalloutCore(s, i, "<", ">", true)
			if !ok {
				continue
			}
			set2(c, 0, escStart(s, i, from), end)
			set2(c, 1, gs, ge)
			set2(c, 2, ns, ne)
			return true
		}
		return false
	})

// calloutScanner builds the scanner for
//
//	((?:PREFIXES) ?)?(\\)?OPEN!?(|--)(\d+|\.)\3CLOSE(?=...)
//
// used to find callouts in verbatim text.
func calloutScanner(pat string, prefixes []string, open, closer string, xml bool) *scanner {
	sc := makeCalloutScanner(pat, prefixes, open, closer, xml)
	allScanners = append(allScanners, sc)
	return sc
}

func makeCalloutScanner(pat string, prefixes []string, open, closer string, xml bool) *scanner {
	return &scanner{pat: pat, n: 4, hand: true, find: func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], open)
			if j < 0 {
				return false
			}
			i += j
			gs, ge, ns, ne, end, ok := scanCalloutCore(s, i, open, closer, xml)
			if !ok {
				continue
			}
			x := escStart(s, i, from)
			st := x
			// optional line comment prefix followed by an optional space
			y := x
			if y > from && s[y-1] == ' ' {
				y--
			}
			found := false
			for _, y := range [2]int{y, x} {
				for _, pre := range prefixes {
					if pre == "" {
						continue
					}
					if y-len(pre) >= from && s[y-len(pre):y] == pre {
						if !found || y-len(pre) < st {
							st = y - len(pre)
						}
						found = true
					}
				}
			}
			set2(c, 0, st, end)
			if found {
				set2(c, 1, st, x)
			} else if len(prefixes) == 1 && prefixes[0] == "" {
				set2(c, 1, x, x)
			}
			if x < i {
				set2(c, 2, x, i)
			}
			set2(c, 3, gs, ge)
			set2(c, 4, ns, ne)
			return true
		}
		return false
	}}
}

var commentPrefixes = []string{"//", "#", "--", ";;"}

var (
	calloutExtractRx = calloutScanner(`((?://|#|--|;;) ?)?(\\)?<!?(|--)(\d+|\.)\3>(?=(?: ?\\?<!?\3(?:\d+|\.)\3>)*$)`, commentPrefixes, "<", ">", true)
	calloutSourceRx  = calloutScanner(`((?://|#|--|;;) ?)?(\\)?&lt;!?(|--)(\d+|\.)\3&gt;(?=(?: ?\\?&lt;!?\3(?:\d+|\.)\3&gt;)*$)`, commentPrefixes, "&lt;", "&gt;", true)
)

// quoteRegexp escapes the regular expression metacharacters in s. It is
// only used to describe dynamically built scanners.
func quoteRegexp(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case strings.IndexByte(`\.+*?()|[]{}^$-`, c) >= 0:
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == ' ':
			b.WriteString("\\ ")
		case c == '\n':
			b.WriteString("\\n")
		case c == '\t':
			b.WriteString("\\t")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// lineCommentCalloutScanner builds the callout scanner for a block with a
// custom line-comment attribute.
func lineCommentCalloutScanner(lc string) *scanner {
	pre := ""
	if lc != "" {
		pre = quoteRegexp(lc) + " ?"
	}
	pat := `(` + pre + `)?(\\)?&lt;()(\d+|\.)&gt;(?=(?: ?\\?&lt;(?:\d+|\.)&gt;)*$)`
	return makeCalloutScanner(pat, []string{lc}, "&lt;", "&gt;", false)
}

// scanAnchorMacroTail matches \[(?:\]|(.*?[^\\])\]) at b (no dotall).
func scanBracketText(s string, b int, dotall, allowEmpty bool) (ts, te, end int, ok bool) {
	if b >= len(s) || s[b] != '[' {
		return 0, 0, 0, false
	}
	if allowEmpty && b+1 < len(s) && s[b+1] == ']' {
		return b + 1, b + 1, b + 2, true
	}
	k := findClose(s, b+1, dotall)
	if k < 0 {
		return 0, 0, 0, false
	}
	return b + 1, k, k + 1, true
}

// tryInlineAnchor matches the body shared by the inline anchor patterns at
// p: \[\[ID(?:, *(.+?))? ?\]\] or anchor:ID\[(?:\]|(.*?[^\\])\]).
func tryInlineAnchor(s string, p int, g *[8]int) (int, bool) {
	for k := range g {
		g[k] = -1
	}
	if strings.HasPrefix(s[p:], "[[") {
		e := scanID(s, p+2)
		if e < 0 {
			return 0, false
		}
		g[0], g[1] = p+2, e
		closeAt := func(x int) int {
			if x < len(s) && s[x] == ' ' && strings.HasPrefix(s[x+1:], "]]") {
				return x + 3
			}
			if strings.HasPrefix(s[x:], "]]") {
				return x + 2
			}
			return -1
		}
		if e < len(s) && s[e] == ',' {
			q := skipSpaces(s, e+1)
			le := lineEnd(s, e)
			if q < le {
				_, sz := runeAt(s, q)
				for x := q + sz; x < le; x++ {
					if end := closeAt(x); end >= 0 {
						g[2], g[3] = q, x
						return end, true
					}
				}
			}
			if q > e+1 {
				if end := closeAt(q); end >= 0 {
					g[2], g[3] = q-1, q
					return end, true
				}
			}
		}
		if end := closeAt(e); end >= 0 {
			return end, true
		}
		return 0, false
	}
	if strings.HasPrefix(s[p:], "anchor:") {
		e := scanID(s, p+7)
		if e < 0 {
			return 0, false
		}
		ts, te, end, ok := scanBracketText(s, e, false, true)
		if !ok {
			return 0, false
		}
		g[4], g[5] = p+7, e
		if te > ts {
			g[6], g[7] = ts, te
		}
		return end, true
	}
	return 0, false
}

var inlineAnchorRx = newScanner(`(\\)?(?:\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\]))`, "", 5,
	func(s string, from int, c []int) bool {
		var g [8]int
		for i := from; i < len(s); i++ {
			ch := s[i]
			p := i
			if ch == '\\' {
				p++
			} else if ch != '[' && ch != 'a' {
				continue
			}
			end, ok := tryInlineAnchor(s, p, &g)
			if !ok {
				continue
			}
			set2(c, 0, i, end)
			if p > i {
				set2(c, 1, i, p)
			}
			copy(c[4:], g[:])
			return true
		}
		return false
	})

var inlineAnchorScanRx = newScanner(`(?:^|[^\\\[])\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|(?:^|[^\\])anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\])`, "", 4,
	func(s string, from int, c []int) bool {
		var g [8]int
		try := func(i, p int) bool {
			end, ok := tryInlineAnchor(s, p, &g)
			if !ok {
				return false
			}
			set2(c, 0, i, end)
			copy(c[2:], g[:])
			return true
		}
		for j := from; j < len(s); j++ {
			ch := s[j]
			if ch != '[' && ch != 'a' {
				continue
			}
			if !(ch == '[' && j+1 < len(s) && s[j+1] == '[') && !(ch == 'a' && strings.HasPrefix(s[j:], "anchor:")) {
				continue
			}
			if j > from {
				_, sz := runeBefore(s, j)
				if i := j - sz; i >= from && validStart(s, i) && s[j-1] != '\\' && !(ch == '[' && s[j-1] == '[') {
					if try(i, j) {
						return true
					}
				}
			}
			if isBOL(s, j) && try(j, j) {
				return true
			}
		}
		return false
	})

var inlineEmailRx = newScanner(`([\\>:/])?\p{Word}(?:&amp;|[\p{Word}\-.%+])*@\p{Alnum}[\p{Alnum}_\-.]*\.[a-zA-Z]{2,5}\b`, "", 1,
	func(s string, from int, c []int) bool {
		for a := from; a < len(s); a++ {
			j := strings.IndexByte(s[a:], '@')
			if j < 0 {
				return false
			}
			a += j
			// domain
			ok, sz := alnumAt(s, a+1)
			if !ok {
				continue
			}
			r2 := a + 1 + sz
			for r2 < len(s) {
				if ch := s[r2]; ch == '_' || ch == '-' || ch == '.' {
					r2++
				} else if ok, sz := alnumAt(s, r2); ok {
					r2 += sz
				} else {
					break
				}
			}
			end := -1
			for k := r2 - 1; k > a+1 && end < 0; k-- {
				if s[k] != '.' {
					continue
				}
				n := 0
				for n < 5 && k+1+n < len(s) && isASCIILetter(s[k+1+n]) {
					n++
				}
				for ; n >= 2; n-- {
					if !isWordAt(s, k+1+n) {
						end = k + 1 + n
						break
					}
				}
			}
			if end < 0 {
				continue
			}
			// local part: walk back over the run of allowed tokens
			l := a
			for l > from {
				if l-5 >= from && s[l-5:l] == "&amp;" {
					l -= 5
					continue
				}
				ch := s[l-1]
				if ch == '-' || ch == '.' || ch == '%' || ch == '+' {
					l--
					continue
				}
				r, sz := runeBefore(s, l)
				if !isWordRune(r) || l-sz < from {
					break
				}
				l -= sz
			}
			// the match starts at the first word character token
			st := -1
			for p := l; p < a; {
				if strings.HasPrefix(s[p:], "&amp;") {
					p += 5
					continue
				}
				ok, sz := wordAt(s, p)
				if ok && validStart(s, p) {
					st = p
					break
				}
				p += sz
			}
			if st < 0 {
				continue
			}
			if st == l && st > from {
				if ch := s[st-1]; ch == '\\' || ch == '>' || ch == ':' || ch == '/' {
					set2(c, 1, st-1, st)
					st--
				}
			}
			set2(c, 0, st, end)
			return true
		}
		return false
	})

var inlineFootnoteMacroRx = newScanner(`\\?footnote(?:(ref):|:([\p{Word}-]+)?)\[(?:|(.*?[^\\]))\](?!</a>)`, "m", 3,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], "footnote")
			if j < 0 {
				return false
			}
			i += j
			p := i + 8
			g1, g2s, g2e := false, -1, -1
			if strings.HasPrefix(s[p:], "ref:") {
				g1 = true
				p += 4
			} else if p < len(s) && s[p] == ':' {
				p++
				e := p
				for e < len(s) {
					if s[e] == '-' {
						e++
					} else if ok, sz := wordAt(s, e); ok {
						e += sz
					} else {
						break
					}
				}
				if e > p {
					g2s, g2e = p, e
					p = e
				}
			} else {
				continue
			}
			if p >= len(s) || s[p] != '[' {
				continue
			}
			for k := p + 1; k < len(s); k++ {
				j := strings.IndexByte(s[k:], ']')
				if j < 0 {
					break
				}
				k += j
				if k > p+1 && s[k-1] == '\\' {
					continue
				}
				if strings.HasPrefix(s[k+1:], "</a>") {
					continue
				}
				set2(c, 0, escStart(s, i, from), k+1)
				if g1 {
					set2(c, 1, i+8, i+11)
				}
				set2(c, 2, g2s, g2e)
				if k > p+1 {
					set2(c, 3, p+1, k)
				}
				return true
			}
		}
		return false
	})

var inlineImageMacroRx = newScanner(`\\?i(?:mage|con):([^:\s\[](?:[^\n\[]*[^\s\[])?)\[(|.*?[^\\])\]`, "m", 2,
	func(s string, from int, c []int) bool {
		for q := from; q < len(s); q++ {
			j := strings.IndexByte(s[q:], ':')
			if j < 0 {
				return false
			}
			q += j
			var i int
			if q-5 >= from && s[q-5:q] == "image" {
				i = q - 5
			} else if q-4 >= from && s[q-4:q] == "icon" {
				i = q - 4
			} else {
				continue
			}
			t := q + 1
			if t >= len(s) || s[t] == ':' || s[t] == '[' || isSpaceByte(s[t]) {
				continue
			}
			_, sz := runeAt(s, t)
			b := t + sz
			for b < len(s) && s[b] != '[' && s[b] != '\n' {
				b++
			}
			if b >= len(s) || s[b] != '[' || (b > t+sz && isSpaceByte(s[b-1])) {
				continue
			}
			ts, te, end, ok := scanBracketText(s, b, true, true)
			if !ok {
				continue
			}
			set2(c, 0, escStart(s, i, from), end)
			set2(c, 1, t, b)
			set2(c, 2, ts, te)
			return true
		}
		return false
	})

var inlineIndextermMacroRx = newScanner(`\\?(?:(indexterm2?):\[(.*?[^\\])\]|\(\((.+?)\)\)(?!\)))`, "m", 3,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			ch := s[i]
			if ch == 'i' {
				if !strings.HasPrefix(s[i:], "indexterm") {
					continue
				}
				p := i + 9
				if p < len(s) && s[p] == '2' {
					p++
				}
				if !strings.HasPrefix(s[p:], ":[") {
					continue
				}
				ts, te, end, ok := scanBracketText(s, p+1, true, false)
				if !ok {
					continue
				}
				set2(c, 0, escStart(s, i, from), end)
				set2(c, 1, i, p)
				set2(c, 2, ts, te)
				return true
			}
			if ch == '(' && i+1 < len(s) && s[i+1] == '(' {
				cs := i + 2
				if cs >= len(s) {
					continue
				}
				_, sz := runeAt(s, cs)
				for e := cs + sz; e+1 < len(s); e++ {
					j := strings.Index(s[e:], "))")
					if j < 0 {
						break
					}
					e += j
					if e+2 < len(s) && s[e+2] == ')' {
						continue
					}
					set2(c, 0, escStart(s, i, from), e+2)
					set2(c, 3, cs, e)
					return true
				}
			}
		}
		return false
	})

var inlineKbdBtnMacroRx = newScanner(`(\\)?(kbd|btn):\[(.*?[^\\])\]`, "m", 3,
	func(s string, from int, c []int) bool {
		for q := from; q < len(s); q++ {
			j := strings.Index(s[q:], ":[")
			if j < 0 {
				return false
			}
			q += j
			i := q - 3
			if i < from || (s[i:q] != "kbd" && s[i:q] != "btn") {
				continue
			}
			ts, te, end, ok := scanBracketText(s, q+1, true, false)
			if !ok {
				continue
			}
			st := escStart(s, i, from)
			set2(c, 0, st, end)
			if st < i {
				set2(c, 1, st, i)
			}
			set2(c, 2, i, q)
			set2(c, 3, ts, te)
			return true
		}
		return false
	})

var inlineLinkMacroRx = newScanner(`\\?(?:link|(mailto)):(|[^:\s\[][^\s\[]*)\[(|.*?[^\\])\]`, "m", 3,
	func(s string, from int, c []int) bool {
		for q := from; q < len(s); q++ {
			j := strings.IndexByte(s[q:], ':')
			if j < 0 {
				return false
			}
			q += j
			var i int
			mailto := false
			if q-4 >= from && s[q-4:q] == "link" {
				i = q - 4
			} else if q-6 >= from && s[q-6:q] == "mailto" {
				i = q - 6
				mailto = true
			} else {
				continue
			}
			t := q + 1
			b := t
			if t < len(s) && s[t] != '[' {
				if s[t] == ':' || isSpaceByte(s[t]) {
					continue
				}
				for b < len(s) && s[b] != '[' && !isSpaceByte(s[b]) {
					b++
				}
			}
			ts, te, end, ok := scanBracketText(s, b, true, true)
			if !ok {
				continue
			}
			set2(c, 0, escStart(s, i, from), end)
			if mailto {
				set2(c, 1, i, q)
			}
			set2(c, 2, t, b)
			set2(c, 3, ts, te)
			return true
		}
		return false
	})

// scanSubsList matches ([a-z]+(?:,[a-z-]+)*)? at p.
func scanSubsList(s string, p int) int {
	e := p
	for e < len(s) && s[e] >= 'a' && s[e] <= 'z' {
		e++
	}
	if e == p {
		return p
	}
	for e < len(s) && s[e] == ',' {
		q := e + 1
		for q < len(s) && ((s[q] >= 'a' && s[q] <= 'z') || s[q] == '-') {
			q++
		}
		if q == e+1 {
			break
		}
		e = q
	}
	return e
}

var inlineStemMacroRx = newScanner(`\\?(stem|(?:latex|ascii)math):([a-z]+(?:,[a-z-]+)*)?\[(.*?[^\\])\]`, "m", 3,
	func(s string, from int, c []int) bool {
		for q := from; q < len(s); q++ {
			j := strings.IndexByte(s[q:], ':')
			if j < 0 {
				return false
			}
			q += j
			var i int
			if q-4 >= from && s[q-4:q] == "stem" {
				i = q - 4
			} else if q-9 >= from && (s[q-9:q] == "latexmath" || s[q-9:q] == "asciimath") {
				i = q - 9
			} else {
				continue
			}
			e := scanSubsList(s, q+1)
			ts, te, end, ok := scanBracketText(s, e, true, false)
			if !ok {
				continue
			}
			set2(c, 0, escStart(s, i, from), end)
			set2(c, 1, i, q)
			if e > q+1 {
				set2(c, 2, q+1, e)
			}
			set2(c, 3, ts, te)
			return true
		}
		return false
	})

var inlineMenuMacroRx = newScanner(`\\?menu:(\p{Word}|[\p{Word}&][^\n\[]*[^\s\[])\[ *(?:|(.*?[^\\]))\]`, "m", 2,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.Index(s[i:], "menu:")
			if j < 0 {
				return false
			}
			i += j
			t := i + 5
			if t >= len(s) {
				return false
			}
			isWord, sz := wordAt(s, t)
			if !isWord && s[t] != '&' {
				continue
			}
			b := t + sz
			for b < len(s) && s[b] != '[' && s[b] != '\n' {
				b++
			}
			if b >= len(s) || s[b] != '[' {
				continue
			}
			if b == t+sz {
				if !isWord {
					continue
				}
			} else if isSpaceByte(s[b-1]) {
				continue
			}
			q := skipSpaces(s, b+1)
			gs, ge := -1, -1
			var end int
			if q < len(s) && s[q] == ']' {
				end = q + 1
			} else if k := findClose(s, q, true); k >= 0 {
				gs, ge, end = q, k, k+1
			} else {
				continue
			}
			set2(c, 0, escStart(s, i, from), end)
			set2(c, 1, t, b)
			set2(c, 2, gs, ge)
			return true
		}
		return false
	})

var inlineMenuRx = newScanner(`\\?"([\p{Word}&][^"]*?[ \n]+&gt;[ \n]+[^"]*)"`, "", 1,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			j := strings.IndexByte(s[i:], '"')
			if j < 0 {
				return false
			}
			i += j
			t := i + 1
			if t >= len(s) {
				return false
			}
			isWord, sz := wordAt(s, t)
			if !isWord && s[t] != '&' {
				continue
			}
			k := strings.IndexByte(s[t:], '"')
			if k < 0 {
				return false
			}
			e := t + k
			found := false
			body := s[t+sz : e]
			for x := 0; x < len(body); x++ {
				g := strings.Index(body[x:], "&gt;")
				if g < 0 {
					break
				}
				x += g
				if x > 0 && (body[x-1] == ' ' || body[x-1] == '\n') && x+4 < len(body) && (body[x+4] == ' ' || body[x+4] == '\n') {
					found = true
					break
				}
			}
			if !found {
				continue
			}
			set2(c, 0, escStart(s, i, from), e+1)
			set2(c, 1, t, e)
			return true
		}
		return false
	})

// isXrefTargetStart reports whether [\p{Word}#/.:{] matches at i.
func isXrefTargetStart(s string, i int) (bool, int) {
	if i >= len(s) {
		return false, 0
	}
	switch s[i] {
	case '#', '/', '.', ':', '{':
		return true, 1
	}
	return wordAt(s, i)
}

var inlineXrefMacroRx = newScanner(`\\?(?:&lt;&lt;([\p{Word}#/.:{].*?)&gt;&gt;|xref:([\p{Word}#/.:{].*?)\[(?:\]|(.*?[^\\])\]))`, "m", 3,
	func(s string, from int, c []int) bool {
		for i := from; i < len(s); i++ {
			ch := s[i]
			if ch == '&' {
				if !strings.HasPrefix(s[i:], "&lt;&lt;") {
					continue
				}
				t := i + 8
				ok, sz := isXrefTargetStart(s, t)
				if !ok {
					continue
				}
				j := strings.Index(s[t+sz:], "&gt;&gt;")
				if j < 0 {
					continue
				}
				e := t + sz + j
				set2(c, 0, escStart(s, i, from), e+8)
				set2(c, 1, t, e)
				return true
			}
			if ch == 'x' {
				if !strings.HasPrefix(s[i:], "xref:") {
					continue
				}
				t := i + 5
				ok, sz := isXrefTargetStart(s, t)
				if !ok {
					continue
				}
				j := strings.IndexByte(s[t+sz:], '[')
				if j < 0 {
					continue
				}
				b := t + sz + j
				ts, te, end, ok := scanBracketText(s, b, true, true)
				if !ok {
					continue
				}
				set2(c, 0, escStart(s, i, from), end)
				set2(c, 2, t, b)
				if te > ts {
					set2(c, 3, ts, te)
				}
				return true
			}
		}
		return false
	})
