package adocgo

import (
	_ "embed"
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"github.com/jdkato/adocgo/internal/rx"
)

// This file is a port of Asciidoctor's converter/html5.rb.

//go:embed data/asciidoctor-default.css
var defaultStylesheet string

const (
	fontAwesomeVersion = "4.7.0"
	highlightJSVersion = "9.18.3"
	mathjaxVersion     = "2.7.9"
)

var (
	dropAnchorRx         = rx.MustCompile(`<(?:a\b[^>]*|/a)>`)
	leadingAnchorsRx     = rx.MustCompile(`^(?:<a id="[^"]+"></a>)+`)
	stemBreakRx          = rx.MustCompile(` *\\\n(?:\\?\n)*|\n\n+`)
	svgPreambleRx        = rx.MustCompile(`\A.*?(?=<svg[\s>])`, "m")
	svgStartTagRx        = rx.MustCompile(`\A<svg(?:\s[^>]*)?>`)
	dimensionAttributeRx = rx.MustCompile(`\s(?:width|height|style)=(["']).*?\1`)
)

type html5Converter struct {
	doc           *Document
	resolvingXref bool
}

type quoteTag struct {
	open, close string
	tag         bool
}

var quoteTags = map[string]quoteTag{
	"monospaced":  {"<code>", "</code>", true},
	"emphasis":    {"<em>", "</em>", true},
	"strong":      {"<strong>", "</strong>", true},
	"double":      {"&#8220;", "&#8221;", false},
	"single":      {"&#8216;", "&#8217;", false},
	"mark":        {"<mark>", "</mark>", true},
	"superscript": {"<sup>", "</sup>", true},
	"subscript":   {"<sub>", "</sub>", true},
	"asciimath":   {`\$`, `\$`, false},
	"latexmath":   {`\(`, `\)`, false},
}

// convert converts a block node (Ruby: node.convert).
func (b *Block) convert() string {
	b.Document.playbackAttributes(b.Attributes)
	return b.Document.conv.convertBlock(b)
}

// content returns the converted content of the block.
func (b *Block) content() string {
	switch b.ContentModel {
	case "compound":
		if b.Context == "document" {
			delete(b.Document.Attributes, "title")
		}
		parts := make([]string, len(b.Blocks))
		for i, c := range b.Blocks {
			parts[i] = c.convert()
		}
		return strings.Join(parts, "\n")
	case "simple":
		return b.applySubs(strings.Join(b.Lines, "\n"), b.subs)
	case "verbatim", "raw":
		result := strings.Split(b.applySubs(strings.Join(b.Lines, "\n"), b.subs), "\n")
		if len(b.Lines) == 0 {
			return ""
		}
		if len(result) < 2 {
			return result[0]
		}
		for len(result) > 0 && rstrip(result[0]) == "" {
			result = result[1:]
		}
		for len(result) > 0 && rstrip(result[len(result)-1]) == "" {
			result = result[:len(result)-1]
		}
		return strings.Join(result, "\n")
	}
	return ""
}

func (d *Document) convertDocument() string {
	if d.Doctype == "inline" {
		var block *Block
		if len(d.Blocks) > 0 {
			block = d.Blocks[0]
		} else if d.header != nil {
			block = d.header
		}
		if block == nil || block.ContentModel == "compound" || block.ContentModel == "empty" {
			return ""
		}
		return block.content()
	}
	if d.standalone && d.parentDocument == nil {
		return d.conv.document(d)
	}
	return d.conv.embedded(d)
}

func idAttr(id string) string {
	if id != "" {
		return ` id="` + id + `"`
	}
	return ""
}

func roleSuffix(b *Block) string {
	if r, ok := b.Attributes["role"]; ok {
		return " " + r
	}
	return ""
}

func (c *html5Converter) convertBlock(n *Block) string {
	switch n.Context {
	case "paragraph":
		return c.paragraph(n)
	case "section":
		return c.section(n)
	case "listing":
		return c.listing(n)
	case "literal":
		return c.literal(n)
	case "ulist":
		return c.ulist(n)
	case "olist":
		return c.olist(n)
	case "dlist":
		return c.dlist(n)
	case "admonition":
		return c.admonition(n)
	case "colist":
		res := c.colist(n)
		n.Document.callouts.nextList()
		return res
	case "example":
		return c.example(n)
	case "floating_title":
		return c.floatingTitle(n)
	case "image":
		return c.image(n)
	case "open":
		return c.open(n)
	case "page_break":
		return `<div style="page-break-after: always;"></div>`
	case "preamble":
		return c.preamble(n)
	case "quote":
		return c.quote(n)
	case "sidebar":
		return c.sidebar(n)
	case "stem":
		return c.stem(n)
	case "table":
		return c.table(n)
	case "thematic_break":
		return "<hr>"
	case "verse":
		return c.verse(n)
	case "video":
		return c.video(n)
	case "toc":
		return c.toc(n)
	case "pass":
		return n.content()
	case "audio":
		return c.audio(n)
	}
	return n.content()
}

func (d *Document) attrOr(name, def string) string {
	if v, ok := d.Attributes[name]; ok {
		return v
	}
	return def
}

func (d *Document) hasAttr(name string) bool {
	_, ok := d.Attributes[name]
	return ok
}

type author struct {
	name, email string
	hasEmail    bool
}

func (d *Document) authors() []author {
	attrs := d.Attributes
	name, ok := attrs["author"]
	if !ok {
		return nil
	}
	email, hasEmail := attrs["email"]
	out := []author{{name, email, hasEmail}}
	n := atoi(attrs["authorcount"])
	for idx := 2; idx <= n; idx++ {
		s := strconv.Itoa(idx)
		e, he := attrs["email_"+s]
		out = append(out, author{attrs["author_"+s], e, he})
	}
	return out
}

func (c *html5Converter) footnotesDiv(d *Document, maxWidthAttr string) []string {
	var result []string
	if len(*d.footnotes) > 0 && !d.hasAttr("nofootnotes") {
		result = append(result, `<div id="footnotes"`+maxWidthAttr+">\n<hr>")
		for _, fn := range *d.footnotes {
			result = append(result, `<div class="footnote" id="_footnotedef_`+fn.index+"\">\n<a href=\"#_footnoteref_"+fn.index+`">`+fn.index+"</a>. "+fn.text+"\n</div>")
		}
		result = append(result, "</div>")
	}
	return result
}

func (c *html5Converter) document(node *Document) string {
	br := "<br>"
	assetURIScheme := node.attrOr("asset-uri-scheme", "https")
	if assetURIScheme != "" {
		assetURIScheme += ":"
	}
	cdnBaseURL := assetURIScheme + "//cdnjs.cloudflare.com/ajax/libs"
	linkcss := node.hasAttr("linkcss")
	maxWidthAttr := ""
	if node.hasAttr("max-width") {
		maxWidthAttr = ` style="max-width: ` + node.Attributes["max-width"] + `;"`
	}
	result := []string{"<!DOCTYPE html>"}
	langAttribute := ""
	if !node.hasAttr("nolang") {
		langAttribute = ` lang="` + node.attrOr("lang", "en") + `"`
	}
	result = append(result, "<html"+langAttribute+">")
	result = append(result, `<head>
<meta charset="`+node.attrOr("encoding", "UTF-8")+`">
<meta http-equiv="X-UA-Compatible" content="IE=edge">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="generator" content="Asciidoctor `+node.Attributes["asciidoctor-version"]+`">`)
	for _, m := range [][2]string{{"app-name", "application-name"}, {"description", "description"}, {"keywords", "keywords"}} {
		if node.hasAttr(m[0]) {
			result = append(result, `<meta name="`+m[1]+`" content="`+node.Attributes[m[0]]+`">`)
		}
	}
	if node.hasAttr("authors") {
		authors := subReplacements(node.Attributes["authors"])
		if strings.Contains(authors, "<") {
			authors = xmlSanitizeRx.ReplaceAll(authors, "")
		}
		result = append(result, `<meta name="author" content="`+authors+`">`)
	}
	if node.hasAttr("copyright") {
		result = append(result, `<meta name="copyright" content="`+node.Attributes["copyright"]+`">`)
	}
	if node.hasAttr("favicon") {
		iconHref := node.Attributes["favicon"]
		iconType := "image/x-icon"
		if iconHref == "" {
			iconHref = "favicon.ico"
		} else if ext := extname(iconHref); ext != "" && ext != ".ico" {
			iconType = "image/" + ext[1:]
		}
		result = append(result, `<link rel="icon" type="`+iconType+`" href="`+iconHref+`">`)
	}
	title, _ := node.doctitle(true)
	result = append(result, "<title>"+sanitizeTitle(title)+"</title>")
	if ss, ok := node.Attributes["stylesheet"]; ok && (ss == "" || ss == "DEFAULT") {
		if webfonts, ok := node.Attributes["webfonts"]; ok {
			if webfonts == "" {
				webfonts = "Open+Sans:300,300italic,400,400italic,600,600italic%7CNoto+Serif:400,400italic,700,700italic%7CDroid+Sans+Mono:400,700"
			}
			result = append(result, `<link rel="stylesheet" href="`+assetURIScheme+"//fonts.googleapis.com/css?family="+webfonts+`">`)
		}
		if linkcss {
			result = append(result, `<link rel="stylesheet" href="`+node.normalizeWebPath("asciidoctor.css", node.attrOr("stylesdir", ""), false)+`">`)
		} else {
			result = append(result, "<style>\n"+rstrip(defaultStylesheet)+"\n</style>")
		}
	} else if ok {
		if linkcss {
			result = append(result, `<link rel="stylesheet" href="`+node.normalizeWebPath(ss, node.attrOr("stylesdir", ""), true)+`">`)
		} else {
			data, _ := os.ReadFile(node.normalizeSystemPath(ss, node.Attributes["stylesdir"]))
			result = append(result, "<style>\n"+string(data)+"\n</style>")
		}
	}
	if node.Attributes["icons"] == "font" {
		if node.hasAttr("iconfont-remote") {
			result = append(result, `<link rel="stylesheet" href="`+node.attrOr("iconfont-cdn", cdnBaseURL+"/font-awesome/"+fontAwesomeVersion+"/css/font-awesome.min.css")+`">`)
		} else {
			result = append(result, `<link rel="stylesheet" href="`+node.normalizeWebPath(node.attrOr("iconfont-name", "font-awesome")+".css", node.attrOr("stylesdir", ""), false)+`">`)
		}
	}
	hljsBase := node.attrOr("highlightjsdir", cdnBaseURL+"/highlight.js/"+highlightJSVersion)
	if node.highlight {
		result = append(result, `<link rel="stylesheet" href="`+hljsBase+"/styles/"+node.attrOr("highlightjs-theme", "github")+`.min.css">`)
	}
	result = append(result, "</head>")
	sectioned := node.hasSections()
	classes := []string{node.Doctype}
	if sectioned && node.hasAttr("toc-class") && node.hasAttr("toc") && node.Attributes["toc-placement"] == "auto" {
		classes = append(classes, node.Attributes["toc-class"], "toc-"+node.attrOr("toc-position", "header"))
	}
	if r, ok := node.Attributes["role"]; ok {
		classes = append(classes, r)
	}
	result = append(result, "<body"+idAttr(node.ID)+` class="`+strings.Join(classes, " ")+`">`)
	tocDiv := func() string {
		return `<div id="toc" class="` + node.attrOr("toc-class", "toc") + "\">\n<div id=\"toctitle\">" + node.Attributes["toc-title"] + "</div>\n" + c.outline(&node.Block, 0, 0) + "\n</div>"
	}
	if !node.hasAttr("noheader") {
		result = append(result, `<div id="header"`+maxWidthAttr+">")
		if node.Doctype == "manpage" {
			dt, _ := node.doctitle(false)
			result = append(result, "<h1>"+dt+" Manual Page</h1>")
			if sectioned && node.hasAttr("toc") && node.Attributes["toc-placement"] == "auto" {
				result = append(result, tocDiv())
			}
			if node.hasAttr("manpurpose") {
				result = append(result, c.mannameSection(node))
			}
		} else {
			if node.hasHeader() {
				if !node.hasAttr("notitle") {
					result = append(result, "<h1>"+node.header.Title()+"</h1>")
				}
				var details []string
				for i, a := range node.authors() {
					sfx := ""
					if i > 0 {
						sfx = strconv.Itoa(i + 1)
					}
					details = append(details, `<span id="author`+sfx+`" class="author">`+subReplacements(a.name)+"</span>"+br)
					if a.hasEmail {
						details = append(details, `<span id="email`+sfx+`" class="email">`+node.subMacros(a.email)+"</span>"+br)
					}
				}
				if node.hasAttr("revnumber") {
					comma := ""
					if node.hasAttr("revdate") {
						comma = ","
					}
					details = append(details, `<span id="revnumber">`+strings.ToLower(node.Attributes["version-label"])+" "+node.Attributes["revnumber"]+comma+"</span>")
				}
				if node.hasAttr("revdate") {
					details = append(details, `<span id="revdate">`+node.Attributes["revdate"]+"</span>")
				}
				if node.hasAttr("revremark") {
					details = append(details, br+`<span id="revremark">`+node.Attributes["revremark"]+"</span>")
				}
				if len(details) > 0 {
					result = append(result, `<div class="details">`)
					result = append(result, details...)
					result = append(result, "</div>")
				}
			}
			if sectioned && node.hasAttr("toc") && node.Attributes["toc-placement"] == "auto" {
				result = append(result, tocDiv())
			}
		}
		result = append(result, "</div>")
	}
	result = append(result, `<div id="content"`+maxWidthAttr+">\n"+node.content()+"\n</div>")
	result = append(result, c.footnotesDiv(node, maxWidthAttr)...)
	if !node.hasAttr("nofooter") {
		result = append(result, `<div id="footer"`+maxWidthAttr+">", `<div id="footer-text">`)
		if node.hasAttr("revnumber") {
			result = append(result, node.Attributes["version-label"]+" "+node.Attributes["revnumber"]+br)
		}
		if node.hasAttr("last-update-label") && !node.hasAttr("reproducible") {
			result = append(result, node.Attributes["last-update-label"]+" "+node.Attributes["docdatetime"])
		}
		result = append(result, "</div>", "</div>")
	}
	if node.highlight {
		langs := ""
		if v, ok := node.Attributes["highlightjs-languages"]; ok {
			for _, lang := range rubySplit(v, ",") {
				langs += `<script src="` + hljsBase + "/languages/" + lstrip(lang) + ".min.js\"></script>\n"
			}
		}
		result = append(result, `<script src="`+hljsBase+`/highlight.min.js"></script>
`+langs+`<script>
if (!hljs.initHighlighting.called) {
  hljs.initHighlighting.called = true
  ;[].slice.call(document.querySelectorAll('pre.highlight > code[data-lang]')).forEach(function (el) { hljs.highlightBlock(el) })
}
</script>`)
	}
	if node.hasAttr("stem") {
		eqnums := node.attrOr("eqnums", "none")
		if eqnums == "" {
			eqnums = "AMS"
		}
		result = append(result, `<script type="text/x-mathjax-config">
MathJax.Hub.Config({
  messageStyle: "none",
  tex2jax: {
    inlineMath: [["\\(", "\\)"]],
    displayMath: [["\\[", "\\]"]],
    ignoreClass: "nostem|nolatexmath"
  },
  asciimath2jax: {
    delimiters: [["\\$", "\\$"]],
    ignoreClass: "nostem|noasciimath"
  },
  TeX: { equationNumbers: { autoNumber: "`+eqnums+`" } }
})
MathJax.Hub.Register.StartupHook("AsciiMath Jax Ready", function () {
  MathJax.InputJax.AsciiMath.postfilterHooks.Add(function (data, node) {
    if ((node = data.script.parentNode) && (node = node.parentNode) && node.classList.contains("stemblock")) {
      data.math.root.display = "block"
    }
    return data
  })
})
</script>
<script src="`+cdnBaseURL+"/mathjax/"+mathjaxVersion+`/MathJax.js?config=TeX-MML-AM_CHTML"></script>`)
	}
	result = append(result, "</body>", "</html>")
	return strings.Join(result, "\n")
}

func (c *html5Converter) embedded(node *Document) string {
	var result []string
	if node.Doctype == "manpage" {
		if !node.hasAttr("notitle") {
			dt, _ := node.doctitle(false)
			result = append(result, "<h1"+idAttr(node.ID)+">"+dt+" Manual Page</h1>")
		}
		if node.hasAttr("manpurpose") {
			result = append(result, c.mannameSection(node))
		}
	} else if node.hasHeader() && !node.hasAttr("notitle") {
		result = append(result, "<h1"+idAttr(node.ID)+">"+node.header.Title()+"</h1>")
	}
	if node.hasSections() && node.hasAttr("toc") {
		if p := node.Attributes["toc-placement"]; p != "macro" && p != "preamble" {
			result = append(result, "<div id=\"toc\" class=\"toc\">\n<div id=\"toctitle\">"+node.Attributes["toc-title"]+"</div>\n"+c.outline(&node.Block, 0, 0)+"\n</div>")
		}
	}
	result = append(result, node.content())
	result = append(result, c.footnotesDiv(node, "")...)
	return strings.Join(result, "\n")
}

func (c *html5Converter) mannameSection(node *Document) string {
	title := node.attrOr("manname-title", "Name")
	if sects := node.sections(); len(sects) > 0 {
		if t := sects[0].Title(); t == strings.ToUpper(t) {
			title = strings.ToUpper(title)
		}
	}
	return "<h2" + idAttr(node.Attributes["manname-id"]) + ">" + title + "</h2>\n<div class=\"sectionbody\">\n<p>" + strings.Join(node.mannames, ", ") + " - " + node.Attributes["manpurpose"] + "</p>\n</div>"
}

func (c *html5Converter) sectionTitle(section *Block, sectnumlevels int) string {
	doc := section.Document
	if section.hasCaption {
		return section.captionedTitle()
	}
	if section.Numbered && section.Level <= sectnumlevels {
		if section.Level < 2 && doc.Doctype == "book" {
			switch section.Sectname {
			case "chapter":
				sig := ""
				if s, ok := doc.Attributes["chapter-signifier"]; ok {
					sig = s + " "
				}
				return sig + section.sectnum(".", nil) + " " + section.Title()
			case "part":
				sig := ""
				if s, ok := doc.Attributes["part-signifier"]; ok {
					sig = s + " "
				}
				colon := ":"
				return sig + section.sectnum("", &colon) + " " + section.Title()
			}
		}
		return section.sectnum(".", nil) + " " + section.Title()
	}
	return section.Title()
}

func intAttr(attrs map[string]string, name string, def int) int {
	if v, ok := attrs[name]; ok {
		return atoi(v)
	}
	return def
}

func (c *html5Converter) outline(node *Block, toclevels, sectnumlevels int) string {
	if !node.hasSections() {
		return ""
	}
	docAttrs := node.Document.Attributes
	if sectnumlevels == 0 {
		sectnumlevels = intAttr(docAttrs, "sectnumlevels", 3)
	}
	if toclevels == 0 {
		toclevels = intAttr(docAttrs, "toclevels", 2)
	}
	sections := node.sections()
	if len(sections) == 0 {
		return ""
	}
	result := []string{`<ul class="sectlevel` + strconv.Itoa(sections[0].Level) + `">`}
	for _, section := range sections {
		stitle := c.sectionTitle(section, sectnumlevels)
		if strings.Contains(stitle, "<a") {
			stitle = dropAnchorRx.ReplaceAll(stitle, "")
		}
		child := ""
		if section.Level < toclevels {
			child = c.outline(section, toclevels, sectnumlevels)
		}
		if child != "" {
			result = append(result, `<li><a href="#`+section.ID+`">`+stitle+"</a>", child, "</li>")
		} else {
			result = append(result, `<li><a href="#`+section.ID+`">`+stitle+"</a></li>")
		}
	}
	result = append(result, "</ul>")
	return strings.Join(result, "\n")
}

func (c *html5Converter) section(node *Block) string {
	docAttrs := node.Document.Attributes
	level := node.Level
	title := c.sectionTitle(node, intAttr(docAttrs, "sectnumlevels", 3))
	ida := ""
	if id := node.ID; id != "" || node.emptyID {
		ida = ` id="` + id + `"`
		if _, ok := docAttrs["sectlinks"]; ok {
			if m := leadingAnchorsRx.Find(title); strings.HasPrefix(title, "<a ") && m != nil {
				title = m.Group(0) + `<a class="link" href="#` + id + `">` + title[len(m.Group(0)):] + "</a>"
			} else {
				title = `<a class="link" href="#` + id + `">` + title + "</a>"
			}
		}
		if sa, ok := docAttrs["sectanchors"]; ok {
			if sa == "after" {
				title = title + `<a class="anchor" href="#` + id + `"></a>`
			} else {
				title = `<a class="anchor" href="#` + id + `"></a>` + title
			}
		}
	}
	if level == 0 {
		return "<h1" + ida + ` class="sect0` + roleSuffix(node) + `">` + title + "</h1>\n" + node.content()
	}
	h := strconv.Itoa(level + 1)
	head := `<div class="sect` + strconv.Itoa(level) + roleSuffix(node) + "\">\n<h" + h + ida + ">" + title + "</h" + h + ">\n"
	if level == 1 {
		return head + "<div class=\"sectionbody\">\n" + node.content() + "\n</div>\n</div>"
	}
	return head + node.content() + "\n</div>"
}

func titleDiv(node *Block) string {
	if node.hasTitle {
		return `<div class="title">` + node.Title() + "</div>\n"
	}
	return ""
}

func (b *Block) docHasAttr(name string) bool {
	_, ok := b.Document.Attributes[name]
	return ok
}

func (b *Block) iconURI(name string) string {
	icontype := b.Document.attrOr("icontype", "png")
	var icon string
	if v, ok := b.Attributes["icon"]; ok {
		icon = v
		if !hasExtname(icon) {
			icon += "." + icontype
		}
	} else {
		icon = name + "." + icontype
	}
	return b.imageURI(icon, "iconsdir")
}

func (b *Block) imageURI(target, assetDirKey string) string {
	doc := b.Document
	assetDir, hasDir := "", false
	if assetDirKey != "" {
		assetDir, hasDir = doc.Attributes[assetDirKey]
	}
	if doc.Safe < SafeSecure && doc.hasAttr("data-uri") {
		if isUriish(target) {
			return encodeSpacesInURI(target)
		}
		if hasDir && isUriish(assetDir) {
			return b.normalizeWebPath(target, assetDir, false)
		}
		mimetype := "application/octet-stream"
		if ext := extname(target); ext != "" {
			if ext == ".svg" {
				mimetype = "image/svg+xml"
			} else {
				mimetype = "image/" + ext[1:]
			}
		}
		data, err := os.ReadFile(doc.normalizeSystemPath(target, assetDir))
		if err != nil {
			return "data:" + mimetype + ";base64,"
		}
		return "data:" + mimetype + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	return b.normalizeWebPath(target, assetDir, true)
}

func (b *Block) mediaURI(target string) string {
	return b.normalizeWebPath(target, b.Document.Attributes["imagesdir"], true)
}

func (b *Block) alt() string {
	text, ok := b.Attributes["alt"]
	if !ok {
		return ""
	}
	if da, ok := b.Attributes["default-alt"]; ok && text == da {
		return subSpecialchars(text)
	}
	text = subSpecialchars(text)
	if replaceableTextRx.MatchString(text) {
		return subReplacements(text)
	}
	return text
}

func encodeAttributeValue(val string) string { return strings.ReplaceAll(val, `"`, "&quot;") }

func (c *html5Converter) admonition(node *Block) string {
	name := node.Attributes["name"]
	titleElement := titleDiv(node)
	var label string
	if node.docHasAttr("icons") {
		if _, hasIcon := node.Attributes["icon"]; node.Document.Attributes["icons"] == "font" && !hasIcon {
			label = `<i class="fa icon-` + name + `" title="` + node.Attributes["textlabel"] + `"></i>`
		} else {
			label = `<img src="` + node.iconURI(name) + `" alt="` + node.Attributes["textlabel"] + `">`
		}
	} else {
		label = `<div class="title">` + node.Attributes["textlabel"] + "</div>"
	}
	return "<div" + idAttr(node.ID) + ` class="admonitionblock ` + name + roleSuffix(node) + "\">\n<table>\n<tr>\n<td class=\"icon\">\n" + label + "\n</td>\n<td class=\"content\">\n" + titleElement + node.content() + "\n</td>\n</tr>\n</table>\n</div>"
}

func classList(items ...string) string {
	var out []string
	for _, i := range items {
		if i != "" {
			out = append(out, i)
		}
	}
	return strings.Join(out, " ")
}

// roleOf returns the role attribute if present (an empty role still counts
// as absent for class lists built with compact).
func roleOf(b *Block) string { return b.Attributes["role"] }

func timeAnchor(node *Block) string {
	start, hasStart := node.Attributes["start"]
	end, hasEnd := node.Attributes["end"]
	if !hasStart && !hasEnd {
		return ""
	}
	s := "#t=" + start
	if hasEnd {
		s += "," + end
	}
	return s
}

func (c *html5Converter) audio(node *Block) string {
	title := titleDiv(node)
	opts := ""
	if node.hasOption("autoplay") {
		opts += " autoplay"
	}
	if !node.hasOption("nocontrols") {
		opts += " controls"
	}
	if node.hasOption("loop") {
		opts += " loop"
	}
	return "<div" + idAttr(node.ID) + ` class="` + joinClasses(node, "audioblock") + "\">\n" + title + "<div class=\"content\">\n<audio src=\"" + node.mediaURI(node.Attributes["target"]) + timeAnchor(node) + `"` + opts + ">\nYour browser does not support the audio tag.\n</audio>\n</div>\n</div>"
}

// joinClasses builds a class list from base classes plus the node role,
// mirroring `[..., node.role].compact.join ' '` (an empty role is kept).
func joinClasses(node *Block, base ...string) string {
	out := append([]string(nil), base...)
	if r, ok := node.Attributes["role"]; ok {
		out = append(out, r)
	}
	return strings.Join(out, " ")
}

func itemContent(item *Block) string {
	parts := make([]string, len(item.Blocks))
	for i, b := range item.Blocks {
		parts[i] = b.convert()
	}
	return strings.Join(parts, "\n")
}

func (c *html5Converter) colist(node *Block) string {
	var result []string
	base := []string{"colist"}
	if node.Style != "" {
		base = append(base, node.Style)
	}
	result = append(result, "<div"+idAttr(node.ID)+` class="`+joinClasses(node, base...)+`">`)
	if node.hasTitle {
		result = append(result, `<div class="title">`+node.Title()+"</div>")
	}
	if node.docHasAttr("icons") {
		result = append(result, "<table>")
		fontIcons := node.Document.Attributes["icons"] == "font"
		for i, item := range node.Blocks {
			num := strconv.Itoa(i + 1)
			var numLabel string
			if fontIcons {
				numLabel = `<i class="conum" data-value="` + num + `"></i><b>` + num + "</b>"
			} else {
				numLabel = `<img src="` + node.iconURI("callouts/"+num) + `" alt="` + num + `">`
			}
			extra := ""
			text := item.itemText()
			if item.hasBlocks() {
				extra = "\n" + itemContent(item)
			}
			result = append(result, "<tr>\n<td>"+numLabel+"</td>\n<td>"+text+extra+"</td>\n</tr>")
		}
		result = append(result, "</table>")
	} else {
		result = append(result, "<ol>")
		for _, item := range node.Blocks {
			text := item.itemText()
			extra := ""
			if item.hasBlocks() {
				extra = "\n" + itemContent(item)
			}
			result = append(result, "<li>\n<p>"+text+"</p>"+extra+"\n</li>")
		}
		result = append(result, "</ol>")
	}
	result = append(result, "</div>")
	return strings.Join(result, "\n")
}

func (c *html5Converter) dlist(node *Block) string {
	var result []string
	var base []string
	switch node.Style {
	case "qanda":
		base = []string{"qlist", "qanda"}
	case "horizontal":
		base = []string{"hdlist"}
	default:
		base = []string{"dlist"}
		if node.Style != "" {
			base = append(base, node.Style)
		}
	}
	result = append(result, "<div"+idAttr(node.ID)+` class="`+joinClasses(node, base...)+`">`)
	if node.hasTitle {
		result = append(result, `<div class="title">`+node.Title()+"</div>")
	}
	ddBody := func(dd *Block) {
		if dd.itemHasText() {
			result = append(result, "<p>"+dd.itemText()+"</p>")
		}
		if dd.hasBlocks() {
			result = append(result, itemContent(dd))
		}
	}
	switch node.Style {
	case "qanda":
		result = append(result, "<ol>")
		for _, e := range node.DlistItems {
			result = append(result, "<li>")
			for _, dt := range e.Terms {
				result = append(result, "<p><em>"+dt.itemText()+"</em></p>")
			}
			if e.Desc != nil {
				ddBody(e.Desc)
			}
			result = append(result, "</li>")
		}
		result = append(result, "</ol>")
	case "horizontal":
		result = append(result, "<table>")
		lw, hasLW := node.Attributes["labelwidth"]
		iw, hasIW := node.Attributes["itemwidth"]
		if hasLW || hasIW {
			result = append(result, "<colgroup>")
			st := ""
			if hasLW {
				st = ` style="width: ` + strings.TrimSuffix(lw, "%") + `%;"`
			}
			result = append(result, "<col"+st+">")
			st = ""
			if hasIW {
				st = ` style="width: ` + strings.TrimSuffix(iw, "%") + `%;"`
			}
			result = append(result, "<col"+st+">", "</colgroup>")
		}
		for _, e := range node.DlistItems {
			result = append(result, "<tr>")
			strong := ""
			if node.hasOption("strong") {
				strong = " strong"
			}
			result = append(result, `<td class="hdlist1`+strong+`">`)
			for i, dt := range e.Terms {
				if i > 0 {
					result = append(result, "<br>")
				}
				result = append(result, dt.itemText())
			}
			result = append(result, "</td>", `<td class="hdlist2">`)
			if e.Desc != nil {
				ddBody(e.Desc)
			}
			result = append(result, "</td>", "</tr>")
		}
		result = append(result, "</table>")
	default:
		result = append(result, "<dl>")
		dtStyle := ` class="hdlist1"`
		if node.Style != "" {
			dtStyle = ""
		}
		for _, e := range node.DlistItems {
			for _, dt := range e.Terms {
				result = append(result, "<dt"+dtStyle+">"+dt.itemText()+"</dt>")
			}
			if e.Desc == nil {
				continue
			}
			result = append(result, "<dd>")
			ddBody(e.Desc)
			result = append(result, "</dd>")
		}
		result = append(result, "</dl>")
	}
	result = append(result, "</div>")
	return strings.Join(result, "\n")
}

func (c *html5Converter) example(node *Block) string {
	if node.hasOption("collapsible") {
		classAttr := ""
		if r, ok := node.Attributes["role"]; ok {
			classAttr = ` class="` + r + `"`
		}
		summary := `<summary class="title">Details</summary>`
		if node.hasTitle {
			summary = `<summary class="title">` + node.Title() + "</summary>"
		}
		open := ""
		if node.hasOption("open") {
			open = " open"
		}
		return "<details" + idAttr(node.ID) + classAttr + open + ">\n" + summary + "\n<div class=\"content\">\n" + node.content() + "\n</div>\n</details>"
	}
	title := ""
	if node.hasTitle {
		title = `<div class="title">` + node.captionedTitle() + "</div>\n"
	}
	return "<div" + idAttr(node.ID) + ` class="exampleblock` + roleSuffix(node) + "\">\n" + title + "<div class=\"content\">\n" + node.content() + "\n</div>\n</div>"
}

func (c *html5Converter) floatingTitle(node *Block) string {
	tag := "h" + strconv.Itoa(node.Level+1)
	var base []string
	if node.Style != "" {
		base = append(base, node.Style)
	}
	ida := idAttr(node.ID)
	if node.emptyID {
		ida = ` id=""`
	}
	return "<" + tag + ida + ` class="` + joinClasses(node, base...) + `">` + node.Title() + "</" + tag + ">"
}

func linkConstraintAttrs(attrs map[string]string) string {
	rel := ""
	if _, ok := attrs["nofollow-option"]; ok {
		rel = "nofollow"
	}
	out := ""
	if window, ok := attrs["window"]; ok {
		out += ` target="` + window + `"`
		if _, noopener := attrs["noopener-option"]; window == "_blank" || noopener {
			if rel != "" {
				out += ` rel="` + rel + ` noopener"`
			} else {
				out += ` rel="noopener"`
			}
		}
	} else if rel != "" {
		out += ` rel="` + rel + `"`
	}
	return out
}

func (b *Block) readSVG(target string, attrs map[string]string) (string, bool) {
	doc := b.Document
	if isUriish(target) {
		return "", false
	}
	data, err := os.ReadFile(doc.normalizeSystemPath(target, doc.Attributes["imagesdir"]))
	if err != nil {
		return "", false
	}
	svg := strings.Join(prepareSourceString(string(data), true), "\n")
	if svg == "" {
		return "", false
	}
	if !strings.HasPrefix(svg, "<svg") {
		svg = svgPreambleRx.ReplaceFirstFunc(svg, func(*rx.Match) string { return "" })
	}
	oldStart, newStart := "", ""
	for _, dim := range []string{"width", "height"} {
		v, ok := attrs[dim]
		if !ok {
			continue
		}
		if newStart == "" {
			m := svgStartTagRx.Find(svg)
			if m == nil {
				continue
			}
			oldStart = m.Group(0)
			newStart = dimensionAttributeRx.ReplaceAll(oldStart, "")
		}
		newStart = newStart[:len(newStart)-1] + " " + dim + `="` + v + `">`
	}
	if newStart != "" {
		svg = newStart + svg[len(oldStart):]
	}
	return svg, true
}

func (c *html5Converter) image(node *Block) string {
	target := node.Attributes["target"]
	dims := ""
	if w, ok := node.Attributes["width"]; ok {
		dims += ` width="` + w + `"`
	}
	if h, ok := node.Attributes["height"]; ok {
		dims += ` height="` + h + `"`
	}
	var img string
	plain := func() string {
		return `<img src="` + node.imageURI(target, "imagesdir") + `" alt="` + encodeAttributeValue(node.alt()) + `"` + dims + ">"
	}
	if (node.Attributes["format"] == "svg" || strings.Contains(target, ".svg")) && node.Document.Safe < SafeSecure {
		if node.hasOption("inline") {
			if svg, ok := node.readSVG(target, node.Attributes); ok {
				img = svg
			} else {
				img = `<span class="alt">` + node.alt() + "</span>"
			}
		} else if node.hasOption("interactive") {
			fallback := `<span class="alt">` + node.alt() + "</span>"
			if fb, ok := node.Attributes["fallback"]; ok {
				fallback = `<img src="` + node.imageURI(fb, "imagesdir") + `" alt="` + encodeAttributeValue(node.alt()) + `"` + dims + ">"
			}
			img = `<object type="image/svg+xml" data="` + node.imageURI(target, "imagesdir") + `"` + dims + ">" + fallback + "</object>"
		} else {
			img = plain()
		}
	} else {
		img = plain()
	}
	if link, ok := node.Attributes["link"]; ok {
		img = `<a class="image" href="` + link + `"` + linkConstraintAttrs(node.Attributes) + ">" + img + "</a>"
	}
	classes := []string{"imageblock"}
	if f, ok := node.Attributes["float"]; ok {
		classes = append(classes, f)
	}
	if a, ok := node.Attributes["align"]; ok {
		classes = append(classes, "text-"+a)
	}
	if r, ok := node.Attributes["role"]; ok {
		classes = append(classes, r)
	}
	titleEl := ""
	if node.hasTitle {
		titleEl = "\n<div class=\"title\">" + node.captionedTitle() + "</div>"
	}
	return "<div" + idAttr(node.ID) + ` class="` + strings.Join(classes, " ") + "\">\n<div class=\"content\">\n" + img + "\n</div>" + titleEl + "\n</div>"
}

func (c *html5Converter) listing(node *Block) string {
	nowrap := node.hasOption("nowrap") || !node.docHasAttr("prewrap")
	var preOpen, preClose string
	hl := false
	lang, hasLang := node.Attributes["language"]
	if node.Style == "source" {
		if node.Document.highlight {
			hl = true
		} else {
			nw := ""
			if nowrap {
				nw = " nowrap"
			}
			code := ""
			if hasLang {
				code = ` class="language-` + lang + `" data-lang="` + lang + `"`
			}
			preOpen = `<pre class="highlight` + nw + `"><code` + code + ">"
			preClose = "</code></pre>"
		}
	} else {
		if nowrap {
			preOpen = `<pre class="nowrap">`
		} else {
			preOpen = "<pre>"
		}
		preClose = "</pre>"
	}
	title := ""
	if node.hasTitle {
		title = `<div class="title">` + node.captionedTitle() + "</div>\n"
	}
	var body string
	if fb := node.Document.fallbackHL; fb != "" && node.Style == "source" {
		cls := fb + " highlight"
		if nowrap {
			cls += " nowrap"
		}
		dl := ""
		if hasLang {
			if i := strings.Index(lang, "?"); i >= 0 && fb == "rouge" {
				lang = lang[:i]
			}
			dl = ` data-lang="` + lang + `"`
		}
		body = `<pre class="` + cls + `"><code` + dl + ">" + node.content() + "</code></pre>"
	} else if hl {
		cls := "highlightjs highlight"
		if nowrap {
			cls += " nowrap"
		}
		l := "none"
		dl := ""
		if hasLang {
			l = lang
			dl = ` data-lang="` + lang + `"`
		}
		body = `<pre class="` + cls + `"><code class="language-` + l + ` hljs"` + dl + ">" + node.content() + "</code></pre>"
	} else {
		body = preOpen + node.content() + preClose
	}
	return "<div" + idAttr(node.ID) + ` class="listingblock` + roleSuffix(node) + "\">\n" + title + "<div class=\"content\">\n" + body + "\n</div>\n</div>"
}

func (c *html5Converter) literal(node *Block) string {
	title := titleDiv(node)
	nowrap := !node.docHasAttr("prewrap") || node.hasOption("nowrap")
	pre := "<pre>"
	if nowrap {
		pre = `<pre class="nowrap">`
	}
	return "<div" + idAttr(node.ID) + ` class="literalblock` + roleSuffix(node) + "\">\n" + title + "<div class=\"content\">\n" + pre + node.content() + "</pre>\n</div>\n</div>"
}

func (c *html5Converter) stem(node *Block) string {
	title := titleDiv(node)
	open, close := `\$`, `\$`
	if node.Style == "latexmath" {
		open, close = `\[`, `\]`
	}
	equation := node.content()
	if node.Style == "asciimath" && strings.Contains(equation, "\n") {
		equation = stemBreakRx.ReplaceAllFunc(equation, func(m *rx.Match) string {
			return close + strings.Repeat("\n<br>", strings.Count(m.Group(0), "\n")-1) + "\n" + open
		})
	}
	if !(strings.HasPrefix(equation, open) && strings.HasSuffix(equation, close)) {
		equation = open + equation + close
	}
	return "<div" + idAttr(node.ID) + ` class="stemblock` + roleSuffix(node) + "\">\n" + title + "<div class=\"content\">\n" + equation + "\n</div>\n</div>"
}

var orderedListKeywords = map[string]string{"loweralpha": "a", "lowerroman": "i", "upperalpha": "A", "upperroman": "I"}

func liOpen(item *Block) string {
	r, hasRole := item.Attributes["role"]
	if item.ID != "" {
		if hasRole {
			return `<li id="` + item.ID + `" class="` + r + `">`
		}
		return `<li id="` + item.ID + `">`
	} else if hasRole {
		return `<li class="` + r + `">`
	}
	return "<li>"
}

func (c *html5Converter) olist(node *Block) string {
	var result []string
	base := []string{"olist"}
	if node.Style != "" {
		base = append(base, node.Style)
	}
	result = append(result, "<div"+idAttr(node.ID)+` class="`+joinClasses(node, base...)+`">`)
	if node.hasTitle {
		result = append(result, `<div class="title">`+node.Title()+"</div>")
	}
	typ := ""
	if k, ok := orderedListKeywords[node.Style]; ok && !node.implicitStyle {
		typ = ` type="` + k + `"`
	}
	start := ""
	if s, ok := node.Attributes["start"]; ok {
		start = ` start="` + s + `"`
	}
	rev := ""
	if node.hasOption("reversed") {
		rev = " reversed"
	}
	result = append(result, `<ol class="`+node.Style+`"`+typ+start+rev+">")
	for _, item := range node.Blocks {
		result = append(result, liOpen(item), "<p>"+item.itemText()+"</p>")
		if item.hasBlocks() {
			result = append(result, itemContent(item))
		}
		result = append(result, "</li>")
	}
	result = append(result, "</ol>", "</div>")
	return strings.Join(result, "\n")
}

func (c *html5Converter) open(node *Block) string {
	style := node.Style
	if style == "abstract" {
		if node.Parent == &node.Document.Block && node.Document.Doctype == "book" {
			return ""
		}
		title := titleDiv(node)
		return "<div" + idAttr(node.ID) + ` class="quoteblock abstract` + roleSuffix(node) + "\">\n" + title + "<blockquote>\n" + node.content() + "\n</blockquote>\n</div>"
	} else if style == "partintro" && (node.Level > 0 || node.Parent.Context != "section" || node.Document.Doctype != "book") {
		return ""
	}
	title := titleDiv(node)
	st := ""
	if style != "" && style != "open" {
		st = " " + style
	}
	return "<div" + idAttr(node.ID) + ` class="openblock` + st + roleSuffix(node) + "\">\n" + title + "<div class=\"content\">\n" + node.content() + "\n</div>\n</div>"
}

func (c *html5Converter) paragraph(node *Block) string {
	var attributes string
	if r, ok := node.Attributes["role"]; ok {
		attributes = idAttr(node.ID) + ` class="paragraph ` + r + `"`
	} else {
		attributes = idAttr(node.ID) + ` class="paragraph"`
	}
	if node.hasTitle {
		t := node.Title()
		return "<div" + attributes + ">\n<div class=\"title\">" + t + "</div>\n<p>" + node.content() + "</p>\n</div>"
	}
	return "<div" + attributes + ">\n<p>" + node.content() + "</p>\n</div>"
}

func (c *html5Converter) preamble(node *Block) string {
	doc := node.Document
	toc := ""
	if doc.Attributes["toc-placement"] == "preamble" && doc.hasSections() && doc.hasAttr("toc") {
		toc = "\n<div id=\"toc\" class=\"" + doc.attrOr("toc-class", "toc") + "\">\n<div id=\"toctitle\">" + doc.Attributes["toc-title"] + "</div>\n" + c.outline(&doc.Block, 0, 0) + "\n</div>"
	}
	return "<div id=\"preamble\">\n<div class=\"sectionbody\">\n" + node.content() + "\n</div>" + toc + "\n</div>"
}

func attributionElement(node *Block) string {
	attribution, hasAttr := node.Attributes["attribution"]
	citetitle, hasCite := node.Attributes["citetitle"]
	if !hasAttr && !hasCite {
		return ""
	}
	cite := ""
	if hasCite {
		cite = "<cite>" + citetitle + "</cite>"
	}
	text := ""
	if hasAttr {
		text = "&#8212; " + attribution
		if hasCite {
			text += "<br>\n"
		}
	}
	return "\n<div class=\"attribution\">\n" + text + cite + "\n</div>"
}

func (c *html5Converter) quote(node *Block) string {
	title := ""
	if node.hasTitle {
		title = "\n<div class=\"title\">" + node.Title() + "</div>"
	}
	attr := attributionElement(node)
	return "<div" + idAttr(node.ID) + ` class="` + joinClasses(node, "quoteblock") + `">` + title + "\n<blockquote>\n" + node.content() + "\n</blockquote>" + attr + "\n</div>"
}

func (c *html5Converter) verse(node *Block) string {
	title := ""
	if node.hasTitle {
		title = "\n<div class=\"title\">" + node.Title() + "</div>"
	}
	attr := attributionElement(node)
	return "<div" + idAttr(node.ID) + ` class="` + joinClasses(node, "verseblock") + `">` + title + "\n<pre class=\"content\">" + node.content() + "</pre>" + attr + "\n</div>"
}

func (c *html5Converter) sidebar(node *Block) string {
	title := titleDiv(node)
	return "<div" + idAttr(node.ID) + ` class="sidebarblock` + roleSuffix(node) + "\">\n<div class=\"content\">\n" + title + node.content() + "\n</div>\n</div>"
}

func (c *html5Converter) table(node *Block) string {
	var result []string
	t := node.Table
	doc := node.Document
	frame, ok := node.attrFallback2("frame", "table-frame")
	if !ok {
		frame = "all"
	}
	if frame == "topbot" {
		frame = "ends"
	}
	grid, ok := node.attrFallback2("grid", "table-grid")
	if !ok {
		grid = "all"
	}
	classes := []string{"tableblock", "frame-" + frame, "grid-" + grid}
	if stripes, ok := node.attrFallback2("stripes", "table-stripes"); ok {
		classes = append(classes, "stripes-"+stripes)
	}
	styleAttr := ""
	autowidth := node.hasOption("autowidth")
	if _, hasWidth := node.Attributes["width"]; autowidth && !hasWidth {
		classes = append(classes, "fit-content")
	} else if tw := node.Attributes["tablepcwidth"]; tw == "100" {
		classes = append(classes, "stretch")
	} else {
		styleAttr = ` style="width: ` + tw + `%;"`
	}
	if f, ok := node.Attributes["float"]; ok {
		classes = append(classes, f)
	}
	if r, ok := node.Attributes["role"]; ok {
		classes = append(classes, r)
	}
	result = append(result, "<table"+idAttr(node.ID)+` class="`+strings.Join(classes, " ")+`"`+styleAttr+">")
	if node.hasTitle {
		result = append(result, `<caption class="title">`+node.captionedTitle()+"</caption>")
	}
	if atoi(node.Attributes["rowcount"]) > 0 {
		result = append(result, "<colgroup>")
		for _, col := range t.Columns {
			if autowidth || col.Autowidth {
				result = append(result, "<col>")
			} else {
				result = append(result, `<col style="width: `+col.Pcwidth.String()+`%;">`)
			}
		}
		result = append(result, "</colgroup>")
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
				result = append(result, "<tr>")
				for _, cell := range row {
					var content string
					if sec.name == "head" {
						content = cell.text()
					} else {
						switch cell.Style {
						case "asciidoc":
							content = `<div class="content">` + cell.InnerDoc.convertDocument() + "</div>"
						case "literal":
							content = `<div class="literal"><pre>` + cell.text() + "</pre></div>"
						default:
							if paras := cell.content(); len(paras) > 0 {
								content = `<p class="tableblock">` + strings.Join(paras, "</p>\n<p class=\"tableblock\">") + "</p>"
							}
						}
					}
					tag := "td"
					if sec.name == "head" || cell.Style == "header" {
						tag = "th"
					}
					attrs := ` class="tableblock halign-` + cell.Halign + " valign-" + cell.Valign + `"`
					if cell.Colspan != 0 {
						attrs += ` colspan="` + strconv.Itoa(cell.Colspan) + `"`
					}
					if cell.Rowspan != 0 {
						attrs += ` rowspan="` + strconv.Itoa(cell.Rowspan) + `"`
					}
					if bg, ok := doc.Attributes["cellbgcolor"]; ok {
						attrs += ` style="background-color: ` + bg + `;"`
					}
					result = append(result, "<"+tag+attrs+">"+content+"</"+tag+">")
				}
				result = append(result, "</tr>")
			}
			result = append(result, "</t"+sec.name+">")
		}
	}
	result = append(result, "</table>")
	return strings.Join(result, "\n")
}

// attrFallback2 returns the block attribute name or the document attribute
// fallback.
func (b *Block) attrFallback2(name, fallback string) (string, bool) {
	if v, ok := b.Attributes[name]; ok {
		return v, true
	}
	v, ok := b.Document.Attributes[fallback]
	return v, ok
}

func (c *html5Converter) toc(node *Block) string {
	doc := node.Document
	if !(doc.Attributes["toc-placement"] == "macro" && doc.hasSections() && doc.hasAttr("toc")) {
		return "<!-- toc disabled -->"
	}
	ida, titleID := ` id="toc"`, ` id="toctitle"`
	if node.ID != "" {
		ida, titleID = ` id="`+node.ID+`"`, ` id="`+node.ID+`title"`
	}
	title := doc.Attributes["toc-title"]
	if node.hasTitle {
		title = node.Title()
	}
	levels := 0
	if l, ok := node.Attributes["levels"]; ok {
		levels = atoi(l)
	}
	role, ok := node.Attributes["role"]
	if !ok {
		role = doc.attrOr("toc-class", "toc")
	}
	return "<div" + ida + ` class="` + role + "\">\n<div" + titleID + ` class="title">` + title + "</div>\n" + c.outline(&doc.Block, levels, 0) + "\n</div>"
}

func (c *html5Converter) ulist(node *Block) string {
	var result []string
	divClasses := []string{"ulist"}
	checklist := node.hasOption("checklist")
	markerChecked, markerUnchecked := "", ""
	ulClass := ""
	if checklist {
		divClasses = append(divClasses, "checklist")
		ulClass = ` class="checklist"`
		if node.hasOption("interactive") {
			markerChecked = `<input type="checkbox" data-item-complete="1" checked> `
			markerUnchecked = `<input type="checkbox" data-item-complete="0"> `
		} else if node.Document.Attributes["icons"] == "font" {
			markerChecked = `<i class="fa fa-check-square-o"></i> `
			markerUnchecked = `<i class="fa fa-square-o"></i> `
		} else {
			markerChecked = "&#10003; "
			markerUnchecked = "&#10063; "
		}
	} else if node.Style != "" {
		ulClass = ` class="` + node.Style + `"`
	}
	if node.Style != "" {
		divClasses = append(divClasses, node.Style)
	}
	result = append(result, "<div"+idAttr(node.ID)+` class="`+joinClasses(node, divClasses...)+`">`)
	if node.hasTitle {
		result = append(result, `<div class="title">`+node.Title()+"</div>")
	}
	result = append(result, "<ul"+ulClass+">")
	for _, item := range node.Blocks {
		result = append(result, liOpen(item))
		if _, ok := item.Attributes["checkbox"]; checklist && ok {
			marker := markerUnchecked
			if _, ok := item.Attributes["checked"]; ok {
				marker = markerChecked
			}
			result = append(result, "<p>"+marker+item.itemText()+"</p>")
		} else {
			result = append(result, "<p>"+item.itemText()+"</p>")
		}
		if item.hasBlocks() {
			result = append(result, itemContent(item))
		}
		result = append(result, "</li>")
	}
	result = append(result, "</ul>", "</div>")
	return strings.Join(result, "\n")
}

func (c *html5Converter) video(node *Block) string {
	classes := []string{"videoblock"}
	if f, ok := node.Attributes["float"]; ok {
		classes = append(classes, f)
	}
	if a, ok := node.Attributes["align"]; ok {
		classes = append(classes, "text-"+a)
	}
	if r, ok := node.Attributes["role"]; ok {
		classes = append(classes, r)
	}
	head := "<div" + idAttr(node.ID) + ` class="` + strings.Join(classes, " ") + `">`
	if node.hasTitle {
		head += "\n<div class=\"title\">" + node.Title() + "</div>"
	}
	dims := ""
	if w, ok := node.Attributes["width"]; ok {
		dims += ` width="` + w + `"`
	}
	if h, ok := node.Attributes["height"]; ok {
		dims += ` height="` + h + `"`
	}
	scheme := node.Document.attrOr("asset-uri-scheme", "https")
	if scheme != "" {
		scheme += ":"
	}
	opt := node.hasOption
	target := node.Attributes["target"]
	switch node.Attributes["poster"] {
	case "vimeo":
		startAnchor := ""
		if s, ok := node.Attributes["start"]; ok {
			startAnchor = "#at=" + s
		}
		delim := "?"
		next := func() string {
			d := delim
			delim = "&amp;"
			return d
		}
		hash, hasHash := "", false
		if i := strings.Index(target, "/"); i >= 0 {
			target, hash, hasHash = target[:i], target[i+1:], true
		} else {
			hash, hasHash = node.Attributes["hash"]
		}
		params := ""
		if hasHash {
			params += next() + "h=" + hash
		}
		if opt("autoplay") {
			params += next() + "autoplay=1"
		}
		if opt("loop") {
			params += next() + "loop=1"
		}
		if opt("muted") {
			params += next() + "muted=1"
		}
		fs := " allowfullscreen"
		if opt("nofullscreen") {
			fs = ""
		}
		return head + "\n<div class=\"content\">\n<iframe" + dims + ` src="` + scheme + "//player.vimeo.com/video/" + target + params + startAnchor + `" frameborder="0"` + fs + "></iframe>\n</div>\n</div>"
	case "youtube":
		rel := "0"
		if opt("related") {
			rel = "1"
		}
		params := ""
		if s, ok := node.Attributes["start"]; ok {
			params += "&amp;start=" + s
		}
		if e, ok := node.Attributes["end"]; ok {
			params += "&amp;end=" + e
		}
		if opt("autoplay") {
			params += "&amp;autoplay=1"
		}
		hasLoop := opt("loop")
		if hasLoop {
			params += "&amp;loop=1"
		}
		if opt("muted") {
			params += "&amp;mute=1"
		}
		if opt("nocontrols") {
			params += "&amp;controls=0"
		}
		fsParam, fsAttr := "", " allowfullscreen"
		if opt("nofullscreen") {
			fsParam, fsAttr = "&amp;fs=0", ""
		}
		tail := fsParam
		if opt("modest") {
			tail += "&amp;modestbranding=1"
		}
		if t, ok := node.Attributes["theme"]; ok {
			tail += "&amp;theme=" + t
		}
		if l, ok := node.Attributes["lang"]; ok {
			tail += "&amp;hl=" + l
		}
		list, hasList := "", false
		if i := strings.Index(target, "/"); i >= 0 {
			target, list, hasList = target[:i], target[i+1:], true
		} else {
			list, hasList = node.Attributes["list"]
		}
		listParam := ""
		if hasList {
			listParam = "&amp;list=" + list
		} else {
			playlist, hasPl := "", false
			if i := strings.Index(target, ","); i >= 0 {
				target, playlist, hasPl = target[:i], target[i+1:], true
			} else {
				playlist, hasPl = node.Attributes["playlist"]
			}
			if hasPl {
				listParam = "&amp;playlist=" + target + "," + playlist
			} else if hasLoop {
				listParam = "&amp;playlist=" + target
			}
		}
		return head + "\n<div class=\"content\">\n<iframe" + dims + ` src="` + scheme + "//www.youtube.com/embed/" + target + "?rel=" + rel + params + listParam + tail + `" frameborder="0"` + fsAttr + "></iframe>\n</div>\n</div>"
	}
	poster := ""
	if v := node.Attributes["poster"]; v != "" {
		poster = ` poster="` + node.mediaURI(v) + `"`
	}
	preload := ""
	if v := node.Attributes["preload"]; v != "" {
		preload = ` preload="` + v + `"`
	}
	opts := ""
	if opt("autoplay") {
		opts += " autoplay"
	}
	if opt("muted") {
		opts += " muted"
	}
	if !opt("nocontrols") {
		opts += " controls"
	}
	if opt("loop") {
		opts += " loop"
	}
	return head + "\n<div class=\"content\">\n<video src=\"" + node.mediaURI(target) + timeAnchor(node) + `"` + dims + poster + opts + preload + ">\nYour browser does not support the video tag.\n</video>\n</div>\n</div>"
}

// ---------------------------------------------------------------------------
// Inline nodes

func (c *html5Converter) convertInline(n *Inline) string {
	switch n.Context {
	case "quoted":
		return c.inlineQuoted(n)
	case "anchor":
		return c.inlineAnchor(n)
	case "break":
		return n.Text + "<br>"
	case "button":
		return `<b class="button">` + n.Text + "</b>"
	case "callout":
		return c.inlineCallout(n)
	case "footnote":
		return c.inlineFootnote(n)
	case "image":
		return c.inlineImage(n)
	case "indexterm":
		if n.Type == "visible" {
			return n.Text
		}
		return ""
	case "kbd":
		if len(n.Keys) == 1 {
			return "<kbd>" + n.Keys[0] + "</kbd>"
		}
		return `<span class="keyseq"><kbd>` + strings.Join(n.Keys, "</kbd>+<kbd>") + "</kbd></span>"
	case "menu":
		return c.inlineMenu(n)
	}
	return n.Text
}

func (c *html5Converter) inlineAnchor(node *Inline) string {
	role, hasRole := node.Attrs["role"]
	switch node.Type {
	case "xref":
		var attrs, text string
		if path, ok := node.Attrs["path"]; ok {
			if hasRole {
				attrs = ` class="` + role + `"`
			}
			attrs += linkConstraintAttrs(node.Attrs)
			text = path
			if node.hasText {
				text = node.Text
			}
		} else {
			if hasRole {
				attrs = ` class="` + role + `"`
			}
			if node.hasText {
				text = node.Text
			} else {
				doc := node.Parent.Document
				refid, hasRefid := node.Attrs["refid"]
				ref, found := doc.refs[refid]
				top := false
				if !found && (!hasRefid || refid == "") {
					root := doc
					for root.parentDocument != nil {
						root = root.parentDocument
					}
					ref, found, top = refTarget{block: &root.Block}, true, true
				}
				fallback := "[" + refid + "]"
				if top {
					fallback = "[^top]"
				}
				if found {
					if !c.resolvingXref {
						c.resolvingXref = true
						xrefstyle, ok := node.Attrs["xrefstyle"]
						if !ok {
							xrefstyle = doc.Attributes["xrefstyle"]
						}
						if t, ok := doc.refXreftext(ref, xrefstyle); ok {
							text = t
							if strings.Contains(text, "<a") {
								text = dropAnchorRx.ReplaceAll(text, "")
							}
						} else {
							text = fallback
						}
						c.resolvingXref = false
					} else {
						text = fallback
					}
				} else {
					text = "[" + refid + "]"
				}
			}
		}
		return `<a href="` + node.Target + `"` + attrs + ">" + text + "</a>"
	case "ref":
		return `<a id="` + node.ID + `"></a>`
	case "link":
		attrs := idAttr(node.ID)
		if hasRole {
			attrs += ` class="` + role + `"`
		}
		if t, ok := node.Attrs["title"]; ok {
			attrs += ` title="` + t + `"`
		}
		return `<a href="` + node.Target + `"` + attrs + linkConstraintAttrs(node.Attrs) + ">" + node.Text + "</a>"
	case "bibref":
		text := node.ID
		if node.hasText {
			text = node.Parent.applySubs(node.Text, reftextSubs)
		}
		return `<a id="` + node.ID + `"></a>[` + text + "]"
	}
	return ""
}

func (c *html5Converter) inlineCallout(node *Inline) string {
	doc := node.Parent.Document
	if icons, ok := doc.Attributes["icons"]; ok {
		if icons == "font" {
			return `<i class="conum" data-value="` + node.Text + `"></i><b>(` + node.Text + ")</b>"
		}
		return `<img src="` + node.Parent.iconURI("callouts/"+node.Text) + `" alt="` + node.Text + `">`
	}
	if len(node.Guard) == 2 {
		return `&lt;!--<b class="conum">(` + node.Text + ")</b>--&gt;"
	}
	guard := ""
	if len(node.Guard) == 1 {
		guard = node.Guard[0]
	}
	return guard + `<b class="conum">(` + node.Text + ")</b>"
}

func (c *html5Converter) inlineFootnote(node *Inline) string {
	if index := node.Attrs["index"]; index != "" {
		if node.Type == "xref" {
			return `<sup class="footnoteref">[<a class="footnote" href="#_footnotedef_` + index + `" title="View footnote.">` + index + "</a>]</sup>"
		}
		ida := ""
		if node.ID != "" {
			ida = ` id="_footnote_` + node.ID + `"`
		}
		return `<sup class="footnote"` + ida + `>[<a id="_footnoteref_` + index + `" class="footnote" href="#_footnotedef_` + index + `" title="View footnote.">` + index + "</a>]</sup>"
	} else if node.Type == "xref" {
		return `<sup class="footnoteref red" title="Unresolved footnote reference.">[` + node.Text + "]</sup>"
	}
	return ""
}

func (c *html5Converter) inlineImage(node *Inline) string {
	target := node.Target
	p := node.Parent
	doc := p.Document
	typ := node.Type
	if typ == "" {
		typ = "image"
	}
	alt := node.Attrs["alt"]
	var img string
	stdAttrs := func() string {
		a := ""
		if w, ok := node.Attrs["width"]; ok {
			a += ` width="` + w + `"`
		}
		if h, ok := node.Attrs["height"]; ok {
			a += ` height="` + h + `"`
		}
		if t, ok := node.Attrs["title"]; ok {
			a += ` title="` + t + `"`
		}
		return a
	}
	if typ == "icon" {
		icons, hasIcons := doc.Attributes["icons"]
		if hasIcons && icons == "font" {
			cls := "fa fa-" + target
			if s, ok := node.Attrs["size"]; ok {
				cls += " fa-" + s
			}
			if f, ok := node.Attrs["flip"]; ok {
				cls += " fa-flip-" + f
			} else if r, ok := node.Attrs["rotate"]; ok {
				cls += " fa-rotate-" + r
			}
			attrs := ""
			if t, ok := node.Attrs["title"]; ok {
				attrs = ` title="` + t + `"`
			}
			img = `<i class="` + cls + `"` + attrs + "></i>"
		} else if hasIcons {
			tmp := &Block{Document: doc, Attributes: node.Attrs, Parent: p}
			img = `<img src="` + tmp.iconURI(target) + `" alt="` + encodeAttributeValue(alt) + `"` + stdAttrs() + ">"
		} else {
			img = "[" + alt + "&#93;"
		}
	} else {
		attrs := stdAttrs()
		plain := func() string {
			return `<img src="` + p.imageURI(target, "imagesdir") + `" alt="` + encodeAttributeValue(alt) + `"` + attrs + ">"
		}
		_, inlineOpt := node.Attrs["inline-option"]
		_, interactiveOpt := node.Attrs["interactive-option"]
		if (node.Attrs["format"] == "svg" || strings.Contains(target, ".svg")) && doc.Safe < SafeSecure {
			if inlineOpt {
				if svg, ok := p.readSVG(target, node.Attrs); ok {
					img = svg
				} else {
					img = `<span class="alt">` + alt + "</span>"
				}
			} else if interactiveOpt {
				fallback := `<span class="alt">` + alt + "</span>"
				if fb, ok := node.Attrs["fallback"]; ok {
					fallback = `<img src="` + p.imageURI(fb, "imagesdir") + `" alt="` + encodeAttributeValue(alt) + `"` + attrs + ">"
				}
				img = `<object type="image/svg+xml" data="` + p.imageURI(target, "imagesdir") + `"` + attrs + ">" + fallback + "</object>"
			} else {
				img = plain()
			}
		} else {
			img = plain()
		}
	}
	if link, ok := node.Attrs["link"]; ok {
		img = `<a class="image" href="` + link + `"` + linkConstraintAttrs(node.Attrs) + ">" + img + "</a>"
	}
	cls := typ
	if f, ok := node.Attrs["float"]; ok {
		cls += " " + f
	}
	if r, ok := node.Attrs["role"]; ok {
		cls += " " + r
	}
	return `<span class="` + cls + `">` + img + "</span>"
}

func (c *html5Converter) inlineMenu(node *Inline) string {
	caret := `&#160;<b class="caret">&#8250;</b> `
	if node.Parent.Document.Attributes["icons"] == "font" {
		caret = `&#160;<i class="fa fa-angle-right caret"></i> `
	}
	menu := node.Attrs["menu"]
	menuitem, hasItem := node.Attrs["menuitem"]
	if len(node.Submenus) == 0 {
		if hasItem {
			return `<span class="menuseq"><b class="menu">` + menu + "</b>" + caret + `<b class="menuitem">` + menuitem + "</b></span>"
		}
		return `<b class="menuref">` + menu + "</b>"
	}
	return `<span class="menuseq"><b class="menu">` + menu + "</b>" + caret + `<b class="submenu">` + strings.Join(node.Submenus, "</b>"+caret+`<b class="submenu">`) + "</b>" + caret + `<b class="menuitem">` + menuitem + "</b></span>"
}

func (c *html5Converter) inlineQuoted(node *Inline) string {
	qt := quoteTags[node.Type]
	role, hasRole := node.Attrs["role"]
	if node.ID != "" {
		classAttr := ""
		if hasRole {
			classAttr = ` class="` + role + `"`
		}
		if qt.tag {
			return qt.open[:len(qt.open)-1] + ` id="` + node.ID + `"` + classAttr + ">" + node.Text + qt.close
		}
		return `<span id="` + node.ID + `"` + classAttr + ">" + qt.open + node.Text + qt.close + "</span>"
	} else if hasRole {
		if qt.tag {
			return qt.open[:len(qt.open)-1] + ` class="` + role + `">` + node.Text + qt.close
		}
		return `<span class="` + role + `">` + qt.open + node.Text + qt.close + "</span>"
	}
	return qt.open + node.Text + qt.close
}
