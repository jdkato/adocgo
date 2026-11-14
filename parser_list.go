package adocgo

import (
	"strconv"
	"strings"

	"github.com/jdkato/adocgo/internal/rx"
)

func parseList(reader *Reader, listType string, parent *Block, style string) *Block {
	list := newList(parent, listType)
	listRx := listRxMap[listType]
	for reader.HasMoreLines() {
		line, _ := reader.PeekLine()
		m := listRx.Find(line)
		if m == nil {
			break
		}
		if item := parseListItem(reader, list, m, m.Group(1), nil, style); item != nil {
			list.Blocks = append(list.Blocks, item)
		}
		if reader.SkipBlankLines() < 0 {
			break
		}
	}
	return list
}

func parseDescriptionList(reader *Reader, m *rx.Match, parent *Block) *Block {
	list := newList(parent, "dlist")
	sibling := descriptionListSiblingRx[m.Group(2)]
	current := parseDlistItem(reader, list, m, sibling)
	list.DlistItems = append(list.DlistItems, current)
	for reader.HasMoreLines() {
		line, _ := reader.PeekLine()
		nm := sibling.Find(line)
		if nm == nil {
			break
		}
		next := parseDlistItem(reader, list, nm, sibling)
		if current.Desc != nil {
			current = next
			list.DlistItems = append(list.DlistItems, current)
		} else {
			current.Terms = append(current.Terms, next.Terms[0])
			current.Desc = next.Desc
		}
	}
	return list
}

func parseCalloutList(reader *Reader, m *rx.Match, parent *Block, callouts *callouts) *Block {
	list := newList(parent, "colist")
	autonum := 0
	for {
		if m == nil {
			line, ok := reader.PeekLine()
			if !ok {
				break
			}
			m = calloutListRx.Find(line)
			if m == nil {
				break
			}
			reader.Mark()
		}
		if m.Group(1) == "." {
			autonum++
		}
		if item := parseListItem(reader, list, m, "<1>", nil, ""); item != nil {
			list.Blocks = append(list.Blocks, item)
			if coids := callouts.calloutIDs(len(list.Blocks)); coids != "" {
				item.Attributes["coids"] = coids
			}
		}
		m = nil
	}
	callouts.nextList()
	return list
}

func parseDlistItem(reader *Reader, list *Block, m *rx.Match, sibling *rx.Regexp) *DlistEntry {
	termText := m.Group(1)
	term := newListItem(list, termText, true)
	if strings.HasPrefix(termText, "[[") {
		if am := leadingInlineAnchorRx.Find(termText); am != nil {
			if am.Has(2) {
				catalogInlineAnchor(am.Group(1), am.Group(2), true, term)
			} else {
				catalogInlineAnchor(am.Group(1), strings.TrimLeft(am.Post(), " \t\n\v\f\r"), true, term)
			}
		}
	}
	hasText := m.Has(3)
	item := newListItem(list, m.Group(3), hasText)
	parseListItemBody(reader, list, item, "dlist", "", sibling, hasText)
	entry := &DlistEntry{Terms: []*Block{term}}
	if item.itemHasText() || item.hasBlocks() {
		entry.Desc = item
	}
	return entry
}

func parseListItem(reader *Reader, list *Block, m *rx.Match, siblingTrait string, _ *rx.Regexp, style string) *Block {
	listType := list.Context
	itemText := m.Group(2)
	item := newListItem(list, itemText, true)
	switch listType {
	case "ulist":
		item.Marker = siblingTrait
		if strings.HasPrefix(itemText, "[") {
			if style == "bibliography" {
				if bm := inlineBiblioAnchorRx.Find(itemText); bm != nil {
					catalogInlineBiblioAnchor(bm.Group(1), bm.Group(2), bm.Has(2), item)
				}
			} else if strings.HasPrefix(itemText, "[[") {
				if am := leadingInlineAnchorRx.Find(itemText); am != nil {
					catalogInlineAnchor(am.Group(1), am.Group(2), am.Has(2), item)
				}
			} else if strings.HasPrefix(itemText, "[ ] ") || strings.HasPrefix(itemText, "[x] ") || strings.HasPrefix(itemText, "[*] ") {
				list.setOption("checklist")
				item.Attributes["checkbox"] = ""
				if !strings.HasPrefix(itemText, "[ ") {
					item.Attributes["checked"] = ""
				}
				item.text = itemText[4:]
			}
		}
	case "olist":
		var implicitStyle string
		siblingTrait, implicitStyle = resolveOrderedListMarker(siblingTrait)
		item.Marker = siblingTrait
		if len(list.Blocks) == 0 && style == "" {
			if implicitStyle != "" {
				list.Style = implicitStyle
			} else if n := len(siblingTrait) - 1; n >= 0 && n < len(orderedListStyles) {
				list.Style = orderedListStyles[n]
			} else {
				list.Style = "arabic"
			}
		}
		if strings.HasPrefix(itemText, "[[") {
			if am := leadingInlineAnchorRx.Find(itemText); am != nil {
				catalogInlineAnchor(am.Group(1), am.Group(2), am.Has(2), item)
			}
		}
	default:
		item.Marker = siblingTrait
		if strings.HasPrefix(itemText, "[[") {
			if am := leadingInlineAnchorRx.Find(itemText); am != nil {
				catalogInlineAnchor(am.Group(1), am.Group(2), am.Has(2), item)
			}
		}
	}
	parseListItemBody(reader, list, item, listType, siblingTrait, nil, true)
	return item
}

func parseListItemBody(reader *Reader, list, item *Block, listType, siblingTrait string, siblingRx *rx.Regexp, hasText bool) {
	dlist := listType == "dlist"
	reader.Shift()
	itemReader := newReader(readLinesForListItem(reader, listType, siblingTrait, siblingRx, hasText))
	if !itemReader.HasMoreLines() {
		return
	}
	commentLines := itemReader.SkipLineComments()
	contentAdjacent := false
	if subsequent, ok := itemReader.PeekLine(); ok {
		if len(commentLines) > 0 {
			itemReader.UnshiftLines(commentLines)
		}
		if subsequent != "" {
			contentAdjacent = true
			if !dlist {
				hasText = false
			}
		}
	}
	if block := nextBlock(itemReader, item, map[string]string{}, blockOpts{textOnly: !hasText, listType: listType}); block != nil {
		item.Blocks = append(item.Blocks, block)
	}
	for itemReader.HasMoreLines() {
		if block := nextBlock(itemReader, item, map[string]string{}, blockOpts{listType: listType}); block != nil {
			item.Blocks = append(item.Blocks, block)
		}
	}
	if contentAdjacent && len(item.Blocks) > 0 && item.Blocks[0].Context == "paragraph" {
		item.foldFirst()
	}
}

type listLine struct {
	s      string
	marker bool // list continuation marker (string or placeholder)
}

func nestedListType(line string, withinNested bool) (string, *rx.Match) {
	if withinNested {
		if m := descriptionListRx.Find(line); m != nil {
			return "dlist", m
		}
		return "", nil
	}
	for _, ctx := range []string{"ulist", "olist", "dlist"} {
		if m := listRxMap[ctx].Find(line); m != nil {
			return ctx, m
		}
	}
	return "", nil
}

func readLinesForListItem(reader *Reader, listType, siblingTrait string, siblingRx *rx.Regexp, hasText bool) []string {
	var buffer []listLine
	const (
		inactive = iota
		active
		frozen
	)
	continuation := inactive
	withinNestedList := false
	detachedContinuation := -1
	dlist := listType == "dlist"
	isSibling := func(line string) bool { return isSiblingListItem(line, listType, siblingTrait, siblingRx) }
	var thisLine listLine
	hasThis := false
	for reader.HasMoreLines() {
		s, _ := reader.ReadLine()
		thisLine, hasThis = listLine{s: s}, true
		if isSibling(s) {
			break
		}
		if s == "+" {
			thisLine.marker = true
		}
		var prevLine *listLine
		if len(buffer) > 0 {
			pl := buffer[len(buffer)-1]
			prevLine = &pl
		}
		if prevLine != nil && prevLine.marker {
			if continuation == inactive {
				continuation = active
				hasText = true
				if !withinNestedList {
					buffer[len(buffer)-1] = listLine{s: "", marker: true}
				}
			}
			if thisLine.marker {
				if continuation != frozen {
					continuation = frozen
					buffer = append(buffer, thisLine)
				}
				hasThis = false
				continue
			}
		}
		if match := isDelimitedBlock(thisLine.s); match != nil && !thisLine.marker {
			if continuation != active {
				break
			}
			buffer = append(buffer, thisLine)
			for _, l := range reader.ReadLinesUntil(readOpts{terminator: match.terminator, hasTerminator: true, readLastLine: true}, nil) {
				buffer = append(buffer, listLine{s: l})
			}
			continuation = inactive
		} else if dlist && continuation != active && strings.HasPrefix(thisLine.s, "[") && blockAttributeLineRx.MatchString(thisLine.s) {
			blockAttributeLines := []string{thisLine.s}
			interrupt := false
			for {
				nextLine, ok := reader.PeekLine()
				if !ok {
					break
				}
				if isDelimitedBlock(nextLine) != nil {
					interrupt = true
				} else if nextLine == "" || (strings.HasPrefix(nextLine, "[") && blockAttributeLineRx.MatchString(nextLine)) {
					l, _ := reader.ReadLine()
					blockAttributeLines = append(blockAttributeLines, l)
					continue
				} else if anyListRx.MatchString(nextLine) && !isSibling(nextLine) {
					for _, l := range blockAttributeLines {
						buffer = append(buffer, listLine{s: l})
					}
				} else {
					interrupt = true
				}
				break
			}
			if interrupt {
				hasThis = false
				reader.UnshiftLines(blockAttributeLines)
				break
			}
		} else if continuation == active && thisLine.s != "" {
			if literalParagraphRx.MatchString(thisLine.s) {
				reader.UnshiftLine(thisLine.s)
				var fn func(string) bool
				if dlist {
					fn = isSibling
				}
				for _, l := range reader.ReadLinesUntil(readOpts{preserveLastLine: true, breakOnBlankLines: true, breakOnListContinuation: true}, fn) {
					buffer = append(buffer, listLine{s: l})
				}
				continuation = inactive
			} else if ch0 := thisLine.s[0]; (ch0 == '.' && blockTitleRx.MatchString(thisLine.s)) ||
				(ch0 == '[' && blockAttributeLineRx.MatchString(thisLine.s)) || (ch0 == ':' && attributeEntryRx.MatchString(thisLine.s)) {
				buffer = append(buffer, thisLine)
			} else {
				if nt, nm := nestedListType(thisLine.s, withinNestedList); nt != "" {
					withinNestedList = true
					if nt == "dlist" && nm.Group(3) == "" {
						hasText = false
					}
				}
				buffer = append(buffer, thisLine)
				continuation = inactive
			}
		} else if prevLine != nil && prevLine.s == "" {
			if thisLine.s == "" {
				if reader.SkipBlankLines() < 0 {
					hasThis = false
					break
				}
				s, ok := reader.ReadLine()
				if !ok {
					hasThis = false
					break
				}
				thisLine = listLine{s: s}
				if isSibling(s) {
					break
				}
			}
			if thisLine.s == "+" {
				detachedContinuation = len(buffer)
				buffer = append(buffer, listLine{s: "+", marker: true})
			} else if hasText {
				if isSibling(thisLine.s) {
					break
				} else if nt, nm := nestedListType(thisLine.s, false); nt != "" {
					buffer = append(buffer, thisLine)
					withinNestedList = true
					if nt == "dlist" && nm.Group(3) == "" {
						hasText = false
					}
				} else if literalParagraphRx.MatchString(thisLine.s) {
					reader.UnshiftLine(thisLine.s)
					var fn func(string) bool
					if dlist {
						fn = isSibling
					}
					for _, l := range reader.ReadLinesUntil(readOpts{preserveLastLine: true, breakOnBlankLines: true, breakOnListContinuation: true}, fn) {
						buffer = append(buffer, listLine{s: l})
					}
				} else {
					break
				}
			} else {
				if !withinNestedList {
					buffer = buffer[:len(buffer)-1]
				}
				buffer = append(buffer, thisLine)
				hasText = true
			}
		} else if thisLine.marker {
			hasText = true
			buffer = append(buffer, thisLine)
		} else {
			if thisLine.s != "" {
				hasText = true
			}
			if nt, nm := nestedListType(thisLine.s, withinNestedList); nt != "" {
				withinNestedList = true
				if nt == "dlist" && nm.Group(3) == "" {
					hasText = false
				}
			}
			buffer = append(buffer, thisLine)
		}
		hasThis = false
	}
	if hasThis {
		reader.UnshiftLine(thisLine.s)
	}
	if detachedContinuation >= 0 && detachedContinuation < len(buffer) {
		buffer[detachedContinuation] = listLine{s: "", marker: true}
	}
	for len(buffer) > 0 {
		last := buffer[len(buffer)-1]
		if last.marker {
			buffer = buffer[:len(buffer)-1]
			break
		} else if last.s == "" {
			buffer = buffer[:len(buffer)-1]
		} else {
			break
		}
	}
	out := make([]string, len(buffer))
	for i, l := range buffer {
		out[i] = l.s
	}
	return out
}

func resolveListMarker(listType, marker string) string {
	switch listType {
	case "ulist":
		return marker
	case "olist":
		m, _ := resolveOrderedListMarker(marker)
		return m
	}
	return "<1>"
}

func resolveOrderedListMarker(marker string) (string, string) {
	if strings.HasPrefix(marker, ".") {
		return marker, ""
	}
	for _, style := range orderedListStyles {
		if orderedListMarkerRx[style].MatchString(marker) {
			switch style {
			case "arabic":
				return "1.", style
			case "loweralpha":
				return "a.", style
			case "upperalpha":
				return "A.", style
			case "lowerroman":
				return "i)", style
			case "upperroman":
				return "I)", style
			}
		}
	}
	return marker, ""
}

func isSiblingListItem(line, listType, siblingTrait string, siblingRx *rx.Regexp) bool {
	if siblingRx != nil {
		return siblingRx.MatchString(line)
	}
	m := listRxMap[listType].Find(line)
	return m != nil && siblingTrait == resolveListMarker(listType, m.Group(1))
}

// callouts ports Asciidoctor's Callouts catalog.
type callouts struct {
	lists     [][]calloutEntry
	listIndex int
	coIndex   int
}

type calloutEntry struct {
	ordinal int
	id      string
}

func newCallouts() *callouts {
	c := &callouts{}
	c.nextList()
	return c
}

func (c *callouts) register(liOrdinal string) string {
	id := c.generateNextCalloutID()
	c.lists[c.listIndex-1] = append(c.lists[c.listIndex-1], calloutEntry{atoi(liOrdinal), id})
	c.coIndex++
	return id
}

func (c *callouts) readNextID() string {
	id := ""
	list := c.currentList()
	if c.coIndex <= len(list) {
		id = list[c.coIndex-1].id
	}
	c.coIndex++
	return id
}

func (c *callouts) calloutIDs(liOrdinal int) string {
	var ids []string
	for _, e := range c.currentList() {
		if e.ordinal == liOrdinal {
			ids = append(ids, e.id)
		}
	}
	return strings.Join(ids, " ")
}

func (c *callouts) currentList() []calloutEntry {
	if c.listIndex-1 < len(c.lists) {
		return c.lists[c.listIndex-1]
	}
	return nil
}

func (c *callouts) nextList() {
	c.listIndex++
	if len(c.lists) < c.listIndex {
		c.lists = append(c.lists, nil)
	}
	c.coIndex = 1
}

func (c *callouts) rewind() {
	c.listIndex = 1
	c.coIndex = 1
}

func (c *callouts) generateNextCalloutID() string {
	return "CO" + strconv.Itoa(c.listIndex) + "-" + strconv.Itoa(c.coIndex)
}
