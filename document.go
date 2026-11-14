package adocgo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SafeMode controls which potentially unsafe features are enabled.
type SafeMode int

// Safe mode levels, matching Asciidoctor's SafeMode constants.
const (
	SafeUnsafe SafeMode = 0
	SafeSafe   SafeMode = 1
	SafeServer SafeMode = 10
	SafeSecure SafeMode = 20
)

func (s SafeMode) name() string {
	switch s {
	case SafeUnsafe:
		return "unsafe"
	case SafeSafe:
		return "safe"
	case SafeServer:
		return "server"
	}
	return "secure"
}

// ParseSafeMode converts a safe mode name (unsafe, safe, server, secure)
// into a SafeMode value.
func ParseSafeMode(name string) (SafeMode, bool) {
	switch strings.ToLower(name) {
	case "unsafe":
		return SafeUnsafe, true
	case "safe":
		return SafeSafe, true
	case "server":
		return SafeServer, true
	case "secure":
		return SafeSecure, true
	}
	return SafeSecure, false
}

type override struct {
	val   string
	state byte // 's' set, 'n' hard unset (nil), 'f' soft unset (false)
}

type footnote struct {
	index string
	id    string
	text  string
}

type refTarget struct {
	block  *Block
	inline *Inline
}

// Document is the root of a parsed AsciiDoc document.
type Document struct {
	Block

	// Safe is the safe mode the document was loaded with.
	Safe SafeMode
	// Doctype is the document type (article, book, manpage, inline).
	Doctype string
	// Backend is the output backend (always html5).
	Backend string

	header             *Block
	parentDocument     *Document
	baseDir            string
	standalone         bool
	compatMode         bool
	outfilesuffix      string
	attributeOverrides map[string]override
	attributesModified map[string]bool
	headerAttributes   map[string]string
	counters           map[string]string
	maxAttrValueSize   int
	parsed             bool

	refs       map[string]refTarget
	refOrder   []string
	reftexts   map[string]string
	footnotes  *[]footnote
	callouts   *callouts
	includes   map[string]bool
	mannames   []string
	highlight  bool // highlight.js enabled
	fallbackHL string
	xmlMode    bool
	outFile    string
	nesting    *int
	reader     *Reader
	conv       *html5Converter
}

// Options configures a conversion.
type Options struct {
	// Attributes are document attributes, as passed with -a on the
	// asciidoctor command line. A key ending in "@" (or a value ending in
	// "@") makes the assignment a soft default that the document may
	// override; a key starting or ending with "!" unsets the attribute.
	Attributes map[string]string
	// Standalone produces a full HTML document (with <html>, <head> and
	// <body>) rather than embeddable output.
	Standalone bool
	// Safe is the safe mode. The zero value is SafeUnsafe; use SafeSecure
	// when converting untrusted input.
	Safe SafeMode
	// BaseDir is the base directory used to resolve includes and other
	// relative paths. It defaults to the directory of the input file, or
	// the working directory.
	BaseDir string
	// Doctype overrides the document type (article, book, manpage, inline).
	Doctype string
	// InputFile is the path of the source document, if any. It is used to
	// populate the docfile, docdir and docname attributes.
	InputFile string
	// OutFile is the path of the output file, if any (sets outfile/outdir).
	OutFile string
	// ModTime is the modification time of the input, used for the docdate
	// and doctime attributes. If zero, the current time is used. The
	// SOURCE_DATE_EPOCH environment variable takes precedence.
	ModTime time.Time
}

// Version is the version of Asciidoctor whose behaviour this package
// reproduces; it is exposed through the asciidoctor-version attribute.
const Version = "2.0.26"

func newDocument(opts Options) *Document {
	doc := &Document{}
	doc.Context = "document"
	doc.Document = doc
	doc.Attributes = map[string]string{}
	doc.ContentModel = "compound"
	doc.nextSectionOrdinal = 1
	doc.nesting = new(int)
	doc.refs = map[string]refTarget{}
	doc.footnotes = &[]footnote{}
	doc.callouts = newCallouts()
	doc.includes = map[string]bool{}
	doc.counters = map[string]string{}
	doc.attributesModified = map[string]bool{}
	doc.standalone = opts.Standalone
	doc.Safe = opts.Safe
	ov := map[string]override{}
	doc.attributeOverrides = ov
	userAttrs := map[string]string{}
	for k, v := range opts.Attributes {
		userAttrs[k] = v
	}
	absInput := ""
	if opts.InputFile != "" {
		if p, err := filepath.Abs(opts.InputFile); err == nil {
			absInput = filepath.ToSlash(p)
		} else {
			absInput = opts.InputFile
		}
		userAttrs["docfile"] = absInput
		userAttrs["docdir"] = fileDirname(absInput)
		ext := extname(absInput)
		userAttrs["docfilesuffix"] = ext
		userAttrs["docname"] = strings.TrimSuffix(fileBasename(absInput), ext)
	}
	// process keys in sorted order for determinism
	keys := make([]string, 0, len(userAttrs))
	for k := range userAttrs {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, key := range keys {
		val := userAttrs[key]
		o := override{val: val, state: 's'}
		if strings.HasSuffix(key, "@") {
			if strings.HasPrefix(key, "!") {
				key, o = key[1:len(key)-1], override{state: 'f'}
			} else if strings.HasSuffix(key, "!@") {
				key, o = key[:len(key)-2], override{state: 'f'}
			} else {
				key, o = key[:len(key)-1], override{val: val + "@", state: 's'}
			}
		} else if strings.HasPrefix(key, "!") {
			key = key[1:]
			if val == "@" {
				o = override{state: 'f'}
			} else {
				o = override{state: 'n'}
			}
		} else if strings.HasSuffix(key, "!") {
			key = key[:len(key)-1]
			if val == "@" {
				o = override{state: 'f'}
			} else {
				o = override{state: 'n'}
			}
		}
		ov[strings.ToLower(key)] = o
	}
	if opts.OutFile != "" {
		ov["outfilesuffix"] = override{val: extname(opts.OutFile), state: 's'}
	}
	_, doc.compatMode = ov["compat-mode"]
	attrs := doc.Attributes
	attrs["attribute-undefined"] = "drop-line"
	attrs["attribute-missing"] = "skip"
	for k, v := range defaultAttributes {
		attrs[k] = v
	}
	setOv := func(k, v string) { ov[k] = override{val: v, state: 's'} }
	orNil := func(k string) {
		if o, ok := ov[k]; !ok || o.state != 's' {
			ov[k] = override{state: 'n'}
		}
	}
	if opts.Standalone {
		ov["embedded"] = override{state: 'n'}
		attrs["copycss"] = ""
		attrs["iconfont-remote"] = ""
		attrs["stylesheet"] = ""
		attrs["webfonts"] = ""
	} else {
		setOv("embedded", "")
		flip := func(o override) override {
			switch {
			case o.state == 'n':
				return override{val: "", state: 's'}
			case o.state == 'f':
				return override{val: "@", state: 's'}
			case o.val == "@":
				return override{state: 'f'}
			}
			return override{state: 'n'}
		}
		st, hasST := ov["showtitle"]
		nt, hasNT := ov["notitle"]
		// Ruby checks which of the two keys was specified last; map order is
		// not preserved here so showtitle wins when both are present.
		if hasST {
			ov["notitle"] = flip(st)
		} else if hasNT {
			ov["showtitle"] = flip(nt)
		} else {
			attrs["notitle"] = ""
		}
	}
	setOv("asciidoctor", "")
	setOv("asciidoctor-version", Version)
	smName := doc.Safe.name()
	setOv("safe-mode-name", smName)
	setOv("safe-mode-"+smName, "")
	setOv("safe-mode-level", strconv.Itoa(int(doc.Safe)))
	if o, ok := ov["max-include-depth"]; !ok || o.state != 's' {
		setOv("max-include-depth", "64")
	}
	orNil("allow-uri-read")
	if o, ok := ov["numbered"]; ok {
		ov["sectnums"] = o
		delete(ov, "numbered")
	}
	if o, ok := ov["hardbreaks"]; ok {
		ov["hardbreaks-option"] = o
		delete(ov, "hardbreaks")
	}
	if opts.BaseDir != "" {
		p, err := filepath.Abs(opts.BaseDir)
		if err != nil {
			p = opts.BaseDir
		}
		doc.baseDir = filepath.ToSlash(p)
		setOv("docdir", doc.baseDir)
	} else if o, ok := ov["docdir"]; ok && o.state == 's' && o.val != "" {
		doc.baseDir = o.val
	} else {
		wd, _ := os.Getwd()
		doc.baseDir = filepath.ToSlash(wd)
		setOv("docdir", doc.baseDir)
	}
	if opts.Doctype != "" {
		setOv("doctype", opts.Doctype)
	}
	if doc.Safe >= SafeServer {
		orNil("copycss")
		orNil("source-highlighter")
		if o, ok := ov["backend"]; !ok || o.state != 's' {
			setOv("backend", "html5")
		}
		if o, ok := ov["docfile"]; ok && o.state == 's' {
			dd := ov["docdir"].val
			if len(o.val) > len(dd)+1 {
				setOv("docfile", o.val[len(dd)+1:])
			} else {
				setOv("docfile", "")
			}
		}
		setOv("docdir", "")
		if o, ok := ov["user-home"]; !ok || o.state != 's' {
			setOv("user-home", ".")
		}
		if doc.Safe >= SafeSecure {
			if _, ok := ov["max-attribute-value-size"]; !ok {
				setOv("max-attribute-value-size", "4096")
			}
			if _, ok := ov["linkcss"]; !ok {
				setOv("linkcss", "")
			}
			orNil("icons")
		}
	} else if o, ok := ov["user-home"]; !ok || o.state != 's' {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		setOv("user-home", filepath.ToSlash(home))
	}
	if o, ok := ov["max-attribute-value-size"]; ok && o.state == 's' {
		doc.maxAttrValueSize = abs(atoi(o.val))
	} else {
		ov["max-attribute-value-size"] = override{state: 'n'}
	}
	for key, o := range ov {
		if o.state == 's' {
			val := o.val
			if strings.HasSuffix(val, "@") {
				val = val[:len(val)-1]
				delete(ov, key)
			}
			attrs[key] = val
		} else {
			delete(attrs, key)
			if o.state == 'f' {
				delete(ov, key)
			}
		}
	}
	initialBackend, ok := attrs["backend"]
	if !ok {
		initialBackend = "html5"
	}
	if _, ok := attrs["doctype"]; !ok {
		attrs["doctype"] = "article"
	}
	doc.Doctype = attrs["doctype"]
	doc.updateBackendAttributes(initialBackend, true)
	if _, ok := attrs["stylesdir"]; !ok {
		attrs["stylesdir"] = "."
	}
	if _, ok := attrs["iconsdir"]; !ok {
		imagesdir, ok := attrs["imagesdir"]
		if !ok {
			imagesdir = "./images"
		}
		attrs["iconsdir"] = imagesdir + "/icons"
	}
	doc.fillDatetimeAttributes(opts.ModTime)
	doc.conv = &html5Converter{doc: doc}
	return doc
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func newNestedDocument(parent *Document, lines []string) *Document {
	doc := &Document{}
	doc.Context = "document"
	doc.Document = doc
	doc.Attributes = map[string]string{}
	doc.ContentModel = "compound"
	doc.nextSectionOrdinal = 1
	doc.parentDocument = parent
	doc.nesting = parent.nesting
	doc.baseDir = parent.baseDir
	doc.refs = parent.refs
	doc.footnotes = &[]footnote{}
	doc.callouts = parent.callouts
	doc.includes = parent.includes
	doc.counters = map[string]string{}
	doc.attributesModified = map[string]bool{}
	doc.highlight = parent.highlight
	doc.fallbackHL = parent.fallbackHL
	doc.xmlMode = parent.xmlMode
	ov := map[string]override{}
	for k, v := range parent.attributeOverrides {
		ov[k] = v
	}
	for k, v := range parent.Attributes {
		ov[k] = override{val: v, state: 's'}
	}
	doc.attributeOverrides = ov
	delete(ov, "compat-mode")
	parentDoctype := ov["doctype"].val
	delete(ov, "doctype")
	delete(ov, "notitle")
	delete(ov, "showtitle")
	delete(ov, "toc")
	if o, ok := ov["toc-placement"]; ok && o.state == 's' {
		doc.Attributes["toc-placement"] = o.val
	} else {
		doc.Attributes["toc-placement"] = "auto"
	}
	delete(ov, "toc-placement")
	delete(ov, "toc-position")
	doc.Safe = parent.Safe
	doc.compatMode = parent.compatMode
	if doc.compatMode {
		doc.Attributes["compat-mode"] = ""
	}
	doc.outfilesuffix = parent.outfilesuffix
	doc.conv = &html5Converter{doc: doc}
	attrs := doc.Attributes
	ov["embedded"] = override{val: "", state: 's'}
	attrs["notitle"] = ""
	// the remaining standard overrides are inherited from the parent attributes
	if o, ok := ov["max-attribute-value-size"]; ok && o.state == 's' {
		doc.maxAttrValueSize = abs(atoi(o.val))
	}
	for key, o := range ov {
		if o.state == 's' {
			attrs[key] = o.val
		} else {
			delete(attrs, key)
			if o.state == 'f' {
				delete(ov, key)
			}
		}
	}
	doc.Backend = attrs["backend"]
	doc.Doctype = parentDoctype
	attrs["doctype"] = parentDoctype
	if parentDoctype != "article" {
		doc.Doctype = parentDoctype
		// mirror update_doctype_attributes DEFAULT_DOCTYPE
		doc.updateDoctypeAttributes("article")
	}
	doc.reader = newReader(lines)
	parseDocument(doc.reader, doc, false)
	doc.restoreAttributes()
	doc.parsed = true
	return doc
}

func (d *Document) fillDatetimeAttributes(mtime time.Time) {
	attrs := d.Attributes
	now := time.Now()
	epochSet := false
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			now = time.Unix(n, 0).UTC()
			epochSet = true
		}
	}
	fmtTime := func(t time.Time) string {
		if _, off := t.Zone(); off == 0 {
			return t.Format("15:04:05") + " UTC"
		}
		return t.Format("15:04:05 -0700")
	}
	localdate, ok := attrs["localdate"]
	if ok {
		if _, ok := attrs["localyear"]; !ok && strings.Index(localdate, "-") == 4 {
			attrs["localyear"] = localdate[:4]
		}
	} else {
		localdate = now.Format("2006-01-02")
		attrs["localdate"] = localdate
		if _, ok := attrs["localyear"]; !ok {
			attrs["localyear"] = strconv.Itoa(now.Year())
		}
	}
	if _, ok := attrs["localtime"]; !ok {
		attrs["localtime"] = fmtTime(now)
	}
	if _, ok := attrs["localdatetime"]; !ok {
		attrs["localdatetime"] = localdate + " " + attrs["localtime"]
	}
	input := now
	if !epochSet && !mtime.IsZero() {
		input = mtime
	}
	docdate, ok := attrs["docdate"]
	if ok {
		if _, ok := attrs["docyear"]; !ok && strings.Index(docdate, "-") == 4 {
			attrs["docyear"] = docdate[:4]
		}
	} else {
		docdate = input.Format("2006-01-02")
		attrs["docdate"] = docdate
		if _, ok := attrs["docyear"]; !ok {
			attrs["docyear"] = strconv.Itoa(input.Year())
		}
	}
	if _, ok := attrs["doctime"]; !ok {
		attrs["doctime"] = fmtTime(input)
	}
	if _, ok := attrs["docdatetime"]; !ok {
		attrs["docdatetime"] = docdate + " " + attrs["doctime"]
	}
}

func (d *Document) updateBackendAttributes(newBackend string, init bool) {
	if !init && newBackend == d.Backend {
		return
	}
	attrs := d.Attributes
	currentBackend := d.Backend
	currentBase, hasBase := attrs["basebackend"]
	currentDoctype := d.Doctype
	if i := strings.Index(newBackend, ":"); i >= 0 {
		newBackend = newBackend[:i]
	}
	if strings.HasPrefix(newBackend, "xhtml") {
		attrs["htmlsyntax"] = "xml"
		newBackend = newBackend[1:]
	} else if strings.HasPrefix(newBackend, "html") {
		if _, ok := attrs["htmlsyntax"]; !ok {
			attrs["htmlsyntax"] = "html"
		}
	}
	d.xmlMode = attrs["htmlsyntax"] == "xml"
	switch newBackend {
	case "html":
		newBackend = "html5"
	case "docbook":
		newBackend = "docbook5"
	}
	if currentDoctype != "" {
		if currentBackend != "" {
			delete(attrs, "backend-"+currentBackend)
			delete(attrs, "backend-"+currentBackend+"-doctype-"+currentDoctype)
		}
		attrs["backend-"+newBackend+"-doctype-"+currentDoctype] = ""
		attrs["doctype-"+currentDoctype] = ""
	} else if currentBackend != "" {
		delete(attrs, "backend-"+currentBackend)
	}
	attrs["backend-"+newBackend] = ""
	d.Backend = newBackend
	attrs["backend"] = newBackend
	// derive backend traits
	base := strings.TrimRight(newBackend, "0123456789")
	filetype := base
	suffix := "." + base
	switch base {
	case "html":
		suffix = ".html"
	case "docbook":
		filetype, suffix = "xml", ".xml"
	case "manpage":
		filetype, suffix = "man", ".man"
	case "pdf":
		suffix = ".pdf"
	case "epub":
		suffix = ".epub"
	}
	if init {
		if _, ok := attrs["outfilesuffix"]; !ok {
			attrs["outfilesuffix"] = suffix
		}
	} else if !d.attributeLocked("outfilesuffix") {
		attrs["outfilesuffix"] = suffix
	}
	if cur, ok := attrs["filetype"]; ok {
		delete(attrs, "filetype-"+cur)
	}
	attrs["filetype"] = filetype
	attrs["filetype-"+filetype] = ""
	if base == "docbook" {
		attrs["pagewidth"] = "425"
	} else {
		delete(attrs, "pagewidth")
	}
	if !hasBase || base != currentBase {
		if currentDoctype != "" {
			if hasBase {
				delete(attrs, "basebackend-"+currentBase)
				delete(attrs, "basebackend-"+currentBase+"-doctype-"+currentDoctype)
			}
			attrs["basebackend-"+base+"-doctype-"+currentDoctype] = ""
		} else if hasBase {
			delete(attrs, "basebackend-"+currentBase)
		}
		attrs["basebackend-"+base] = ""
		attrs["basebackend"] = base
	}
}

func (d *Document) updateDoctypeAttributes(newDoctype string) {
	if newDoctype == "" || newDoctype == d.Doctype {
		return
	}
	attrs := d.Attributes
	currentBackend, currentDoctype := d.Backend, d.Doctype
	currentBase, hasBase := attrs["basebackend"]
	if currentDoctype != "" {
		delete(attrs, "doctype-"+currentDoctype)
		if currentBackend != "" {
			delete(attrs, "backend-"+currentBackend+"-doctype-"+currentDoctype)
			attrs["backend-"+currentBackend+"-doctype-"+newDoctype] = ""
		}
		if hasBase {
			delete(attrs, "basebackend-"+currentBase+"-doctype-"+currentDoctype)
			attrs["basebackend-"+currentBase+"-doctype-"+newDoctype] = ""
		}
	} else {
		if currentBackend != "" {
			attrs["backend-"+currentBackend+"-doctype-"+newDoctype] = ""
		}
		if hasBase {
			attrs["basebackend-"+currentBase+"-doctype-"+newDoctype] = ""
		}
	}
	attrs["doctype-"+newDoctype] = ""
	d.Doctype = newDoctype
	attrs["doctype"] = newDoctype
}

func (d *Document) attributeLocked(name string) bool {
	_, ok := d.attributeOverrides[name]
	return ok
}

// setAttribute ports Document#set_attribute; it returns the resolved value
// and true if the attribute was assigned.
func (d *Document) setAttribute(name, value string) (string, bool) {
	if d.attributeLocked(name) {
		return "", false
	}
	if value != "" {
		value = d.applyAttributeValueSubs(value)
	}
	if d.headerAttributes != nil {
		d.Attributes[name] = value
	} else {
		switch name {
		case "backend":
			init := d.attributesModified["htmlsyntax"] && value == d.Backend
			delete(d.attributesModified, "htmlsyntax")
			d.updateBackendAttributes(value, init)
		case "doctype":
			d.updateDoctypeAttributes(value)
		default:
			d.Attributes[name] = value
		}
		d.attributesModified[name] = true
	}
	return value, true
}

func (d *Document) deleteAttribute(name string) bool {
	if d.attributeLocked(name) {
		return false
	}
	delete(d.Attributes, name)
	d.attributesModified[name] = true
	return true
}

func (d *Document) applyAttributeValueSubs(value string) string {
	if m := attributeEntryPassMacroRx.Find(value); m != nil {
		value = m.Group(2)
		if m.Has(1) {
			value = d.applySubs(value, resolveSubs(m.Group(1), "inline", nil))
		}
	} else {
		value = d.applySubs(value, headerSubs)
	}
	if d.maxAttrValueSize > 0 && len(value) > d.maxAttrValueSize {
		max := d.maxAttrValueSize
		for max > 0 && max < len(value) && value[max]&0xC0 == 0x80 {
			max--
		}
		value = value[:max]
	}
	return value
}

func (d *Document) hasHeader() bool { return d.header != nil }

func (d *Document) setDocTitle(title string) {
	if d.header == nil {
		d.header = newSection(&d.Block, 0, true)
		d.header.Sectname = "header"
	}
	d.header.setTitle(title)
}

func (d *Document) firstSection() *Block {
	if d.header != nil {
		return d.header
	}
	for _, b := range d.Blocks {
		if b.Context == "section" {
			return b
		}
	}
	return nil
}

// doctitle returns the document title, if any.
func (d *Document) doctitle(useFallback bool) (string, bool) {
	if v, ok := d.Attributes["title"]; ok {
		return v, true
	}
	if sect := d.firstSection(); sect != nil && sect.hasTitle {
		return sect.Title(), true
	}
	if useFallback {
		if v, ok := d.Attributes["untitled-label"]; ok {
			return v, true
		}
	}
	return "", false
}

func sanitizeTitle(val string) string {
	if strings.Contains(val, "<") {
		return strip(squeezeSpaces(xmlSanitizeRx.ReplaceAll(val, "")))
	}
	return val
}

func (d *Document) finalizeHeader(unrooted map[string]string, headerValid bool) map[string]string {
	delete(unrooted, attrEntriesKey)
	d.saveAttributes()
	if !headerValid {
		unrooted["invalid-header"] = ""
	}
	return unrooted
}

func (d *Document) saveAttributes() {
	attrs := d.Attributes
	if _, ok := attrs["doctitle"]; !ok {
		if v, ok := d.doctitle(false); ok {
			attrs["doctitle"] = v
		}
	}
	if d.ID == "" {
		d.ID = attrs["css-signature"]
	}
	tocVal, hasToc := attrs["toc"]
	if _, ok := attrs["toc2"]; ok {
		delete(attrs, "toc2")
		tocVal, hasToc = "left", true
	}
	if hasToc {
		placement, ok := attrs["toc-placement"]
		if !ok {
			placement = "macro"
		}
		var posVal string
		if placement != "auto" {
			posVal = placement
		} else {
			posVal = attrs["toc-position"]
		}
		if !(tocVal == "" && posVal == "") {
			defaultClass := "toc2"
			position := posVal
			if position == "" {
				if tocVal == "" {
					position = "left"
				} else {
					position = tocVal
				}
			}
			attrs["toc"] = ""
			attrs["toc-placement"] = "auto"
			switch position {
			case "left", "<", "&lt;":
				attrs["toc-position"] = "left"
			case "right", ">", "&gt;":
				attrs["toc-position"] = "right"
			case "top", "^":
				attrs["toc-position"] = "top"
			case "bottom", "v":
				attrs["toc-position"] = "bottom"
			case "preamble", "macro":
				attrs["toc-position"] = "content"
				attrs["toc-placement"] = position
				defaultClass = ""
			default:
				delete(attrs, "toc-position")
				defaultClass = ""
			}
			if defaultClass != "" {
				if _, ok := attrs["toc-class"]; !ok {
					attrs["toc-class"] = defaultClass
				}
			}
		}
	}
	if icons, ok := attrs["icons"]; ok {
		if _, ok := attrs["icontype"]; !ok {
			switch icons {
			case "", "font":
			default:
				attrs["icons"] = ""
				if icons != "image" {
					attrs["icontype"] = icons
				}
			}
		}
	}
	_, d.compatMode = attrs["compat-mode"]
	if d.compatMode {
		if v, ok := attrs["language"]; ok {
			attrs["source-language"] = v
		}
	}
	if d.parentDocument == nil {
		if hl, ok := attrs["source-highlighter"]; ok {
			d.highlight = hl == "highlight.js" || hl == "highlightjs"
			// The server-side highlighters are not available; mimic the
			// markup Asciidoctor emits when the highlighter library cannot
			// be loaded (the adapter still formats the <pre> wrapper).
			switch hl {
			case "coderay":
				d.fallbackHL = "CodeRay"
			case "rouge", "pygments":
				d.fallbackHL = hl
			}
		}
		d.outfilesuffix = attrs["outfilesuffix"]
		if o, ok := d.attributeOverrides["sectnums"]; ok && o.state == 's' {
			delete(d.attributeOverrides, "sectnums")
		}
	}
	d.headerAttributes = copyAttrs(attrs)
}

func (d *Document) restoreAttributes() {
	if d.parentDocument == nil {
		d.callouts.rewind()
	}
	if d.headerAttributes != nil {
		d.Attributes = copyAttrs(d.headerAttributes)
	}
}

func (d *Document) playbackAttributes(blockAttrs map[string]string) {
	for _, e := range loadAttributeEntries(blockAttrs) {
		if e.negate {
			delete(d.Attributes, e.name)
			if e.name == "compat-mode" {
				d.compatMode = false
			}
		} else {
			d.Attributes[e.name] = e.value
			if e.name == "compat-mode" {
				d.compatMode = true
			}
		}
	}
}

// counter ports Document#counter.
func (d *Document) counter(name, seed string, hasSeed bool) string {
	if d.parentDocument != nil {
		return d.parentDocument.counter(name, seed, hasSeed)
	}
	locked := d.attributeLocked(name)
	var next string
	if cur, ok := d.counters[name]; locked && ok {
		next = nextval(cur)
	} else if cur := d.Attributes[name]; cur != "" {
		next = nextval(cur)
	} else if hasSeed {
		next = seed
	} else {
		next = "1"
	}
	d.counters[name] = next
	if !locked {
		d.Attributes[name] = next
	}
	return next
}

func (d *Document) incrementAndStoreCounter(name string, block *Block) string {
	val := d.counter(name, "", false)
	saveAttributeEntry(block.Attributes, attributeEntry{name, val, false})
	return val
}

func nextval(cur string) string {
	if n := atoi(cur); strconv.Itoa(n) == cur {
		return strconv.Itoa(n + 1)
	}
	return rubySucc(cur)
}

// rubySucc emulates String#succ for the common alphanumeric cases.
func rubySucc(s string) string {
	if s == "" {
		return ""
	}
	b := []byte(s)
	isAlnum := func(c byte) bool {
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	hasAlnum := false
	for _, c := range b {
		if isAlnum(c) {
			hasAlnum = true
			break
		}
	}
	if !hasAlnum {
		b[len(b)-1]++
		return string(b)
	}
	i := len(b) - 1
	for i >= 0 && !isAlnum(b[i]) {
		i--
	}
	for {
		c := b[i]
		switch {
		case c == 'z':
			b[i] = 'a'
		case c == 'Z':
			b[i] = 'A'
		case c == '9':
			b[i] = '0'
		default:
			b[i]++
			return string(b)
		}
		// carry
		j := i - 1
		for j >= 0 && !isAlnum(b[j]) {
			j--
		}
		if j < 0 {
			var ins byte
			switch c {
			case 'z':
				ins = 'a'
			case 'Z':
				ins = 'A'
			default:
				ins = '1'
			}
			nb := append([]byte{}, b[:i]...)
			nb = append(nb, ins)
			nb = append(nb, b[i:]...)
			return string(nb)
		}
		i = j
	}
}

func (d *Document) registerRef(id string, block *Block, inline *Inline) bool {
	if _, ok := d.refs[id]; ok {
		return false
	}
	d.refs[id] = refTarget{block, inline}
	root := d
	for root.parentDocument != nil {
		root = root.parentDocument
	}
	root.refOrder = append(root.refOrder, id)
	return true
}

func (d *Document) registerImage(target string) {}

func (d *Document) refXreftext(ref refTarget, xrefstyle string) (string, bool) {
	if ref.inline != nil {
		if ref.inline.hasText {
			return ref.inline.Parent.applySubs(ref.inline.Text, reftextSubs), true
		}
		return "", false
	}
	return ref.block.xreftext(xrefstyle)
}

func (d *Document) resolveID(text string) (string, bool) {
	root := d
	for root.parentDocument != nil {
		root = root.parentDocument
	}
	if d.reftexts != nil {
		id, ok := d.reftexts[text]
		return id, ok
	}
	if d.parsed {
		d.reftexts = map[string]string{}
		for _, id := range root.refOrder {
			if xt, ok := d.refXreftext(d.refs[id], ""); ok {
				if _, seen := d.reftexts[xt]; !seen {
					d.reftexts[xt] = id
				}
			}
		}
		id, ok := d.reftexts[text]
		return id, ok
	}
	// not yet parsed: scan without caching; guard against reentrancy
	d.reftexts = map[string]string{}
	defer func() { d.reftexts = nil }()
	for _, id := range append([]string(nil), root.refOrder...) {
		if xt, ok := d.refXreftext(d.refs[id], ""); ok && xt == text {
			return id, true
		}
	}
	return "", false
}

// xreftext ports AbstractBlock#xreftext / Section#xreftext / Document#xreftext.
func (b *Block) xreftext(xrefstyle string) (string, bool) {
	if val, ok := b.reftext(); ok && val != "" {
		return val, true
	}
	if b.Context == "document" {
		return b.Document.doctitle(false)
	}
	if b.Context == "section" {
		if xrefstyle != "" {
			chapOrApp := b.Sectname == "chapter" || b.Sectname == "appendix"
			if b.Numbered {
				switch xrefstyle {
				case "full":
					var quoted string
					if chapOrApp {
						quoted = strings.Replace(b.subQuotes("_%s_"), "%s", b.Title(), 1)
					} else {
						quoted = strings.Replace(b.subQuotes(b.quoteTemplate()), "%s", b.Title(), 1)
					}
					comma := ","
					if sig, ok := b.Document.Attributes[b.Sectname+"-refsig"]; ok {
						return sig + " " + b.sectnum(".", &comma) + " " + quoted, true
					}
					return b.sectnum(".", &comma) + " " + quoted, true
				case "short":
					empty := ""
					if sig, ok := b.Document.Attributes[b.Sectname+"-refsig"]; ok {
						return sig + " " + b.sectnum(".", &empty), true
					}
					return b.sectnum(".", &empty), true
				}
			}
			if chapOrApp {
				return strings.Replace(b.subQuotes("_%s_"), "%s", b.Title(), 1), true
			}
		}
		return b.Title(), b.hasTitle
	}
	if xrefstyle != "" && b.hasTitle && b.hasCaption && b.Caption != "" {
		switch xrefstyle {
		case "full":
			quoted := strings.Replace(b.subQuotes(b.quoteTemplate()), "%s", b.Title(), 1)
			if b.Numeral != "" {
				if an, ok := captionAttributeNames[b.Context]; ok {
					if prefix, ok := b.Document.Attributes[an]; ok {
						return prefix + " " + b.Numeral + ", " + quoted, true
					}
				}
			}
			return strings.TrimSuffix(b.Caption, ". ") + ", " + quoted, true
		case "short":
			if b.Numeral != "" {
				if an, ok := captionAttributeNames[b.Context]; ok {
					if prefix, ok := b.Document.Attributes[an]; ok {
						return prefix + " " + b.Numeral, true
					}
				}
			}
			return strings.TrimSuffix(b.Caption, ". "), true
		}
	}
	return b.Title(), b.hasTitle
}

func (b *Block) quoteTemplate() string {
	if b.Document.compatMode {
		return "``%s''"
	}
	return "\"`%s`\""
}

func (d *Document) normalizeSystemPath(target, start string) string {
	jail := ""
	if d.Safe < SafeSafe {
		if start != "" {
			if !strings.HasPrefix(start, "/") {
				start = d.baseDir + "/" + start
			}
		} else {
			start = d.baseDir
		}
	} else {
		if start == "" {
			start = d.baseDir
		}
		jail = d.baseDir
	}
	wd, _ := os.Getwd()
	return systemPath(target, start, jail, filepath.ToSlash(wd))
}

func (b *Block) normalizeWebPath(target, start string, preserveURITarget bool) string {
	if preserveURITarget && isUriish(target) {
		return encodeSpacesInURI(target)
	}
	return webPath(target, start)
}

// docinfo ports Document#docinfo: it reads the shared and/or private docinfo
// files for the given location ("head", "header" or "footer").
func (d *Document) docinfo(location string) string {
	if d.Safe >= SafeSecure {
		return ""
	}
	attrs := d.Attributes
	qualifier := ""
	if location != "head" {
		qualifier = "-" + location
	}
	var kinds []string
	if v, ok := attrs["docinfo"]; !ok || v == "" {
		if _, ok2 := attrs["docinfo2"]; ok2 {
			kinds = []string{"private", "shared"}
		} else if _, ok1 := attrs["docinfo1"]; ok1 {
			kinds = []string{"shared"}
		} else if ok {
			kinds = []string{"private"}
		} else {
			return ""
		}
	} else {
		for _, k := range rubySplit(v, ",") {
			kinds = append(kinds, strip(k))
		}
	}
	has := func(names ...string) bool {
		for _, k := range kinds {
			for _, n := range names {
				if k == n {
					return true
				}
			}
		}
		return false
	}
	file := "docinfo" + qualifier + d.outfilesuffix
	dir := attrs["docinfodir"]
	subs := []string{"attributes"}
	if v, ok := attrs["docinfosubs"]; ok {
		subs = resolveSubs(v, "block", nil)
	}
	var content []string
	read := func(name string) {
		data, err := os.ReadFile(d.normalizeSystemPath(name, dir))
		if err != nil {
			return
		}
		text := strings.Join(prepareSourceString(string(data), true), "\n")
		content = append(content, d.applySubs(text, subs))
	}
	if has("shared", "shared-"+location) {
		read(file)
	}
	if docname := attrs["docname"]; docname != "" && has("private", "private-"+location) {
		read(docname + "-" + file)
	}
	return strings.Join(content, "\n")
}
