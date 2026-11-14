package adocgo

import (
	"strconv"
	"strings"

	"github.com/jdkato/adocgo/internal/rx"
)

// This file is a port of Asciidoctor's parser.rb.

const attrEntriesKey = "$attribute_entries"

var (
	admonitionStyles = map[string]bool{"NOTE": true, "TIP": true, "IMPORTANT": true, "WARNING": true, "CAUTION": true}
	paragraphStyles  = map[string]bool{"comment": true, "example": true, "literal": true, "listing": true, "normal": true, "open": true, "pass": true, "quote": true, "sidebar": true, "source": true, "verse": true, "abstract": true, "partintro": true}
	verbatimStyles   = map[string]bool{"literal": true, "listing": true, "source": true, "verse": true}
	setextLevels     = map[byte]int{'=': 0, '-': 1, '~': 2, '^': 3, '+': 4}
)

type delimitedBlockDef struct {
	context string
	masq    map[string]bool
}

func set(items ...string) map[string]bool {
	m := map[string]bool{}
	for _, i := range items {
		m[i] = true
	}
	return m
}

var delimitedBlocks = map[string]delimitedBlockDef{
	"--":   {"open", set("comment", "example", "literal", "listing", "pass", "quote", "sidebar", "source", "verse", "admonition", "abstract", "partintro")},
	"----": {"listing", set("literal", "source")},
	"....": {"literal", set("listing", "source")},
	"====": {"example", set("admonition")},
	"****": {"sidebar", set()},
	"____": {"quote", set("verse")},
	"++++": {"pass", set("stem", "latexmath", "asciimath")},
	"|===": {"table", set()},
	",===": {"table", set()},
	":===": {"table", set()},
	"!===": {"table", set()},
	"////": {"comment", set()},
	"```":  {"fenced_code", set()},
}

var delimitedBlockHeads = func() map[string]bool {
	m := map[string]bool{}
	for k := range delimitedBlocks {
		m[k[:2]] = true
	}
	return m
}()

type blockMatch struct {
	context    string
	masq       map[string]bool
	tip        string
	terminator string
}

func isDelimitedBlock(line string) *blockMatch {
	lineLen := len(line)
	if lineLen < 2 || !delimitedBlockHeads[line[:2]] {
		return nil
	}
	var tip string
	var tipLen int
	if lineLen == 2 {
		tip, tipLen = line, 2
	} else {
		if lineLen < 5 {
			tip, tipLen = line, lineLen
		} else {
			tip, tipLen = line[:4], 4
		}
		if strings.HasPrefix(tip, "`") {
			if tipLen == 4 {
				if tip == "````" {
					return nil
				}
				tip = tip[:3]
				if tip != "```" {
					return nil
				}
				line = tip
				lineLen, tipLen = 3, 3
			} else if tip != "```" {
				return nil
			}
		} else if tipLen == 3 {
			return nil
		}
	}
	def, ok := delimitedBlocks[tip]
	if !ok {
		return nil
	}
	if lineLen == tipLen || (len(tip) == 4 && uniform(line[1:], tip[3], lineLen-1)) {
		return &blockMatch{def.context, def.masq, tip, line}
	}
	return nil
}

func uniform(s string, ch byte, n int) bool {
	return strings.Count(s, string(ch)) == n
}

func startOfBlock(l string) bool {
	return (strings.HasPrefix(l, "[") && blockAttributeLineRx.MatchString(l)) || isDelimitedBlock(l) != nil
}

func startOfList(l string) bool { return anyListRx.MatchString(l) }

func startOfBlockOrList(l string) bool {
	return isDelimitedBlock(l) != nil || (strings.HasPrefix(l, "[") && blockAttributeLineRx.MatchString(l)) || anyListRx.MatchString(l)
}

func parseDocument(reader *Reader, doc *Document, headerOnly bool) {
	blockAttrs := parseDocumentHeader(reader, doc, headerOnly)
	if headerOnly {
		return
	}
	for reader.HasMoreLines() {
		var sect *Block
		sect, blockAttrs = nextSection(reader, &doc.Block, blockAttrs)
		if sect != nil {
			doc.assignNumeral(sect)
			doc.Blocks = append(doc.Blocks, sect)
		}
	}
}

func parseDocumentHeader(reader *Reader, doc *Document, headerOnly bool) map[string]string {
	blockAttrs := map[string]string{}
	if reader.SkipBlankLines() >= 0 {
		parseBlockMetadataLines(reader, doc, blockAttrs, false)
	}
	docAttrs := doc.Attributes
	leveloffset, hasOffset := docAttrs["leveloffset"]
	implicitDoctitle := isNextLineDoctitle(reader, blockAttrs, leveloffset, hasOffset)
	if _, ok := blockAttrs["title"]; implicitDoctitle && ok {
		docAttrs["authorcount"] = "0"
		return doc.finalizeHeader(blockAttrs, false)
	}
	var doctitleAttrVal string
	hasDoctitleAttrVal := false
	if val := docAttrs["doctitle"]; val != "" {
		doc.setDocTitle(val)
		doctitleAttrVal, hasDoctitleAttrVal = val, true
	}
	if implicitDoctitle {
		id, _, l0Title, _, atx := parseSectionTitle(reader, doc, "")
		doc.ID = id
		hasL0 := true
		if hasDoctitleAttrVal {
			hasL0 = false
		} else {
			doc.setDocTitle(l0Title)
			doctitleAttrVal = subSpecialchars(l0Title)
			if strings.Contains(doctitleAttrVal, "{") {
				doctitleAttrVal = doc.subAttributes(doctitleAttrVal, subAttrOpts{attributeMissing: "skip"})
			}
			docAttrs["doctitle"] = doctitleAttrVal
			hasDoctitleAttrVal = true
		}
		if !atx && !doc.attributeLocked("compat-mode") {
			docAttrs["compat-mode"] = ""
		}
		if sep, ok := blockAttrs["separator"]; ok {
			if !doc.attributeLocked("title-separator") {
				docAttrs["title-separator"] = sep
			}
		}
		docID := ""
		if v, ok := blockAttrs["id"]; ok {
			doc.ID = v
			docID = v
		} else {
			docID = doc.ID
		}
		if role, ok := blockAttrs["role"]; ok {
			docAttrs["role"] = role
		}
		if reftext, ok := blockAttrs["reftext"]; ok {
			docAttrs["reftext"] = reftext
		}
		for k := range blockAttrs {
			delete(blockAttrs, k)
		}
		delete(doc.attributesModified, "doctitle")
		parseHeaderMetadata(reader, doc)
		if doc.attributesModified["doctitle"] {
			if val := docAttrs["doctitle"]; val == "" || val == doctitleAttrVal {
				docAttrs["doctitle"] = doctitleAttrVal
			} else {
				doc.setDocTitle(val)
			}
		} else if !hasL0 {
			doc.attributesModified["doctitle"] = true
		}
		if docID != "" {
			doc.registerRef(docID, &doc.Block, nil)
		}
	} else if author, ok := docAttrs["author"]; ok {
		meta, _ := processAuthors([]string{author}, true, false)
		if _, ok := docAttrs["authorinitials"]; ok {
			delete(meta, "authorinitials")
		}
		for k, v := range meta {
			docAttrs[k] = v
		}
	} else if author, ok := docAttrs["authors"]; ok {
		meta, _ := processAuthors([]string{author}, true, true)
		for k, v := range meta {
			docAttrs[k] = v
		}
	} else {
		docAttrs["authorcount"] = "0"
	}
	if doc.Doctype == "manpage" {
		parseManpageHeader(reader, doc, blockAttrs, headerOnly)
	}
	return doc.finalizeHeader(blockAttrs, true)
}

func parseManpageHeader(reader *Reader, doc *Document, blockAttrs map[string]string, headerOnly bool) {
	docAttrs := doc.Attributes
	var manvolnum string
	if m := manpageTitleVolnumRx.Find(docAttrs["doctitle"]); m != nil {
		manvolnum = m.Group(2)
		docAttrs["manvolnum"] = manvolnum
		mantitle := m.Group(1)
		if strings.Contains(mantitle, "{") {
			mantitle = doc.subAttributes(mantitle, subAttrOpts{})
		}
		docAttrs["mantitle"] = strings.ToLower(mantitle)
	} else {
		if v, ok := docAttrs["doctitle"]; ok {
			docAttrs["mantitle"] = v
		} else if v, ok := docAttrs["docname"]; ok {
			docAttrs["mantitle"] = v
		} else {
			docAttrs["mantitle"] = "command"
		}
		manvolnum = "1"
		docAttrs["manvolnum"] = manvolnum
	}
	_, hasName := docAttrs["manname"]
	_, hasPurpose := docAttrs["manpurpose"]
	if hasName && hasPurpose {
		if _, ok := docAttrs["manname-title"]; !ok {
			docAttrs["manname-title"] = "Name"
		}
		doc.mannames = []string{docAttrs["manname"]}
	} else if headerOnly {
		// done
	} else {
		reader.SkipBlankLines()
		reader.save()
		parseBlockMetadataLines(reader, doc, blockAttrs, false)
		errMsg := false
		if lvl, ok := isNextLineSection(reader, map[string]string{}); ok {
			if lvl == 1 {
				nameSection := initializeSection(reader, &doc.Block, map[string]string{})
				lines := reader.ReadLinesUntil(readOpts{breakOnBlankLines: true, skipLineComments: true}, nil)
				for i, l := range lines {
					lines[i] = strings.TrimLeft(l, " \t")
				}
				buf := strings.Join(lines, " ")
				if m := manpageNamePurposeRx.Find(buf); m != nil {
					manname := m.Group(1)
					if strings.Contains(manname, "{") {
						manname = doc.subAttributes(manname, subAttrOpts{})
					}
					var mannames []string
					if strings.Contains(manname, ",") {
						for _, n := range rubySplit(manname, ",") {
							mannames = append(mannames, strings.TrimLeft(n, " \t"))
						}
						manname = mannames[0]
					} else {
						mannames = []string{manname}
					}
					manpurpose := m.Group(2)
					if strings.Contains(manpurpose, "{") {
						manpurpose = doc.subAttributes(manpurpose, subAttrOpts{})
					}
					if _, ok := docAttrs["manname-title"]; !ok {
						docAttrs["manname-title"] = nameSection.Title()
					}
					if nameSection.ID != "" {
						docAttrs["manname-id"] = nameSection.ID
					}
					docAttrs["manname"] = manname
					doc.mannames = mannames
					docAttrs["manpurpose"] = manpurpose
				} else {
					errMsg = true
				}
			} else {
				errMsg = true
			}
		} else {
			errMsg = true
		}
		if errMsg {
			reader.restoreSave()
			manname := "command"
			if v, ok := docAttrs["docname"]; ok {
				manname = v
			}
			docAttrs["manname"] = manname
			doc.mannames = []string{manname}
		} else {
			reader.discardSave()
		}
	}
}

func copyAttrs(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func clearAttrs(m map[string]string) {
	for k := range m {
		delete(m, k)
	}
}

func nextSection(reader *Reader, parent *Block, attributes map[string]string) (*Block, map[string]string) {
	var preamble, intro *Block
	part := false
	var section *Block
	var document *Document
	var book bool
	currentLevel := 0
	expectedNextLevel := 0
	_ = expectedNextLevel
	invalidHeader := false
	if _, ok := attributes["invalid-header"]; ok {
		invalidHeader = true
	}
	isDocStart := parent.Context == "document" && len(parent.Blocks) == 0
	hasHeader := false
	enter := false
	if isDocStart {
		hasHeader = parent.Document.hasHeader()
		if hasHeader {
			enter = true
		} else if invalidHeader {
			delete(attributes, "invalid-header")
			enter = true
		} else if _, ok := isNextLineSection(reader, attributes); !ok {
			enter = true
		}
	}
	if enter {
		document = parent.Document
		book = document.Doctype == "book"
		if hasHeader || (book && attributes["$1"] != "abstract") {
			preamble = newBlock(parent, "preamble", "compound", nil, nil)
			intro = preamble
			if book {
				if pt, ok := parent.Attributes["preface-title"]; ok {
					preamble.setTitle(pt)
				}
			}
			parent.Blocks = append(parent.Blocks, preamble)
		}
		section = parent
		currentLevel = 0
	} else {
		document = parent.Document
		book = document.Doctype == "book"
		section = initializeSection(reader, parent, attributes)
		if title, ok := attributes["title"]; ok {
			attributes = map[string]string{"title": title}
		} else {
			attributes = map[string]string{}
		}
		currentLevel = section.Level
		if currentLevel == 0 {
			part = book
		}
	}
	reader.SkipBlankLines()
	for reader.HasMoreLines() {
		parseBlockMetadataLines(reader, document, attributes, false)
		if nextLevel, ok := isNextLineSection(reader, attributes); ok {
			if lo, ok := document.Attributes["leveloffset"]; ok {
				nextLevel += atoi(lo)
				if nextLevel < 0 {
					nextLevel = 0
				}
			}
			if nextLevel > currentLevel {
				var ns *Block
				ns, attributes = nextSection(reader, section, attributes)
				section.assignNumeral(ns)
				section.Blocks = append(section.Blocks, ns)
			} else if nextLevel == 0 && section == &document.Block {
				var ns *Block
				ns, attributes = nextSection(reader, section, attributes)
				section.assignNumeral(ns)
				section.Blocks = append(section.Blocks, ns)
			} else {
				break
			}
		} else {
			target := section
			if intro != nil {
				target = intro
			}
			if nb := nextBlock(reader, target, attributes, blockOpts{noParseMetadata: true}); nb != nil {
				if part {
					if !section.hasBlocks() {
						if nb.Style != "partintro" {
							if nb.Style == "open" && nb.Context == "open" {
								nb.Style = "partintro"
							} else {
								intro = newBlock(section, "open", "compound", nil, nil)
								nb.setParent(intro)
								intro.Style = "partintro"
								section.Blocks = append(section.Blocks, intro)
							}
						} else if nb.ContentModel == "simple" {
							nb.ContentModel = "compound"
							para := newBlock(nb, "paragraph", "", nb.Lines, nil)
							para.subs = append([]string(nil), nb.subs...)
							para.hasDefSubs = true
							para.defaultSubs = append([]string(nil), nb.subs...)
							nb.Blocks = append(nb.Blocks, para)
							nb.Lines = nil
							nb.subs = nil
						}
					} else if len(section.Blocks) == 1 {
						first := section.Blocks[0]
						if intro == nil && first.ContentModel == "compound" {
							// illegal block content outside of partintro block
						} else if first.ContentModel != "compound" {
							intro = newBlock(section, "open", "compound", nil, nil)
							nb.setParent(intro)
							intro.Style = "partintro"
							if first.Style == "partintro" {
								first.Context = "paragraph"
								first.Style = ""
							}
							section.Blocks = section.Blocks[1:]
							intro.append(first)
							section.Blocks = append(section.Blocks, intro)
						}
					}
				}
				if intro != nil {
					intro.Blocks = append(intro.Blocks, nb)
				} else {
					section.Blocks = append(section.Blocks, nb)
				}
				clearAttrs(attributes)
			}
		}
		if reader.SkipBlankLines() < 0 {
			break
		}
	}
	if part {
		// invalid part check omitted (logging only)
	} else if preamble != nil {
		if preamble.hasBlocks() {
			if book || len(document.Blocks) > 1 {
				// keep preamble
			} else {
				document.Blocks = document.Blocks[1:]
				for _, child := range preamble.Blocks {
					document.Block.append(child)
				}
				preamble.Blocks = nil
			}
		} else {
			document.Blocks = document.Blocks[1:]
		}
	}
	if section == parent {
		return nil, copyAttrs(attributes)
	}
	return section, copyAttrs(attributes)
}

type blockOpts struct {
	noParseMetadata bool
	textOnly        bool
	listType        string
}

func atoi(s string) int {
	// Ruby String#to_i: parse leading integer, ignoring trailing garbage
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	end := 0
	if end < len(s) && (s[end] == '-' || s[end] == '+') {
		end++
	}
	for end < len(s) && ((s[end] >= '0' && s[end] <= '9') || (s[end] == '_' && end > 0 && end+1 < len(s) && s[end-1] != '_')) {
		end++
	}
	n, err := strconv.Atoi(strings.ReplaceAll(s[:end], "_", ""))
	if err != nil {
		return 0
	}
	return n
}

func nextBlock(reader *Reader, parent *Block, attributes map[string]string, options blockOpts) *Block {
	skipped := reader.SkipBlankLines()
	if skipped < 0 {
		return nil
	}
	textOnly := options.textOnly
	if textOnly && skipped > 0 {
		options.textOnly = false
		textOnly = false
	}
	document := parent.Document
	if !options.noParseMetadata {
		for parseBlockMetadataLine(reader, document, attributes, options.textOnly) {
			reader.Shift()
			if reader.SkipBlankLines() < 0 {
				return nil
			}
		}
	}
	reader.Mark()
	thisLine, _ := reader.ReadLine()
	docAttrs := document.Attributes
	style, hasStyle := attributes["$1"]
	if style == "" {
		hasStyle = false
	}
	var block *Block
	var blockContext, cloakedContext, terminator string
	hasTerminator := false
	delimited := isDelimitedBlock(thisLine)
	if delimited != nil {
		blockContext, cloakedContext = delimited.context, delimited.context
		terminator, hasTerminator = delimited.terminator, true
		if hasStyle {
			if style != blockContext {
				if delimited.masq[style] {
					blockContext = style
				} else if delimited.masq["admonition"] && admonitionStyles[style] {
					blockContext = "admonition"
				} else {
					style = blockContext
				}
			}
		} else {
			style, hasStyle = blockContext, true
			attributes["style"] = style
		}
	}
	if delimited == nil {
		// this loop only executes once; it is used so "break" can leave early
		for once := true; once; once = false {
			if hasStyle && verbatimStyles[style] {
				blockContext = style
				cloakedContext = "paragraph"
				reader.UnshiftLine(thisLine)
				break
			}
			var indented bool
			var ch0 byte
			if textOnly {
				indented = strings.HasPrefix(thisLine, " ") || strings.HasPrefix(thisLine, "\t")
			} else {
				if strings.HasPrefix(thisLine, " ") {
					indented, ch0 = true, ' '
					ls := strings.TrimLeft(thisLine, " \t\n\v\f\r")
					if (strings.HasPrefix(ls, "-") || strings.HasPrefix(ls, "*") || strings.HasPrefix(ls, "_")) && markdownThematicBreakRx.MatchString(thisLine) {
						block = newBlock(parent, "thematic_break", "empty", nil, nil)
						break
					}
				} else if strings.HasPrefix(thisLine, "\t") {
					indented, ch0 = true, '\t'
				} else {
					indented = false
					if thisLine != "" {
						ch0 = thisLine[0]
					}
					breakCtx := ""
					switch ch0 {
					case '\'', '-', '*', '_':
						breakCtx = "thematic_break"
					case '<':
						breakCtx = "page_break"
					}
					if breakCtx != "" && extLayoutBreakRx.MatchString(thisLine) {
						block = newBlock(parent, breakCtx, "empty", nil, nil)
						break
					} else if strings.HasSuffix(thisLine, "]") && strings.Contains(thisLine, "::") {
						if ch0 == 'i' || strings.HasPrefix(thisLine, "video:") || strings.HasPrefix(thisLine, "audio:") {
							if m := blockMediaMacroRx.Find(thisLine); m != nil {
								blkCtx, target := m.Group(1), m.Group(2)
								block = newBlock(parent, blkCtx, "empty", nil, nil)
								if m.Has(3) {
									var posattrs []string
									switch blkCtx {
									case "video":
										posattrs = []string{"poster", "width", "height"}
									case "audio":
									default:
										posattrs = []string{"alt", "width", "height"}
									}
									block.parseAttributes(m.Group(3), posattrs, parseAttrOpts{subInput: true, into: attributes})
								}
								delete(attributes, "style")
								if strings.Contains(target, "{") {
									expanded := block.subAttributes(target, subAttrOpts{})
									if expanded == "" && docAttrs["attribute-missing"] == "drop-line" &&
										block.subAttributes(target+" ", subAttrOpts{attributeMissing: "drop-line"}) == "" {
										clearAttrs(attributes)
										return nil
									}
									target = expanded
								}
								if blkCtx == "image" {
									document.registerImage(target)
									if v, ok := docAttrs["imagesdir"]; ok {
										attributes["imagesdir"] = v
									} else {
										delete(attributes, "imagesdir")
									}
									if _, ok := attributes["alt"]; !ok {
										if hasStyle {
											attributes["alt"] = style
										} else {
											alt := strings.NewReplacer("_", " ", "-", " ").Replace(basenameNoExt(target))
											attributes["default-alt"] = alt
											attributes["alt"] = alt
										}
									}
									if sw, ok := attributes["scaledwidth"]; ok {
										delete(attributes, "scaledwidth")
										if sw != "" {
											if m := trailingDigitsRx.Find(sw); m != nil {
												sw += "%"
											}
											attributes["scaledwidth"] = sw
										}
									}
									if t, ok := attributes["title"]; ok {
										block.setTitle(t)
										delete(attributes, "title")
										cap, hasCap := attributes["caption"]
										delete(attributes, "caption")
										block.assignCaption(cap, hasCap, "figure")
									}
								}
								attributes["target"] = target
								break
							}
						}
						if ch0 == 't' && strings.HasPrefix(thisLine, "toc:") {
							if m := blockTocMacroRx.Find(thisLine); m != nil {
								block = newBlock(parent, "toc", "empty", nil, nil)
								if m.Has(1) {
									block.parseAttributes(m.Group(1), nil, parseAttrOpts{subInput: true, into: attributes})
								}
								break
							}
						}
					}
				}
			}
			if !indented && ch0 == 0 && thisLine != "" {
				ch0 = thisLine[0]
			}
			if !indented && ch0 == '<' {
				if m := calloutListRx.Find(thisLine); m != nil {
					reader.UnshiftLine(thisLine)
					block = parseCalloutList(reader, m, parent, document.callouts)
					attributes["style"] = "arabic"
					break
				}
			}
			if unorderedListRx.MatchString(thisLine) {
				reader.UnshiftLine(thisLine)
				if !hasStyle && parent.Context == "section" && parent.Sectname == "bibliography" {
					style, hasStyle = "bibliography", true
					attributes["style"] = style
				}
				block = parseList(reader, "ulist", parent, style)
				break
			} else if orderedListRx.MatchString(thisLine) {
				reader.UnshiftLine(thisLine)
				block = parseList(reader, "olist", parent, style)
				if block.Style != "" {
					attributes["style"] = block.Style
				}
				break
			} else if strings.Contains(thisLine, "::") || strings.Contains(thisLine, ";;") {
				if m := descriptionListRx.Find(thisLine); m != nil {
					reader.UnshiftLine(thisLine)
					block = parseDescriptionList(reader, m, parent)
					break
				}
			}
			if style == "float" || style == "discrete" {
				nl, hasNl := reader.PeekLine()
				if _, ok := isSectionTitle(thisLine, nl, hasNl); ok {
					reader.UnshiftLine(thisLine)
					floatID, floatReftext, blockTitle, floatLevel, _ := parseSectionTitle(reader, document, attributes["id"])
					if floatReftext != "" {
						attributes["reftext"] = floatReftext
					}
					block = newBlock(parent, "floating_title", "empty", nil, nil)
					block.setTitle(blockTitle)
					delete(attributes, "title")
					if floatID != "" {
						block.ID = floatID
					} else if _, ok := docAttrs["sectids"]; ok {
						block.ID = generateSectionID(block.Title(), document)
					}
					block.Level = floatLevel
					break
				}
			}
			if hasStyle && style != "normal" {
				if paragraphStyles[style] {
					blockContext = style
					cloakedContext = "paragraph"
					reader.UnshiftLine(thisLine)
					break
				} else if admonitionStyles[style] {
					blockContext = "admonition"
					cloakedContext = "paragraph"
					reader.UnshiftLine(thisLine)
					break
				} else {
					style, hasStyle = "", false
				}
			}
			reader.UnshiftLine(thisLine)
			if indented && !hasStyle {
				contentAdjacent := ""
				if skipped == 0 {
					contentAdjacent = options.listType
				}
				lines := readParagraphLines(reader, contentAdjacent != "", readOpts{skipLineComments: textOnly})
				adjustIndentation(lines, 0, 0)
				if textOnly || contentAdjacent == "dlist" {
					block = newBlock(parent, "paragraph", "simple", lines, attributes)
				} else {
					block = newBlock(parent, "literal", "verbatim", lines, attributes)
				}
			} else {
				lines := readParagraphLines(reader, skipped == 0 && options.listType != "", readOpts{skipLineComments: true})
				if textOnly {
					if indented && style == "normal" {
						adjustIndentation(lines, 0, 0)
					}
					block = newBlock(parent, "paragraph", "simple", lines, attributes)
				} else if m := admonitionMatch(ch0, thisLine); m != nil {
					lines[0] = m.Post()
					name := strings.ToLower(m.Group(1))
					attributes["style"] = m.Group(1)
					attributes["name"] = name
					if cap, ok := attributes["caption"]; ok {
						delete(attributes, "caption")
						attributes["textlabel"] = cap
					} else if v, ok := docAttrs[name+"-caption"]; ok {
						attributes["textlabel"] = v
					} else {
						delete(attributes, "textlabel")
					}
					block = newBlock(parent, "admonition", "simple", lines, attributes)
				} else if ch0 == '>' && strings.HasPrefix(thisLine, "> ") {
					for i, line := range lines {
						if line == ">" {
							lines[i] = ""
						} else if strings.HasPrefix(line, "> ") {
							lines[i] = line[2:]
						}
					}
					creditLine, hasCredit := "", false
					if strings.HasPrefix(lines[len(lines)-1], "-- ") {
						creditLine, hasCredit = lines[len(lines)-1][3:], true
						lines = lines[:len(lines)-1]
						for len(lines) > 0 && lines[len(lines)-1] == "" {
							lines = lines[:len(lines)-1]
						}
					}
					attributes["style"] = "quote"
					block = buildBlock("quote", "compound", "", false, true, parent, newReader(lines), attributes)
					if hasCredit {
						applyCredit(block, creditLine, attributes)
					}
				} else if ch0 == '"' && len(lines) > 1 && strings.HasPrefix(lines[len(lines)-1], "-- ") && strings.HasSuffix(lines[len(lines)-2], `"`) {
					lines[0] = thisLine[1:]
					creditLine := lines[len(lines)-1][3:]
					lines = lines[:len(lines)-1]
					for len(lines) > 0 && lines[len(lines)-1] == "" {
						lines = lines[:len(lines)-1]
					}
					if len(lines) > 0 {
						last := lines[len(lines)-1]
						lines[len(lines)-1] = chop(last)
					}
					attributes["style"] = "quote"
					block = newBlock(parent, "quote", "simple", lines, attributes)
					applyCredit(block, creditLine, attributes)
				} else {
					if indented && style == "normal" {
						adjustIndentation(lines, 0, 0)
					}
					block = newBlock(parent, "paragraph", "simple", lines, attributes)
				}
				catalogInlineAnchors(strings.Join(lines, "\n"), block, document)
			}
		}
	}
	if block == nil {
		switch blockContext {
		case "listing", "source":
			language, hasLang := "", false
			if blockContext != "source" {
				if _, ok := attributes["$1"]; !ok {
					if v, ok := attributes["$2"]; ok {
						language, hasLang = v, true
					} else if v, ok := docAttrs["source-language"]; ok {
						language, hasLang = v, true
					}
				}
			}
			if blockContext == "source" || hasLang {
				if hasLang {
					attributes["style"] = "source"
					attributes["language"] = language
					rekey(attributes, []string{"", "", "linenums"})
				} else {
					rekey(attributes, []string{"", "language", "linenums"})
					if _, ok := attributes["language"]; !ok {
						if v, ok := docAttrs["source-language"]; ok {
							attributes["language"] = v
						}
					}
					if cloakedContext != "listing" {
						attributes["cloaked-context"] = cloakedContext
					}
				}
				setSourceDefaults(attributes, docAttrs)
			}
			block = buildBlock("listing", "verbatim", terminator, hasTerminator, false, parent, reader, attributes)
		case "fenced_code":
			attributes["style"] = "source"
			language := ""
			if ll := len(thisLine); ll > 3 {
				language = thisLine[3:]
				if commaIdx := strings.Index(language, ","); commaIdx >= 0 {
					if commaIdx > 0 {
						if commaIdx < ll-4 {
							attributes["linenums"] = ""
						}
						language = strings.TrimSpace(language[:commaIdx])
					} else if ll > 4 {
						attributes["linenums"] = ""
					}
				} else {
					language = strings.TrimLeft(language, " \t")
				}
			}
			if language == "" {
				if v, ok := docAttrs["source-language"]; ok {
					attributes["language"] = v
				}
			} else {
				attributes["language"] = language
			}
			attributes["cloaked-context"] = cloakedContext
			setSourceDefaults(attributes, docAttrs)
			terminator = terminator[:3]
			block = buildBlock("listing", "verbatim", terminator, true, false, parent, reader, attributes)
		case "table":
			lines := reader.ReadLinesUntil(readOpts{terminator: terminator, hasTerminator: true, skipLineComments: true, context: "table"}, nil)
			blockReader := newReader(lines)
			if !strings.HasPrefix(terminator, "|") && !strings.HasPrefix(terminator, "!") {
				if _, ok := attributes["format"]; !ok {
					if strings.HasPrefix(terminator, ",") {
						attributes["format"] = "csv"
					} else {
						attributes["format"] = "dsv"
					}
				}
			}
			block = parseTable(blockReader, parent, attributes)
		case "sidebar":
			block = buildBlock(blockContext, "compound", terminator, hasTerminator, false, parent, reader, attributes)
		case "admonition":
			name := strings.ToLower(style)
			attributes["name"] = name
			if cap, ok := attributes["caption"]; ok {
				delete(attributes, "caption")
				attributes["textlabel"] = cap
			} else if v, ok := docAttrs[name+"-caption"]; ok {
				attributes["textlabel"] = v
			} else {
				delete(attributes, "textlabel")
			}
			block = buildBlock(blockContext, "compound", terminator, hasTerminator, false, parent, reader, attributes)
		case "open", "abstract", "partintro":
			block = buildBlock("open", "compound", terminator, hasTerminator, false, parent, reader, attributes)
		case "literal":
			block = buildBlock(blockContext, "verbatim", terminator, hasTerminator, false, parent, reader, attributes)
		case "example":
			if _, ok := attributes["collapsible-option"]; ok {
				attributes["caption"] = ""
			}
			block = buildBlock(blockContext, "compound", terminator, hasTerminator, false, parent, reader, attributes)
		case "quote", "verse":
			rekey(attributes, []string{"", "attribution", "citetitle"})
			cm := "compound"
			if blockContext == "verse" {
				cm = "verbatim"
			}
			block = buildBlock(blockContext, cm, terminator, hasTerminator, false, parent, reader, attributes)
		case "stem", "latexmath", "asciimath":
			if blockContext == "stem" {
				key, ok := attributes["$2"]
				if !ok {
					key = docAttrs["stem"]
				}
				attributes["style"] = stemTypeAlias(key)
			}
			block = buildBlock("stem", "raw", terminator, hasTerminator, false, parent, reader, attributes)
		case "pass":
			block = buildBlock(blockContext, "raw", terminator, hasTerminator, false, parent, reader, attributes)
		case "comment":
			buildBlock(blockContext, "skip", terminator, hasTerminator, false, parent, reader, attributes)
			clearAttrs(attributes)
			return nil
		default:
			// unsupported block type; treat as paragraph to stay robust
			block = buildBlock("paragraph", "simple", terminator, hasTerminator, false, parent, reader, attributes)
		}
	}
	blockTitle, hasBlockTitle := "", false
	if t, ok := attributes["title"]; ok {
		blockTitle, hasBlockTitle = t, true
		block.setTitle(t)
		delete(attributes, "title")
		if _, ok := captionAttributeNames[block.Context]; ok {
			cap, hasCap := attributes["caption"]
			delete(attributes, "caption")
			block.assignCaption(cap, hasCap, "")
		}
	}
	block.Style = attributes["style"]
	if block.ID == "" {
		block.ID = attributes["id"]
	}
	if block.ID != "" {
		if hasBlockTitle {
			if strings.Contains(blockTitle, "{") {
				block.Title()
			}
		} else if block.hasTitle {
			block.Title()
		}
		document.registerRef(block.ID, block, nil)
	}
	for k, v := range attributes {
		block.Attributes[k] = v
	}
	block.commitSubs()
	if block.hasSub("callouts") {
		if !catalogCallouts(block.source(), document) {
			block.removeSub("callouts")
		}
	}
	return block
}

func stemTypeAlias(key string) string {
	switch key {
	case "latexmath", "latex", "tex":
		return "latexmath"
	}
	return "asciimath"
}

func setSourceDefaults(attributes, docAttrs map[string]string) {
	if _, ok := attributes["linenums"]; !ok {
		_, a := attributes["linenums-option"]
		_, b := docAttrs["source-linenums-option"]
		if a || b {
			attributes["linenums"] = ""
		}
	}
	if _, ok := attributes["indent"]; !ok {
		if v, ok := docAttrs["source-indent"]; ok {
			attributes["indent"] = v
		}
	}
}

func admonitionMatch(ch0 byte, line string) *rx.Match {
	switch ch0 {
	case 'N', 'T', 'I', 'W', 'C':
		if strings.Contains(line, ":") {
			return admonitionParagraphRx.Find(line)
		}
	}
	return nil
}

func applyCredit(block *Block, creditLine string, attributes map[string]string) {
	subbed := block.applySubs(creditLine, normalSubs)
	parts := strings.SplitN(subbed, ", ", 2)
	if subbed == "" {
		return
	}
	attributes["attribution"] = parts[0]
	if len(parts) > 1 {
		attributes["citetitle"] = parts[1]
	}
}

func chop(s string) string {
	if s == "" {
		return s
	}
	_, size := lastRune(s)
	return s[:len(s)-size]
}

func readParagraphLines(reader *Reader, breakAtList bool, opts readOpts) []string {
	opts.breakOnBlankLines = true
	opts.breakOnListContinuation = true
	opts.preserveLastLine = true
	if breakAtList {
		return reader.ReadLinesUntil(opts, startOfBlockOrList)
	}
	return reader.ReadLinesUntil(opts, startOfBlock)
}

// buildBlock ports Parser.build_block. When hasTerminator is false the block
// is being built from a paragraph. inplace corresponds to terminator == false
// in the Ruby code (parse the given reader directly).
func buildBlock(blockContext, contentModel, terminator string, hasTerminator, inplace bool, parent *Block, reader *Reader, attributes map[string]string) *Block {
	skipProcessing := false
	parseAs := contentModel
	switch contentModel {
	case "skip":
		skipProcessing, parseAs = true, "simple"
	case "raw":
		parseAs = "simple"
	}
	var lines []string
	var blockReader *Reader
	if inplace {
		blockReader = reader
	} else if !hasTerminator {
		if parseAs == "verbatim" {
			lines = reader.ReadLinesUntil(readOpts{breakOnBlankLines: true, breakOnListContinuation: true}, nil)
		} else {
			if contentModel == "compound" {
				contentModel = "simple"
			}
			lines = readParagraphLines(reader, false, readOpts{skipLineComments: true, skipProcessing: skipProcessing})
		}
	} else if parseAs != "compound" {
		lines = reader.ReadLinesUntil(readOpts{terminator: terminator, hasTerminator: true, skipProcessing: skipProcessing, context: blockContext}, nil)
	} else {
		blockReader = newReader(reader.ReadLinesUntil(readOpts{terminator: terminator, hasTerminator: true, skipProcessing: skipProcessing, context: blockContext}, nil))
	}
	switch contentModel {
	case "verbatim":
		ts, ok := attributes["tabsize"]
		if !ok {
			ts = parent.Document.Attributes["tabsize"]
		}
		tabSize := atoi(ts)
		if indent, ok := attributes["indent"]; ok {
			lines = adjustIndentation(lines, atoi(indent), tabSize)
		} else if tabSize > 0 {
			lines = adjustIndentation(lines, -1, tabSize)
		}
	case "skip":
		return nil
	}
	block := newBlock(parent, blockContext, contentModel, lines, attributes)
	if contentModel == "compound" {
		parseBlocks(blockReader, block)
	}
	return block
}

func parseBlocks(reader *Reader, parent *Block) {
	for {
		block := nextBlock(reader, parent, map[string]string{}, blockOpts{})
		if block != nil {
			parent.Blocks = append(parent.Blocks, block)
		} else if !reader.HasMoreLines() {
			break
		}
	}
}

func catalogCallouts(text string, document *Document) bool {
	found := false
	autonum := 0
	if strings.Contains(text, "<") {
		for _, m := range calloutScanRx.FindAll(text) {
			if !strings.HasPrefix(m.Group(0), `\`) {
				num := m.Group(2)
				if num == "." {
					autonum++
					num = strconv.Itoa(autonum)
				}
				document.callouts.register(num)
			}
			found = true
		}
	}
	return found
}

func catalogInlineAnchor(id, reftext string, hasReftext bool, node *Block) {
	doc := node.Document
	if hasReftext && strings.Contains(reftext, "{") {
		reftext = doc.subAttributes(reftext, subAttrOpts{})
	}
	doc.registerRef(id, nil, &Inline{Context: "anchor", Parent: node, Text: reftext, hasText: hasReftext, Type: "ref", ID: id})
}

func catalogInlineAnchors(text string, block *Block, document *Document) {
	if !strings.Contains(text, "[[") && !strings.Contains(text, "or:") {
		return
	}
	for _, m := range inlineAnchorScanRx.FindAll(text) {
		var id, reftext string
		hasReftext := false
		if m.Has(1) {
			id = m.Group(1)
			if m.Has(2) {
				reftext, hasReftext = m.Group(2), true
				if strings.Contains(reftext, "{") {
					reftext = document.subAttributes(reftext, subAttrOpts{})
					if reftext == "" {
						continue
					}
				}
			}
		} else {
			id = m.Group(3)
			if m.Has(4) {
				reftext, hasReftext = m.Group(4), true
				if strings.Contains(reftext, "]") {
					reftext = strings.ReplaceAll(reftext, `\]`, "]")
					if strings.Contains(reftext, "{") {
						reftext = document.subAttributes(reftext, subAttrOpts{})
					}
				} else if strings.Contains(reftext, "{") {
					reftext = document.subAttributes(reftext, subAttrOpts{})
					if reftext == "" {
						hasReftext = false
					}
				}
			}
		}
		document.registerRef(id, nil, &Inline{Context: "anchor", Parent: block, Text: reftext, hasText: hasReftext, Type: "ref", ID: id})
	}
}

func catalogInlineBiblioAnchor(id, reftext string, hasReftext bool, node *Block) {
	txt := ""
	if hasReftext {
		txt = "[" + reftext + "]"
	}
	node.Document.registerRef(id, nil, &Inline{Context: "anchor", Parent: node, Text: txt, hasText: hasReftext, Type: "bibref", ID: id})
}

func initializeSection(reader *Reader, parent *Block, attributes map[string]string) *Block {
	document := parent.Document
	doctype := document.Doctype
	book := doctype == "book"
	sectStyle, hasSectStyle := attributes["$1"]
	if sectStyle == "" {
		hasSectStyle = false
	}
	sectID, sectReftext, sectTitle, sectLevel, _ := parseSectionTitle(reader, document, attributes["id"])
	var sectName string
	sectSpecial, sectNumbered := false, false
	if hasSectStyle {
		if book && sectStyle == "abstract" {
			sectName, sectLevel = "chapter", 1
		} else if strings.HasPrefix(sectStyle, "sect") && sectionLevelStyleRx.MatchString(sectStyle) {
			sectName = "section"
		} else {
			sectName, sectSpecial = sectStyle, true
			if sectLevel == 0 {
				sectLevel = 1
			}
			sectNumbered = sectName == "appendix"
		}
	} else if book {
		switch {
		case sectLevel == 0:
			sectName = "part"
		case sectLevel > 1:
			sectName = "section"
		default:
			sectName = "chapter"
		}
	} else if doctype == "manpage" && strings.EqualFold(sectTitle, "synopsis") {
		sectName, sectSpecial = "synopsis", true
	} else {
		sectName = "section"
	}
	if sectReftext != "" {
		attributes["reftext"] = sectReftext
	}
	section := newSection(parent, sectLevel, true)
	section.ID = sectID
	section.setTitle(sectTitle)
	section.Sectname = sectName
	sectnums, hasSectnums := document.Attributes["sectnums"]
	if sectSpecial {
		section.Special = true
		if sectNumbered {
			section.Numbered = true
		} else if sectnums == "all" {
			section.Numbered = true
			if book && sectLevel == 1 {
				section.numberedChapter = true
			}
		}
	} else if hasSectnums && sectLevel > 0 {
		if section.Special {
			section.Numbered = parent.Numbered
		} else {
			section.Numbered = true
		}
	} else if _, ok := document.Attributes["partnums"]; ok && book && sectLevel == 0 {
		section.Numbered = true
	}
	generated := false
	if section.ID == "" {
		if _, ok := document.Attributes["sectids"]; ok {
			section.ID = generateSectionID(section.Title(), document)
			generated = true
		}
	}
	if section.ID != "" {
		if !generated && strings.Contains(sectTitle, "{") {
			section.Title()
		}
		document.registerRef(section.ID, section, nil)
	}
	for k, v := range attributes {
		section.Attributes[k] = v
	}
	reader.SkipBlankLines()
	return section
}

func isNextLineSection(reader *Reader, attributes map[string]string) (int, bool) {
	style, hasStyle := attributes["$1"]
	if hasStyle && (style == "discrete" || style == "float") {
		return 0, false
	}
	lines := reader.PeekLines(2, hasStyle && style == "comment")
	l1, l2 := "", ""
	has2 := false
	if len(lines) > 0 {
		l1 = lines[0]
	}
	if len(lines) > 1 {
		l2, has2 = lines[1], true
	}
	return isSectionTitle(l1, l2, has2)
}

func isNextLineDoctitle(reader *Reader, attributes map[string]string, leveloffset string, hasOffset bool) bool {
	lvl, ok := isNextLineSection(reader, attributes)
	if !ok {
		return false
	}
	if hasOffset {
		return lvl+atoi(leveloffset) == 0
	}
	return lvl == 0
}

func isSectionTitle(line1, line2 string, has2 bool) (int, bool) {
	if lvl, ok := atxSectionTitle(line1); ok {
		return lvl, true
	}
	if !has2 || line2 == "" {
		return 0, false
	}
	return setextSectionTitle(line1, line2)
}

func atxSectionTitle(line string) (int, bool) {
	if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "#") {
		if m := extAtxSectionTitleRx.Find(line); m != nil {
			return len(m.Group(1)) - 1, true
		}
	}
	return 0, false
}

func setextSectionTitle(line1, line2 string) (int, bool) {
	level, ok := setextLevels[line2[0]]
	if !ok {
		return 0, false
	}
	l2len := runeLen(line2)
	if uniform(line2, line2[0], len(line2)) && setextSectionTitleRx.MatchString(line1) && abs(runeLen(line1)-l2len) < 2 {
		return level, true
	}
	return 0, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func parseSectionTitle(reader *Reader, document *Document, sectID string) (id, reftext, title string, level int, atx bool) {
	line1, _ := reader.ReadLine()
	id = sectID
	matched := false
	if strings.HasPrefix(line1, "=") || strings.HasPrefix(line1, "#") {
		if m := extAtxSectionTitleRx.Find(line1); m != nil {
			matched = true
			level, title, atx = len(m.Group(1))-1, m.Group(2), true
		}
	}
	if !matched {
		if line2, ok := reader.PeekLineDirect(); ok && line2 != "" {
			if lvl, ok := setextSectionTitle(line1, line2); ok {
				matched = true
				level = lvl
				title = line1
				atx = false
				reader.Shift()
			}
		}
	}
	if !matched {
		// unrecognized section; treat the line as a level-1 title to stay robust
		title, level, atx = line1, 1, true
	}
	if id == "" && strings.HasSuffix(title, "]]") {
		if m := inlineSectionAnchorRx.Find(title); m != nil && !m.Has(1) {
			title = title[:len(title)-len(m.Group(0))]
			id = m.Group(2)
			reftext = m.Group(3)
		}
	}
	if lo, ok := document.Attributes["leveloffset"]; ok {
		level += atoi(lo)
		if level < 0 {
			level = 0
		}
	}
	return
}

func parseHeaderMetadata(reader *Reader, document *Document) {
	docAttrs := document.Attributes
	processAttributeEntries(reader, document)
	implicit := map[string]string{}
	authorcount := -1
	var implicitAuthor, implicitInitials, implicitAuthors string
	hasImplicitAuthor, hasImplicitInitials, hasImplicitAuthors := false, false, false
	if reader.HasMoreLines() && !reader.NextLineEmpty() {
		line, _ := reader.ReadLine()
		implicit, authorcount = processAuthors([]string{line}, false, true)
		docAttrs["authorcount"] = strconv.Itoa(authorcount)
		if authorcount > 0 {
			for key, val := range implicit {
				if _, ok := docAttrs[key]; !ok {
					docAttrs[key] = document.applySubs(val, headerSubs)
				}
			}
			implicitAuthor, hasImplicitAuthor = docAttrs["author"]
			implicitInitials, hasImplicitInitials = docAttrs["authorinitials"]
			implicitAuthors, hasImplicitAuthors = docAttrs["authors"]
		}
		processAttributeEntries(reader, document)
		if reader.HasMoreLines() && !reader.NextLineEmpty() {
			revLine, _ := reader.ReadLine()
			if m := revisionInfoLineRx.Find(revLine); m != nil {
				rev := map[string]string{}
				if m.Has(1) {
					rev["revnumber"] = rstrip(m.Group(1))
				}
				if component := strings.TrimSpace(m.Group(2)); component != "" {
					if !m.Has(1) && strings.HasPrefix(component, "v") {
						rev["revnumber"] = component[1:]
					} else {
						rev["revdate"] = component
					}
				}
				if m.Has(3) {
					rev["revremark"] = rstrip(m.Group(3))
				}
				for key, val := range rev {
					if _, ok := docAttrs[key]; !ok {
						docAttrs[key] = document.applySubs(val, headerSubs)
					}
				}
			} else {
				reader.UnshiftLine(revLine)
			}
		}
		processAttributeEntries(reader, document)
		reader.SkipBlankLines()
	}
	var authorMeta map[string]string
	metaCount := 0
	if a, ok := docAttrs["author"]; ok && !(hasImplicitAuthor && a == implicitAuthor) {
		authorMeta, metaCount = processAuthors([]string{a}, true, false)
		ai, hasAI := docAttrs["authorinitials"]
		if hasAI != hasImplicitInitials || ai != implicitInitials {
			delete(authorMeta, "authorinitials")
		}
	} else if a, ok := docAttrs["authors"]; ok && !(hasImplicitAuthors && a == implicitAuthors) {
		authorMeta, metaCount = processAuthors([]string{a}, true, true)
	} else {
		var authors []string
		var present []bool
		explicit, sparse := false, false
		idx := 1
		key := "author_1"
		for {
			override, ok := docAttrs[key]
			if !ok {
				break
			}
			if iv, iok := implicit[key]; iok && iv == override {
				authors = append(authors, "")
				present = append(present, false)
				sparse = true
			} else {
				authors = append(authors, override)
				present = append(present, true)
				explicit = true
			}
			idx++
			key = "author_" + strconv.Itoa(idx)
		}
		if explicit {
			if sparse {
				for i := range authors {
					if present[i] {
						continue
					}
					var names []string
					n := strconv.Itoa(i + 1)
					for _, k := range []string{"firstname_", "middlename_", "lastname_"} {
						if v, ok := implicit[k+n]; ok {
							names = append(names, strings.ReplaceAll(v, " ", "_"))
						}
					}
					authors[i] = strings.Join(names, " ")
				}
			}
			authorMeta, metaCount = processAuthors(authors, true, false)
		} else {
			authorMeta, metaCount = map[string]string{}, 0
		}
	}
	if metaCount == 0 {
		if authorcount < 0 {
			docAttrs["authorcount"] = "0"
		}
	} else {
		for k, v := range authorMeta {
			docAttrs[k] = v
		}
		docAttrs["authorcount"] = strconv.Itoa(metaCount)
		if _, ok := docAttrs["email"]; !ok {
			if v, ok := docAttrs["email_1"]; ok {
				docAttrs["email"] = v
			}
		}
	}
}

var authorKeys = []string{"author", "authorinitials", "firstname", "middlename", "lastname", "email"}

func firstChar(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

func squeezeSpaces(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func processAuthors(entries []string, namesOnly, multiple bool) (map[string]string, int) {
	meta := map[string]string{}
	idx := 0
	if multiple && len(entries) == 1 && strings.Contains(entries[0], ";") {
		entries = rubySplitRx(entries[0], authorDelimiterRx)
	}
	for _, entry := range entries {
		if entry == "" {
			continue
		}
		idx++
		key := func(k string) string {
			if idx == 1 {
				return k
			}
			return k + "_" + strconv.Itoa(idx)
		}
		var segments []string
		var has []bool
		if namesOnly {
			if strings.Contains(entry, "<") {
				meta[key("author")] = strings.ReplaceAll(entry, "_", " ")
				entry = xmlSanitizeRx.ReplaceAll(entry, "")
			}
			segments = splitWhitespaceN(entry, 3)
			if len(segments) == 3 {
				segments[2] = squeezeSpaces(segments[2])
			}
			for range segments {
				has = append(has, true)
			}
			if len(segments) == 0 {
				segments = nil
			}
		} else if m := authorInfoLineRx.Find(entry); m != nil {
			for i := 1; i <= 4; i++ {
				segments = append(segments, m.Group(i))
				has = append(has, m.Has(i))
			}
		}
		get := func(i int) (string, bool) {
			if i < len(segments) && has[i] {
				return segments[i], true
			}
			return "", false
		}
		if segments != nil {
			fname := strings.ReplaceAll(segments[0], "_", " ")
			author := fname
			meta[key("firstname")] = fname
			meta[key("authorinitials")] = firstChar(fname)
			if s1, ok := get(1); ok {
				if s2, ok := get(2); ok {
					mname := strings.ReplaceAll(s1, "_", " ")
					lname := strings.ReplaceAll(s2, "_", " ")
					meta[key("middlename")] = mname
					meta[key("lastname")] = lname
					author = fname + " " + mname + " " + lname
					meta[key("authorinitials")] = firstChar(fname) + firstChar(mname) + firstChar(lname)
				} else {
					lname := strings.ReplaceAll(s1, "_", " ")
					meta[key("lastname")] = lname
					author = fname + " " + lname
					meta[key("authorinitials")] = firstChar(fname) + firstChar(lname)
				}
			}
			if _, ok := meta[key("author")]; !ok {
				meta[key("author")] = author
			}
			if !namesOnly {
				if s3, ok := get(3); ok {
					meta[key("email")] = s3
				}
			}
		} else {
			fname := strings.TrimSpace(squeezeSpaces(entry))
			meta[key("author")] = fname
			meta[key("firstname")] = fname
			meta[key("authorinitials")] = firstChar(fname)
		}
		if idx == 1 {
			meta["authors"] = meta[key("author")]
		} else {
			if idx == 2 {
				for _, k := range authorKeys {
					if v, ok := meta[k]; ok {
						meta[k+"_1"] = v
					}
				}
			}
			meta["authors"] = meta["authors"] + ", " + meta[key("author")]
		}
	}
	return meta, idx
}

func parseBlockMetadataLines(reader *Reader, document *Document, attributes map[string]string, textOnly bool) {
	for parseBlockMetadataLine(reader, document, attributes, textOnly) {
		reader.Shift()
		if reader.SkipBlankLines() < 0 {
			break
		}
	}
}

func parseBlockMetadataLine(reader *Reader, document *Document, attributes map[string]string, textOnly bool) bool {
	nextLine, ok := reader.PeekLine()
	if !ok || nextLine == "" {
		return false
	}
	c := nextLine[0]
	normal := false
	if textOnly {
		if c != '[' && c != '/' {
			return false
		}
	} else {
		if c != '[' && c != '.' && c != '/' && c != ':' {
			return false
		}
		normal = true
	}
	if c == '[' {
		if strings.HasPrefix(nextLine, "[[") {
			if strings.HasSuffix(nextLine, "]]") {
				if m := blockAnchorRx.Find(nextLine); m != nil {
					if m.Has(1) {
						attributes["id"] = m.Group(1)
					} else {
						delete(attributes, "id")
					}
					if m.Has(2) {
						reftext := m.Group(2)
						if strings.Contains(reftext, "{") {
							reftext = document.subAttributes(reftext, subAttrOpts{})
						}
						attributes["reftext"] = reftext
					}
					return true
				}
			}
		} else if strings.HasSuffix(nextLine, "]") {
			if m := blockAttributeListRx.Find(nextLine); m != nil {
				currentStyle, hasCurrent := attributes["$1"]
				if m.Group(1) != "" {
					document.parseAttributes(m.Group(1), nil, parseAttrOpts{subInput: true, subResult: true, into: attributes})
					if v, ok := attributes["$1"]; ok && v != "" {
						if st, ok := parseStyleAttribute(attributes); ok {
							attributes["$1"] = st
						} else if hasCurrent {
							attributes["$1"] = currentStyle
						} else {
							delete(attributes, "$1")
						}
					}
				}
				return true
			}
		}
	} else if normal && c == '.' {
		if m := blockTitleRx.Find(nextLine); m != nil {
			attributes["title"] = m.Group(1)
			return true
		}
	} else if !normal || c == '/' {
		if strings.HasPrefix(nextLine, "//") {
			if nextLine == "//" {
				return true
			} else if normal && uniform(nextLine, '/', len(nextLine)) {
				if len(nextLine) != 3 {
					reader.ReadLinesUntil(readOpts{terminator: nextLine, hasTerminator: true, skipFirstLine: true, preserveLastLine: true, skipProcessing: true, context: "comment"}, nil)
					return true
				}
			} else if !strings.HasPrefix(nextLine, "///") {
				return true
			}
		}
	} else if normal && c == ':' {
		if m := attributeEntryRx.Find(nextLine); m != nil {
			processAttributeEntry(reader, document, attributes, m)
			return true
		}
	}
	return false
}

func processAttributeEntries(reader *Reader, document *Document) {
	reader.SkipCommentLines()
	for processAttributeEntry(reader, document, nil, nil) {
		reader.Shift()
		reader.SkipCommentLines()
	}
}

func processAttributeEntry(reader *Reader, document *Document, attributes map[string]string, m *rx.Match) bool {
	if m == nil {
		if !reader.HasMoreLines() {
			return false
		}
		line, _ := reader.PeekLine()
		m = attributeEntryRx.Find(line)
		if m == nil {
			return false
		}
	}
	value := m.Group(2)
	if value != "" && (strings.HasSuffix(value, ` \`) || strings.HasSuffix(value, " +")) {
		con := value[len(value)-2:]
		value = rstrip(value[:len(value)-2])
		for reader.Advance() {
			nextLine, _ := reader.PeekLine()
			if nextLine == "" {
				break
			}
			nextLine = strings.TrimLeft(nextLine, " \t\n\v\f\r")
			keepOpen := strings.HasSuffix(nextLine, con)
			if keepOpen {
				nextLine = rstrip(nextLine[:len(nextLine)-2])
			}
			if strings.HasSuffix(value, " +") {
				value += "\n" + nextLine
			} else {
				value += " " + nextLine
			}
			if !keepOpen {
				break
			}
		}
	}
	storeAttribute(m.Group(1), value, true, document, attributes)
	return true
}

type attributeEntry struct {
	name   string
	value  string
	negate bool
}

func saveAttributeEntry(attrs map[string]string, e attributeEntry) {
	neg := "0"
	if e.negate {
		neg = "1"
	}
	attrs[attrEntriesKey] += e.name + "\x01" + e.value + "\x01" + neg + "\x02"
}

func loadAttributeEntries(attrs map[string]string) []attributeEntry {
	raw, ok := attrs[attrEntriesKey]
	if !ok {
		return nil
	}
	var out []attributeEntry
	for _, rec := range strings.Split(raw, "\x02") {
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x01")
		if len(parts) != 3 {
			continue
		}
		out = append(out, attributeEntry{parts[0], parts[1], parts[2] == "1"})
	}
	return out
}

// storeAttribute ports Parser.store_attribute. hasValue false corresponds to
// a nil value (unset).
func storeAttribute(name, value string, hasValue bool, doc *Document, attrs map[string]string) (string, string, bool) {
	if strings.HasSuffix(name, "!") {
		name = name[:len(name)-1]
		hasValue = false
	} else if strings.HasPrefix(name, "!") {
		name = name[1:]
		hasValue = false
	}
	if !hasValue {
		value = ""
	}
	name = sanitizeAttributeName(name)
	switch name {
	case "numbered":
		name = "sectnums"
	case "hardbreaks":
		name = "hardbreaks-option"
	case "showtitle":
		storeAttribute("notitle", "", !hasValue, doc, attrs)
	}
	if doc != nil {
		if hasValue {
			if name == "leveloffset" {
				if strings.HasPrefix(value, "+") {
					value = strconv.Itoa(atoi(doc.Attributes["leveloffset"]) + atoi(value[1:]))
				} else if strings.HasPrefix(value, "-") {
					value = strconv.Itoa(atoi(doc.Attributes["leveloffset"]) - atoi(value[1:]))
				}
			}
			if resolved, ok := doc.setAttribute(name, value); ok {
				value = resolved
				if attrs != nil {
					saveAttributeEntry(attrs, attributeEntry{name, value, false})
				}
			}
		} else if doc.deleteAttribute(name) && attrs != nil {
			saveAttributeEntry(attrs, attributeEntry{name, "", true})
		}
	} else if attrs != nil {
		saveAttributeEntry(attrs, attributeEntry{name, value, !hasValue})
	}
	return name, value, hasValue
}

func sanitizeAttributeName(name string) string {
	return strings.ToLower(invalidAttributeNameCharsRx.ReplaceAll(name, ""))
}

// parseStyleAttribute ports Parser.parse_style_attribute. It returns the
// parsed style (if any).
func parseStyleAttribute(attributes map[string]string) (string, bool) {
	rawStyle, ok := attributes["$1"]
	if !ok {
		return "", false
	}
	if strings.Contains(rawStyle, " ") {
		attributes["style"] = rawStyle
		return rawStyle, true
	}
	name := ""
	var accum strings.Builder
	var parsedStyle, parsedID string
	hasStyle, hasID := false, false
	var roles, options []string
	hasRole, hasOption := false, false
	flush := func() {
		value := accum.String()
		accum.Reset()
		switch name {
		case "":
			if value != "" {
				parsedStyle, hasStyle = value, true
			}
		case "id":
			if value != "" {
				parsedID, hasID = value, true
			}
		case "role":
			if value != "" {
				roles = append(roles, value)
				hasRole = true
			}
		case "option":
			if value != "" {
				options = append(options, value)
				hasOption = true
			}
		}
	}
	sawName := false
	for _, c := range rawStyle {
		switch c {
		case '.':
			flush()
			name, sawName = "role", true
		case '#':
			flush()
			name, sawName = "id", true
		case '%':
			flush()
			name, sawName = "option", true
		default:
			accum.WriteRune(c)
		}
	}
	if sawName {
		flush()
		if hasStyle {
			attributes["style"] = parsedStyle
		}
		if hasID {
			attributes["id"] = parsedID
		}
		if hasRole {
			if existing := attributes["role"]; existing == "" {
				attributes["role"] = strings.Join(roles, " ")
			} else {
				attributes["role"] = existing + " " + strings.Join(roles, " ")
			}
		}
		if hasOption {
			for _, opt := range options {
				attributes[opt+"-option"] = ""
			}
		}
		return parsedStyle, hasStyle
	}
	attributes["style"] = rawStyle
	return rawStyle, true
}

// adjustIndentation ports Parser.adjust_indentation!.
func adjustIndentation(lines []string, indentSize, tabSize int) []string {
	if len(lines) == 0 {
		return lines
	}
	if tabSize > 0 {
		hasTab := false
		for _, l := range lines {
			if strings.Contains(l, "\t") {
				hasTab = true
				break
			}
		}
		if hasTab {
			fullTab := strings.Repeat(" ", tabSize)
			for i, line := range lines {
				tabIdx := strings.Index(line, "\t")
				if line == "" || tabIdx < 0 {
					continue
				}
				if tabIdx == 0 {
					leading := 0
					for leading < len(line) && line[leading] == '\t' {
						leading++
					}
					line = strings.Repeat(fullTab, leading) + line[leading:]
					if !strings.Contains(line, "\t") {
						lines[i] = line
						continue
					}
				}
				spacesAdded := 0
				idx := 0
				var sb strings.Builder
				for _, c := range line {
					if c == '\t' {
						offset := idx + spacesAdded
						if offset%tabSize == 0 {
							spacesAdded += tabSize - 1
							sb.WriteString(fullTab)
						} else {
							spaces := tabSize - offset%tabSize
							if spaces != 1 {
								spacesAdded += spaces - 1
							}
							sb.WriteString(strings.Repeat(" ", spaces))
						}
					} else {
						sb.WriteRune(c)
					}
					idx++
				}
				lines[i] = sb.String()
			}
		}
	}
	if indentSize < 0 {
		return lines
	}
	blockIndent := -1
	for _, line := range lines {
		if line == "" {
			continue
		}
		lineIndent := len(line) - len(strings.TrimLeft(line, " \t\n\v\f\r\x00"))
		if lineIndent == 0 {
			blockIndent = -1
			break
		}
		if blockIndent < 0 || lineIndent < blockIndent {
			blockIndent = lineIndent
		}
	}
	if indentSize == 0 {
		if blockIndent > 0 {
			for i, line := range lines {
				if line != "" {
					lines[i] = line[blockIndent:]
				}
			}
		}
	} else {
		newIndent := strings.Repeat(" ", indentSize)
		for i, line := range lines {
			if line == "" {
				continue
			}
			if blockIndent > 0 {
				lines[i] = newIndent + line[blockIndent:]
			} else {
				lines[i] = newIndent + line
			}
		}
	}
	return lines
}
