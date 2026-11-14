package adocgo

import (
	"strconv"
	"strings"
)

// Block is a node in the parsed document tree. A single struct is used for
// every block-level node (sections, paragraphs, lists, list items, tables,
// ...); the Context field identifies the kind of node, mirroring the way
// Asciidoctor models its AST.
type Block struct {
	// Context identifies the kind of node, e.g. "document", "section",
	// "paragraph", "listing", "ulist", "list_item", "table".
	Context string
	// Document is the document this node belongs to.
	Document *Document
	// Parent is the enclosing node (nil for the document).
	Parent *Block
	// ID is the node's id, if any.
	ID string
	// emptyID is set when an id was generated but turned out empty (Ruby
	// distinguishes an empty id from a missing one).
	emptyID bool
	// Attributes holds the block attributes. Positional attributes are
	// stored under the keys "$1", "$2", ...
	Attributes map[string]string
	// Style is the block style (e.g. "source", "NOTE", "arabic").
	Style string
	// implicitStyle is set when an ordered list style was inferred from an
	// explicit marker such as "A." (Asciidoctor stores a Symbol in that case,
	// which suppresses the type attribute on the <ol> element).
	implicitStyle bool
	// Caption is the caption prefix assigned to the block title.
	Caption    string
	hasCaption bool
	// Numeral is the number assigned to a captioned block or section.
	Numeral string
	// Level is the section nesting level.
	Level int
	// ContentModel is one of "compound", "simple", "verbatim", "raw", "empty".
	ContentModel string
	// Lines holds the source lines of a leaf block.
	Lines []string
	// Blocks holds the child blocks (or list items for a list).
	Blocks []*Block

	title          string
	hasTitle       bool
	convertedTitle string
	titleConverted bool

	subs        []string
	defaultSubs []string
	hasDefSubs  bool

	passthroughs []passthru
	passLocked   bool

	nextSectionIndex   int
	nextSectionOrdinal int

	// Section fields.
	Sectname        string
	Special         bool
	Numbered        bool
	numberedChapter bool
	Index           int

	// List item fields.
	Marker  string
	text    string
	hasText bool

	// DlistItems holds the entries of a description list.
	DlistItems []*DlistEntry

	// Table holds table data when Context is "table".
	Table *Table
	// cell holds cell data when Context is "table_cell".
	cell *Cell
}

// DlistEntry is one entry of a description list: one or more terms and an
// optional description.
type DlistEntry struct {
	Terms []*Block
	Desc  *Block
}

type passthru struct {
	text    string
	subs    []string
	typ     string
	attrs   map[string]string
	hasAttr bool
}

var defaultContentModel = map[string]string{
	"audio": "empty", "image": "empty", "listing": "verbatim", "literal": "verbatim",
	"stem": "raw", "open": "compound", "page_break": "empty", "pass": "raw",
	"thematic_break": "empty", "video": "empty",
}

func newBlock(parent *Block, context string, contentModel string, lines []string, attrs map[string]string) *Block {
	b := &Block{Context: context, Attributes: map[string]string{}}
	if parent != nil {
		b.Parent = parent
		b.Document = parent.Document
		b.Level = parent.Level
	}
	for k, v := range attrs {
		b.Attributes[k] = v
	}
	if contentModel == "" {
		contentModel = defaultContentModel[context]
		if contentModel == "" {
			contentModel = "simple"
		}
	}
	b.ContentModel = contentModel
	if len(lines) > 0 {
		b.Lines = append([]string(nil), lines...)
	}
	return b
}

func newSection(parent *Block, level int, hasLevel bool) *Block {
	s := &Block{Context: "section", Attributes: map[string]string{}, ContentModel: "compound"}
	s.Parent = parent
	s.Document = parent.Document
	s.nextSectionOrdinal = 1
	if parent.Context == "section" {
		if hasLevel {
			s.Level = level
		} else {
			s.Level = parent.Level + 1
		}
		s.Special = parent.Special
	} else {
		if hasLevel {
			s.Level = level
		} else {
			s.Level = 1
		}
	}
	return s
}

func newList(parent *Block, context string) *Block {
	b := &Block{Context: context, Attributes: map[string]string{}, ContentModel: "compound"}
	b.Parent = parent
	b.Document = parent.Document
	b.Level = parent.Level
	return b
}

func newListItem(parent *Block, text string, hasText bool) *Block {
	b := &Block{Context: "list_item", Attributes: map[string]string{}, ContentModel: "compound"}
	b.Parent = parent
	b.Document = parent.Document
	b.Level = parent.Level
	b.text = text
	b.hasText = hasText
	b.subs = append([]string(nil), normalSubs...)
	return b
}

func (b *Block) setParent(p *Block) {
	b.Parent = p
	b.Document = p.Document
}

// append adds a child block (Ruby's <<).
func (b *Block) append(child *Block) {
	if b.Context == "section" && child.Context == "section" {
		b.assignNumeral(child)
	}
	if child.Parent != b {
		child.setParent(b)
	}
	b.Blocks = append(b.Blocks, child)
}

// Attr returns the value of the named attribute.
func (b *Block) Attr(name string) (string, bool) {
	v, ok := b.Attributes[name]
	return v, ok
}

func (b *Block) attr(name string) string { return b.Attributes[name] }

func (b *Block) hasAttr(name string) bool {
	_, ok := b.Attributes[name]
	return ok
}

// attrFallback returns the block attribute or, failing that, the document
// attribute of the same name.
func (b *Block) attrFallback(name string) (string, bool) {
	if v, ok := b.Attributes[name]; ok {
		return v, true
	}
	if b.Parent != nil {
		if v, ok := b.Document.Attributes[name]; ok {
			return v, true
		}
	}
	return "", false
}

func (b *Block) hasOption(name string) bool {
	_, ok := b.Attributes[name+"-option"]
	return ok
}

func (b *Block) setOption(name string) { b.Attributes[name+"-option"] = "" }

func (b *Block) role() string { return b.Attributes["role"] }

func (b *Block) hasRole(name string) bool {
	v, ok := b.Attributes["role"]
	return ok && strings.Contains(" "+v+" ", " "+name+" ")
}

// HasTitle reports whether the block has a title.
func (b *Block) HasTitle() bool { return b.hasTitle }

// RawTitle returns the unconverted title.
func (b *Block) RawTitle() string { return b.title }

func (b *Block) setTitle(t string) {
	b.title = t
	b.hasTitle = true
	b.titleConverted = false
	b.convertedTitle = ""
}

// Title returns the title with substitutions applied.
func (b *Block) Title() string {
	if !b.hasTitle {
		return ""
	}
	if !b.titleConverted {
		b.convertedTitle = b.applySubs(b.title, normalSubs)
		b.titleConverted = true
	}
	return b.convertedTitle
}

func (b *Block) captionedTitle() string { return b.Caption + b.Title() }

func (b *Block) hasSub(name string) bool {
	for _, s := range b.subs {
		if s == name {
			return true
		}
	}
	return false
}

func (b *Block) removeSub(name string) {
	out := b.subs[:0]
	for _, s := range b.subs {
		if s != name {
			out = append(out, s)
		}
	}
	b.subs = out
}

func (b *Block) source() string { return strings.Join(b.Lines, "\n") }

func (b *Block) hasBlocks() bool { return len(b.Blocks) > 0 }

func (b *Block) sections() []*Block {
	var out []*Block
	for _, c := range b.Blocks {
		if c.Context == "section" {
			out = append(out, c)
		}
	}
	return out
}

func (b *Block) hasSections() bool { return b.nextSectionIndex > 0 }

// reftext returns the converted reftext attribute.
func (b *Block) reftext() (string, bool) {
	if v, ok := b.Attributes["reftext"]; ok {
		return b.applySubs(v, reftextSubs), true
	}
	return "", false
}

var captionAttributeNames = map[string]string{
	"example": "example-caption", "figure": "figure-caption", "listing": "listing-caption", "table": "table-caption",
}

func (b *Block) assignCaption(value string, hasValue bool, captionContext string) {
	if b.hasCaption || !b.hasTitle {
		return
	}
	if hasValue {
		b.Caption, b.hasCaption = value, true
		return
	}
	if v, ok := b.Document.Attributes["caption"]; ok {
		b.Caption, b.hasCaption = v, true
		return
	}
	if captionContext == "" {
		captionContext = b.Context
	}
	if attrName, ok := captionAttributeNames[captionContext]; ok {
		if prefix, ok := b.Document.Attributes[attrName]; ok {
			b.Numeral = b.Document.incrementAndStoreCounter(captionContext+"-number", b)
			b.Caption = prefix + " " + b.Numeral + ". "
			b.hasCaption = true
		}
	}
}

func (b *Block) assignNumeral(section *Block) {
	section.Index = b.nextSectionIndex
	b.nextSectionIndex++
	if section.Numbered {
		sectname := section.Sectname
		if sectname == "appendix" {
			section.Numeral = b.Document.counter("appendix-number", "A", true)
			if caption, ok := b.Document.Attributes["appendix-caption"]; ok {
				section.Caption = caption + " " + section.Numeral + ": "
			} else {
				section.Caption = section.Numeral + ". "
			}
			section.hasCaption = true
		} else if sectname == "chapter" || section.numberedChapter {
			section.Numeral = b.Document.counter("chapter-number", "1", true)
		} else {
			if sectname == "part" {
				section.Numeral = intToRoman(b.nextSectionOrdinal)
			} else {
				section.Numeral = strconv.Itoa(b.nextSectionOrdinal)
			}
			b.nextSectionOrdinal++
		}
	}
}

func (b *Block) sectnum(delimiter string, appendStr *string) string {
	app := delimiter
	if appendStr != nil {
		app = *appendStr
	}
	if b.Level > 1 && b.Parent != nil && b.Parent.Context == "section" {
		return b.Parent.sectnum(delimiter, &delimiter) + b.Numeral + app
	}
	return b.Numeral + app
}

// list item helpers

func (b *Block) itemHasText() bool { return b.hasText && b.text != "" }

func (b *Block) itemText() string {
	if !b.hasText {
		return ""
	}
	return b.applySubs(b.text, b.subs)
}

func (b *Block) isSimpleItem() bool {
	if len(b.Blocks) == 0 {
		return true
	}
	if len(b.Blocks) == 1 {
		c := b.Blocks[0].Context
		return c == "ulist" || c == "olist"
	}
	return false
}

func (b *Block) foldFirst() {
	first := b.Blocks[0]
	b.Blocks = b.Blocks[1:]
	if !b.hasText || b.text == "" {
		b.text = first.source()
	} else {
		b.text = b.text + "\n" + first.source()
	}
	b.hasText = true
}

// Inline is an inline node produced while applying substitutions.
type Inline struct {
	Context string
	Parent  *Block
	Text    string
	hasText bool
	ID      string
	Type    string
	Target  string
	Attrs   map[string]string
	// list-valued attributes
	Keys     []string
	Submenus []string
	Terms    []string
	SeeAlso  []string
	Guard    []string
}

func (n *Inline) attr(name string) string { return n.Attrs[name] }
func (n *Inline) hasAttr(name string) bool {
	_, ok := n.Attrs[name]
	return ok
}
func (n *Inline) role() string { return n.Attrs["role"] }

func intToRoman(val int) string {
	romans := []struct {
		l string
		n int
	}{{"M", 1000}, {"CM", 900}, {"D", 500}, {"CD", 400}, {"C", 100}, {"XC", 90}, {"L", 50}, {"XL", 40}, {"X", 10}, {"IX", 9}, {"V", 5}, {"IV", 4}, {"I", 1}}
	var sb strings.Builder
	for _, r := range romans {
		for val >= r.n {
			sb.WriteString(r.l)
			val -= r.n
		}
	}
	return sb.String()
}
