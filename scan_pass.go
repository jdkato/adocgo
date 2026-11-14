package adocgo

import "strings"

// Scanners for URLs and inline passthroughs. These replace the most
// intricate regular expressions (alternations with backreferences and
// captures inside lookaheads), so each alternative is tried in the same
// order as the regular expression engine would.

var inlineLinkRx = newScanner(`(^|link:|\p{Blank}|\\?&lt;()|[>\(\)\[\];"'])(\\?(?:https?|file|ftp|irc)://)(?:([^\s\[\]]+)\[(|.*?[^\\])\]|\2([^\s]+?)&gt;|([^\s\[\]<]*([^\s,.?!\[\]<\)])))`, "m", 8,
	func(s string, from int, c []int) bool {
		for k := from; k < len(s); k++ {
			j := strings.Index(s[k:], "://")
			if j < 0 {
				return false
			}
			k += j
			head := s[:k]
			var sch int
			switch {
			case strings.HasSuffix(head, "http"), strings.HasSuffix(head, "file"):
				sch = k - 4
			case strings.HasSuffix(head, "https"):
				sch = k - 5
			case strings.HasSuffix(head, "ftp"), strings.HasSuffix(head, "irc"):
				sch = k - 3
			default:
				continue
			}
			if sch < from {
				continue
			}
			u0 := sch
			if sch > from && s[sch-1] == '\\' {
				u0 = sch - 1
			}
			u := k + 3
			// the alternatives for the leading group, by ascending start
			var starts [4]int
			var angle [4]bool
			n := 0
			if u0-5 >= from {
				if pre := s[u0-5 : u0]; pre == "link:" {
					starts[n] = u0 - 5
					n++
				} else if pre == `\&lt;` {
					starts[n], angle[n] = u0-5, true
					n++
				}
			}
			if u0-4 >= from && s[u0-4:u0] == "&lt;" {
				starts[n], angle[n] = u0-4, true
				n++
			}
			if u0 > from {
				r, sz := runeBefore(s, u0)
				if u0-sz >= from && validStart(s, u0-sz) && (isBlankRune(r) || (r < 0x80 && strings.IndexByte(`>()[];"'`, byte(r)) >= 0)) {
					starts[n] = u0 - sz
					n++
				}
			}
			if isBOL(s, u0) {
				starts[n] = u0
				n++
			}
			for x := 0; x < n; x++ {
				// 1st form: target[text]
				r := u
				for r < len(s) && !isSpaceByte(s[r]) && s[r] != '[' && s[r] != ']' {
					r++
				}
				if r > u && r < len(s) && s[r] == '[' {
					if ts, te, end, ok := scanBracketText(s, r, true, true); ok {
						set2(c, 0, starts[x], end)
						set2(c, 4, u, r)
						set2(c, 5, ts, te)
						goto matched
					}
				}
				// 2nd form: <target> (only after &lt;)
				if angle[x] && u < len(s) {
					_, sz := runeAt(s, u)
					if j := strings.Index(s[u+sz:], "&gt;"); j >= 0 {
						e := u + sz + j
						clean := true
						for y := u; y < e; y++ {
							if isSpaceByte(s[y]) {
								clean = false
								break
							}
						}
						if clean {
							set2(c, 0, starts[x], e+4)
							set2(c, 6, u, e)
							goto matched
						}
					}
				}
				// 3rd form: bare target
				{
					r := u
					for r < len(s) && !isSpaceByte(s[r]) && s[r] != '[' && s[r] != ']' && s[r] != '<' {
						r++
					}
					e := r
					for e > u {
						if ch := s[e-1]; ch == ',' || ch == '.' || ch == '?' || ch == '!' || ch == ')' {
							e--
						} else {
							break
						}
					}
					if e > u {
						_, sz := runeBefore(s, e)
						if e-sz < u {
							sz = e - u
						}
						set2(c, 0, starts[x], e)
						set2(c, 7, u, e)
						set2(c, 8, e-sz, e)
						goto matched
					}
				}
				continue
			matched:
				set2(c, 1, starts[x], u0)
				if angle[x] {
					set2(c, 2, u0, u0)
				}
				set2(c, 3, u0, u)
				return true
			}
		}
		return false
	})

var inlinePassMacroRx = newScanner(`(?:(?:(\\?)\[([^\]]+)\])?(\\{0,2})(\+\+\+?|\$\$)(.*?)\4|(\\?)pass:([a-z]+(?:,[a-z-]+)*)?\[(|.*?[^\\])\])`, "m", 8,
	func(s string, from int, c []int) bool {
		// core matches (\\{0,2})(\+\+\+?|\$\$)(.*?)\4 at q
		core := func(q int) bool {
			bs := q
			for bs < len(s) && bs-q < 2 && s[bs] == '\\' {
				bs++
			}
			rest := s[bs:]
			for _, b := range [...]string{"+++", "++", "$$"} {
				if !strings.HasPrefix(rest, b) {
					continue
				}
				if j := strings.Index(rest[len(b):], b); j >= 0 {
					cs := bs + len(b)
					set2(c, 3, q, bs)
					set2(c, 4, bs, cs)
					set2(c, 5, cs, cs+j)
					c[1] = cs + j + len(b)
					return true
				}
			}
			return false
		}
		for i := from; i < len(s); i++ {
			ch := s[i]
			switch ch {
			case '\\', '[', '+', '$', 'p':
			default:
				continue
			}
			// first alternative
			if ch != 'p' {
				b := -1
				if ch == '[' {
					b = i
				} else if ch == '\\' && i+1 < len(s) && s[i+1] == '[' {
					b = i + 1
				}
				if b >= 0 {
					if j := strings.IndexByte(s[b+1:], ']'); j > 0 {
						r := b + 1 + j
						if core(r + 1) {
							c[0] = i
							set2(c, 1, i, b)
							set2(c, 2, b+1, r)
							return true
						}
					}
				}
				if ch != '[' && core(i) {
					c[0] = i
					return true
				}
			}
			// second alternative
			if ch == 'p' || ch == '\\' {
				p := i
				if ch == '\\' {
					p++
				}
				if strings.HasPrefix(s[p:], "pass:") {
					e := scanSubsList(s, p+5)
					if ts, te, end, ok := scanBracketText(s, e, true, true); ok {
						set2(c, 0, i, end)
						set2(c, 6, i, p)
						if e > p+5 {
							set2(c, 7, p+5, e)
						}
						set2(c, 8, ts, te)
						return true
					}
				}
			}
		}
		return false
	})

var inlinePassRx = newScanner(`((?:^|[^\p{Word};:\\])(?=(\[)|\+)|\\(?=\[)|(?=\\\+))(?:\2(x-|[^\]]+ x-)\]|(?:\[([^\]]+)\])?(?=(\\)?\+))(\5?(\+|`+"`"+`)(\S|\S.*?\S)\7)(?!\p{Word})`, "m", 8,
	func(s string, from int, c []int) bool {
		// quoted matches (\5?(\+|`)(\S|\S.*?\S)\7)(?!\p{Word}) at q, where
		// esc tells whether group 5 (a backslash) is set
		quoted := func(q int, esc bool) bool {
			m := q
			if esc {
				m++
			}
			if m >= len(s) || (s[m] != '+' && s[m] != '`') {
				return false
			}
			e := scanConstrainedBody(s, m+1, s[m:m+1], "")
			if e < 0 {
				return false
			}
			set2(c, 6, q, e+1)
			set2(c, 7, m, m+1)
			set2(c, 8, m+1, e)
			c[1] = e + 1
			return true
		}
		// rest matches everything after group 1 at p; viaBracket tells
		// whether group 2 was captured by the lookahead
		rest := func(p int, viaBracket bool) bool {
			if viaBracket {
				if j := strings.IndexByte(s[p+1:], ']'); j > 0 {
					r := p + 1 + j
					attrs := s[p+1 : r]
					if (attrs == "x-" || (len(attrs) > 3 && strings.HasSuffix(attrs, " x-"))) && quoted(r+1, false) {
						set2(c, 3, p+1, r)
						return true
					}
				}
			}
			q := p
			as, ae := -1, -1
			if p < len(s) && s[p] == '[' {
				j := strings.IndexByte(s[p+1:], ']')
				if j <= 0 {
					return false
				}
				as, ae = p+1, p+1+j
				q = ae + 1
			}
			esc := false
			if q+1 < len(s) && s[q] == '\\' && s[q+1] == '+' {
				esc = true
			} else if q >= len(s) || s[q] != '+' {
				return false
			}
			if !quoted(q, esc) {
				return false
			}
			set2(c, 4, as, ae)
			if esc {
				set2(c, 5, q, q+1)
			}
			return true
		}
		for j := from; j < len(s); j++ {
			ch := s[j]
			if ch == '\\' {
				if j+1 < len(s) && s[j+1] == '[' {
					if rest(j+1, false) {
						c[0] = j
						set2(c, 1, j, j+1)
						return true
					}
				} else if j+1 < len(s) && s[j+1] == '+' {
					if rest(j, false) {
						c[0] = j
						set2(c, 1, j, j)
						return true
					}
				}
				continue
			}
			if ch != '[' && ch != '+' {
				continue
			}
			if j > from {
				r, sz := runeBefore(s, j)
				if i := j - sz; i >= from && validStart(s, i) && !isWordRune(r) && r != ';' && r != ':' && r != '\\' {
					if rest(j, ch == '[') {
						c[0] = i
						set2(c, 1, i, j)
						if ch == '[' {
							set2(c, 2, j, j+1)
						}
						return true
					}
				}
			}
			if isBOL(s, j) && rest(j, ch == '[') {
				c[0] = j
				set2(c, 1, j, j)
				if ch == '[' {
					set2(c, 2, j, j+1)
				}
				return true
			}
		}
		return false
	})

var inlinePassCompatRx = newScanner("(^|[^`\\p{Word}])(?:(\\Z)()|\\[([^\\]]+)\\](?=(\\\\))?)?(\\5?(`)([^`\\s]|[^`\\s].*?\\S)\\7)(?![`\\p{Word}])", "m", 8,
	func(s string, from int, c []int) bool {
		quoted := func(q int, esc bool) bool {
			m := q
			if esc {
				m++
			}
			if m >= len(s) || s[m] != '`' {
				return false
			}
			cs := m + 1
			if cs >= len(s) || s[cs] == '`' {
				return false
			}
			e := scanConstrainedBody(s, cs, "`", "`")
			if e < 0 {
				return false
			}
			set2(c, 6, q, e+1)
			set2(c, 7, m, m+1)
			set2(c, 8, cs, e)
			c[1] = e + 1
			return true
		}
		rest := func(p int) bool {
			if p < len(s) && s[p] == '[' {
				if j := strings.IndexByte(s[p+1:], ']'); j > 0 {
					r := p + 1 + j
					esc := r+1 < len(s) && s[r+1] == '\\'
					if quoted(r+1, esc) {
						set2(c, 4, p+1, r)
						if esc {
							set2(c, 5, r+1, r+2)
						}
						return true
					}
				}
				return false
			}
			return quoted(p, false)
		}
		for j := from; j < len(s); j++ {
			ch := s[j]
			if ch != '[' && ch != '`' {
				continue
			}
			if j > from {
				r, sz := runeBefore(s, j)
				if i := j - sz; i >= from && validStart(s, i) && !isWordRune(r) && r != '`' {
					if rest(j) {
						c[0] = i
						set2(c, 1, i, j)
						return true
					}
				}
			}
			if isBOL(s, j) && rest(j) {
				c[0] = j
				set2(c, 1, j, j)
				return true
			}
		}
		return false
	})
