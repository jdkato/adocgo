package adocgo

import (
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Reader is a line-oriented reader with look-ahead, ported from
// Asciidoctor's Reader. When doc is set it also acts as the
// PreprocessorReader, handling conditional and include directives.
type Reader struct {
	lines        []string // reversed: next line is last element
	file         string
	dir          string
	path         string
	lineno       int
	lookAhead    int
	processLines bool
	unescapeNext bool
	saved        *Reader
	// placeholders records which pending lines (by stack index) are list
	// continuation placeholders. Asciidoctor tags such (empty) strings by
	// object identity, and that identity survives as lines flow through
	// nested readers, so it has to be tracked explicitly here.
	placeholders    map[int]bool
	lastPlaceholder bool

	// preprocessor state
	doc              *Document
	maxdepth         *maxDepth
	includeStack     []includeFrame
	skipping         bool
	conditionalStack []conditional
}

type maxDepth struct{ abs, curr, rel int }

type includeFrame struct {
	lines        []string
	file         string
	dir          string
	path         string
	lineno       int
	maxdepth     *maxDepth
	processLines bool
}

type conditional struct {
	name     string
	target   string
	skip     bool
	skipping bool
}

type readOpts struct {
	terminator              string
	hasTerminator           bool
	breakOnBlankLines       bool
	breakOnListContinuation bool
	skipLineComments        bool
	preserveLastLine        bool
	readLastLine            bool
	skipFirstLine           bool
	skipProcessing          bool
	context                 string
}

func reverseLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[len(lines)-1-i] = l
	}
	return out
}

func newReader(lines []string) *Reader {
	return &Reader{lines: reverseLines(lines), dir: ".", path: "<stdin>", lineno: 1, processLines: true}
}

// newListItemReader creates a reader over the lines collected for a list
// item, preserving list continuation placeholders.
func newListItemReader(lines []listLine) *Reader {
	strs := make([]string, len(lines))
	r := &Reader{dir: ".", path: "<stdin>", lineno: 1, processLines: true}
	for i, l := range lines {
		strs[i] = l.s
		if l.marker && l.s == "" {
			if r.placeholders == nil {
				r.placeholders = map[int]bool{}
			}
			r.placeholders[len(lines)-1-i] = true
		}
	}
	r.lines = reverseLines(strs)
	return r
}

func newPreprocessorReader(doc *Document, data string, file string) *Reader {
	r := &Reader{dir: ".", path: "<stdin>", lineno: 1, processLines: true, doc: doc}
	if file != "" {
		r.file = file
		r.dir = fileDirname(file)
		r.path = fileBasename(file)
	}
	depth := 64
	if v, ok := doc.Attributes["max-include-depth"]; ok {
		depth = atoi(v)
	}
	if depth > 0 {
		r.maxdepth = &maxDepth{depth, depth, depth}
	}
	r.lines = reverseLines(r.prepareLines(data, true, true, "", false))
	return r
}

func (r *Reader) prepareLines(data string, trimEnd, condense bool, indent string, hasIndent bool) []string {
	result := prepareSourceString(data, trimEnd)
	if r.doc != nil {
		if _, ok := r.doc.Attributes["skip-front-matter"]; ok {
			if fm, rest, ok := skipFrontMatter(result); ok {
				r.doc.Attributes["front-matter"] = strings.Join(fm, "\n")
				r.lineno += len(fm) + 2
				result = rest
			}
		}
	}
	if condense {
		for len(result) > 0 && result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}
	}
	if hasIndent {
		result = adjustIndentation(result, atoi(indent), atoi(r.doc.Attributes["tabsize"]))
	}
	return result
}

func skipFrontMatter(data []string) (fm, rest []string, ok bool) {
	if len(data) == 0 || data[0] != "---" {
		return nil, data, false
	}
	for i := 1; i < len(data); i++ {
		if data[i] == "---" {
			return data[1:i], data[i+1:], true
		}
	}
	return nil, data, false
}

// HasMoreLines reports whether there are more lines to read.
func (r *Reader) HasMoreLines() bool {
	if r.doc != nil {
		_, ok := r.PeekLine()
		return ok
	}
	if len(r.lines) == 0 {
		r.lookAhead = 0
		return false
	}
	return true
}

func (r *Reader) empty() bool { return !r.HasMoreLines() }

// NextLineEmpty reports whether the next line is blank or missing.
func (r *Reader) NextLineEmpty() bool {
	l, ok := r.PeekLine()
	return !ok || l == ""
}

func (r *Reader) basePeek(direct bool) (string, bool) {
	for {
		if len(r.lines) == 0 {
			if !(direct || r.lookAhead > 0) {
				r.lookAhead = 0
			}
			return "", false
		}
		next := r.lines[len(r.lines)-1]
		if direct || r.lookAhead > 0 {
			if r.unescapeNext {
				return next[1:], true
			}
			return next, true
		}
		if line, ok := r.processLine(next); ok {
			return line, true
		}
	}
}

// PeekLine returns the next line without consuming it.
func (r *Reader) PeekLine() (string, bool) { return r.peek(false) }

// PeekLineDirect returns the next line without preprocessing it.
func (r *Reader) PeekLineDirect() (string, bool) { return r.peek(true) }

func (r *Reader) peek(direct bool) (string, bool) {
	if r.doc == nil {
		return r.basePeek(direct)
	}
	for {
		if line, ok := r.basePeek(direct); ok {
			return line, true
		}
		if len(r.includeStack) == 0 {
			r.conditionalStack = nil
			return "", false
		}
		r.popInclude()
	}
}

// PeekLines returns up to num lines without consuming them.
func (r *Reader) PeekLines(num int, direct bool) []string {
	oldLookAhead := r.lookAhead
	var result []string
	for i := 0; i < num; i++ {
		var line string
		var ok bool
		if direct {
			line, ok = r.shift()
		} else {
			line, ok = r.ReadLine()
		}
		if ok {
			result = append(result, line)
		} else {
			if direct {
				r.lineno--
			}
			break
		}
	}
	if len(result) > 0 {
		r.UnshiftLines(result)
		if direct {
			r.lookAhead = oldLookAhead
		}
	}
	return result
}

// ReadLine consumes and returns the next line.
func (r *Reader) ReadLine() (string, bool) {
	if r.lookAhead > 0 || r.HasMoreLines() {
		return r.shift()
	}
	return "", false
}

// Advance consumes the next line, reporting whether there was one.
func (r *Reader) Advance() bool {
	_, ok := r.shift()
	return ok
}

// Shift discards the next line.
func (r *Reader) Shift() { r.shift() }

func (r *Reader) shift() (string, bool) {
	r.lineno++
	if r.lookAhead != 0 {
		r.lookAhead--
	}
	if len(r.lines) == 0 {
		return "", false
	}
	line := r.lines[len(r.lines)-1]
	r.lines = r.lines[:len(r.lines)-1]
	r.lastPlaceholder = false
	if r.placeholders != nil && r.placeholders[len(r.lines)] {
		r.lastPlaceholder = true
		delete(r.placeholders, len(r.lines))
	}
	if r.unescapeNext {
		r.unescapeNext = false
		if line != "" {
			line = line[1:]
		}
	}
	return line, true
}

// UnshiftLine pushes a line back onto the reader.
func (r *Reader) UnshiftLine(line string) {
	r.lineno--
	r.lookAhead++
	if r.lastPlaceholder && line == "" {
		if r.placeholders == nil {
			r.placeholders = map[int]bool{}
		}
		r.placeholders[len(r.lines)] = true
	}
	r.lastPlaceholder = false
	r.lines = append(r.lines, line)
}

// UnshiftLines pushes lines back onto the reader.
func (r *Reader) UnshiftLines(lines []string) {
	r.lineno -= len(lines)
	r.lookAhead += len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		r.lines = append(r.lines, lines[i])
	}
}

func (r *Reader) replaceNextLine(replacement string) {
	r.shift()
	r.UnshiftLine(replacement)
}

// SkipBlankLines skips blank lines and returns the number skipped, or -1 if
// the reader is exhausted.
func (r *Reader) SkipBlankLines() int {
	if r.empty() {
		return -1
	}
	n := 0
	for {
		next, ok := r.PeekLine()
		if !ok {
			return -1
		}
		if next == "" {
			r.shift()
			n++
		} else {
			return n
		}
	}
}

// SkipCommentLines skips line and block comments.
func (r *Reader) SkipCommentLines() {
	if r.empty() {
		return
	}
	for {
		next, ok := r.PeekLine()
		if !ok || next == "" {
			return
		}
		if strings.HasPrefix(next, "//") {
			if strings.HasPrefix(next, "///") {
				if ll := len(next); ll > 3 && next == strings.Repeat("/", ll) {
					r.ReadLinesUntil(readOpts{terminator: next, hasTerminator: true, skipFirstLine: true, readLastLine: true, skipProcessing: true, context: "comment"}, nil)
				} else {
					return
				}
			} else {
				r.shift()
			}
		} else {
			return
		}
	}
}

// SkipLineComments skips consecutive line comments and returns them.
func (r *Reader) SkipLineComments() []string {
	if r.empty() {
		return nil
	}
	var out []string
	for {
		next, ok := r.PeekLine()
		if !ok || next == "" {
			break
		}
		if strings.HasPrefix(next, "//") {
			l, _ := r.shift()
			out = append(out, l)
		} else {
			break
		}
	}
	return out
}

// Mark records the current position (used only for diagnostics upstream).
func (r *Reader) Mark() {}

// ReadLinesUntil reads lines until a terminating condition is met.
func (r *Reader) ReadLinesUntil(o readOpts, breakFn func(string) bool) []string {
	var result []string
	restoreProcessLines := false
	if r.processLines && o.skipProcessing {
		r.processLines = false
		restoreProcessLines = true
	}
	breakOnBlank, breakOnCont := o.breakOnBlankLines, o.breakOnListContinuation
	if o.hasTerminator {
		breakOnBlank, breakOnCont = false, false
	}
	lineRead, lineRestored := false, false
	if o.skipFirstLine {
		r.shift()
	}
	for {
		line, ok := r.ReadLine()
		if !ok {
			break
		}
		stop := false
		if o.hasTerminator {
			stop = line == o.terminator
		} else if breakOnBlank && line == "" {
			stop = true
		} else if breakOnCont && lineRead && line == "+" {
			o.preserveLastLine = true
			stop = true
		} else if breakFn != nil && breakFn(line) {
			stop = true
		}
		if stop {
			if o.readLastLine {
				result = append(result, line)
			}
			if o.preserveLastLine {
				r.UnshiftLine(line)
				lineRestored = true
			}
			break
		}
		if !(o.skipLineComments && strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "///")) {
			result = append(result, line)
			lineRead = true
		}
	}
	if restoreProcessLines {
		r.processLines = true
		if lineRestored && !o.hasTerminator {
			r.lookAhead--
		}
	}
	return result
}

func (r *Reader) save() {
	cp := *r
	cp.lines = append([]string(nil), r.lines...)
	cp.includeStack = append([]includeFrame(nil), r.includeStack...)
	cp.conditionalStack = append([]conditional(nil), r.conditionalStack...)
	cp.saved = nil
	r.saved = &cp
}

func (r *Reader) restoreSave() {
	if r.saved != nil {
		s := r.saved
		*r = *s
		r.saved = nil
	}
}

func (r *Reader) discardSave() { r.saved = nil }

func (r *Reader) processLine(line string) (string, bool) {
	if r.doc == nil {
		if r.processLines {
			r.lookAhead++
		}
		return line, true
	}
	if !r.processLines {
		return line, true
	}
	if line == "" {
		if r.skipping {
			r.shift()
			return "", false
		}
		r.lookAhead++
		return line, true
	}
	if strings.HasSuffix(line, "]") && !strings.HasPrefix(line, "[") && strings.Contains(line, "::") {
		if strings.Contains(line, "if") {
			if m := conditionalDirectiveRx.Find(line); m != nil {
				if m.Group(1) == `\` {
					r.unescapeNext = true
					r.lookAhead++
					return line[1:], true
				} else if r.preprocessConditionalDirective(m.Group(2), m.Group(3), m.Group(4), m.Group(5), m.Has(5)) {
					r.shift()
					return "", false
				}
				r.lookAhead++
				return line, true
			}
		}
		if r.skipping {
			r.shift()
			return "", false
		}
		if strings.HasPrefix(line, "inc") || strings.HasPrefix(line, `\inc`) {
			if m := includeDirectiveRx.Find(line); m != nil {
				if m.Group(1) == `\` {
					r.unescapeNext = true
					r.lookAhead++
					return line[1:], true
				} else if r.preprocessIncludeDirective(m.Group(2), m.Group(3), m.Has(3)) {
					return "", false
				}
				r.lookAhead++
				return line, true
			}
		}
		r.lookAhead++
		return line, true
	} else if r.skipping {
		r.shift()
		return "", false
	}
	r.lookAhead++
	return line, true
}

func (r *Reader) preprocessConditionalDirective(name, target, delimiter, text string, hasText bool) bool {
	noTarget := target == ""
	if !noTarget {
		target = strings.ToLower(target)
	}
	attrs := r.doc.Attributes
	has := func(n string) bool { _, ok := attrs[n]; return ok }
	skip := false
	if name == "endif" {
		if hasText {
			// malformed: text not permitted
		} else if len(r.conditionalStack) == 0 {
			// unmatched
		} else if noTarget || target == r.conditionalStack[len(r.conditionalStack)-1].target {
			r.conditionalStack = r.conditionalStack[:len(r.conditionalStack)-1]
			if len(r.conditionalStack) == 0 {
				r.skipping = false
			} else {
				r.skipping = r.conditionalStack[len(r.conditionalStack)-1].skipping
			}
		}
		return true
	} else if r.skipping {
		if name == "ifeval" {
			if !(noTarget && hasText && evalExpressionRx.MatchString(strip(text))) {
				return true
			}
		} else if noTarget {
			return true
		}
		skip = false
	} else {
		switch name {
		case "ifdef":
			if noTarget {
				return true
			}
			switch delimiter {
			case ",":
				skip = true
				for _, n := range strings.Split(target, ",") {
					if has(n) {
						skip = false
					}
				}
			case "+":
				skip = false
				for _, n := range strings.Split(target, "+") {
					if !has(n) {
						skip = true
					}
				}
			default:
				skip = !has(target)
			}
		case "ifndef":
			if noTarget {
				return true
			}
			switch delimiter {
			case ",":
				skip = false
				for _, n := range strings.Split(target, ",") {
					if has(n) {
						skip = true
					}
				}
			case "+":
				skip = true
				for _, n := range strings.Split(target, "+") {
					if !has(n) {
						skip = false
					}
				}
			default:
				skip = has(target)
			}
		case "ifeval":
			if !noTarget {
				return true
			}
			m := evalExpressionRx.Find(strip(text))
			if !hasText || m == nil {
				return true
			}
			skip = !evalExpr(r.resolveExprVal(m.Group(1)), m.Group(2), r.resolveExprVal(m.Group(3)))
		}
	}
	if name == "ifeval" {
		if skip {
			r.skipping = true
		}
		r.conditionalStack = append(r.conditionalStack, conditional{name: name, skip: skip, skipping: r.skipping})
	} else if hasText {
		if !r.skipping && !skip {
			r.replaceNextLine(rstrip(text))
			r.UnshiftLine("")
			if strings.HasPrefix(text, "include::") {
				r.lookAhead--
			}
		}
	} else {
		if skip {
			r.skipping = true
		}
		r.conditionalStack = append(r.conditionalStack, conditional{name: name, target: target, skip: skip, skipping: r.skipping})
	}
	return true
}

// exprVal is a value in an ifeval expression.
type exprVal struct {
	kind byte // 's' string, 'n' nil, 'b' bool, 'f' float, 'i' int
	s    string
	b    bool
	f    float64
}

func (r *Reader) resolveExprVal(val string) exprVal {
	quoted := false
	if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) || (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
		quoted = true
		val = val[1:]
	}
	if strings.Contains(val, "{") {
		val = r.doc.subAttributes(val, subAttrOpts{attributeMissing: "drop"})
	}
	switch {
	case quoted:
		return exprVal{kind: 's', s: val}
	case val == "":
		return exprVal{kind: 'n'}
	case val == "true":
		return exprVal{kind: 'b', b: true}
	case val == "false":
		return exprVal{kind: 'b', b: false}
	case rstrip(val) == "":
		return exprVal{kind: 's', s: " "}
	case strings.Contains(val, "."):
		return exprVal{kind: 'f', f: rubyToF(val)}
	}
	return exprVal{kind: 'i', f: float64(atoi(val))}
}

func rubyToF(s string) float64 {
	s = lstrip(s)
	end := 0
	if end < len(s) && (s[end] == '-' || s[end] == '+') {
		end++
	}
	digits := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
		digits++
	}
	if end < len(s)-1 && s[end] == '.' && s[end+1] >= '0' && s[end+1] <= '9' && digits > 0 {
		end++
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
	}
	f, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0
	}
	return f
}

func evalExpr(l exprVal, op string, r exprVal) bool {
	num := func(v exprVal) bool { return v.kind == 'f' || v.kind == 'i' }
	switch op {
	case "==", "!=":
		eq := false
		switch {
		case num(l) && num(r):
			eq = l.f == r.f
		case l.kind != r.kind:
			eq = false
		case l.kind == 's':
			eq = l.s == r.s
		case l.kind == 'b':
			eq = l.b == r.b
		case l.kind == 'n':
			eq = true
		}
		return eq == (op == "==")
	}
	var cmp int
	switch {
	case num(l) && num(r):
		switch {
		case l.f < r.f:
			cmp = -1
		case l.f > r.f:
			cmp = 1
		}
	case l.kind == 's' && r.kind == 's':
		cmp = strings.Compare(l.s, r.s)
	default:
		return false
	}
	switch op {
	case "<":
		return cmp < 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case ">=":
		return cmp >= 0
	}
	return false
}

func (r *Reader) preprocessIncludeDirective(target, attrlist string, hasAttrlist bool) bool {
	doc := r.doc
	expanded := target
	if strings.Contains(target, "{") {
		attrMissing, ok := doc.Attributes["attribute-missing"]
		if !ok {
			attrMissing = "skip"
		}
		am := attrMissing
		if am == "warn" {
			am = "drop-line"
		}
		expanded = doc.subAttributes(target, subAttrOpts{attributeMissing: am})
		if expanded == "" {
			if attrMissing == "drop-line" && doc.subAttributes(target+" ", subAttrOpts{attributeMissing: "drop-line"}) == "" {
				r.shift()
				return true
			} else if _, ok := doc.parseAttributes(attrlist, nil, parseAttrOpts{subInput: true})["optional-option"]; ok {
				r.shift()
				return true
			}
			r.replaceNextLine("Unresolved directive in " + r.path + " - include::" + target + "[" + attrlist + "]")
			return true
		}
	}
	if doc.Safe >= SafeSecure {
		if strings.Contains(expanded, " ") {
			expanded = "pass:c[" + expanded + "]"
		}
		role := "role=include"
		if _, ok := doc.Attributes["compat-mode"]; ok {
			role = ""
		}
		r.replaceNextLine("link:" + expanded + "[" + role + "]")
		return true
	}
	if r.maxdepth == nil {
		return false
	}
	if len(r.includeStack) >= r.maxdepth.curr {
		return false
	}
	parsed := doc.parseAttributes(attrlist, nil, parseAttrOpts{subInput: true})
	// resolve include path
	if isUriish(expanded) {
		// reading remote includes is not supported; behave as if allow-uri-read is unset
		t := expanded
		if strings.Contains(t, " ") {
			t = "pass:c[" + t + "]"
		}
		role := "role=include"
		if _, ok := doc.Attributes["compat-mode"]; ok {
			role = ""
		}
		r.replaceNextLine("link:" + t + "[" + role + "]")
		return true
	}
	incPath := doc.normalizeSystemPath(expanded, r.dir)
	st, err := os.Stat(incPath)
	if err != nil || !st.Mode().IsRegular() {
		if _, ok := parsed["optional-option"]; ok {
			r.shift()
			return true
		}
		r.replaceNextLine("Unresolved directive in " + r.path + " - include::" + expanded + "[" + attrlist + "]")
		return true
	}
	relpath := relativePath(incPath, doc.baseDir)
	raw, err := os.ReadFile(incPath)
	if err != nil {
		r.replaceNextLine("Unresolved directive in " + r.path + " - include::" + expanded + "[" + attrlist + "]")
		return true
	}
	content := string(raw)
	var incLinenos []int
	hasLinenos := false
	var incTags map[string]bool
	var tagOrder []string
	if hasAttrlist {
		if v, ok := parsed["lines"]; ok {
			infinite := false
			for _, linedef := range splitDelimitedValue(v) {
				if strings.Contains(linedef, "..") {
					i := strings.Index(linedef, "..")
					from, to := linedef[:i], linedef[i+2:]
					if to == "" || atoi(to) < 0 {
						incLinenos = append(incLinenos, atoi(from))
						infinite = true
					} else {
						for n := atoi(from); n <= atoi(to); n++ {
							incLinenos = append(incLinenos, n)
						}
					}
				} else {
					incLinenos = append(incLinenos, atoi(linedef))
				}
			}
			if len(incLinenos) > 0 {
				sort.Ints(incLinenos)
				uniq := incLinenos[:0]
				for i, n := range incLinenos {
					if i == 0 || n != incLinenos[i-1] {
						uniq = append(uniq, n)
					}
				}
				incLinenos = uniq
				if infinite {
					incLinenos = append(incLinenos, math.MaxInt)
				}
				hasLinenos = true
			}
		} else if tag, ok := parsed["tag"]; ok {
			if tag != "" && tag != "!" {
				incTags = map[string]bool{}
				if strings.HasPrefix(tag, "!") {
					incTags[tag[1:]] = false
					tagOrder = append(tagOrder, tag[1:])
				} else {
					incTags[tag] = true
					tagOrder = append(tagOrder, tag)
				}
			}
		} else if tags, ok := parsed["tags"]; ok {
			incTags = map[string]bool{}
			for _, tagdef := range splitDelimitedValue(tags) {
				if tagdef == "" || tagdef == "!" {
					continue
				}
				name, val := tagdef, true
				if strings.HasPrefix(tagdef, "!") {
					name, val = tagdef[1:], false
				}
				if _, seen := incTags[name]; !seen {
					tagOrder = append(tagOrder, name)
				}
				incTags[name] = val
			}
			if len(incTags) == 0 {
				incTags = nil
			}
		}
	}
	eachLine := func(fn func(l string) bool) {
		data := content
		for len(data) > 0 {
			i := strings.IndexByte(data, '\n')
			var l string
			if i < 0 {
				l, data = data, ""
			} else {
				l, data = data[:i+1], data[i+1:]
			}
			if !fn(l) {
				return
			}
		}
	}
	if hasLinenos {
		var incLines []string
		incOffset, incLineno := 0, 0
		selectRemaining := false
		eachLine(func(l string) bool {
			incLineno++
			if !selectRemaining && len(incLinenos) > 0 && incLinenos[0] == math.MaxInt {
				selectRemaining = true
			}
			if selectRemaining {
				if incOffset == 0 {
					incOffset = incLineno
				}
				incLines = append(incLines, l)
			} else {
				if len(incLinenos) > 0 && incLinenos[0] == incLineno {
					if incOffset == 0 {
						incOffset = incLineno
					}
					incLines = append(incLines, l)
					incLinenos = incLinenos[1:]
				}
				if len(incLinenos) == 0 {
					return false
				}
			}
			return true
		})
		r.shift()
		if incOffset > 0 {
			parsed["partial-option"] = ""
			r.pushInclude(strings.Join(incLines, ""), incPath, relpath, incOffset, parsed)
		}
	} else if incTags != nil {
		var incLines []string
		incOffset, incLineno := 0, 0
		type tagEntry struct {
			name string
			sel  bool
		}
		var tagStack []tagEntry
		activeTag := ""
		var sel, baseSelect bool
		wildcard, hasWildcard := false, false
		firstKey := ""
		if len(tagOrder) > 0 {
			firstKey = tagOrder[0]
		}
		if v, ok := incTags["**"]; ok {
			sel, baseSelect = v, v
			delete(incTags, "**")
			if w, ok := incTags["*"]; ok {
				wildcard, hasWildcard = w, true
				delete(incTags, "*")
			} else if !sel {
				// first remaining value is false?
				for _, k := range tagOrder {
					if k == "**" {
						continue
					}
					if v2, ok := incTags[k]; ok {
						if !v2 {
							wildcard, hasWildcard = true, true
						}
						break
					}
				}
			}
		} else if w, ok := incTags["*"]; ok {
			if firstKey == "*" {
				wildcard, hasWildcard = w, true
				sel, baseSelect = !w, !w
			} else {
				sel, baseSelect = false, false
				wildcard, hasWildcard = w, true
			}
			delete(incTags, "*")
		} else {
			anyTrue := false
			for _, v := range incTags {
				if v {
					anyTrue = true
				}
			}
			sel, baseSelect = !anyTrue, !anyTrue
		}
		eachLine(func(l string) bool {
			incLineno++
			if strings.Contains(l, "::") && strings.Contains(l, "[]") {
				if m := tagDirectiveRx.Find(l); m != nil {
					thisTag := m.Group(2)
					if m.Has(1) {
						if thisTag == activeTag {
							tagStack = tagStack[:len(tagStack)-1]
							if len(tagStack) == 0 {
								activeTag, sel = "", baseSelect
							} else {
								activeTag, sel = tagStack[len(tagStack)-1].name, tagStack[len(tagStack)-1].sel
							}
						} else if _, ok := incTags[thisTag]; ok {
							for i := len(tagStack) - 1; i >= 0; i-- {
								if tagStack[i].name == thisTag {
									tagStack = append(tagStack[:i], tagStack[i+1:]...)
									break
								}
							}
						}
					} else if v, ok := incTags[thisTag]; ok {
						sel = v
						activeTag = thisTag
						tagStack = append(tagStack, tagEntry{thisTag, sel})
					} else if hasWildcard {
						if activeTag != "" && !sel {
							sel = false
						} else {
							sel = wildcard
						}
						activeTag = thisTag
						tagStack = append(tagStack, tagEntry{thisTag, sel})
					}
					return true
				}
			}
			if sel {
				if incOffset == 0 {
					incOffset = incLineno
				}
				incLines = append(incLines, l)
			}
			return true
		})
		r.shift()
		if incOffset > 0 {
			if !(baseSelect && !(hasWildcard && !wildcard) && len(incTags) == 0) {
				parsed["partial-option"] = ""
			}
			r.pushInclude(strings.Join(incLines, ""), incPath, relpath, incOffset, parsed)
		}
	} else {
		r.shift()
		r.pushInclude(content, incPath, relpath, 1, parsed)
	}
	return true
}

func splitDelimitedValue(val string) []string {
	if strings.Contains(val, ",") {
		return rubySplit(val, ",")
	}
	return rubySplit(val, ";")
}

var asciidocExtensions = []string{".adoc", ".asciidoc", ".asc", ".ad", ".txt"}

func (r *Reader) pushInclude(data, file, path string, lineno int, attributes map[string]string) {
	r.includeStack = append(r.includeStack, includeFrame{r.lines, r.file, r.dir, r.path, r.lineno, r.maxdepth, r.processLines})
	r.file = file
	r.dir = fileDirname(file)
	if path == "" {
		path = fileBasename(file)
	}
	r.path = path
	r.processLines = false
	for _, ext := range asciidocExtensions {
		if strings.HasSuffix(file, ext) {
			r.processLines = true
		}
	}
	if r.processLines {
		key := path[:strings.LastIndex(path, ".")]
		if !r.doc.includes[key] {
			_, partial := attributes["partial-option"]
			r.doc.includes[key] = !partial
		}
	}
	r.lineno = lineno
	if r.maxdepth != nil {
		if d, ok := attributes["depth"]; ok {
			if rel := atoi(d); rel > 0 {
				curr := len(r.includeStack) + rel
				abs := r.maxdepth.abs
				if curr > abs {
					curr, rel = abs, abs
				}
				r.maxdepth = &maxDepth{abs, curr, rel}
			} else {
				r.maxdepth = &maxDepth{r.maxdepth.abs, len(r.includeStack), 0}
			}
		}
	}
	indent, hasIndent := attributes["indent"]
	lines := r.prepareLines(data, r.processLines, false, indent, hasIndent)
	if len(lines) == 0 {
		r.popInclude()
		return
	}
	if lo, ok := attributes["leveloffset"]; ok {
		pre := ":leveloffset!:"
		if cur, ok := r.doc.Attributes["leveloffset"]; ok {
			pre = ":leveloffset: " + cur
		}
		all := append([]string{":leveloffset: " + lo, ""}, lines...)
		all = append(all, "", pre)
		r.lines = reverseLines(all)
		r.lineno -= 2
	} else {
		r.lines = reverseLines(lines)
	}
	r.lookAhead = 0
}

func (r *Reader) popInclude() {
	if len(r.includeStack) == 0 {
		return
	}
	f := r.includeStack[len(r.includeStack)-1]
	r.includeStack = r.includeStack[:len(r.includeStack)-1]
	r.lines, r.file, r.dir, r.path, r.lineno, r.maxdepth, r.processLines = f.lines, f.file, f.dir, f.path, f.lineno, f.maxdepth, f.processLines
	r.lookAhead = 0
}
