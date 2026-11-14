package adocgo

import (
	"math"
	"strconv"
	"strings"
)

// This file is a port of Asciidoctor's table.rb plus Parser.parse_table.

// num is a numeric value that remembers whether Ruby would treat it as a
// Float (which affects how it is printed).
type num struct {
	f       float64
	isFloat bool
}

func intNum(n int) num { return num{f: float64(n)} }

func (n num) String() string {
	if !n.isFloat {
		return strconv.Itoa(int(n.f))
	}
	if n.f == math.Trunc(n.f) && math.Abs(n.f) < 1e16 {
		return strconv.FormatFloat(n.f, 'f', 1, 64)
	}
	return strconv.FormatFloat(n.f, 'f', -1, 64)
}

// truncate4 ports Ruby's Float#truncate(4) (float floor/ceil with digits),
// including its correction for values that are already representable with
// four decimals.
func truncate4(f float64) float64 {
	const s = 10000.0
	if f >= 0 {
		mul := math.Floor(f * s)
		if res := (mul + 1) / s; res <= f {
			return res
		}
		return mul / s
	}
	mul := math.Ceil(f * s)
	if res := (mul - 1) / s; res >= f {
		return res
	}
	return mul / s
}

// round4 ports Ruby's Float#round(4) (round half up).
func round4(x float64) float64 {
	const s = 10000.0
	f := math.Round(x * s)
	if x > 0 {
		if (f+0.5)/s <= x {
			f++
		}
	} else if (f-0.5)/s >= x {
		f--
	}
	return f / s
}

// floatOrInt mirrors the Ruby idiom `x = x.to_i if x.to_i == x`.
func floatOrInt(f float64) num {
	if f == math.Trunc(f) {
		return num{f: f}
	}
	return num{f: f, isFloat: true}
}

// Table holds the structure of a table block.
type Table struct {
	Columns []*Column
	Head    [][]*Cell
	Body    [][]*Cell
	Foot    [][]*Cell
	// hasHeaderOption: 0 false, 1 true, 2 implicit, 3 nil
	hasHeaderOption int
	absWidth        num
	hasAbsWidth     bool
}

// Column describes a table column.
type Column struct {
	table     *Block
	Style     string
	Colnumber int
	Width     num
	Halign    string
	Valign    string
	Pcwidth   num
	Abswidth  num
	Autowidth bool
}

// Cell is a table cell.
type Cell struct {
	block     *Block
	column    *Column
	Text      string
	Style     string
	Colspan   int
	Rowspan   int
	Halign    string
	Valign    string
	InnerDoc  *Document
	reinit    *cellInit
	deferred  bool
	specStyle string
}

type cellInit struct {
	column *Column
	text   string
	spec   *cellSpec
}

type colSpec struct {
	width          int
	halign, valign string
	style          string
}

type cellSpec struct {
	colspan, rowspan, repeatcol int
	halign, valign, style       string
}

func (s *cellSpec) empty() bool {
	return s.colspan == 0 && s.rowspan == 0 && s.repeatcol == 0 && s.halign == "" && s.valign == "" && s.style == ""
}

var (
	tableCellHorzAlignments = map[string]string{"<": "left", ">": "right", "^": "center"}
	tableCellVertAlignments = map[string]string{"<": "top", ">": "bottom", "^": "middle"}
	tableCellStyles         = map[string]string{"d": "none", "s": "strong", "e": "emphasis", "m": "monospaced", "h": "header", "l": "literal", "a": "asciidoc"}
)

func newTable(parent *Block, attributes map[string]string) *Block {
	t := &Block{Context: "table", Attributes: map[string]string{}, ContentModel: "compound"}
	t.Parent = parent
	t.Document = parent.Document
	t.Level = parent.Level
	t.Table = &Table{}
	pc := 100
	if w, ok := attributes["width"]; ok {
		pc = atoi(w)
		if pc > 100 || pc < 1 {
			if !(pc == 0 && (w == "0" || w == "0%")) {
				pc = 100
			}
		}
	}
	t.Attributes["tablepcwidth"] = strconv.Itoa(pc)
	if pw, ok := t.Document.Attributes["pagewidth"]; ok {
		// only backends with a page width (DocBook) track absolute widths
		abs := floatOrInt(truncate4(float64(pc) / 100.0 * rubyToF(pw)))
		t.Table.absWidth, t.Table.hasAbsWidth = abs, true
		t.Attributes["tableabswidth"] = abs.String()
	}
	if _, ok := attributes["rotate-option"]; ok {
		t.Attributes["orientation"] = "landscape"
	}
	return t
}

func (t *Table) headerRow() int {
	if t.hasHeaderOption != 0 && t.hasHeaderOption != 3 && len(t.Body) == 0 {
		return t.hasHeaderOption
	}
	return 0
}

func newColumn(table *Block, index int, spec *colSpec) *Column {
	c := &Column{table: table, Colnumber: index + 1, Width: intNum(1), Halign: "left", Valign: "top"}
	if spec != nil {
		c.Style = spec.style
		c.Width = intNum(spec.width)
		if spec.halign != "" {
			c.Halign = spec.halign
		}
		if spec.valign != "" {
			c.Valign = spec.valign
		}
	}
	return c
}

func (c *Column) assignWidth(colPcwidth num, widthBase float64, hasBase bool) num {
	if hasBase {
		colPcwidth = floatOrInt(truncate4(c.Width.f * 100.0 / widthBase))
	}
	if t := c.table.Table; t.hasAbsWidth {
		c.Abswidth = floatOrInt(truncate4(colPcwidth.f / 100.0 * t.absWidth.f))
	}
	c.Pcwidth = colPcwidth
	return colPcwidth
}

func (tb *Block) createColumns(colspecs []*colSpec) {
	t := tb.Table
	var cols []*Column
	var autowidthCols []*Column
	widthBase := 0
	for _, spec := range colspecs {
		col := newColumn(tb, len(cols), spec)
		cols = append(cols, col)
		if spec.width < 0 {
			autowidthCols = append(autowidthCols, col)
		} else {
			widthBase += spec.width
		}
	}
	t.Columns = cols
	if len(cols) > 0 {
		tb.Attributes["colcount"] = strconv.Itoa(len(cols))
		hasBase := widthBase > 0 || autowidthCols != nil
		tb.assignColumnWidths(float64(widthBase), hasBase, autowidthCols)
	}
}

func (tb *Block) assignColumnWidths(widthBase float64, hasBase bool, autowidthCols []*Column) {
	t := tb.Table
	totalWidth := 0.0
	var colPcwidth num
	if hasBase {
		if autowidthCols != nil {
			var autowidth num
			if widthBase > 100 {
				autowidth = intNum(0)
			} else {
				autowidth = floatOrInt(truncate4((100.0 - widthBase) / float64(len(autowidthCols))))
				widthBase = 100
			}
			for _, col := range autowidthCols {
				col.Width = autowidth
				col.Autowidth = true
			}
		}
		for _, col := range t.Columns {
			colPcwidth = col.assignWidth(num{}, widthBase, true)
			totalWidth += colPcwidth.f
		}
	} else {
		colPcwidth = floatOrInt(truncate4(100.0 / float64(len(t.Columns))))
		for _, col := range t.Columns {
			totalWidth += col.assignWidth(colPcwidth, 0, false).f
		}
	}
	if totalWidth != 100 {
		last := t.Columns[len(t.Columns)-1]
		v := round4(100 - totalWidth + colPcwidth.f)
		last.assignWidth(num{f: v, isFloat: true}, 0, false)
	}
}

func (tb *Block) partitionHeaderFooter(attrs map[string]string) {
	t := tb.Table
	n := len(t.Body)
	tb.Attributes["rowcount"] = strconv.Itoa(n)
	if n > 0 {
		if t.hasHeaderOption == 1 || t.hasHeaderOption == 2 {
			head := t.Body[0]
			t.Body = t.Body[1:]
			for i, cell := range head {
				head[i] = cell.reinitialize(true)
			}
			t.Head = [][]*Cell{head}
			n--
		} else if t.hasHeaderOption == 3 {
			t.hasHeaderOption = 0
			for i, cell := range t.Body[0] {
				t.Body[0][i] = cell.reinitialize(false)
			}
		}
	}
	if _, ok := attrs["footer-option"]; ok && n > 0 {
		t.Foot = [][]*Cell{t.Body[len(t.Body)-1]}
		t.Body = t.Body[:len(t.Body)-1]
	}
}

func newCell(column *Column, cellText string, spec *cellSpec) *Cell {
	table := column.table
	doc := table.Document
	c := &Cell{column: column}
	c.block = &Block{Context: "table_cell", Attributes: map[string]string{}, Document: doc, Parent: table, Level: table.Level, ContentModel: "compound"}
	var cellStyle string
	inHeaderRow := table.Table.headerRow()
	if inHeaderRow != 0 {
		if inHeaderRow == 2 {
			cs := column.Style
			if cs == "" && spec != nil {
				cs = spec.style
			}
			if cs == "asciidoc" || cs == "literal" {
				var sc *cellSpec
				if spec != nil {
					cp := *spec
					sc = &cp
				}
				c.reinit = &cellInit{column, cellText, sc}
			}
		}
		cellStyle = ""
	} else {
		cellStyle = column.Style
	}
	c.Halign, c.Valign = column.Halign, column.Valign
	asciidoc, literal, normalPsv := false, false, false
	if spec != nil {
		if !spec.empty() {
			c.specStyle = spec.style
			c.Colspan, c.Rowspan = spec.colspan, spec.rowspan
			if inHeaderRow == 0 && spec.style != "" {
				cellStyle = spec.style
			}
			if spec.halign != "" {
				c.Halign = spec.halign
			}
			if spec.valign != "" {
				c.Valign = spec.valign
			}
		}
		switch cellStyle {
		case "asciidoc":
			asciidoc = true
			cellText = rstrip(cellText)
			if strings.HasPrefix(cellText, "\n") {
				for strings.HasPrefix(cellText, "\n") {
					cellText = cellText[1:]
				}
			} else {
				cellText = lstrip(cellText)
			}
		case "literal":
			literal = true
			cellText = rstrip(cellText)
			for strings.HasPrefix(cellText, "\n") {
				cellText = cellText[1:]
			}
		default:
			normalPsv = true
			cellText = strip(cellText)
		}
	} else if cellStyle == "asciidoc" {
		asciidoc = true
	}
	if asciidoc {
		parentDoctitle, hadDoctitle := doc.Attributes["doctitle"]
		delete(doc.Attributes, "doctitle")
		innerLines := strings.Split(cellText, "\n")
		if len(innerLines) > 0 && strings.Contains(innerLines[0], "::") {
			line1 := innerLines[0]
			pr := &Reader{dir: ".", path: "<stdin>", lineno: 1, processLines: true, doc: doc}
			if doc.reader != nil {
				pr.dir, pr.file, pr.path = doc.reader.dir, doc.reader.file, doc.reader.path
			}
			depth := 64
			if v, ok := doc.Attributes["max-include-depth"]; ok {
				depth = atoi(v)
			}
			if depth > 0 {
				pr.maxdepth = &maxDepth{depth, depth, depth}
			}
			pr.lines = []string{line1}
			var pre []string
			for pr.HasMoreLines() {
				l, _ := pr.shift()
				pre = append(pre, l)
			}
			if !(len(pre) == 1 && pre[0] == line1) {
				innerLines = append(pre, innerLines[1:]...)
			}
		}
		c.InnerDoc = newNestedDocument(doc, innerLines)
		if hadDoctitle {
			doc.Attributes["doctitle"] = parentDoctitle
		}
	} else if literal {
		c.block.ContentModel = "verbatim"
		c.block.subs = basicSubs
	} else {
		if normalPsv {
			if inHeaderRow != 0 {
				c.deferred = true
			} else {
				c.catalogInlineAnchor(cellText)
			}
		}
		c.block.ContentModel = "simple"
		c.block.subs = normalSubs
	}
	c.Text = cellText
	c.Style = cellStyle
	return c
}

func (c *Cell) reinitialize(hasHeader bool) *Cell {
	if hasHeader {
		c.reinit = nil
	} else if c.reinit != nil {
		return newCell(c.reinit.column, c.reinit.text, c.reinit.spec)
	} else {
		c.Style = c.column.Style
		// a style given in the cell spec is merged into the cell attributes
		if c.specStyle != "" {
			c.Style = c.specStyle
		}
	}
	if c.deferred {
		c.deferred = false
		c.catalogInlineAnchor(c.Text)
	}
	return c
}

func (c *Cell) catalogInlineAnchor(text string) {
	if strings.HasPrefix(text, "[[") {
		if m := leadingInlineAnchorRx.Find(text); m != nil {
			catalogInlineAnchor(m.Group(1), m.Group(2), m.Has(2), c.block)
		}
	}
}

func (c *Cell) text() string { return c.block.applySubs(c.Text, c.block.subs) }

// content returns the converted paragraphs of a non-AsciiDoc cell.
func (c *Cell) content() []string {
	style := c.Style
	wrap := func(s string) string {
		if style != "" && style != "header" {
			return c.block.inline(&Inline{Context: "quoted", Text: s, hasText: true, Type: style})
		}
		return s
	}
	if strings.Contains(c.Text, "\n\n") {
		var out []string
		for _, para := range rubySplitRx(c.text(), blankLineRx) {
			out = append(out, wrap(para))
		}
		return out
	}
	subbed := c.text()
	if subbed == "" {
		return nil
	}
	return []string{wrap(subbed)}
}

type tableParserContext struct {
	table          *Block
	format         string
	delimiter      string
	colcount       int
	buffer         string
	cellspecs      []*cellSpec
	cellOpen       bool
	activeRowspans []int
	columnVisits   int
	currentRow     []*Cell
	linenum        int
}

func newTableParserContext(table *Block, attributes map[string]string) *tableParserContext {
	ctx := &tableParserContext{table: table, activeRowspans: []int{0}, linenum: -1}
	nested := table.Document.parentDocument != nil
	xsv := "psv"
	ctx.format = "psv"
	if f, ok := attributes["format"]; ok {
		switch f {
		case "psv", "csv", "dsv", "tsv":
			xsv = f
			if f == "tsv" {
				ctx.format = "csv"
			} else {
				ctx.format = f
				if f == "psv" && nested {
					xsv = "!sv"
				}
			}
		default:
			if nested {
				xsv = "!sv"
			}
		}
	} else if nested {
		xsv = "!sv"
	}
	delims := map[string]string{"psv": "|", "csv": ",", "dsv": ":", "tsv": "\t", "!sv": "!"}
	if sep, ok := attributes["separator"]; ok {
		switch sep {
		case "":
			ctx.delimiter = delims[xsv]
		case `\t`:
			ctx.delimiter = "\t"
		default:
			ctx.delimiter = sep
		}
	} else {
		ctx.delimiter = delims[xsv]
	}
	ctx.colcount = -1
	if n := len(table.Table.Columns); n > 0 {
		ctx.colcount = n
	}
	return ctx
}

func (ctx *tableParserContext) bufferHasUnclosedQuotes(appendStr string) bool {
	record := strip(ctx.buffer + appendStr)
	if record == `"` {
		return true
	}
	if strings.HasPrefix(record, `"`) {
		trailing := strings.HasSuffix(record, `"`)
		if (trailing && strings.HasSuffix(record, `""`)) || strings.HasPrefix(record, `""`) {
			record = strings.ReplaceAll(record, `""`, "")
			return strings.HasPrefix(record, `"`) && !strings.HasSuffix(record, `"`)
		}
		return !trailing
	}
	return false
}

func (ctx *tableParserContext) closeOpenCell(next *cellSpec) {
	if next == nil {
		next = &cellSpec{}
	}
	ctx.cellspecs = append(ctx.cellspecs, next)
	if ctx.cellOpen {
		ctx.closeCell(true)
	}
	ctx.linenum++
}

func (ctx *tableParserContext) closeCell(eol bool) {
	var cellText string
	var spec *cellSpec
	repeat := 1
	if ctx.format == "psv" {
		cellText = ctx.buffer
		ctx.buffer = ""
		if len(ctx.cellspecs) > 0 {
			spec = ctx.cellspecs[0]
			ctx.cellspecs = ctx.cellspecs[1:]
			if spec.repeatcol > 0 {
				repeat = spec.repeatcol
				spec.repeatcol = 0
			}
		} else {
			spec = &cellSpec{}
		}
	} else {
		cellText = strip(ctx.buffer)
		ctx.buffer = ""
		if ctx.format == "csv" && cellText != "" && strings.Contains(cellText, `"`) {
			if strings.HasPrefix(cellText, `"`) && strings.HasSuffix(cellText, `"`) {
				if len(cellText) >= 2 {
					cellText = squeezeChar(strip(cellText[1:len(cellText)-1]), '"')
				} else {
					cellText = ""
				}
			} else {
				cellText = squeezeChar(cellText, '"')
			}
		}
	}
	t := ctx.table.Table
	for i := 1; i <= repeat; i++ {
		var column *Column
		if ctx.colcount == -1 {
			column = newColumn(ctx.table, len(t.Columns)+i-1, nil)
			t.Columns = append(t.Columns, column)
			if spec != nil && spec.colspan > 1 {
				offset := len(t.Columns)
				for j := 0; j < spec.colspan-1; j++ {
					t.Columns = append(t.Columns, newColumn(ctx.table, offset+j, nil))
				}
			}
		} else if len(ctx.currentRow) < len(t.Columns) {
			column = t.Columns[len(ctx.currentRow)]
		}
		if column == nil {
			// more cells than columns in this row; Asciidoctor would raise
			// here, so drop the cell and move on.
			column = newColumn(ctx.table, len(t.Columns), nil)
		}
		var sc *cellSpec
		if spec != nil {
			cp := *spec
			sc = &cp
		}
		cell := newCell(column, cellText, sc)
		if cell.Rowspan > 1 {
			cs := cell.Colspan
			if cs == 0 {
				cs = 1
			}
			ctx.activateRowspan(cell.Rowspan, cs)
		}
		if cell.Colspan > 0 {
			ctx.columnVisits += cell.Colspan
		} else {
			ctx.columnVisits++
		}
		ctx.currentRow = append(ctx.currentRow, cell)
		if rowStatus := ctx.endOfRow(); rowStatus > -1 && (ctx.colcount != -1 || ctx.linenum > 0 || (eol && i == repeat)) {
			if rowStatus > 0 {
				ctx.closeRow(true)
			} else {
				ctx.closeRow(false)
			}
		}
	}
	ctx.cellOpen = false
}

func (ctx *tableParserContext) closeRow(drop bool) {
	t := ctx.table.Table
	if !drop {
		t.Body = append(t.Body, ctx.currentRow)
	}
	if ctx.colcount == -1 {
		ctx.colcount = ctx.columnVisits
	}
	ctx.columnVisits = 0
	ctx.currentRow = nil
	ctx.activeRowspans = ctx.activeRowspans[1:]
	if len(ctx.activeRowspans) == 0 {
		ctx.activeRowspans = []int{0}
	}
}

func (ctx *tableParserContext) activateRowspan(rowspan, colspan int) {
	for i := 1; i <= rowspan-1; i++ {
		for len(ctx.activeRowspans) <= i {
			ctx.activeRowspans = append(ctx.activeRowspans, 0)
		}
		ctx.activeRowspans[i] += colspan
	}
}

func (ctx *tableParserContext) endOfRow() int {
	if ctx.colcount == -1 {
		return 0
	}
	v := ctx.columnVisits + ctx.activeRowspans[0]
	switch {
	case v < ctx.colcount:
		return -1
	case v > ctx.colcount:
		return 1
	}
	return 0
}

func parseTable(reader *Reader, parent *Block, attributes map[string]string) *Block {
	table := newTable(parent, attributes)
	t := table.Table
	explicitColspecs := false
	if cols, ok := attributes["cols"]; ok {
		if specs := parseColspecs(cols); len(specs) > 0 {
			table.createColumns(specs)
			explicitColspecs = true
		}
	}
	skipped := reader.SkipBlankLines()
	if skipped < 0 {
		skipped = 0
	}
	implicitHeader := false
	if _, ok := attributes["header-option"]; ok {
		t.hasHeaderOption = 1
	} else if _, ok := attributes["noheader-option"]; skipped == 0 && !ok {
		t.hasHeaderOption = 2
		implicitHeader = true
	}
	ctx := newTableParserContext(table, attributes)
	format := ctx.format
	loopIdx := -1
	implicitHeaderBoundary := -1
	for {
		line, ok := reader.ReadLine()
		if !ok {
			break
		}
		hasLine := true
		loopIdx++
		beyondFirst := loopIdx > 0
		if beyondFirst && line == "" {
			hasLine = false
			if implicitHeaderBoundary >= 0 {
				implicitHeaderBoundary++
			}
		} else if format == "psv" {
			if strings.HasPrefix(line, ctx.delimiter) {
				line = line[len(ctx.delimiter):]
				ctx.closeOpenCell(nil)
				implicitHeaderBoundary = -1
			} else {
				nextSpec, rest := parseCellspecStart(line, ctx.delimiter)
				line = rest
				if nextSpec != nil {
					ctx.closeOpenCell(nextSpec)
					implicitHeaderBoundary = -1
				} else if implicitHeaderBoundary >= 0 && implicitHeaderBoundary == loopIdx {
					t.hasHeaderOption = 3
					implicitHeader = false
					implicitHeaderBoundary = -1
				}
			}
		}
		if !beyondFirst {
			if implicitHeader {
				if nl, ok := reader.PeekLine(); reader.HasMoreLines() && ok && nl == "" {
					implicitHeaderBoundary = 1
				} else {
					t.hasHeaderOption = 3
					implicitHeader = false
				}
			}
		}
	inner:
		for {
			idx := -1
			if hasLine {
				idx = strings.Index(line, ctx.delimiter)
			}
			if idx >= 0 {
				preMatch, postMatch := line[:idx], line[idx+len(ctx.delimiter):]
				switch format {
				case "csv":
					if ctx.bufferHasUnclosedQuotes(preMatch) {
						ctx.buffer += preMatch + ctx.delimiter
						line = postMatch
						if line == "" {
							break inner
						}
						continue inner
					}
					ctx.buffer += preMatch
				case "dsv":
					if strings.HasSuffix(preMatch, `\`) {
						ctx.buffer += chop(preMatch) + ctx.delimiter
						line = postMatch
						if line == "" {
							ctx.buffer += "\n"
							ctx.cellOpen = true
							break inner
						}
						continue inner
					}
					ctx.buffer += preMatch
				default:
					if strings.HasSuffix(preMatch, `\`) {
						ctx.buffer += chop(preMatch) + ctx.delimiter
						line = postMatch
						if line == "" {
							ctx.buffer += "\n"
							ctx.cellOpen = true
							break inner
						}
						continue inner
					}
					nextSpec, cellText := parseCellspecEnd(preMatch)
					ctx.cellspecs = append(ctx.cellspecs, nextSpec)
					ctx.buffer += cellText
				}
				line = postMatch
				if line == "" {
					hasLine = false
				}
				ctx.closeCell(false)
			} else {
				if hasLine {
					ctx.buffer += line
				}
				ctx.buffer += "\n"
				switch format {
				case "csv":
					if ctx.bufferHasUnclosedQuotes("") {
						if implicitHeaderBoundary >= 0 && loopIdx == 0 {
							t.hasHeaderOption = 3
							implicitHeader = false
							implicitHeaderBoundary = -1
						}
						ctx.cellOpen = true
					} else {
						ctx.closeCell(true)
					}
				case "dsv":
					ctx.closeCell(true)
				default:
					ctx.cellOpen = true
				}
				break inner
			}
		}
		if ctx.cellOpen {
			if !reader.HasMoreLines() {
				ctx.closeCell(true)
			}
		} else if reader.SkipBlankLines() < 0 {
			break
		}
	}
	if _, ok := table.Attributes["colcount"]; !ok {
		table.Attributes["colcount"] = strconv.Itoa(len(t.Columns))
	}
	if table.Attributes["colcount"] != "0" && !explicitColspecs {
		table.assignColumnWidths(0, false, nil)
	}
	if implicitHeader {
		t.hasHeaderOption = 1
	}
	table.partitionHeaderFooter(attributes)
	return table
}

func parseColspecs(records string) []*colSpec {
	records = strings.ReplaceAll(records, " ", "")
	if records == strconv.Itoa(atoi(records)) {
		n := atoi(records)
		if n > 1000 {
			n = 1000
		}
		var specs []*colSpec
		for i := 0; i < n; i++ {
			specs = append(specs, &colSpec{width: 1})
		}
		return specs
	}
	var specs []*colSpec
	var parts []string
	if records == "" {
		return nil
	}
	if strings.Contains(records, ",") {
		parts = strings.Split(records, ",")
	} else {
		parts = strings.Split(records, ";")
	}
	for _, record := range parts {
		if record == "" {
			specs = append(specs, &colSpec{width: 1})
		} else if m := columnSpecRx.Find(record); m != nil {
			spec := &colSpec{}
			if m.Has(2) {
				colspec, rowspec := splitDot(m.Group(2))
				if h, ok := tableCellHorzAlignments[colspec]; ok {
					spec.halign = h
				}
				if v, ok := tableCellVertAlignments[rowspec]; ok {
					spec.valign = v
				}
			}
			if m.Has(3) {
				if m.Group(3) == "~" {
					spec.width = -1
				} else {
					spec.width = atoi(m.Group(3))
				}
			} else {
				spec.width = 1
			}
			if m.Has(4) {
				if s, ok := tableCellStyles[m.Group(4)]; ok {
					spec.style = s
				}
			}
			if m.Has(1) {
				n := atoi(m.Group(1))
				if n > 1000 {
					n = 1000
				}
				for i := 0; i < n; i++ {
					cp := *spec
					specs = append(specs, &cp)
				}
			} else {
				specs = append(specs, spec)
			}
		}
	}
	return specs
}

// splitDot emulates `a, b = str.split '.'`.
func splitDot(s string) (string, string) {
	parts := rubySplit(s, ".")
	a, b := "", ""
	if len(parts) > 0 {
		a = parts[0]
	}
	if len(parts) > 1 {
		b = parts[1]
	}
	return a, b
}

// parseCellspecStart ports parse_cellspec with pos == :start. A nil spec
// means no cellspec was found.
func parseCellspecStart(line, delimiter string) (*cellSpec, string) {
	idx := strings.Index(line, delimiter)
	if idx < 0 {
		return nil, line
	}
	specPart, rest := line[:idx], line[idx+len(delimiter):]
	m := cellSpecStartRx.Find(specPart)
	if m == nil {
		return nil, line
	}
	if m.Group(0) == "" {
		return &cellSpec{}, rest
	}
	return buildCellSpec(m.Group(1), m.Has(1), m.Group(2), m.Group(3), m.Has(3), m.Group(4), m.Has(4)), rest
}

// parseCellspecEnd ports parse_cellspec with pos == :end.
func parseCellspecEnd(line string) (*cellSpec, string) {
	m := cellSpecEndRx.Find(line)
	if m == nil {
		return &cellSpec{}, line
	}
	if lstrip(m.Group(0)) == "" {
		return &cellSpec{}, rstrip(line)
	}
	return buildCellSpec(m.Group(1), m.Has(1), m.Group(2), m.Group(3), m.Has(3), m.Group(4), m.Has(4)), m.Pre()
}

func buildCellSpec(g1 string, has1 bool, g2, g3 string, has3 bool, g4 string, has4 bool) *cellSpec {
	spec := &cellSpec{}
	if has1 {
		c, r := splitDot(g1)
		colspec, rowspec := 1, 1
		if c != "" {
			colspec = atoi(c)
		}
		if r != "" {
			rowspec = atoi(r)
		}
		switch g2 {
		case "+":
			if colspec != 1 {
				spec.colspan = colspec
			}
			if rowspec != 1 {
				spec.rowspan = rowspec
			}
		case "*":
			if colspec != 1 {
				spec.repeatcol = colspec
			}
		}
	}
	if has3 {
		c, r := splitDot(g3)
		if h, ok := tableCellHorzAlignments[c]; ok {
			spec.halign = h
		}
		if v, ok := tableCellVertAlignments[r]; ok {
			spec.valign = v
		}
	}
	if has4 {
		if s, ok := tableCellStyles[g4]; ok {
			spec.style = s
		}
	}
	return spec
}
