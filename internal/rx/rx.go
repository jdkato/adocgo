// Package rx implements a small backtracking regular expression engine that
// understands the subset of Ruby (Onigmo) syntax used by Asciidoctor,
// including backreferences and lookahead assertions, which Go's RE2-based
// regexp package cannot express.
//
// Semantics follow Ruby: ^ and $ are always line anchors, the "m" flag makes
// the dot match newlines, \w \d \s are ASCII-only, and \p{Word}, \p{Alpha},
// \p{Alnum} and \p{Blank} are Unicode-aware.
package rx

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type opcode uint8

const (
	opChar opcode = iota
	opStr
	opClass
	opAny   // any char except newline
	opAnyNL // any char
	opSplit // try x first, then y
	opJmp
	opSave
	opBOL
	opEOL
	opBOT
	opEOT
	opEOTZ
	opWordB
	opNWordB
	opBackref
	opLook    // lookahead: x = sub program index, neg flag
	opMark    // record position for empty-loop check
	opChkMark // fail if position unchanged since mark
	opMatch
)

type inst struct {
	op   opcode
	c    byte
	s    string
	cls  *class
	x, y int
	neg  bool
}

type class struct {
	ascii  [128]bool
	ranges []rune // pairs lo, hi (non-ASCII part)
	fns    []func(rune) bool
	neg    bool
}

func (c *class) matches(r rune) bool {
	var ok bool
	if r < 128 {
		ok = c.ascii[r]
	} else {
		for i := 0; i < len(c.ranges); i += 2 {
			if r >= c.ranges[i] && r <= c.ranges[i+1] {
				ok = true
				break
			}
		}
		if !ok {
			for _, f := range c.fns {
				if f(r) {
					ok = true
					break
				}
			}
		}
	}
	return ok != c.neg
}

// Regexp is a compiled regular expression.
type Regexp struct {
	src    string
	progs  [][]inst // progs[0] is the main program
	ncap   int      // number of capture groups (excluding group 0)
	nslots int
	first  *[256]bool
	bot    bool // pattern can only match at start of text
	// memoOK reports whether failed (split, position) states of the main
	// program may be memoized: true when the pattern has no backreferences
	// and no empty-loop guards, so the outcome of a state depends only on
	// the program counter and the input position.
	memoOK  bool
	splitID []int // dense index of each split instruction in progs[0], or -1
	nsplits int
	bol     bool // pattern can only match at start of a line
}

func (re *Regexp) String() string { return re.src }

// NumGroups returns the number of capture groups.
func (re *Regexp) NumGroups() int { return re.ncap }

// ---------------------------------------------------------------------------
// Parser

type nodeKind uint8

const (
	nEmpty nodeKind = iota
	nChar
	nClass
	nAny
	nSeq
	nAlt
	nRepeat
	nGroup // capturing
	nAssert
	nBackref
	nLook
)

type node struct {
	kind     nodeKind
	c        byte
	cls      *class
	subs     []*node
	min, max int // repeat; max -1 = inf
	lazy     bool
	idx      int // group index / backref index
	op       opcode
	neg      bool
}

type parser struct {
	src    string
	pos    int
	ncap   int
	dotall bool
	icase  bool
}

// MustCompile compiles pattern with the given flags ("m" dotall, "i"
// case-insensitive) and panics on error.
func MustCompile(pattern string, flags ...string) *Regexp {
	re, err := Compile(pattern, flags...)
	if err != nil {
		panic(err)
	}
	return re
}

// Compile compiles pattern.
func Compile(pattern string, flags ...string) (re *Regexp, err error) {
	p := &parser{src: pattern}
	for _, f := range flags {
		if strings.Contains(f, "m") {
			p.dotall = true
		}
		if strings.Contains(f, "i") {
			p.icase = true
		}
	}
	defer func() {
		if r := recover(); r != nil {
			re = nil
			err = fmt.Errorf("rx: %v in %q", r, pattern)
		}
	}()
	n := p.parseAlt()
	if p.pos < len(p.src) {
		panic("unexpected )")
	}
	re = &Regexp{src: pattern, ncap: p.ncap}
	c := &compiler{re: re}
	re.progs = append(re.progs, nil)
	c.nslots = 2 * (p.ncap + 1)
	prog := c.compile(&node{kind: nGroup, idx: 0, subs: []*node{n}})
	prog = append(prog, inst{op: opMatch})
	re.progs[0] = prog
	re.nslots = c.nslots
	re.memoOK = true
	for _, pr := range re.progs {
		for _, in := range pr {
			if in.op == opBackref || in.op == opChkMark {
				re.memoOK = false
			}
		}
	}
	if re.memoOK {
		re.splitID = make([]int, len(prog))
		for i, in := range prog {
			re.splitID[i] = -1
			if in.op == opSplit {
				re.splitID[i] = re.nsplits
				re.nsplits++
			}
		}
	}
	var set [256]bool
	if !firstSet(n, &set) {
		re.first = &set
	}
	re.bot, re.bol = leadAnchor(n)
	return re, nil
}

func leadAnchor(n *node) (bot, bol bool) {
	switch n.kind {
	case nAssert:
		return n.op == opBOT, n.op == opBOL
	case nSeq:
		if len(n.subs) > 0 {
			return leadAnchor(n.subs[0])
		}
	case nGroup:
		return leadAnchor(n.subs[0])
	case nAlt:
		bot, bol = true, true
		for _, s := range n.subs {
			a, b := leadAnchor(s)
			bot = bot && a
			bol = bol && b
		}
		return
	}
	return false, false
}

// firstSet adds the bytes that can start a match of n to set and reports
// whether n can match the empty string (or its first byte is unknown).
func firstSet(n *node, set *[256]bool) bool {
	switch n.kind {
	case nEmpty, nAssert, nLook:
		return true
	case nBackref:
		for i := range set {
			set[i] = true
		}
		return true
	case nChar:
		set[n.c] = true
		return false
	case nAny:
		for i := range set {
			set[i] = true
		}
		return false
	case nClass:
		for i := 0; i < 128; i++ {
			if n.cls.ascii[i] != n.cls.neg {
				set[i] = true
			}
		}
		for i := 128; i < 256; i++ {
			set[i] = true
		}
		return false
	case nSeq:
		for _, s := range n.subs {
			if !firstSet(s, set) {
				return false
			}
		}
		return true
	case nAlt:
		nullable := false
		for _, s := range n.subs {
			if firstSet(s, set) {
				nullable = true
			}
		}
		return nullable
	case nRepeat:
		e := firstSet(n.subs[0], set)
		return e || n.min == 0
	case nGroup:
		return firstSet(n.subs[0], set)
	}
	return true
}

func (p *parser) more() bool { return p.pos < len(p.src) }
func (p *parser) peek() byte { return p.src[p.pos] }

func (p *parser) parseAlt() *node {
	alts := []*node{p.parseSeq()}
	for p.more() && p.peek() == '|' {
		p.pos++
		alts = append(alts, p.parseSeq())
	}
	if len(alts) == 1 {
		return alts[0]
	}
	return &node{kind: nAlt, subs: alts}
}

func (p *parser) parseSeq() *node {
	var items []*node
	for p.more() {
		c := p.peek()
		if c == '|' || c == ')' {
			break
		}
		atom := p.parseAtom()
		atom = p.parseQuant(atom)
		items = append(items, atom)
	}
	if len(items) == 0 {
		return &node{kind: nEmpty}
	}
	if len(items) == 1 {
		return items[0]
	}
	return &node{kind: nSeq, subs: items}
}

func (p *parser) parseQuant(atom *node) *node {
	for p.more() {
		c := p.peek()
		min, max := 0, -1
		switch c {
		case '*':
			p.pos++
		case '+':
			min = 1
			p.pos++
		case '?':
			max = 1
			p.pos++
		case '{':
			save := p.pos
			p.pos++
			n1, ok1 := p.parseInt()
			if !ok1 {
				p.pos = save
				return atom
			}
			min, max = n1, n1
			if p.more() && p.peek() == ',' {
				p.pos++
				n2, ok2 := p.parseInt()
				if ok2 {
					max = n2
				} else {
					max = -1
				}
			}
			if !p.more() || p.peek() != '}' {
				p.pos = save
				return atom
			}
			p.pos++
		default:
			return atom
		}
		lazy := false
		if p.more() && p.peek() == '?' {
			lazy = true
			p.pos++
		}
		atom = &node{kind: nRepeat, subs: []*node{atom}, min: min, max: max, lazy: lazy}
	}
	return atom
}

func (p *parser) parseInt() (int, bool) {
	start := p.pos
	n := 0
	for p.more() && p.peek() >= '0' && p.peek() <= '9' {
		n = n*10 + int(p.peek()-'0')
		p.pos++
	}
	return n, p.pos > start
}

func (p *parser) literal(r rune) *node {
	if p.icase && unicode.IsLetter(r) {
		cls := &class{}
		addRune(cls, unicode.ToLower(r))
		addRune(cls, unicode.ToUpper(r))
		return &node{kind: nClass, cls: cls}
	}
	if r < 128 {
		return &node{kind: nChar, c: byte(r)}
	}
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], r)
	seq := &node{kind: nSeq}
	for i := 0; i < n; i++ {
		seq.subs = append(seq.subs, &node{kind: nChar, c: buf[i]})
	}
	return seq
}

func addRune(c *class, r rune) {
	if r < 128 {
		c.ascii[r] = true
	} else {
		c.ranges = append(c.ranges, r, r)
	}
}

func addRange(c *class, lo, hi rune) {
	for r := lo; r <= hi && r < 128; r++ {
		c.ascii[r] = true
	}
	if hi >= 128 {
		if lo < 128 {
			lo = 128
		}
		c.ranges = append(c.ranges, lo, hi)
	}
}

func isAlpha(r rune) bool {
	return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Other_Alphabetic, r)
}
func isAlnum(r rune) bool { return isAlpha(r) || unicode.Is(unicode.Nd, r) }
func isWord(r rune) bool {
	if r < 128 {
		return r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r) || unicode.IsMark(r) || unicode.Is(unicode.Nd, r) || unicode.Is(unicode.Pc, r)
}
func isBlank(r rune) bool { return r == '\t' || unicode.Is(unicode.Zs, r) }

func addProp(c *class, name string) {
	var f func(rune) bool
	switch strings.ToLower(name) {
	case "alpha":
		f = isAlpha
	case "alnum":
		f = isAlnum
	case "word":
		f = isWord
	case "blank":
		f = isBlank
	case "digit":
		f = func(r rune) bool { return unicode.Is(unicode.Nd, r) }
	case "space":
		f = unicode.IsSpace
	case "upper":
		f = unicode.IsUpper
	case "lower":
		f = unicode.IsLower
	case "punct":
		f = unicode.IsPunct
	default:
		panic("unknown property " + name)
	}
	for r := rune(0); r < 128; r++ {
		if f(r) {
			c.ascii[r] = true
		}
	}
	c.fns = append(c.fns, f)
}

func addShorthand(c *class, ch byte) bool {
	switch ch {
	case 'd':
		addRange(c, '0', '9')
	case 'w':
		addRange(c, '0', '9')
		addRange(c, 'a', 'z')
		addRange(c, 'A', 'Z')
		addRune(c, '_')
	case 's':
		for _, r := range " \t\r\n\f\v" {
			addRune(c, r)
		}
	case 'h':
		addRange(c, '0', '9')
		addRange(c, 'a', 'f')
		addRange(c, 'A', 'F')
	case 'D', 'W', 'S', 'H':
		sub := &class{}
		addShorthand(sub, ch+32)
		for r := rune(0); r < 128; r++ {
			if !sub.ascii[r] {
				c.ascii[r] = true
			}
		}
		c.ranges = append(c.ranges, 128, unicode.MaxRune)
	default:
		return false
	}
	return true
}

func (p *parser) parseEscapeRune() (rune, bool) {
	// called after the backslash; returns a literal rune if the escape is a literal
	c := p.peek()
	switch c {
	case 'n':
		p.pos++
		return '\n', true
	case 't':
		p.pos++
		return '\t', true
	case 'r':
		p.pos++
		return '\r', true
	case 'f':
		p.pos++
		return '\f', true
	case 'v':
		p.pos++
		return '\v', true
	case 'e':
		p.pos++
		return 0x1b, true
	case 'u':
		p.pos++
		if p.pos+4 > len(p.src) {
			panic("bad \\u escape")
		}
		var r rune
		for i := 0; i < 4; i++ {
			r = r*16 + rune(hexval(p.src[p.pos+i]))
		}
		p.pos += 4
		return r, true
	case 'x':
		p.pos++
		var r rune
		for i := 0; i < 2 && p.more() && hexval(p.peek()) >= 0; i++ {
			r = r*16 + rune(hexval(p.peek()))
			p.pos++
		}
		return r, true
	}
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
		return 0, false
	}
	r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
	p.pos += sz
	return r, true
}

func hexval(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func (p *parser) parseProp() string {
	// at 'p' ; expects p{Name}
	p.pos++
	if !p.more() || p.peek() != '{' {
		panic("bad \\p")
	}
	end := strings.IndexByte(p.src[p.pos:], '}')
	if end < 0 {
		panic("bad \\p")
	}
	name := p.src[p.pos+1 : p.pos+end]
	p.pos += end + 1
	return name
}

func (p *parser) parseAtom() *node {
	c := p.peek()
	switch c {
	case '(':
		p.pos++
		kind := nGroup
		var n *node
		if strings.HasPrefix(p.src[p.pos:], "?:") {
			p.pos += 2
			sub := p.parseAlt()
			p.expect(')')
			return &node{kind: nSeq, subs: []*node{sub}}
		} else if strings.HasPrefix(p.src[p.pos:], "?=") || strings.HasPrefix(p.src[p.pos:], "?!") {
			neg := p.src[p.pos+1] == '!'
			p.pos += 2
			sub := p.parseAlt()
			p.expect(')')
			return &node{kind: nLook, subs: []*node{sub}, neg: neg}
		} else if strings.HasPrefix(p.src[p.pos:], "?<") {
			end := strings.IndexByte(p.src[p.pos:], '>')
			if end < 0 {
				panic("bad group name")
			}
			p.pos += end + 1
		} else if strings.HasPrefix(p.src[p.pos:], "?") {
			panic("unsupported group syntax")
		}
		p.ncap++
		idx := p.ncap
		sub := p.parseAlt()
		p.expect(')')
		n = &node{kind: kind, idx: idx, subs: []*node{sub}}
		return n
	case '[':
		p.pos++
		return &node{kind: nClass, cls: p.parseClass()}
	case '.':
		p.pos++
		if p.dotall {
			return &node{kind: nAny, neg: true}
		}
		return &node{kind: nAny}
	case '^':
		p.pos++
		return &node{kind: nAssert, op: opBOL}
	case '$':
		p.pos++
		return &node{kind: nAssert, op: opEOL}
	case '\\':
		p.pos++
		if !p.more() {
			panic("trailing backslash")
		}
		e := p.peek()
		switch e {
		case 'A':
			p.pos++
			return &node{kind: nAssert, op: opBOT}
		case 'z':
			p.pos++
			return &node{kind: nAssert, op: opEOT}
		case 'Z':
			p.pos++
			return &node{kind: nAssert, op: opEOTZ}
		case 'b':
			p.pos++
			return &node{kind: nAssert, op: opWordB}
		case 'B':
			p.pos++
			return &node{kind: nAssert, op: opNWordB}
		case 'p':
			cls := &class{}
			addProp(cls, p.parseProp())
			return &node{kind: nClass, cls: cls}
		case 'd', 'w', 's', 'h', 'D', 'W', 'S', 'H':
			p.pos++
			cls := &class{}
			addShorthand(cls, e)
			return &node{kind: nClass, cls: cls}
		}
		if e >= '1' && e <= '9' {
			p.pos++
			return &node{kind: nBackref, idx: int(e - '0')}
		}
		r, ok := p.parseEscapeRune()
		if !ok {
			panic(fmt.Sprintf("unsupported escape \\%c", e))
		}
		return p.literal(r)
	}
	r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
	p.pos += sz
	return p.literal(r)
}

func (p *parser) expect(c byte) {
	if !p.more() || p.peek() != c {
		panic(fmt.Sprintf("expected %c", c))
	}
	p.pos++
}

func (p *parser) parseClass() *class {
	cls := &class{}
	if p.more() && p.peek() == '^' {
		cls.neg = true
		p.pos++
	}
	first := true
	for {
		if !p.more() {
			panic("unterminated character class")
		}
		c := p.peek()
		if c == ']' && !first {
			p.pos++
			break
		}
		first = false
		var lo rune
		if c == '[' && strings.HasPrefix(p.src[p.pos:], "[:") {
			end := strings.Index(p.src[p.pos:], ":]")
			if end < 0 {
				panic("bad posix class")
			}
			addProp(cls, p.src[p.pos+2:p.pos+end])
			p.pos += end + 2
			continue
		}
		if c == '\\' {
			p.pos++
			if !p.more() {
				panic("trailing backslash")
			}
			e := p.peek()
			if e == 'p' {
				addProp(cls, p.parseProp())
				continue
			}
			if addShorthand(cls, e) {
				p.pos++
				continue
			}
			if e == 'b' {
				p.pos++
				lo = '\b'
			} else {
				r, ok := p.parseEscapeRune()
				if !ok {
					panic(fmt.Sprintf("unsupported escape \\%c in class", e))
				}
				lo = r
			}
		} else {
			r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
			p.pos += sz
			lo = r
		}
		hi := lo
		if p.pos+1 < len(p.src) && p.peek() == '-' && p.src[p.pos+1] != ']' {
			p.pos++
			if p.peek() == '\\' {
				p.pos++
				r, ok := p.parseEscapeRune()
				if !ok {
					panic("bad range end")
				}
				hi = r
			} else {
				r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
				p.pos += sz
				hi = r
			}
		}
		if p.icase {
			for r := lo; r <= hi; r++ {
				addRune(cls, unicode.ToLower(r))
				addRune(cls, unicode.ToUpper(r))
			}
		} else {
			addRange(cls, lo, hi)
		}
	}
	return cls
}

// ---------------------------------------------------------------------------
// Compiler

type compiler struct {
	re     *Regexp
	nslots int
}

func (c *compiler) compile(n *node) []inst {
	switch n.kind {
	case nEmpty:
		return nil
	case nChar:
		return []inst{{op: opChar, c: n.c}}
	case nClass:
		return []inst{{op: opClass, cls: n.cls}}
	case nAny:
		if n.neg {
			return []inst{{op: opAnyNL}}
		}
		return []inst{{op: opAny}}
	case nAssert:
		return []inst{{op: n.op}}
	case nBackref:
		return []inst{{op: opBackref, x: n.idx}}
	case nSeq:
		var out []inst
		for _, s := range n.subs {
			out = appendProg(out, c.compile(s))
		}
		return mergeChars(out)
	case nGroup:
		out := []inst{{op: opSave, x: 2 * n.idx}}
		out = appendProg(out, c.compile(n.subs[0]))
		out = append(out, inst{op: opSave, x: 2*n.idx + 1})
		return out
	case nLook:
		sub := c.compile(n.subs[0])
		sub = append(sub, inst{op: opMatch})
		c.re.progs = append(c.re.progs, sub)
		return []inst{{op: opLook, x: len(c.re.progs) - 1, neg: n.neg}}
	case nAlt:
		// split L1, L2; L1: a; jmp end; L2: split ...
		var out []inst
		var jmps []int
		for i, s := range n.subs {
			body := c.compile(s)
			if i < len(n.subs)-1 {
				out = append(out, inst{op: opSplit, x: len(out) + 1, y: len(out) + 1 + len(body) + 1})
				out = appendProg(out, body)
				jmps = append(jmps, len(out))
				out = append(out, inst{op: opJmp})
			} else {
				out = appendProg(out, body)
			}
		}
		for _, j := range jmps {
			out[j].x = len(out)
		}
		return out
	case nRepeat:
		var out []inst
		sub := n.subs[0]
		for i := 0; i < n.min; i++ {
			out = appendProg(out, c.compile(sub))
		}
		if n.max == -1 {
			body := c.compile(sub)
			var tmp [256]bool
			nullable := firstSet(sub, &tmp)
			if nullable {
				slot := c.nslots
				c.nslots++
				body = append([]inst{{op: opMark, x: slot}}, shift(body, 1)...)
				body = append(body, inst{op: opChkMark, x: slot})
			}
			// L1: split L2, L3; L2: body; jmp L1; L3:
			start := len(out)
			sp := inst{op: opSplit, x: start + 1, y: start + 1 + len(body) + 1}
			if n.lazy {
				sp.x, sp.y = sp.y, sp.x
			}
			out = append(out, sp)
			out = appendProg(out, body)
			out = append(out, inst{op: opJmp, x: start})
		} else {
			// (body (body (body)?)?)?
			var splits []int
			for i := n.min; i < n.max; i++ {
				splits = append(splits, len(out))
				out = append(out, inst{op: opSplit, x: len(out) + 1})
				out = appendProg(out, c.compile(sub))
			}
			for _, s := range splits {
				out[s].y = len(out)
				if n.lazy {
					out[s].x, out[s].y = out[s].y, out[s].x
				}
			}
		}
		return out
	}
	panic("unreachable")
}

func shift(prog []inst, off int) []inst {
	out := make([]inst, len(prog))
	for i, in := range prog {
		if in.op == opSplit {
			in.x += off
			in.y += off
		} else if in.op == opJmp {
			in.x += off
		}
		out[i] = in
	}
	return out
}

func appendProg(out, body []inst) []inst {
	return append(out, shift(body, len(out))...)
}

// mergeChars is a no-op placeholder kept for clarity; char runs are matched
// one instruction at a time since jump targets may point into them.
func mergeChars(p []inst) []inst { return p }

// ---------------------------------------------------------------------------
// Matcher

type frame struct {
	pc  int // >= 0: backtrack point; -1: capture restore
	pos int // position, or old slot value
	sl  int // slot index for restore
}

type machine struct {
	re    *Regexp
	s     string
	slots []int
	stack []frame
	// memoization of failed states (see Regexp.memoOK); enabled lazily
	// once a scan has backtracked a lot, which keeps the common case cheap
	// while bounding pathological inputs to O(len(prog) * len(s)).
	backtracks int
	memoOn     bool
	memo       []uint64
}

func wordAt(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	if s[i] < 128 {
		return isWord(rune(s[i]))
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return isWord(r)
}

func wordBefore(s string, i int) bool {
	if i <= 0 {
		return false
	}
	if s[i-1] < 128 {
		return isWord(rune(s[i-1]))
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return isWord(r)
}

// run executes program pi starting at pos. It returns the end position of
// the match or -1.
func (m *machine) run(pi int, pos int) int {
	prog := m.re.progs[pi]
	s := m.s
	base := len(m.stack)
	pc := 0
	for {
		in := &prog[pc]
		ok := true
		switch in.op {
		case opChar:
			if pos < len(s) && s[pos] == in.c {
				pos++
				pc++
			} else {
				ok = false
			}
		case opAny:
			if pos < len(s) && s[pos] != '\n' {
				if s[pos] < 128 {
					pos++
				} else {
					_, sz := utf8.DecodeRuneInString(s[pos:])
					pos += sz
				}
				pc++
			} else {
				ok = false
			}
		case opAnyNL:
			if pos < len(s) {
				if s[pos] < 128 {
					pos++
				} else {
					_, sz := utf8.DecodeRuneInString(s[pos:])
					pos += sz
				}
				pc++
			} else {
				ok = false
			}
		case opClass:
			if pos < len(s) {
				if b := s[pos]; b < 128 {
					if in.cls.ascii[b] != in.cls.neg {
						pos++
						pc++
					} else {
						ok = false
					}
				} else {
					r, sz := utf8.DecodeRuneInString(s[pos:])
					if in.cls.matches(r) {
						pos += sz
						pc++
					} else {
						ok = false
					}
				}
			} else {
				ok = false
			}
		case opSplit:
			if m.memoOn && pi == 0 {
				bit := m.re.splitID[pc]*(len(s)+1) + pos
				if m.memo[bit>>6]&(1<<(uint(bit)&63)) != 0 {
					ok = false
					break
				}
				m.memo[bit>>6] |= 1 << (uint(bit) & 63)
			}
			m.stack = append(m.stack, frame{pc: in.y, pos: pos})
			pc = in.x
		case opJmp:
			pc = in.x
		case opSave, opMark:
			m.stack = append(m.stack, frame{pc: -1, pos: m.slots[in.x], sl: in.x})
			m.slots[in.x] = pos
			pc++
		case opChkMark:
			if m.slots[in.x] == pos {
				ok = false
			} else {
				pc++
			}
		case opBOL:
			if pos == 0 || s[pos-1] == '\n' {
				pc++
			} else {
				ok = false
			}
		case opEOL:
			if pos == len(s) || s[pos] == '\n' {
				pc++
			} else {
				ok = false
			}
		case opBOT:
			if pos == 0 {
				pc++
			} else {
				ok = false
			}
		case opEOT:
			if pos == len(s) {
				pc++
			} else {
				ok = false
			}
		case opEOTZ:
			if pos == len(s) || (pos == len(s)-1 && s[pos] == '\n') {
				pc++
			} else {
				ok = false
			}
		case opWordB:
			if wordBefore(s, pos) != wordAt(s, pos) {
				pc++
			} else {
				ok = false
			}
		case opNWordB:
			if wordBefore(s, pos) == wordAt(s, pos) {
				pc++
			} else {
				ok = false
			}
		case opBackref:
			a, b := m.slots[2*in.x], m.slots[2*in.x+1]
			if a < 0 || b < 0 {
				ok = false
			} else {
				sub := s[a:b]
				if strings.HasPrefix(s[pos:], sub) {
					pos += len(sub)
					pc++
				} else {
					ok = false
				}
			}
		case opLook:
			mark := len(m.stack)
			end := m.run(in.x, pos)
			if in.neg {
				if end >= 0 {
					m.unwind(mark)
					ok = false
				} else {
					pc++
				}
			} else if end >= 0 {
				// keep capture restores, drop backtrack points
				j := mark
				for i := mark; i < len(m.stack); i++ {
					if m.stack[i].pc == -1 {
						m.stack[j] = m.stack[i]
						j++
					}
				}
				m.stack = m.stack[:j]
				pc++
			} else {
				ok = false
			}
		case opMatch:
			return pos
		}
		if ok {
			continue
		}
		// backtrack
		for {
			if len(m.stack) == base {
				return -1
			}
			f := m.stack[len(m.stack)-1]
			m.stack = m.stack[:len(m.stack)-1]
			if f.pc == -1 {
				m.slots[f.sl] = f.pos
				continue
			}
			pc, pos = f.pc, f.pos
			if pi == 0 && !m.memoOn && m.re.memoOK {
				m.backtracks++
				if m.backtracks > 4*len(s)+256 {
					m.memoOn = true
					m.memo = make([]uint64, (m.re.nsplits*(len(s)+1))/64+1)
				}
			}
			break
		}
	}
}

func (m *machine) unwind(mark int) {
	for len(m.stack) > mark {
		f := m.stack[len(m.stack)-1]
		m.stack = m.stack[:len(m.stack)-1]
		if f.pc == -1 {
			m.slots[f.sl] = f.pos
		}
	}
}

// Match holds the result of a successful match.
type Match struct {
	s    string
	caps []int
}

// Has reports whether group i participated in the match.
func (m *Match) Has(i int) bool { return 2*i+1 < len(m.caps) && m.caps[2*i] >= 0 }

// Group returns the text of group i ("" if it did not participate).
func (m *Match) Group(i int) string {
	if !m.Has(i) {
		return ""
	}
	return m.s[m.caps[2*i]:m.caps[2*i+1]]
}

// Start returns the start offset of group i.
func (m *Match) Start(i int) int { return m.caps[2*i] }

// End returns the end offset of group i.
func (m *Match) End(i int) int { return m.caps[2*i+1] }

// Pre returns the text before the match.
func (m *Match) Pre() string { return m.s[:m.caps[0]] }

// Post returns the text after the match.
func (m *Match) Post() string { return m.s[m.caps[1]:] }

func (re *Regexp) matchFrom(m *machine, start int) *Match {
	s := m.s
	for i := start; i <= len(s); i++ {
		if re.bot && i > 0 {
			return nil
		}
		if re.bol && i > 0 && s[i-1] != '\n' {
			nl := strings.IndexByte(s[i:], '\n')
			if nl < 0 {
				return nil
			}
			i += nl
			continue
		}
		if re.first != nil {
			if i == len(s) {
				return nil
			}
			if !re.first[s[i]] {
				continue
			}
		}
		if i > 0 && i < len(s) && s[i]&0xC0 == 0x80 {
			continue // not a rune boundary
		}
		for j := range m.slots {
			m.slots[j] = -1
		}
		m.stack = m.stack[:0]
		if end := m.run(0, i); end >= 0 {
			caps := make([]int, 2*(re.ncap+1))
			copy(caps, m.slots)
			// states on the successful path were recorded as visited, so
			// the memo must not be reused for the next scan
			m.memoOn, m.memo, m.backtracks = false, nil, 0
			return &Match{s: s, caps: caps}
		}
	}
	return nil
}

func (re *Regexp) newMachine(s string) *machine {
	return &machine{re: re, s: s, slots: make([]int, re.nslots)}
}

// Find returns the first match in s or nil.
func (re *Regexp) Find(s string) *Match { return re.matchFrom(re.newMachine(s), 0) }

// FindAt returns the first match in s starting at or after offset start.
func (re *Regexp) FindAt(s string, start int) *Match {
	return re.matchFrom(re.newMachine(s), start)
}

// MatchString reports whether s contains a match.
func (re *Regexp) MatchString(s string) bool { return re.Find(s) != nil }

// FindAll returns all successive non-overlapping matches.
func (re *Regexp) FindAll(s string) []*Match {
	var out []*Match
	m := re.newMachine(s)
	pos := 0
	for pos <= len(s) {
		mt := re.matchFrom(m, pos)
		if mt == nil {
			break
		}
		out = append(out, mt)
		if mt.caps[1] == mt.caps[0] {
			if mt.caps[1] >= len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[mt.caps[1]:])
			pos = mt.caps[1] + sz
		} else {
			pos = mt.caps[1]
		}
	}
	return out
}

// ReplaceAllFunc replaces every match with the result of f (Ruby gsub).
func (re *Regexp) ReplaceAllFunc(s string, f func(m *Match) string) string {
	m := re.newMachine(s)
	mt := re.matchFrom(m, 0)
	if mt == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for mt != nil {
		b.WriteString(s[last:mt.caps[0]])
		b.WriteString(f(mt))
		last = mt.caps[1]
		pos := last
		if mt.caps[1] == mt.caps[0] {
			if pos >= len(s) {
				break
			}
			_, sz := utf8.DecodeRuneInString(s[pos:])
			b.WriteString(s[pos : pos+sz])
			pos += sz
			last = pos
		}
		m.s = s
		mt = re.matchFrom(m, pos)
	}
	b.WriteString(s[last:])
	return b.String()
}

// ReplaceAll replaces every match with the literal string repl.
func (re *Regexp) ReplaceAll(s, repl string) string {
	return re.ReplaceAllFunc(s, func(*Match) string { return repl })
}

// ReplaceFirstFunc replaces the first match with the result of f (Ruby sub).
func (re *Regexp) ReplaceFirstFunc(s string, f func(m *Match) string) string {
	mt := re.Find(s)
	if mt == nil {
		return s
	}
	return s[:mt.caps[0]] + f(mt) + s[mt.caps[1]:]
}

// Quote escapes all regular expression metacharacters in s.
func Quote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(`\.+*?()|[]{}^$-`, c) >= 0 {
			b.WriteByte('\\')
		} else if c == ' ' {
			b.WriteString("\\ ")
			continue
		} else if c == '\n' {
			b.WriteString("\\n")
			continue
		} else if c == '\t' {
			b.WriteString("\\t")
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// MatchAt attempts a match anchored exactly at offset pos (like Ruby's
// StringScanner#scan) and returns it, or nil.
func (re *Regexp) MatchAt(s string, pos int) *Match {
	m := re.newMachine(s)
	for j := range m.slots {
		m.slots[j] = -1
	}
	if end := m.run(0, pos); end >= 0 {
		caps := make([]int, 2*(re.ncap+1))
		copy(caps, m.slots)
		return &Match{s: s, caps: caps}
	}
	return nil
}
