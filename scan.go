package adocgo

import (
	"strings"
	"unicode/utf8"
)

// A scanner finds occurrences of one syntactic construct in a string. Each
// scanner is a hand-written replacement for one of the regular expressions
// in Asciidoctor's rx.rb and reports its match the same way the regular
// expression would: as byte offsets of the whole match and of every capture
// group.
type scanner struct {
	// pat and flags are the regular expression (Ruby syntax; flag "m" makes
	// the dot match newlines) that the scanner replaces. They document the
	// scanner and serve as the oracle in the differential tests.
	pat, flags string
	// hand reports whether find is hand-written (as opposed to delegating
	// to the regular expression engine).
	hand bool
	// n is the number of capture groups.
	n int
	// find locates the first match that starts at or after from. c has
	// 2*(n+1) entries, all preset to -1; on success find stores the start
	// and end offset of the match in c[0], c[1] and of group i in c[2*i],
	// c[2*i+1] (leaving -1 for groups that did not participate).
	find func(s string, from int, c []int) bool
	// at, if set, matches anchored at pos (like StringScanner#scan).
	at func(s string, pos int, c []int) bool
}

// scanCheck, when set by a test, is called with the outcome of every scan so
// that it can be compared with the reference regular expression.
var scanCheck func(sc *scanner, s string, from int, anchored bool, c []int, ok bool)

const maxCaps = 18

// rmatch is the result of a successful scan.
type rmatch struct {
	s string
	c [maxCaps]int
}

// Has reports whether group i participated in the match.
func (m *rmatch) Has(i int) bool { return m.c[2*i] >= 0 }

// Group returns the text of group i ("" if it did not participate).
func (m *rmatch) Group(i int) string {
	if m.c[2*i] < 0 {
		return ""
	}
	return m.s[m.c[2*i]:m.c[2*i+1]]
}

func (m *rmatch) Start(i int) int { return m.c[2*i] }
func (m *rmatch) End(i int) int   { return m.c[2*i+1] }
func (m *rmatch) Pre() string     { return m.s[:m.c[0]] }
func (m *rmatch) Post() string    { return m.s[m.c[1]:] }

func (sc *scanner) run(m *rmatch, s string, from int) bool {
	c := m.c[:2*(sc.n+1)]
	for i := range c {
		c[i] = -1
	}
	ok := sc.find(s, from, c)
	if scanCheck != nil {
		scanCheck(sc, s, from, false, c, ok)
	}
	m.s = s
	return ok
}

// Find returns the first match in s or nil.
func (sc *scanner) Find(s string) *rmatch { return sc.FindAt(s, 0) }

// FindAt returns the first match in s starting at or after from, or nil.
func (sc *scanner) FindAt(s string, from int) *rmatch {
	var m rmatch
	if sc.run(&m, s, from) {
		return &m
	}
	return nil
}

// MatchString reports whether s contains a match.
func (sc *scanner) MatchString(s string) bool {
	var m rmatch
	return sc.run(&m, s, 0)
}

// MatchAt matches anchored at pos.
func (sc *scanner) MatchAt(s string, pos int) *rmatch {
	var m rmatch
	c := m.c[:2*(sc.n+1)]
	for i := range c {
		c[i] = -1
	}
	ok := sc.at(s, pos, c)
	if scanCheck != nil {
		scanCheck(sc, s, pos, true, c, ok)
	}
	if !ok {
		return nil
	}
	m.s = s
	return &m
}

// FindAll returns all successive non-overlapping matches.
func (sc *scanner) FindAll(s string) []*rmatch {
	var out []*rmatch
	pos := 0
	for pos <= len(s) {
		m := sc.FindAt(s, pos)
		if m == nil {
			break
		}
		out = append(out, m)
		if m.c[1] == m.c[0] {
			if m.c[1] >= len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[m.c[1]:])
			pos = m.c[1] + sz
		} else {
			pos = m.c[1]
		}
	}
	return out
}

// ReplaceAllFunc replaces every match with the result of f (Ruby gsub).
func (sc *scanner) ReplaceAllFunc(s string, f func(m *rmatch) string) string {
	var m rmatch
	if !sc.run(&m, s, 0) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	last := 0
	for {
		b.WriteString(s[last:m.c[0]])
		b.WriteString(f(&m))
		last = m.c[1]
		pos := last
		if m.c[1] == m.c[0] {
			if pos >= len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[pos:])
			b.WriteString(s[pos : pos+sz])
			pos += sz
			last = pos
		}
		if !sc.run(&m, s, pos) {
			break
		}
	}
	b.WriteString(s[last:])
	return b.String()
}

// ReplaceAll replaces every match with the literal string repl.
func (sc *scanner) ReplaceAll(s, repl string) string {
	return sc.ReplaceAllFunc(s, func(*rmatch) string { return repl })
}

// ReplaceFirstFunc replaces the first match with the result of f (Ruby sub).
func (sc *scanner) ReplaceFirstFunc(s string, f func(m *rmatch) string) string {
	m := sc.FindAt(s, 0)
	if m == nil {
		return s
	}
	return s[:m.c[0]] + f(m) + s[m.c[1]:]
}

// newScanner declares a hand-written scanner for the given pattern with n
// capture groups.
func newScanner(pat, flags string, n int, find func(s string, from int, c []int) bool) *scanner {
	return &scanner{pat: pat, flags: flags, n: n, find: find, hand: true}
}
