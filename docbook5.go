package adocgo

import (
	"bytes"
	"strconv"
	"strings"
)

// This file is a port of Asciidoctor's converter/docbook5.rb.

// docbook5Converter converts the document tree to DocBook 5.
type docbook5Converter struct{}

type dlistTags struct{ list, entry, label, term, item string }

var dbQuoteTags = map[string]struct {
	open, close string
	phrase      bool
}{
	"monospaced":  {"<literal>", "</literal>", false},
	"emphasis":    {"<emphasis>", "</emphasis>", true},
	"strong":      {`<emphasis role="strong">`, "</emphasis>", true},
	"double":      {"<quote>", "</quote>", true},
	"single":      {"<quote>", "</quote>", true},
	"mark":        {`<emphasis role="marked">`, "</emphasis>", false},
	"superscript": {"<superscript>", "</superscript>", false},
	"subscript":   {"<subscript>", "</subscript>", false},
}

var copyrightRx = bolScanner(`^(.+?)(?: ((?:\d{4}-)?\d{4}))?$`, "", 2,
	func(s string, i int, c []int) bool {
		le := lineEnd(s, i)
		if le == i {
			return false
		}
		_, sz := runeAt(s, i)
		for e := i + sz; e < le; e++ {
			if s[e] != ' ' {
				continue
			}
			y := e + 1
			for _, n := range [2]int{9, 4} {
				if y+n != le {
					continue
				}
				ok := true
				for k := y; k < le; k++ {
					if n == 9 && k == y+4 {
						ok = ok && s[k] == '-'
					} else {
						ok = ok && isDigit(s[k])
					}
				}
				if ok {
					set2(c, 0, i, le)
					set2(c, 1, i, e)
					set2(c, 2, y, le)
					return true
				}
			}
		}
		set2(c, 0, i, le)
		set2(c, 1, i, le)
		return true
	})

var imageMacroRx = bolScanner(`^image::?(\S|\S.*?\S)\[(.+)?\]$`, "", 2,
	func(s string, i int, c []int) bool {
		if !strings.HasPrefix(s[i:], "image:") {
			return false
		}
		// the second colon is optional, so try both readings in turn
		for _, t := range [2]int{i + 7, i + 6} {
			if t == i+7 && (i+6 >= len(s) || s[i+6] != ':') {
				continue
			}
			if t >= len(s) || isSpaceByte(s[t]) {
				continue
			}
			_, sz := runeAt(s, t)
			b := t + sz
			le, ok := scanMacroTail(s, b)
			if !ok {
				tl := lineEnd(s, b)
				for b = t + sz + 1; b < tl; b++ {
					if s[b] == '[' && !isSpaceByte(s[b-1]) {
						if le, ok = scanMacroTail(s, b); ok {
							break
						}
					}
				}
			}
			if !ok {
				continue
			}
			set2(c, 0, i, le)
			set2(c, 1, t, b)
			if b+1 < le-1 {
				set2(c, 2, b+1, le-1)
			}
			return true
		}
		return false
	})

// dbCommon ports common_attributes. hasID/hasRole/hasReftext tell whether
// the respective Ruby argument is non-nil.
func dbCommon(id string, hasID bool, role string, hasRole bool, reftext string, hasReftext bool) string {
	attrs := ""
	if hasID {
		attrs = ` xml:id="` + id + `"`
		if hasRole {
			attrs += ` role="` + role + `"`
		}
	} else if hasRole {
		attrs = ` role="` + role + `"`
	}
	if hasReftext {
		if strings.Contains(reftext, "<") {
			reftext = xmlSanitizeRx.ReplaceAll(reftext, "")
			if strings.Contains(reftext, " ") {
				reftext = strip(squeezeSpaces(reftext))
			}
		}
		reftext = strings.ReplaceAll(reftext, `"`, "&quot;")
		return attrs + ` xreflabel="` + reftext + `"`
	}
	return attrs
}

func (b *Block) hasID() bool { return b.ID != "" || b.emptyID }

// dbAttrs returns the common attributes (id, role, reftext) of a block.
func dbAttrs(n *Block) string {
	role, hasRole := n.Attributes["role"]
	reftext, hasReftext := n.reftext()
	return dbCommon(n.ID, n.hasID(), role, hasRole, reftext, hasReftext)
}

// dbItemAttrs returns the common attributes (id, role) of a list item.
func dbItemAttrs(n *Block) string {
	role, hasRole := n.Attributes["role"]
	return dbCommon(n.ID, n.hasID(), role, hasRole, "", false)
}

func dbTitleTag(n *Block, optional bool) string {
	if !optional || n.hasTitle {
		return "<title>" + n.Title() + "</title>\n"
	}
	return ""
}

func dbEnclose(n *Block) string {
	if n.ContentModel == "compound" {
		return n.content()
	}
	return "<simpara>" + n.content() + "</simpara>"
}

func (c *docbook5Converter) writeBlock(w *bytes.Buffer, n *Block) {
	if n.Context == "section" {
		c.section(w, n)
		return
	}
	s, _ := c.convertBlock(n)
	w.WriteString(s)
}

// convertNode converts a block (Ruby: node.convert) and reports whether the
// converter produced output at all (false corresponds to nil).
func (c *docbook5Converter) convertNode(n *Block) (string, bool) {
	n.Document.playbackAttributes(n.Attributes)
	return c.convertRaw(n)
}

func (c *docbook5Converter) convertRaw(n *Block) (string, bool) {
	if n.Context == "section" {
		var w bytes.Buffer
		c.section(&w, n)
		return w.String(), true
	}
	return c.convertBlock(n)
}

func (c *docbook5Converter) convertBlock(n *Block) (string, bool) {
	switch n.Context {
	case "paragraph":
		return c.paragraph(n), true
	case "listing":
		return c.listing(n), true
	case "literal":
		return c.literal(n), true
	case "ulist":
		return c.ulist(n), true
	case "olist":
		return c.olist(n), true
	case "dlist":
		return c.dlist(n), true
	case "colist":
		res := c.colist(n)
		n.Document.callouts.nextList()
		return res, true
	case "admonition":
		name := n.Attributes["name"]
		return "<" + name + dbAttrs(n) + ">\n" + dbTitleTag(n, true) + dbEnclose(n) + "\n</" + name + ">", true
	case "example":
		if n.hasTitle {
			return "<example" + dbAttrs(n) + ">\n<title>" + n.Title() + "</title>\n" + dbEnclose(n) + "\n</example>", true
		}
		return "<informalexample" + dbAttrs(n) + ">\n" + dbEnclose(n) + "\n</informalexample>", true
	case "floating_title":
		return "<bridgehead" + dbAttrs(n) + ` renderas="sect` + strconv.Itoa(n.Level) + `">` + n.Title() + "</bridgehead>", true
	case "image":
		return c.image(n), true
	case "open":
		return c.open(n), true
	case "page_break":
		return "<simpara><?asciidoc-pagebreak?></simpara>", true
	case "preamble":
		if n.Document.Doctype == "book" {
			return "<preface" + dbAttrs(n) + ">\n" + dbTitleTag(n, false) + n.content() + "\n</preface>", true
		}
		return n.content(), true
	case "quote":
		attrs := dbAttrs(n)
		return c.blockquote(n, attrs, func() string { return dbEnclose(n) }), true
	case "verse":
		attrs := dbAttrs(n)
		return c.blockquote(n, attrs, func() string { return "<literallayout>" + n.content() + "</literallayout>" }), true
	case "sidebar":
		return "<sidebar" + dbAttrs(n) + ">\n" + dbTitleTag(n, true) + dbEnclose(n) + "\n</sidebar>", true
	case "stem":
		return c.stem(n), true
	case "table":
		return c.table(n), true
	case "thematic_break":
		return "<simpara><?asciidoc-hr?></simpara>", true
	case "pass":
		return n.content(), true
	case "audio", "video", "toc":
		return "", false
	}
	return n.content(), true
}

var manpageSectionTags = map[string]string{"section": "refsection", "synopsis": "refsynopsisdiv"}

func (c *docbook5Converter) section(w *bytes.Buffer, n *Block) {
	tag := n.Sectname
	if n.Document.Doctype == "manpage" {
		if t, ok := manpageSectionTags[tag]; ok {
			tag = t
		}
	}
	titleEl := ""
	if !(n.Special && (n.hasOption("notitle") || n.hasOption("untitled"))) {
		titleEl = "<title>" + n.Title() + "</title>\n"
	}
	w.WriteString("<" + tag + dbAttrs(n) + ">\n" + titleEl)
	n.writeContent(w)
	w.WriteString("\n</" + tag + ">")
}

func (c *docbook5Converter) paragraph(n *Block) string {
	if n.hasTitle {
		return "<formalpara" + dbAttrs(n) + ">\n<title>" + n.Title() + "</title>\n<para>" + n.content() + "</para>\n</formalpara>"
	}
	return "<simpara" + dbAttrs(n) + ">" + n.content() + "</simpara>"
}

func (c *docbook5Converter) colist(n *Block) string {
	result := []string{"<calloutlist" + dbAttrs(n) + ">"}
	if n.hasTitle {
		result = append(result, "<title>"+n.Title()+"</title>")
	}
	for _, item := range n.Blocks {
		result = append(result, `<callout arearefs="`+item.Attributes["coids"]+`">`, "<para>"+item.itemText()+"</para>")
		if item.hasBlocks() {
			result = append(result, itemContent(item))
		}
		result = append(result, "</callout>")
	}
	result = append(result, "</calloutlist>")
	return strings.Join(result, "\n")
}

func (c *docbook5Converter) dlist(n *Block) string {
	var result []string
	dd := func(d *Block) {
		if d == nil {
			return
		}
		if d.itemHasText() {
			result = append(result, "<simpara>"+d.itemText()+"</simpara>")
		}
		if d.hasBlocks() {
			result = append(result, itemContent(d))
		}
	}
	if n.Style == "horizontal" {
		tag := "informaltable"
		if n.hasTitle {
			tag = "table"
		}
		lw, ok := n.Attributes["labelwidth"]
		if !ok {
			lw = "15"
		}
		iw, ok := n.Attributes["itemwidth"]
		if !ok {
			iw = "85"
		}
		result = append(result, "<"+tag+dbAttrs(n)+` tabstyle="horizontal" frame="none" colsep="0" rowsep="0">`+"\n"+dbTitleTag(n, true)+`<tgroup cols="2">`+"\n"+`<colspec colwidth="`+lw+`*"/>`+"\n"+`<colspec colwidth="`+iw+`*"/>`+"\n"+`<tbody valign="top">`)
		for _, e := range n.DlistItems {
			result = append(result, "<row>\n<entry>")
			for _, dt := range e.Terms {
				result = append(result, "<simpara>"+dt.itemText()+"</simpara>")
			}
			result = append(result, "</entry>\n<entry>")
			dd(e.Desc)
			result = append(result, "</entry>\n</row>")
		}
		result = append(result, "</tbody>\n</tgroup>\n</"+tag+">")
		return strings.Join(result, "\n")
	}
	tags := dlistTags{"variablelist", "varlistentry", "", "term", "listitem"}
	switch n.Style {
	case "qanda":
		tags = dlistTags{"qandaset", "qandaentry", "question", "simpara", "answer"}
	case "glossary":
		tags = dlistTags{"", "glossentry", "", "glossterm", "glossdef"}
	}
	if tags.list != "" {
		result = append(result, "<"+tags.list+dbAttrs(n)+">")
		if n.hasTitle {
			result = append(result, "<title>"+n.Title()+"</title>")
		}
	}
	for _, e := range n.DlistItems {
		result = append(result, "<"+tags.entry+">")
		if tags.label != "" {
			result = append(result, "<"+tags.label+">")
		}
		for _, dt := range e.Terms {
			result = append(result, "<"+tags.term+">"+dt.itemText()+"</"+tags.term+">")
		}
		if tags.label != "" {
			result = append(result, "</"+tags.label+">")
		}
		result = append(result, "<"+tags.item+">")
		dd(e.Desc)
		result = append(result, "</"+tags.item+">", "</"+tags.entry+">")
	}
	if tags.list != "" {
		result = append(result, "</"+tags.list+">")
	}
	return strings.Join(result, "\n")
}

func dbImageSizeAttrs(attrs map[string]string) string {
	if v, ok := attrs["scaledwidth"]; ok {
		return ` width="` + v + `"`
	}
	if v, ok := attrs["scale"]; ok {
		return ` scale="` + v + `"`
	}
	out := ""
	if v, ok := attrs["width"]; ok {
		out += ` contentwidth="` + v + `"`
	}
	if v, ok := attrs["height"]; ok {
		out += ` contentdepth="` + v + `"`
	}
	return out
}

func (c *docbook5Converter) image(n *Block) string {
	align := ""
	if a, ok := n.Attributes["align"]; ok {
		align = ` align="` + a + `"`
	}
	media := "<mediaobject>\n<imageobject>\n<imagedata fileref=\"" + n.imageURI(n.Attributes["target"], "imagesdir") + `"` + dbImageSizeAttrs(n.Attributes) + align + "/>\n</imageobject>\n<textobject><phrase>" + n.alt() + "</phrase></textobject>\n</mediaobject>"
	if n.hasTitle {
		return "<figure" + dbAttrs(n) + ">\n<title>" + n.Title() + "</title>\n" + media + "\n</figure>"
	}
	return "<informalfigure" + dbAttrs(n) + ">\n" + media + "\n</informalfigure>"
}

func (c *docbook5Converter) listing(n *Block) string {
	informal := !n.hasTitle
	common := dbAttrs(n)
	inner := ""
	if informal {
		inner = common
	}
	var wrapped string
	if n.Style == "source" {
		numbering := ` linenumbering="unnumbered"`
		if _, ok := n.Attributes["linenums"]; ok {
			numbering = ` linenumbering="numbered"`
			if st, ok := n.Attributes["start"]; ok {
				numbering += ` startinglinenumber="` + strconv.Itoa(atoi(st)) + `"`
			}
		}
		if lang, ok := n.Attributes["language"]; ok {
			wrapped = "<programlisting" + inner + ` language="` + lang + `"` + numbering + ">" + n.content() + "</programlisting>"
		} else {
			wrapped = "<screen" + inner + numbering + ">" + n.content() + "</screen>"
		}
	} else {
		wrapped = "<screen" + inner + ">" + n.content() + "</screen>"
	}
	if informal {
		return wrapped
	}
	return "<formalpara" + common + ">\n<title>" + n.Title() + "</title>\n<para>\n" + wrapped + "\n</para>\n</formalpara>"
}

func (c *docbook5Converter) literal(n *Block) string {
	if n.hasTitle {
		return "<formalpara" + dbAttrs(n) + ">\n<title>" + n.Title() + "</title>\n<para>\n<literallayout class=\"monospaced\">" + n.content() + "</literallayout>\n</para>\n</formalpara>"
	}
	return "<literallayout" + dbAttrs(n) + ` class="monospaced">` + n.content() + "</literallayout>"
}

func (c *docbook5Converter) stem(n *Block) string {
	// the equation goes into a CDATA section, so special characters are
	// not escaped
	saved := n.subs
	if n.hasSub("specialcharacters") {
		n.removeSub("specialcharacters")
	}
	equation := n.content()
	n.subs = saved
	var data string
	if n.Style == "asciimath" {
		data = "<mathphrase><![CDATA[" + equation + "]]></mathphrase>"
	} else {
		data = "<alt><![CDATA[" + equation + "]]></alt>\n<mathphrase><![CDATA[" + equation + "]]></mathphrase>"
	}
	if n.hasTitle {
		return "<equation" + dbAttrs(n) + ">\n<title>" + n.Title() + "</title>\n" + data + "\n</equation>"
	}
	return "<informalequation" + dbAttrs(n) + ">\n" + data + "\n</informalequation>"
}

func (c *docbook5Converter) olist(n *Block) string {
	num := ""
	if n.Style != "" {
		num = ` numeration="` + n.Style + `"`
	}
	start := ""
	if s, ok := n.Attributes["start"]; ok {
		start = ` startingnumber="` + s + `"`
	}
	result := []string{"<orderedlist" + dbAttrs(n) + num + start + ">"}
	if n.hasTitle {
		result = append(result, "<title>"+n.Title()+"</title>")
	}
	for _, item := range n.Blocks {
		result = append(result, "<listitem"+dbItemAttrs(item)+">", "<simpara>"+item.itemText()+"</simpara>")
		if item.hasBlocks() {
			result = append(result, itemContent(item))
		}
		result = append(result, "</listitem>")
	}
	result = append(result, "</orderedlist>")
	return strings.Join(result, "\n")
}

func (c *docbook5Converter) ulist(n *Block) string {
	var result []string
	if n.Style == "bibliography" {
		result = append(result, "<bibliodiv"+dbAttrs(n)+">")
		if n.hasTitle {
			result = append(result, "<title>"+n.Title()+"</title>")
		}
		for _, item := range n.Blocks {
			result = append(result, "<bibliomixed>", "<bibliomisc>"+item.itemText()+"</bibliomisc>")
			if item.hasBlocks() {
				result = append(result, itemContent(item))
			}
			result = append(result, "</bibliomixed>")
		}
		result = append(result, "</bibliodiv>")
		return strings.Join(result, "\n")
	}
	checklist := n.hasOption("checklist")
	mark := ""
	if checklist {
		mark = ` mark="none"`
	} else if n.Style != "" {
		mark = ` mark="` + n.Style + `"`
	}
	result = append(result, "<itemizedlist"+dbAttrs(n)+mark+">")
	if n.hasTitle {
		result = append(result, "<title>"+n.Title()+"</title>")
	}
	for _, item := range n.Blocks {
		marker := ""
		if _, ok := item.Attributes["checkbox"]; checklist && ok {
			marker = "&#10063; "
			if _, ok := item.Attributes["checked"]; ok {
				marker = "&#10003; "
			}
		}
		result = append(result, "<listitem"+dbItemAttrs(item)+">", "<simpara>"+marker+item.itemText()+"</simpara>")
		if item.hasBlocks() {
			result = append(result, itemContent(item))
		}
		result = append(result, "</listitem>")
	}
	result = append(result, "</itemizedlist>")
	return strings.Join(result, "\n")
}

func (c *docbook5Converter) open(n *Block) string {
	switch n.Style {
	case "abstract":
		parent := n.Parent
		if parent == &n.Document.Block && n.Document.Doctype == "book" {
			return ""
		}
		result := "<abstract>\n" + dbTitleTag(n, true) + dbEnclose(n) + "\n</abstract>"
		if !n.hasOption("root") && len(parent.Blocks) > 0 && parent.Blocks[0] == n {
			if (parent.Context == "open" && parent.Style == "partintro") || (parent.Context != "open" && parent.Context == "section" && parent.Sectname == "partintro") {
				result = "<info>\n" + result + "\n</info>"
			}
		}
		return result
	case "partintro":
		if n.Level == 0 && n.Parent.Context == "section" && n.Document.Doctype == "book" {
			return "<partintro" + dbAttrs(n) + ">\n" + dbTitleTag(n, true) + dbEnclose(n) + "\n</partintro>"
		}
		return ""
	}
	hasID := n.hasID()
	reftext, hasReftext := "", false
	if hasID {
		reftext, hasReftext = n.reftext()
	}
	role, hasRole := n.Attributes["role"]
	attrs := dbCommon(n.ID, hasID, role, hasRole, reftext, hasReftext)
	if n.hasTitle {
		title := n.Title()
		spacer := ""
		if n.ContentModel == "compound" {
			spacer = "\n"
		}
		return "<formalpara" + attrs + ">\n<title>" + title + "</title>\n<para>" + spacer + n.content() + spacer + "</para>\n</formalpara>"
	}
	if hasID || hasRole {
		if n.ContentModel == "compound" {
			return "<para" + attrs + ">\n" + n.content() + "\n</para>"
		}
		return "<simpara" + attrs + ">" + n.content() + "</simpara>"
	}
	return dbEnclose(n)
}

func (c *docbook5Converter) blockquote(n *Block, attrs string, body func() string) string {
	tag := "blockquote"
	if n.hasRole("epigraph") {
		tag = "epigraph"
	}
	result := []string{"<" + tag + attrs + ">"}
	if n.hasTitle {
		result = append(result, "<title>"+n.Title()+"</title>")
	}
	attribution, hasAttr := n.Attributes["attribution"]
	citetitle, hasCite := n.Attributes["citetitle"]
	if hasAttr || hasCite {
		result = append(result, "<attribution>")
		if hasAttr {
			result = append(result, attribution)
		}
		if hasCite {
			result = append(result, "<citetitle>"+citetitle+"</citetitle>")
		}
		result = append(result, "</attribution>")
	}
	result = append(result, body(), "</"+tag+">")
	return strings.Join(result, "\n")
}

func (c *docbook5Converter) table(n *Block) string {
	t := n.Table
	doc := n.Document
	var result []string
	pgwide := ""
	if n.hasOption("pgwide") {
		pgwide = ` pgwide="1"`
	}
	frame, ok := n.attrFallback2("frame", "table-frame")
	if !ok {
		frame = "all"
	}
	if frame == "ends" {
		frame = "topbot"
	}
	grid, _ := n.attrFallback2("grid", "table-grid")
	rowsep, colsep := "1", "1"
	if grid == "none" || grid == "cols" {
		rowsep = "0"
	}
	if grid == "none" || grid == "rows" {
		colsep = "0"
	}
	orient := ""
	if o, _ := n.attrFallback2("orientation", "table-orientation"); o == "landscape" {
		orient = ` orient="land"`
	}
	tag := "informaltable"
	if n.hasTitle {
		tag = "table"
	}
	result = append(result, "<"+tag+dbAttrs(n)+pgwide+` frame="`+frame+`" rowsep="`+rowsep+`" colsep="`+colsep+`"`+orient+">")
	if n.hasOption("unbreakable") {
		result = append(result, `<?dbfo keep-together="always"?>`)
	} else if n.hasOption("breakable") {
		result = append(result, `<?dbfo keep-together="auto"?>`)
	}
	if tag == "table" {
		result = append(result, "<title>"+n.Title()+"</title>")
	}
	width, hasWidth := n.Attributes["width"]
	if hasWidth {
		for _, pi := range [...]string{"dbhtml", "dbfo", "dblatex"} {
			result = append(result, "<?"+pi+` table-width="`+width+`"?>`)
		}
	}
	result = append(result, `<tgroup cols="`+n.Attributes["colcount"]+`">`)
	for _, col := range t.Columns {
		w := col.Pcwidth
		if hasWidth {
			w = col.Abswidth
		}
		result = append(result, `<colspec colname="col_`+strconv.Itoa(col.Colnumber)+`" colwidth="`+w.String()+`*"/>`)
	}
	sections := []struct {
		name string
		rows [][]*Cell
	}{{"head", t.Head}, {"body", t.Body}, {"foot", t.Foot}}
	for _, sec := range sections {
		if len(sec.rows) == 0 {
			continue
		}
		result = append(result, "<t"+sec.name+">")
		for _, row := range sec.rows {
			result = append(result, "<row>")
			for _, cell := range row {
				span := ""
				if cell.Colspan != 0 {
					colnum := cell.column.Colnumber
					span = ` namest="col_` + strconv.Itoa(colnum) + `" nameend="col_` + strconv.Itoa(colnum+cell.Colspan-1) + `"`
				}
				if cell.Rowspan != 0 {
					span += ` morerows="` + strconv.Itoa(cell.Rowspan-1) + `"`
				}
				start := `<entry align="` + cell.Halign + `" valign="` + cell.Valign + `"` + span + ">"
				var content string
				if sec.name == "head" {
					content = cell.text()
				} else {
					switch cell.Style {
					case "asciidoc":
						content = cell.InnerDoc.convertDocument()
					case "literal":
						content = `<literallayout class="monospaced">` + cell.text() + "</literallayout>"
					case "header":
						if paras := cell.content(); len(paras) > 0 {
							content = `<simpara><emphasis role="strong">` + strings.Join(paras, `</emphasis></simpara><simpara><emphasis role="strong">`) + "</emphasis></simpara>"
						}
					default:
						if paras := cell.content(); len(paras) > 0 {
							content = "<simpara>" + strings.Join(paras, "</simpara><simpara>") + "</simpara>"
						}
					}
				}
				end := "</entry>"
				if bg, ok := doc.Attributes["cellbgcolor"]; ok {
					end = `<?dbfo bgcolor="` + bg + `"?></entry>`
				}
				result = append(result, start+content+end)
			}
			result = append(result, "</row>")
		}
		result = append(result, "</t"+sec.name+">")
	}
	result = append(result, "</tgroup>", "</"+tag+">")
	return strings.Join(result, "\n")
}

// ---------------------------------------------------------------------------
// Document

func (c *docbook5Converter) findRootAbstract(doc *Document) *Block {
	if len(doc.Blocks) == 0 {
		return nil
	}
	first := doc.Blocks[0]
	if first.Context == "preamble" {
		if len(first.Blocks) == 0 {
			return nil
		}
		first = first.Blocks[0]
	} else if first.Context == "section" {
		if first.Sectname == "abstract" {
			return first
		}
		if first.Sectname != "preface" || len(first.Blocks) == 0 {
			return nil
		}
		first = first.Blocks[0]
	}
	if first.Style == "abstract" && first.Context == "open" {
		return first
	}
	return nil
}

// extractAbstract removes the abstract (or the container that holds nothing
// else) from the tree and returns the removed block and its parent.
func (c *docbook5Converter) extractAbstract(doc *Document, abstract *Block) (*Block, *Block) {
	parent := abstract.Parent
	for parent != &doc.Block && len(parent.Blocks) == 1 && parent.Parent != nil {
		parent = parent.Parent
	}
	removed := parent.Blocks[0]
	parent.Blocks = parent.Blocks[1:]
	return removed, parent
}

func restoreAbstract(removed, parent *Block) {
	parent.Blocks = append([]*Block{removed}, parent.Blocks...)
}

// blocksContent converts the top-level blocks, dropping those the converter
// skips (Ruby: node.blocks.map {|b| b.convert }.compact.join LF).
func (c *docbook5Converter) blocksContent(doc *Document) string {
	var parts []string
	for _, b := range doc.Blocks {
		if s, ok := c.convertNode(b); ok {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

func (c *docbook5Converter) embedded(w *bytes.Buffer, doc *Document) {
	abstract := c.findRootAbstract(doc)
	var removed, parent *Block
	if abstract != nil {
		removed, parent = c.extractAbstract(doc, abstract)
	}
	w.WriteString(c.blocksContent(doc))
	if removed != nil {
		restoreAbstract(removed, parent)
	}
}

func (c *docbook5Converter) document(w *bytes.Buffer, doc *Document) {
	result := []string{`<?xml version="1.0" encoding="UTF-8"?>`}
	if doc.hasAttr("toc") {
		if v, ok := doc.Attributes["toclevels"]; ok {
			result = append(result, `<?asciidoc-toc maxdepth="`+v+`"?>`)
		} else {
			result = append(result, "<?asciidoc-toc?>")
		}
	}
	if doc.hasAttr("sectnums") {
		if v, ok := doc.Attributes["sectnumlevels"]; ok {
			result = append(result, `<?asciidoc-numbered maxdepth="`+v+`"?>`)
		} else {
			result = append(result, "<?asciidoc-numbered?>")
		}
	}
	lang := ""
	if !doc.hasAttr("nolang") {
		lang = ` xml:lang="` + doc.attrOr("lang", "en") + `"`
	}
	rootTag := doc.Doctype
	manpage := rootTag == "manpage"
	if manpage {
		rootTag = "article"
	}
	rootIdx := len(result)
	id, hadID := doc.ID, doc.ID != ""
	abstract := c.findRootAbstract(doc)
	if !doc.hasAttr("noheader") {
		result = append(result, c.documentInfo(doc, abstract))
	}
	if manpage {
		result = append(result, "<refentry>", "<refmeta>")
		if v, ok := doc.Attributes["mantitle"]; ok {
			result = append(result, "<refentrytitle>"+doc.applySubs(v, reftextSubs)+"</refentrytitle>")
		}
		if v, ok := doc.Attributes["manvolnum"]; ok {
			result = append(result, "<manvolnum>"+v+"</manvolnum>")
		}
		result = append(result, `<refmiscinfo class="source">`+doc.attrOr("mansource", "&#160;")+"</refmiscinfo>")
		result = append(result, `<refmiscinfo class="manual">`+doc.attrOr("manmanual", "&#160;")+"</refmiscinfo>")
		result = append(result, "</refmeta>", "<refnamediv>")
		for _, n := range doc.mannames {
			result = append(result, "<refname>"+n+"</refname>")
		}
		if v, ok := doc.Attributes["manpurpose"]; ok {
			result = append(result, "<refpurpose>"+v+"</refpurpose>")
		}
		result = append(result, "</refnamediv>")
	}
	if di := doc.docinfo("header"); di != "" {
		result = append(result, di)
	}
	var removed, parent *Block
	if abstract != nil {
		removed, parent = c.extractAbstract(doc, abstract)
	}
	if len(doc.Blocks) > 0 {
		result = append(result, c.blocksContent(doc))
	}
	if removed != nil {
		restoreAbstract(removed, parent)
	}
	if di := doc.docinfo("footer"); di != "" {
		result = append(result, di)
	}
	if manpage {
		result = append(result, "</refentry>")
	}
	if !hadID {
		// an id generated while converting (for a reference to the document
		// itself) is used for the root element and then discarded
		id, hadID = doc.ID, doc.ID != ""
		doc.ID = ""
	}
	root := "<" + rootTag + ` xmlns="http://docbook.org/ns/docbook" xmlns:xl="http://www.w3.org/1999/xlink" version="5.0"` + lang + dbCommon(id, hadID, "", false, "", false) + ">"
	result = append(result[:rootIdx], append([]string{root}, result[rootIdx:]...)...)
	result = append(result, "</"+rootTag+">")
	w.WriteString(strings.Join(result, "\n"))
}

func (c *docbook5Converter) authorTag(a author) string {
	result := []string{"<author>", "<personname>"}
	if a.hasFirstname {
		result = append(result, "<firstname>"+subReplacements(a.firstname)+"</firstname>")
	}
	if a.hasMiddlename {
		result = append(result, "<othername>"+subReplacements(a.middlename)+"</othername>")
	}
	if a.hasLastname {
		result = append(result, "<surname>"+subReplacements(a.lastname)+"</surname>")
	}
	result = append(result, "</personname>")
	if a.hasEmail {
		result = append(result, "<email>"+a.email+"</email>")
	}
	result = append(result, "</author>")
	return strings.Join(result, "\n")
}

func (c *docbook5Converter) documentInfo(doc *Document, abstract *Block) string {
	result := []string{"<info>"}
	if !doc.hasAttr("notitle") {
		title, _ := doc.doctitle(true)
		sep, ok := doc.Attributes["title-separator"]
		if !ok {
			sep = ":"
		}
		if i := strings.LastIndex(title, sep+" "); sep != "" && i >= 0 {
			result = append(result, "<title>"+title[:i]+"</title>\n<subtitle>"+title[i+len(sep)+1:]+"</subtitle>")
		} else {
			result = append(result, "<title>"+title+"</title>")
		}
	}
	if v, ok := doc.Attributes["revdate"]; ok {
		result = append(result, "<date>"+v+"</date>")
	} else if !doc.hasAttr("reproducible") {
		if v, ok := doc.Attributes["docdate"]; ok {
			result = append(result, "<date>"+v+"</date>")
		}
	}
	if v, ok := doc.Attributes["copyright"]; ok {
		result = append(result, "<copyright>")
		if m := copyrightRx.Find(v); m != nil {
			result = append(result, "<holder>"+m.Group(1)+"</holder>")
			if m.Has(2) {
				result = append(result, "<year>"+m.Group(2)+"</year>")
			}
		} else {
			result = append(result, "<holder></holder>")
		}
		result = append(result, "</copyright>")
	}
	if doc.hasHeader() {
		if authors := doc.authors(); len(authors) > 1 {
			result = append(result, "<authorgroup>")
			for _, a := range authors {
				result = append(result, c.authorTag(a))
			}
			result = append(result, "</authorgroup>")
		} else if len(authors) == 1 {
			result = append(result, c.authorTag(authors[0]))
			if authors[0].hasInitials {
				result = append(result, "<authorinitials>"+authors[0].initials+"</authorinitials>")
			}
		}
		_, hasRevnumber := doc.Attributes["revnumber"]
		_, hasRevremark := doc.Attributes["revremark"]
		if doc.hasAttr("revdate") && (hasRevnumber || hasRevremark) {
			result = append(result, "<revhistory>\n<revision>")
			if hasRevnumber {
				result = append(result, "<revnumber>"+doc.Attributes["revnumber"]+"</revnumber>")
			}
			result = append(result, "<date>"+doc.Attributes["revdate"]+"</date>")
			if v, ok := doc.Attributes["authorinitials"]; ok {
				result = append(result, "<authorinitials>"+v+"</authorinitials>")
			}
			if hasRevremark {
				result = append(result, "<revremark>"+doc.Attributes["revremark"]+"</revremark>")
			}
			result = append(result, "</revision>\n</revhistory>")
		}
		if doc.hasAttr("front-cover-image") || doc.hasAttr("back-cover-image") {
			if back, ok := c.coverTag(doc, "back", false); ok {
				front, _ := c.coverTag(doc, "front", true)
				result = append(result, front, back)
			} else if front, ok := c.coverTag(doc, "front", false); ok {
				result = append(result, front)
			}
		}
		if v, ok := doc.Attributes["orgname"]; ok {
			result = append(result, "<orgname>"+v+"</orgname>")
		}
		if di := doc.docinfo("head"); di != "" {
			result = append(result, di)
		}
	}
	if abstract != nil {
		abstract.setOption("root")
		s, _ := c.convertRaw(abstract)
		result = append(result, s)
		delete(abstract.Attributes, "root-option")
	}
	result = append(result, "</info>")
	return strings.Join(result, "\n")
}

func (c *docbook5Converter) coverTag(doc *Document, face string, placeholder bool) (string, bool) {
	cover, ok := doc.Attributes[face+"-cover-image"]
	if !ok {
		if placeholder {
			return `<cover role="` + face + `"/>`, true
		}
		return "", false
	}
	size := ""
	if strings.Contains(cover, ":") {
		if m := imageMacroRx.Find(cover); m != nil {
			cover = doc.imageURI(m.Group(1), "imagesdir")
			if m.Has(2) {
				size = dbImageSizeAttrs(parseAttrList(m.Group(2), nil, []string{"alt", "width", "height"}))
			}
		}
	}
	return `<cover role="` + face + "\">\n<mediaobject>\n<imageobject>\n<imagedata fileref=\"" + cover + `"` + size + "/>\n</imageobject>\n</mediaobject>\n</cover>", true
}

// ---------------------------------------------------------------------------
// Inline nodes

func (c *docbook5Converter) convertInline(n *Inline) string {
	switch n.Context {
	case "quoted":
		return c.inlineQuoted(n)
	case "anchor":
		return c.inlineAnchor(n)
	case "break":
		return n.Text + "<?asciidoc-br?>"
	case "button":
		return "<guibutton>" + n.Text + "</guibutton>"
	case "callout":
		return "<co" + dbCommon(n.ID, n.ID != "", "", false, "", false) + "/>"
	case "footnote":
		if n.Type == "xref" {
			return `<footnoteref linkend="` + n.Target + `"/>`
		}
		return "<footnote" + dbCommon(n.ID, n.ID != "", "", false, "", false) + "><simpara>" + n.Text + "</simpara></footnote>"
	case "image":
		return c.inlineImage(n)
	case "indexterm":
		return c.inlineIndexterm(n)
	case "kbd":
		if len(n.Keys) == 1 {
			return "<keycap>" + n.Keys[0] + "</keycap>"
		}
		return "<keycombo><keycap>" + strings.Join(n.Keys, "</keycap><keycap>") + "</keycap></keycombo>"
	case "menu":
		menu := n.Attrs["menu"]
		item, hasItem := n.Attrs["menuitem"]
		if len(n.Submenus) == 0 {
			if hasItem {
				return "<menuchoice><guimenu>" + menu + "</guimenu> <guimenuitem>" + item + "</guimenuitem></menuchoice>"
			}
			return "<guimenu>" + menu + "</guimenu>"
		}
		return "<menuchoice><guimenu>" + menu + "</guimenu> <guisubmenu>" + strings.Join(n.Submenus, "</guisubmenu> <guisubmenu>") + "</guisubmenu> <guimenuitem>" + item + "</guimenuitem></menuchoice>"
	}
	return n.Text
}

func (c *docbook5Converter) inlineAnchor(n *Inline) string {
	switch n.Type {
	case "ref":
		reftext := "[" + n.ID + "]"
		if n.hasText {
			reftext = n.Parent.applySubs(n.Text, reftextSubs)
		}
		return "<anchor" + dbCommon(n.ID, true, "", false, reftext, true) + "/>"
	case "xref":
		if path, ok := n.Attrs["path"]; ok {
			text := path
			if n.hasText {
				text = n.Text
			}
			return `<link xl:href="` + n.Target + `">` + text + "</link>"
		}
		linkend := n.Attrs["refid"]
		if linkend == "" {
			root := n.Parent.Document
			for root.parentDocument != nil {
				root = root.parentDocument
			}
			if root.ID == "" {
				root.ID = "__" + root.Doctype + "-root__"
			}
			linkend = root.ID
		}
		if n.hasText {
			return `<link linkend="` + linkend + `">` + n.Text + "</link>"
		}
		return `<xref linkend="` + linkend + `"/>`
	case "link":
		return `<link xl:href="` + n.Target + `">` + n.Text + "</link>"
	case "bibref":
		text := n.ID
		if n.hasText {
			text = n.Parent.applySubs(n.Text, reftextSubs)
		}
		text = "[" + text + "]"
		return "<anchor" + dbCommon(n.ID, true, "", false, text, true) + "/>" + text
	}
	return ""
}

func (c *docbook5Converter) inlineImage(n *Inline) string {
	role, hasRole := n.Attrs["role"]
	var fileref string
	icon := n.Type == "icon"
	if icon {
		tmp := &Block{Document: n.Parent.Document, Attributes: n.Attrs, Parent: n.Parent}
		fileref = tmp.iconURI(n.Target)
	} else {
		fileref = n.Parent.imageURI(n.Target, "imagesdir")
	}
	img := "<inlinemediaobject" + dbCommon("", false, role, hasRole, "", false) + ">\n<imageobject>\n<imagedata fileref=\"" + fileref + `"` + dbImageSizeAttrs(n.Attrs) + "/>\n</imageobject>\n<textobject><phrase>" + n.Attrs["alt"] + "</phrase></textobject>\n</inlinemediaobject>"
	if link, ok := n.Attrs["link"]; ok && !icon && fileref != "" {
		return `<link xl:href="` + link + `">` + img + "</link>"
	}
	return img
}

func (c *docbook5Converter) inlineIndexterm(n *Inline) string {
	rel := ""
	if n.hasSee {
		rel = "\n<see>" + n.See + "</see>"
	} else if n.hasSeeAlso {
		for _, sa := range n.SeeAlso {
			rel += "\n<seealso>" + sa + "</seealso>"
		}
	}
	if n.Type == "visible" {
		return "<indexterm>\n<primary>" + n.Text + "</primary>" + rel + "\n</indexterm>" + n.Text
	}
	_, promote := n.Parent.Document.Attributes["indexterm-promotion-option"]
	terms := n.Terms
	switch {
	case len(terms) > 2:
		out := "<indexterm>\n<primary>" + terms[0] + "</primary><secondary>" + terms[1] + "</secondary><tertiary>" + terms[2] + "</tertiary>" + rel + "\n</indexterm>"
		if promote {
			out += "\n<indexterm>\n<primary>" + terms[1] + "</primary><secondary>" + terms[2] + "</secondary>\n</indexterm>\n<indexterm>\n<primary>" + terms[2] + "</primary>\n</indexterm>"
		}
		return out
	case len(terms) > 1:
		out := "<indexterm>\n<primary>" + terms[0] + "</primary><secondary>" + terms[1] + "</secondary>" + rel + "\n</indexterm>"
		if promote {
			out += "\n<indexterm>\n<primary>" + terms[1] + "</primary>\n</indexterm>"
		}
		return out
	}
	first := ""
	if len(terms) > 0 {
		first = terms[0]
	}
	return "<indexterm>\n<primary>" + first + "</primary>" + rel + "\n</indexterm>"
}

func (c *docbook5Converter) inlineQuoted(n *Inline) string {
	switch n.Type {
	case "asciimath":
		return "<inlineequation><mathphrase><![CDATA[" + n.Text + "]]></mathphrase></inlineequation>"
	case "latexmath":
		return "<inlineequation><alt><![CDATA[" + n.Text + "]]></alt><mathphrase><![CDATA[" + n.Text + "]]></mathphrase></inlineequation>"
	}
	open, closeTag, phrase := "", "", true
	if qt, ok := dbQuoteTags[n.Type]; ok {
		open, closeTag, phrase = qt.open, qt.close, qt.phrase
	}
	var quoted string
	if role, ok := n.Attrs["role"]; ok {
		if phrase {
			quoted = open + `<phrase role="` + role + `">` + n.Text + "</phrase>" + closeTag
		} else {
			quoted = open[:len(open)-1] + ` role="` + role + `">` + n.Text + closeTag
		}
	} else {
		quoted = open + n.Text + closeTag
	}
	if n.ID != "" {
		return "<anchor" + dbCommon(n.ID, true, "", false, "", false) + "/>" + quoted
	}
	return quoted
}
