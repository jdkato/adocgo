package adocgo

import (
	"strconv"
	"strings"

	"github.com/jdkato/adocgo/internal/rx"
)

// This file is a port of Asciidoctor's substitutors.rb.

var (
	basicSubs    = []string{"specialcharacters"}
	headerSubs   = []string{"specialcharacters", "attributes"}
	normalSubs   = []string{"specialcharacters", "quotes", "attributes", "replacements", "macros", "post_replacements"}
	reftextSubs  = []string{"specialcharacters", "quotes", "replacements"}
	verbatimSubs = []string{"specialcharacters", "callouts"}
)

var subGroups = map[string][]string{
	"none":         {},
	"normal":       normalSubs,
	"verbatim":     verbatimSubs,
	"specialchars": basicSubs,
}

var subHints = map[string]string{
	"a": "attributes", "m": "macros", "n": "normal", "p": "post_replacements",
	"q": "quotes", "r": "replacements", "c": "specialcharacters", "v": "verbatim",
}

var subOptions = map[string]map[string]bool{
	"block":  set("none", "normal", "verbatim", "specialchars", "specialcharacters", "quotes", "attributes", "replacements", "macros", "post_replacements", "callouts"),
	"inline": set("none", "normal", "verbatim", "specialchars", "specialcharacters", "quotes", "attributes", "replacements", "macros", "post_replacements"),
}

// maxPassthroughDepth bounds the recursion when restoring passthroughs.
const maxPassthroughDepth = 32

// maxPassthroughRestores bounds the number of placeholders restored for one
// top-level substitution pass.
const maxPassthroughRestores = 100000

const (
	charCAN   = "\u0018"
	charDEL   = "\u007f"
	passStart = "\u0096"
	passEnd   = "\u0097"
)

func containsSub(subs []string, name string) bool {
	for _, s := range subs {
		if s == name {
			return true
		}
	}
	return false
}

// applySubs applies the given substitutions to text.
func (b *Block) applySubs(text string, subs []string) string {
	if text == "" || len(subs) == 0 {
		return text
	}
	hasPassthrus, clearPassthrus := false, false
	if containsSub(subs, "macros") {
		text = b.extractPassthroughs(text)
		if len(b.passthroughs) > 0 {
			hasPassthrus = true
			if !b.passLocked {
				b.passLocked = true
				clearPassthrus = true
			}
		}
	}
	for _, typ := range subs {
		switch typ {
		case "specialcharacters":
			text = subSpecialchars(text)
		case "quotes":
			text = b.subQuotes(text)
		case "attributes":
			if strings.Contains(text, "{") {
				text = b.subAttributes(text, subAttrOpts{})
			}
		case "replacements":
			text = subReplacements(text)
		case "macros":
			text = b.subMacros(text)
		case "highlight":
			text = b.subSource(text, containsSub(subs, "callouts"))
		case "callouts":
			if !containsSub(subs, "highlight") {
				text = b.subCallouts(text)
			}
		case "post_replacements":
			text = b.subPostReplacements(text)
		}
	}
	if hasPassthrus {
		text = b.restorePassthroughs(text)
		if clearPassthrus {
			b.passthroughs = b.passthroughs[:0]
			b.passLocked = false
		}
	}
	return text
}

var specialCharsReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func subSpecialchars(text string) string {
	if strings.ContainsAny(text, "<&>") {
		return specialCharsReplacer.Replace(text)
	}
	return text
}

var (
	quotedTextSniffRx       = rxScan("[*_`#^~]")
	quotedTextSniffCompatRx = rxScan(`[*'_+#^~]`)
)

func (b *Block) subQuotes(text string) string {
	compat := b.Document.compatMode
	sniff, qs := quotedTextSniffRx, quoteSubs
	if compat {
		sniff, qs = quotedTextSniffCompatRx, quoteSubsCompat
	}
	if sniff.MatchString(text) {
		for _, q := range qs {
			q := q
			text = q.re.ReplaceAllFunc(text, func(m *rmatch) string {
				return b.convertQuotedText(m, q.typ, q.constrained)
			})
		}
	}
	return text
}

type subAttrOpts struct {
	attributeMissing string
}

func (b *Block) subAttributes(text string, opts subAttrOpts) string {
	doc := b.Document
	docAttrs := doc.Attributes
	drop, dropLine, dropEmptyLine := false, false, false
	attributeUndefined, attributeMissing := "", ""
	text = attributeReferenceRx.ReplaceAllFunc(text, func(m *rmatch) string {
		if m.Group(1) == `\` || m.Group(4) == `\` {
			return "{" + m.Group(2) + "}"
		}
		if m.Has(3) {
			args := strings.SplitN(m.Group(2), ":", 3)
			directive := args[0]
			args = args[1:]
			switch directive {
			case "set":
				val := ""
				if len(args) > 1 {
					val = args[1]
				}
				_, _, hasValue := storeAttribute(args[0], val, true, doc, nil)
				if attributeUndefined == "" {
					attributeUndefined = docAttrs["attribute-undefined"]
					if attributeUndefined == "" {
						attributeUndefined = "drop-line"
					}
				}
				if hasValue || attributeUndefined != "drop-line" {
					drop, dropEmptyLine = true, true
					return charDEL
				}
				drop, dropLine = true, true
				return charCAN
			case "counter2":
				if len(args) > 1 {
					doc.counter(args[0], args[1], true)
				} else {
					doc.counter(args[0], "", false)
				}
				drop, dropEmptyLine = true, true
				return charDEL
			default:
				if len(args) > 1 {
					return doc.counter(args[0], args[1], true)
				}
				return doc.counter(args[0], "", false)
			}
		}
		key := strings.ToLower(m.Group(2))
		if v, ok := docAttrs[key]; ok {
			return v
		}
		if v, ok := intrinsicAttributes[key]; ok {
			return v
		}
		if attributeMissing == "" {
			attributeMissing = opts.attributeMissing
			if attributeMissing == "" {
				attributeMissing = docAttrs["attribute-missing"]
			}
			if attributeMissing == "" {
				attributeMissing = "skip"
			}
		}
		switch attributeMissing {
		case "drop":
			drop, dropEmptyLine = true, true
			return charDEL
		case "drop-line":
			drop, dropLine = true, true
			return charCAN
		}
		return m.Group(0)
	})
	if !drop {
		return text
	}
	if dropEmptyLine {
		lines := strings.Split(squeezeChar(text, charDEL[0]), "\n")
		var kept []string
		for _, line := range lines {
			if line == charDEL {
				continue
			}
			if dropLine && strings.Contains(line, charCAN) {
				continue
			}
			kept = append(kept, line)
		}
		return strings.ReplaceAll(strings.Join(kept, "\n"), charDEL, "")
	}
	if strings.Contains(text, "\n") {
		var kept []string
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, charCAN) {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, "\n")
	}
	return ""
}

func squeezeChar(s string, c byte) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == c && i > 0 && s[i-1] == c {
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func subReplacements(text string) string {
	if !replaceableTextRx.MatchString(text) {
		return text
	}
	for _, r := range replacements {
		r := r
		text = r.re.ReplaceAllFunc(text, func(m *rmatch) string {
			captured := m.Group(0)
			if strings.Contains(captured, `\`) {
				return strings.Replace(captured, `\`, "", 1)
			}
			switch r.restore {
			case "none":
				return r.repl
			case "bounding":
				return m.Group(1) + r.repl + m.Group(2)
			}
			return m.Group(1) + r.repl
		})
	}
	return text
}

func (b *Block) inline(n *Inline) string {
	n.Parent = b
	return b.Document.conv.convertInline(n)
}

func (b *Block) subMacros(text string) string {
	foundSquareBracket := strings.Contains(text, "[")
	foundColon := strings.Contains(text, ":")
	foundMacroish := foundSquareBracket && foundColon
	foundMacroishShort := foundMacroish && strings.Contains(text, ":[")
	doc := b.Document
	docAttrs := doc.Attributes
	hasDocAttr := func(n string) bool { _, ok := docAttrs[n]; return ok }

	if hasDocAttr("experimental") {
		if foundMacroishShort && (strings.Contains(text, "kbd:") || strings.Contains(text, "btn:")) {
			text = inlineKbdBtnMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
				if m.Has(1) {
					return m.Group(0)[1:]
				}
				if m.Group(2) == "kbd" {
					keys := strip(m.Group(3))
					if strings.Contains(keys, "]") {
						keys = strings.ReplaceAll(keys, `\]`, "]")
					}
					var keyList []string
					delimIdx := -1
					if len(keys) > 1 {
						ci := indexFrom(keys, ",", 1)
						pi := indexFrom(keys, "+", 1)
						if ci >= 0 {
							delimIdx = ci
							if pi >= 0 && pi < ci {
								delimIdx = pi
							}
						} else {
							delimIdx = pi
						}
					}
					if delimIdx >= 0 {
						delim := keys[delimIdx : delimIdx+1]
						if strings.HasSuffix(keys, delim) {
							for _, k := range strings.Split(keys[:len(keys)-1], delim) {
								keyList = append(keyList, strip(k))
							}
							keyList[len(keyList)-1] += delim
						} else {
							for _, k := range rubySplit(keys, delim) {
								keyList = append(keyList, strip(k))
							}
						}
					} else {
						keyList = []string{keys}
					}
					return b.inline(&Inline{Context: "kbd", Keys: keyList})
				}
				return b.inline(&Inline{Context: "button", Text: normalizeText(m.Group(3), true, true), hasText: true})
			})
		}
		if foundMacroish && strings.Contains(text, "menu:") {
			text = inlineMenuMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
				if strings.HasPrefix(m.Group(0), `\`) {
					return m.Group(0)[1:]
				}
				menu := m.Group(1)
				var submenus []string
				menuitem, hasItem := "", false
				if m.Has(2) {
					items := m.Group(2)
					if strings.Contains(items, "]") {
						items = strings.ReplaceAll(items, `\]`, "]")
					}
					delim := ""
					if strings.Contains(items, "&gt;") {
						delim = "&gt;"
					} else if strings.Contains(items, ",") {
						delim = ","
					}
					if delim != "" {
						for _, it := range rubySplit(items, delim) {
							submenus = append(submenus, strip(it))
						}
						if len(submenus) > 0 {
							menuitem, hasItem = submenus[len(submenus)-1], true
							submenus = submenus[:len(submenus)-1]
						}
					} else {
						menuitem, hasItem = rstrip(items), true
					}
				}
				return b.inline(&Inline{Context: "menu", Attrs: menuAttrs(menu, menuitem, hasItem), Submenus: submenus})
			})
		}
		if strings.Contains(text, `"`) && strings.Contains(text, "&gt;") {
			text = inlineMenuRx.ReplaceAllFunc(text, func(m *rmatch) string {
				if strings.HasPrefix(m.Group(0), `\`) {
					return m.Group(0)[1:]
				}
				var parts []string
				for _, it := range rubySplit(m.Group(1), "&gt;") {
					parts = append(parts, strip(it))
				}
				menu := parts[0]
				submenus := parts[1:]
				menuitem, hasItem := "", false
				if len(submenus) > 0 {
					menuitem, hasItem = submenus[len(submenus)-1], true
					submenus = submenus[:len(submenus)-1]
				}
				return b.inline(&Inline{Context: "menu", Attrs: menuAttrs(menu, menuitem, hasItem), Submenus: submenus})
			})
		}
	}

	if foundMacroish && (strings.Contains(text, "image:") || strings.Contains(text, "icon:")) {
		text = inlineImageMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			var typ string
			var posattrs []string
			if strings.HasPrefix(all, `\`) {
				return all[1:]
			} else if strings.HasPrefix(all, "icon:") {
				typ, posattrs = "icon", []string{"size"}
			} else {
				typ, posattrs = "image", []string{"alt", "width", "height"}
			}
			target := m.Group(1)
			attrs := b.parseAttributes(m.Group(2), posattrs, parseAttrOpts{unescapeInput: true})
			if typ != "icon" {
				doc.registerImage(target)
				if v, ok := docAttrs["imagesdir"]; ok {
					attrs["imagesdir"] = v
				}
			}
			if _, ok := attrs["alt"]; !ok {
				alt := strings.NewReplacer("_", " ", "-", " ").Replace(basenameNoExt(target))
				attrs["default-alt"] = alt
				attrs["alt"] = alt
			}
			return b.inline(&Inline{Context: "image", Type: typ, Target: target, Attrs: attrs})
		})
	}

	if (strings.Contains(text, "((") && strings.Contains(text, "))")) || (foundMacroishShort && strings.Contains(text, "dexterm")) {
		text = inlineIndextermMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			switch m.Group(1) {
			case "indexterm":
				if strings.HasPrefix(all, `\`) {
					return all[1:]
				}
				return b.inline(&Inline{Context: "indexterm"})
			case "indexterm2":
				if strings.HasPrefix(all, `\`) {
					return all[1:]
				}
				term := normalizeText(m.Group(2), true, true)
				if strings.Contains(term, "=") {
					attrs := parseAttrList(term, b, nil)
					if t, ok := attrs["$1"]; ok {
						term = t
					}
				}
				return b.inline(&Inline{Context: "indexterm", Text: term, hasText: true, Type: "visible"})
			}
			enclText := m.Group(3)
			visible := true
			before, after := "", ""
			hasBefore := false
			if strings.HasPrefix(all, `\`) {
				if strings.HasPrefix(enclText, "(") && strings.HasSuffix(enclText, ")") {
					enclText = enclText[1 : len(enclText)-1]
					visible, before, after, hasBefore = true, "(", ")", true
				} else {
					return all[1:]
				}
			} else {
				if strings.HasPrefix(enclText, "(") {
					if strings.HasSuffix(enclText, ")") {
						enclText, visible = enclText[1:len(enclText)-1], false
					} else {
						enclText, before, after, hasBefore = enclText[1:], "(", "", true
					}
				} else if strings.HasSuffix(enclText, ")") {
					enclText, before, after, hasBefore = enclText[:len(enclText)-1], "", ")", true
				}
			}
			var subbed string
			if visible {
				term := normalizeText(enclText, true, false)
				if strings.Contains(term, ";&") {
					if i := strings.Index(term, " &gt;&gt; "); i >= 0 {
						term = term[:i]
					} else if i := strings.Index(term, " &amp;&gt; "); i >= 0 {
						term = term[:i]
					}
				}
				subbed = b.inline(&Inline{Context: "indexterm", Text: term, hasText: true, Type: "visible"})
			} else {
				subbed = b.inline(&Inline{Context: "indexterm"})
			}
			if hasBefore {
				return before + subbed + after
			}
			return subbed
		})
	}

	if foundColon && strings.Contains(text, "://") {
		text = inlineLinkRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			g1, g3 := m.Group(1), m.Group(3)
			if m.Has(2) && !m.Has(5) {
				if strings.HasPrefix(g1, `\`) {
					return all[1:]
				}
				if strings.HasPrefix(g3, `\`) {
					return g1 + all[len(g1)+1:]
				}
				if !m.Has(6) {
					return all
				}
				target := g3 + m.Group(6)
				linkText := target
				if hasDocAttr("hide-uri-scheme") {
					linkText = uriSniffRx.ReplaceFirstFunc(target, func(*rmatch) string { return "" })
				}
				return b.inline(&Inline{Context: "anchor", Text: linkText, hasText: true, Type: "link", Target: target, Attrs: map[string]string{"role": "bare"}})
			}
			if strings.HasPrefix(g3, `\`) {
				return g1 + all[len(g1)+1:]
			}
			prefix, suffix := g1, ""
			var target string
			if m.Has(4) {
				target = g3 + m.Group(4)
			} else {
				target = g3 + m.Group(7)
			}
			linkText, hasLinkText := "", false
			if m.Has(5) {
				if prefix == "link:" {
					prefix = ""
				}
				linkText = m.Group(5)
				hasLinkText = linkText != ""
			} else {
				switch prefix {
				case "link:", `"`, "'":
					return all
				}
				switch m.Group(8) {
				case ";":
					target = chop(target)
					if strings.HasSuffix(target, ")") {
						target = chop(target)
						suffix = ");"
					} else {
						suffix = ";"
					}
					if target == g3 {
						return all
					}
				case ":":
					target = chop(target)
					if strings.HasSuffix(target, ")") {
						target = chop(target)
						suffix = "):"
					} else {
						suffix = ":"
					}
					if target == g3 {
						return all
					}
				}
			}
			var attrs map[string]string
			id := ""
			bare := false
			if hasLinkText {
				newLinkText, hasNew := "", false
				if strings.Contains(linkText, "]") {
					linkText = strings.ReplaceAll(linkText, `\]`, "]")
					newLinkText, hasNew = linkText, true
				}
				if !doc.compatMode && strings.Contains(linkText, "=") {
					linkText, attrs = b.extractAttributesFromText(linkText, "")
					newLinkText, hasNew = linkText, true
					id = attrs["id"]
				}
				if strings.HasSuffix(linkText, "^") {
					linkText = chop(linkText)
					newLinkText, hasNew = linkText, true
					if attrs != nil {
						if _, ok := attrs["window"]; !ok {
							attrs["window"] = "_blank"
						}
					} else {
						attrs = map[string]string{"window": "_blank"}
					}
				}
				if hasNew && newLinkText == "" {
					linkText = target
					if hasDocAttr("hide-uri-scheme") {
						linkText = uriSniffRx.ReplaceFirstFunc(target, func(*rmatch) string { return "" })
					}
					bare = true
				}
			} else {
				linkText = target
				if hasDocAttr("hide-uri-scheme") {
					linkText = uriSniffRx.ReplaceFirstFunc(target, func(*rmatch) string { return "" })
				}
				bare = true
			}
			if bare {
				if attrs != nil {
					if r, ok := attrs["role"]; ok {
						attrs["role"] = "bare " + r
					} else {
						attrs["role"] = "bare"
					}
				} else {
					attrs = map[string]string{"role": "bare"}
				}
			}
			return prefix + b.inline(&Inline{Context: "anchor", Text: linkText, hasText: true, Type: "link", Target: target, ID: id, Attrs: attrs}) + suffix
		})
	}

	if foundMacroish && (strings.Contains(text, "link:") || strings.Contains(text, "ilto:")) {
		text = inlineLinkMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			if strings.HasPrefix(all, `\`) {
				return all[1:]
			}
			mailto := m.Has(1)
			target := m.Group(2)
			mailtoText := ""
			if mailto {
				mailtoText = target
				target = "mailto:" + target
			}
			var attrs map[string]string
			id := ""
			linkText := m.Group(3)
			if linkText != "" {
				if strings.Contains(linkText, "]") {
					linkText = strings.ReplaceAll(linkText, `\]`, "]")
				}
				if mailto {
					if !doc.compatMode && strings.Contains(linkText, ",") {
						linkText, attrs = b.extractAttributesFromText(linkText, "")
						id = attrs["id"]
						if a2, ok := attrs["$2"]; ok {
							if a3, ok := attrs["$3"]; ok {
								target = target + "?subject=" + encodeURIComponent(a2) + "&amp;body=" + encodeURIComponent(a3)
							} else {
								target = target + "?subject=" + encodeURIComponent(a2)
							}
						}
					}
				} else if !doc.compatMode && strings.Contains(linkText, "=") {
					linkText, attrs = b.extractAttributesFromText(linkText, "")
					id = attrs["id"]
				}
				if strings.HasSuffix(linkText, "^") {
					linkText = chop(linkText)
					if attrs != nil {
						if _, ok := attrs["window"]; !ok {
							attrs["window"] = "_blank"
						}
					} else {
						attrs = map[string]string{"window": "_blank"}
					}
				}
			}
			if linkText == "" {
				if mailto {
					linkText = mailtoText
				} else {
					if hasDocAttr("hide-uri-scheme") {
						linkText = uriSniffRx.ReplaceFirstFunc(target, func(*rmatch) string { return "" })
						if linkText == "" {
							linkText = target
						}
					} else {
						linkText = target
					}
					if attrs != nil {
						if r, ok := attrs["role"]; ok {
							attrs["role"] = "bare " + r
						} else {
							attrs["role"] = "bare"
						}
					} else {
						attrs = map[string]string{"role": "bare"}
					}
				}
			}
			return b.inline(&Inline{Context: "anchor", Text: linkText, hasText: true, Type: "link", Target: target, ID: id, Attrs: attrs})
		})
	}

	if strings.Contains(text, "@") {
		text = inlineEmailRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			if m.Has(1) {
				if m.Group(1) == `\` {
					return all[1:]
				}
				return all
			}
			return b.inline(&Inline{Context: "anchor", Text: all, hasText: true, Type: "link", Target: "mailto:" + all})
		})
	}

	if foundSquareBracket && b.Context == "list_item" && b.Parent != nil && b.Parent.Style == "bibliography" {
		text = inlineBiblioAnchorRx.ReplaceFirstFunc(text, func(m *rmatch) string {
			return b.inline(&Inline{Context: "anchor", Text: m.Group(2), hasText: m.Has(2), Type: "bibref", ID: m.Group(1)})
		})
	}

	if (foundSquareBracket && strings.Contains(text, "[[")) || (foundMacroish && strings.Contains(text, "or:")) {
		text = inlineAnchorRx.ReplaceAllFunc(text, func(m *rmatch) string {
			if m.Has(1) {
				return m.Group(0)[1:]
			}
			var id, reftext string
			hasReftext := false
			if m.Has(2) {
				id = m.Group(2)
				reftext, hasReftext = m.Group(3), m.Has(3)
			} else {
				id = m.Group(4)
				reftext, hasReftext = m.Group(5), m.Has(5)
				if hasReftext && strings.Contains(reftext, "]") {
					reftext = strings.ReplaceAll(reftext, `\]`, "]")
				}
			}
			return b.inline(&Inline{Context: "anchor", Text: reftext, hasText: hasReftext, Type: "ref", ID: id})
		})
	}

	if (strings.Contains(text, "&") && strings.Contains(text, ";&l")) || (foundMacroish && strings.Contains(text, "xref:")) {
		text = inlineXrefMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			if strings.HasPrefix(all, `\`) {
				return all[1:]
			}
			attrs := map[string]string{}
			var refid, linkText string
			hasLinkText := false
			macro := false
			if m.Has(1) {
				refid = m.Group(1)
				if i := strings.Index(refid, ","); i >= 0 {
					linkText = lstrip(refid[i+1:])
					refid = refid[:i]
					hasLinkText = linkText != ""
				}
			} else {
				macro = true
				refid = m.Group(2)
				if m.Has(3) {
					linkText, hasLinkText = m.Group(3), true
					if strings.Contains(linkText, "]") {
						linkText = strings.ReplaceAll(linkText, `\]`, "]")
					}
					if !doc.compatMode && strings.Contains(linkText, "=") {
						var t string
						var ok bool
						t, attrs, ok = b.extractAttributesFromTextOpt(linkText)
						linkText, hasLinkText = t, ok
					}
				}
			}
			var path, fragment, target, src2src string
			hasPath, hasFragment, hasTarget, hasSrc2src := false, false, false, false
			hashIdx := strings.Index(refid, "#")
			prevIsAmp := false
			if hashIdx > 0 {
				prevIsAmp = refid[hashIdx-1] == '&'
			} else if hashIdx == 0 {
				prevIsAmp = refid[len(refid)-1] == '&'
			}
			if doc.compatMode {
				fragment, hasFragment = refid, true
			} else if hashIdx >= 0 && !prevIsAmp {
				if hashIdx > 0 {
					if fragLen := len(refid) - 1 - hashIdx; fragLen > 0 {
						path, hasPath = refid[:hashIdx], true
						fragment, hasFragment = refid[hashIdx+1:], true
					} else {
						path, hasPath = refid[:len(refid)-1], true
					}
					if macro {
						if strings.HasSuffix(path, ".adoc") {
							path = path[:len(path)-5]
							src2src, hasSrc2src = path, true
						} else if !hasExtname(path) {
							src2src, hasSrc2src = path, true
						}
					} else if hasAsciidocExt(path) {
						path = path[:strings.LastIndex(path, ".")]
						src2src, hasSrc2src = path, true
					} else {
						src2src, hasSrc2src = path, true
					}
				} else {
					target, hasTarget = refid, true
					fragment, hasFragment = refid[1:], true
				}
			} else if macro {
				if strings.HasSuffix(refid, ".adoc") {
					path, hasPath = refid[:len(refid)-5], true
					src2src, hasSrc2src = path, true
				} else if hasExtname(refid) {
					path, hasPath = refid, true
				} else {
					fragment, hasFragment = refid, true
				}
			} else {
				fragment, hasFragment = refid, true
			}
			hasRefid := true
			if hasTarget {
				refid = fragment
			} else if hasPath {
				if hasSrc2src && (docAttrs["docname"] == path || doc.includes[path]) {
					if hasFragment {
						refid, hasPath, target = fragment, false, "#"+fragment
					} else {
						hasRefid, hasPath, target = false, false, "#"
						refid = ""
					}
					path = ""
				} else {
					refid = path
					suffix := ""
					if hasSrc2src {
						if v, ok := docAttrs["relfilesuffix"]; ok {
							suffix = v
						} else {
							suffix = doc.outfilesuffix
						}
					}
					path = docAttrs["relfileprefix"] + path + suffix
					if hasFragment {
						refid, target = refid+"#"+fragment, path+"#"+fragment
					} else {
						target = path
					}
				}
			} else if doc.compatMode {
				refid, target = fragment, "#"+fragment
			} else if _, ok := doc.refs[fragment]; ok {
				refid, target = fragment, "#"+fragment
			} else if strings.Contains(fragment, " ") || strings.ToLower(fragment) != fragment {
				if rid, ok := doc.resolveID(fragment); ok {
					refid = rid
					fragment, target = rid, "#"+rid
				} else {
					refid, target = fragment, "#"+fragment
				}
			} else {
				refid, target = fragment, "#"+fragment
			}
			_ = src2src
			if hasPath {
				attrs["path"] = path
			}
			if hasFragment {
				attrs["fragment"] = fragment
			}
			if hasRefid {
				attrs["refid"] = refid
			}
			return b.inline(&Inline{Context: "anchor", Text: linkText, hasText: hasLinkText, Type: "xref", Target: target, Attrs: attrs})
		})
	}

	if foundMacroish && strings.Contains(text, "tnote") {
		text = inlineFootnoteMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			all := m.Group(0)
			if strings.HasPrefix(all, `\`) {
				return all[1:]
			}
			var id, content string
			hasID, hasContent := false, false
			if m.Has(1) {
				if m.Has(3) {
					parts := strings.SplitN(m.Group(3), ",", 2)
					id, hasID = parts[0], true
					if len(parts) > 1 {
						content, hasContent = parts[1], true
					}
				} else {
					return all
				}
			} else {
				id, hasID = m.Group(2), m.Has(2)
				content, hasContent = m.Group(3), m.Has(3)
			}
			var index, typ, target string
			if hasID {
				var found *footnote
				for i := range *doc.footnotes {
					if (*doc.footnotes)[i].id == id {
						found = &(*doc.footnotes)[i]
						break
					}
				}
				if found != nil {
					index, content = found.index, found.text
					typ, target, id = "xref", id, ""
				} else if hasContent {
					content = b.restorePassthroughs(normalizeText(content, true, true))
					index = doc.counter("footnote-number", "", false)
					*doc.footnotes = append(*doc.footnotes, footnote{index, id, content})
					typ = "ref"
				} else {
					typ, target, content, id = "xref", id, id, ""
				}
			} else if hasContent {
				content = b.restorePassthroughs(normalizeText(content, true, true))
				index = doc.counter("footnote-number", "", false)
				*doc.footnotes = append(*doc.footnotes, footnote{index, "", content})
			} else {
				return all
			}
			return b.inline(&Inline{Context: "footnote", Text: content, hasText: true, ID: id, Target: target, Type: typ, Attrs: map[string]string{"index": index}})
		})
	}
	return text
}

func hasAsciidocExt(path string) bool {
	for _, ext := range asciidocExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func menuAttrs(menu, menuitem string, hasItem bool) map[string]string {
	a := map[string]string{"menu": menu}
	if hasItem {
		a["menuitem"] = menuitem
	}
	return a
}

func indexFrom(s, sub string, from int) int {
	if from > len(s) {
		return -1
	}
	i := strings.Index(s[from:], sub)
	if i < 0 {
		return -1
	}
	return i + from
}

func (b *Block) subPostReplacements(text string) string {
	_, a := b.Attributes["hardbreaks-option"]
	_, d := b.Document.Attributes["hardbreaks-option"]
	if a || d {
		lines := strings.Split(text, "\n")
		if len(lines) < 2 {
			return text
		}
		last := lines[len(lines)-1]
		lines = lines[:len(lines)-1]
		for i, line := range lines {
			if strings.HasSuffix(line, " +") {
				line = line[:len(line)-2]
			}
			lines[i] = b.inline(&Inline{Context: "break", Text: line, hasText: true, Type: "line"})
		}
		return strings.Join(append(lines, last), "\n")
	} else if strings.Contains(text, "+") && strings.Contains(text, " +") {
		return hardLineBreakRx.ReplaceAllFunc(text, func(m *rmatch) string {
			return b.inline(&Inline{Context: "break", Text: m.Group(1), hasText: true, Type: "line"})
		})
	}
	return text
}

func (b *Block) subSource(source string, processCallouts bool) string {
	if processCallouts {
		return b.subCallouts(subSpecialchars(source))
	}
	return subSpecialchars(source)
}

func (b *Block) subCallouts(text string) string {
	re := calloutSourceRx
	if lc, ok := b.Attributes["line-comment"]; ok {
		pre := ""
		if lc != "" {
			pre = rx.Quote(lc) + " ?"
		}
		re = rxScan(`(` + pre + `)?(\\)?&lt;()(\d+|\.)&gt;(?=(?: ?\\?&lt;(?:\d+|\.)&gt;)*$)`)
	}
	autonum := 0
	return re.ReplaceAllFunc(text, func(m *rmatch) string {
		if m.Has(2) {
			return strings.Replace(m.Group(0), `\`, "", 1)
		}
		num := m.Group(4)
		if num == "." {
			autonum++
			num = strconv.Itoa(autonum)
		}
		var guard []string
		if m.Has(1) {
			guard = []string{m.Group(1)}
		} else if m.Group(3) == "--" {
			guard = []string{"<!--", "-->"}
		}
		return b.inline(&Inline{Context: "callout", Text: num, hasText: true, ID: b.Document.callouts.readNextID(), Guard: guard})
	})
}

func (b *Block) extractPassthroughs(text string) string {
	compat := b.Document.compatMode
	if strings.Contains(text, "++") || strings.Contains(text, "$$") || strings.Contains(text, "ss:") {
		text = inlinePassMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			preceding := ""
			var p passthru
			if m.Has(4) {
				boundary := m.Group(4)
				if compat && boundary == "++" {
					if m.Has(2) {
						return m.Group(1) + "[" + m.Group(2) + "]" + m.Group(3) + "++" + b.extractPassthroughs(m.Group(5)) + "++"
					}
					return m.Group(1) + m.Group(3) + "++" + b.extractPassthroughs(m.Group(5)) + "++"
				}
				var attributes map[string]string
				hasAttributes, oldBehavior := false, false
				escapeCount := len(m.Group(3))
				if m.Has(2) {
					attrlist := m.Group(2)
					if escapeCount > 0 {
						return m.Group(1) + "[" + attrlist + "]" + strings.Repeat(`\`, escapeCount-1) + boundary + m.Group(5) + boundary
					} else if m.Group(1) == `\` {
						preceding = "[" + attrlist + "]"
					} else if boundary == "++" {
						if attrlist == "x-" {
							oldBehavior = true
							attributes, hasAttributes = map[string]string{}, true
						} else if strings.HasSuffix(attrlist, " x-") {
							oldBehavior = true
							attributes, hasAttributes = b.parseQuotedTextAttributes(attrlist[:len(attrlist)-3]), true
						} else {
							attributes, hasAttributes = b.parseQuotedTextAttributes(attrlist), true
						}
					} else {
						attributes, hasAttributes = b.parseQuotedTextAttributes(attrlist), true
					}
				} else if escapeCount > 0 {
					return strings.Repeat(`\`, escapeCount-1) + boundary + m.Group(5) + boundary
				}
				subs := basicSubs
				if boundary == "+++" {
					subs = nil
				}
				if hasAttributes {
					if oldBehavior {
						p = passthru{text: m.Group(5), subs: normalSubs, typ: "monospaced", attrs: attributes, hasAttr: true}
					} else {
						p = passthru{text: m.Group(5), subs: subs, typ: "unquoted", attrs: attributes, hasAttr: true}
					}
				} else {
					p = passthru{text: m.Group(5), subs: subs}
				}
			} else {
				if m.Group(6) == `\` {
					return m.Group(0)[1:]
				}
				if m.Has(7) {
					p = passthru{text: normalizeText(m.Group(8), false, true), subs: resolveSubs(m.Group(7), "inline", nil)}
				} else {
					p = passthru{text: normalizeText(m.Group(8), false, true)}
				}
			}
			key := len(b.passthroughs)
			b.passthroughs = append(b.passthroughs, p)
			return preceding + passStart + strconv.Itoa(key) + passEnd
		})
	}
	passRx := inlinePassRx
	trigger := strings.Contains(text, "+") || strings.Contains(text, "-]")
	if compat {
		passRx = inlinePassCompatRx
		trigger = strings.Contains(text, "`")
	}
	if trigger {
		text = passRx.ReplaceAllFunc(text, func(m *rmatch) string {
			preceding := m.Group(1)
			attrlist, hasAttrlist := "", false
			if m.Has(4) {
				attrlist, hasAttrlist = m.Group(4), true
			} else if m.Has(3) {
				attrlist, hasAttrlist = m.Group(3), true
			}
			if compat && hasAttrlist && attrlist == "" {
				hasAttrlist = false
			}
			escaped := m.Has(5)
			quotedText := m.Group(6)
			formatMark := m.Group(7)
			content := m.Group(8)
			oldBehavior, oldBehaviorForced := false, false
			if compat {
				oldBehavior = true
			} else if hasAttrlist && (attrlist == "x-" || strings.HasSuffix(attrlist, " x-")) {
				oldBehavior, oldBehaviorForced = true, true
			}
			var attributes map[string]string
			hasAttributes := false
			if hasAttrlist {
				if escaped {
					return preceding + "[" + attrlist + "]" + quotedText[1:]
				} else if preceding == `\` {
					if oldBehaviorForced && formatMark == "`" {
						return preceding + "[" + attrlist + "]" + quotedText
					}
					preceding = "[" + attrlist + "]"
				} else if oldBehaviorForced {
					if attrlist == "x-" {
						attributes = map[string]string{}
					} else {
						attributes = b.parseQuotedTextAttributes(attrlist[:len(attrlist)-3])
					}
					hasAttributes = true
				} else {
					attributes, hasAttributes = b.parseQuotedTextAttributes(attrlist), true
				}
			} else if escaped {
				return preceding + quotedText[1:]
			} else if compat && preceding == `\` {
				return quotedText
			}
			var p passthru
			if compat {
				p = passthru{text: content, subs: basicSubs, attrs: attributes, hasAttr: hasAttributes, typ: "monospaced"}
			} else if hasAttributes {
				if oldBehavior {
					subs := normalSubs
					if formatMark == "`" {
						subs = basicSubs
					}
					p = passthru{text: content, subs: subs, attrs: attributes, hasAttr: true, typ: "monospaced"}
				} else {
					p = passthru{text: content, subs: basicSubs, attrs: attributes, hasAttr: true, typ: "unquoted"}
				}
			} else {
				p = passthru{text: content, subs: basicSubs}
			}
			key := len(b.passthroughs)
			b.passthroughs = append(b.passthroughs, p)
			return preceding + passStart + strconv.Itoa(key) + passEnd
		})
	}
	if strings.Contains(text, ":") && (strings.Contains(text, "stem:") || strings.Contains(text, "math:")) {
		text = inlineStemMacroRx.ReplaceAllFunc(text, func(m *rmatch) string {
			if strings.HasPrefix(m.Group(0), `\`) {
				return m.Group(0)[1:]
			}
			typ := m.Group(1)
			if typ == "stem" {
				typ = stemTypeAlias(b.Document.Attributes["stem"])
			}
			content := normalizeText(m.Group(3), false, true)
			if typ == "latexmath" && strings.HasPrefix(content, "$") && strings.HasSuffix(content, "$") && len(content) >= 2 {
				content = content[1 : len(content)-1]
			}
			subs := basicSubs
			if m.Has(2) {
				subs = resolveSubs(m.Group(2), "inline", nil)
			}
			key := len(b.passthroughs)
			b.passthroughs = append(b.passthroughs, passthru{text: content, subs: subs, typ: typ})
			return passStart + strconv.Itoa(key) + passEnd
		})
	}
	return text
}

func (b *Block) restorePassthroughs(text string) string {
	// A passthrough whose text contains a placeholder (only possible if the
	// input contains the raw placeholder characters) can recurse forever or
	// expand exponentially; Asciidoctor dies with "stack level too deep" in
	// the cyclic case. Bound both the depth and the total work.
	if b.passDepth == 0 {
		b.passBudget = maxPassthroughRestores
	}
	if b.passDepth >= maxPassthroughDepth {
		return text
	}
	b.passDepth++
	defer func() { b.passDepth-- }()
	return passSlotRx.ReplaceAllFunc(text, func(m *rmatch) string {
		idx := atoi(m.Group(1))
		if len(m.Group(1)) > 9 || idx >= len(b.passthroughs) {
			return "??pass??"
		}
		if b.passBudget <= 0 {
			return m.Group(0)
		}
		b.passBudget--
		pass := b.passthroughs[idx]
		subbed := b.applySubs(pass.text, pass.subs)
		if pass.typ != "" {
			id := ""
			if pass.hasAttr {
				id = pass.attrs["id"]
			}
			subbed = b.inline(&Inline{Context: "quoted", Text: subbed, hasText: true, Type: pass.typ, ID: id, Attrs: pass.attrs})
		}
		if strings.Contains(subbed, passStart) {
			return b.restorePassthroughs(subbed)
		}
		return subbed
	})
}

// resolveSubs ports Substitutors#resolve_subs. A nil result means "no result"
// (Ruby nil) when hasResult is false.
func resolveSubs(subs, typ string, defaults []string) []string {
	r, _ := resolveSubsOpt(subs, typ, defaults)
	return r
}

func resolveSubsOpt(subs, typ string, defaults []string) ([]string, bool) {
	if subs == "" {
		return nil, false
	}
	var candidates []string
	hasCandidates := false
	subs = strings.ReplaceAll(subs, " ", "")
	modifiersPresent := strings.ContainsAny(subs, "+-")
	for _, key := range rubySplit(subs, ",") {
		op := ""
		if modifiersPresent && key != "" {
			if key[0] == '+' {
				op, key = "append", key[1:]
			} else if key[0] == '-' {
				op, key = "remove", key[1:]
			} else if strings.HasSuffix(key, "+") {
				op, key = "prepend", key[:len(key)-1]
			}
		}
		var resolved []string
		if typ == "inline" && (key == "verbatim" || key == "v") {
			resolved = basicSubs
		} else if g, ok := subGroups[key]; ok {
			resolved = g
		} else if h, ok := subHints[key]; ok && typ == "inline" && len(key) == 1 {
			if g, ok := subGroups[h]; ok {
				resolved = g
			} else {
				resolved = []string{h}
			}
		} else {
			resolved = []string{key}
		}
		if op != "" {
			if !hasCandidates {
				candidates = append([]string(nil), defaults...)
				hasCandidates = true
			}
			switch op {
			case "append":
				candidates = append(candidates, resolved...)
			case "prepend":
				candidates = append(append([]string(nil), resolved...), candidates...)
			case "remove":
				var out []string
				for _, c := range candidates {
					if !containsSub(resolved, c) {
						out = append(out, c)
					}
				}
				candidates = out
			}
		} else {
			hasCandidates = true
			candidates = append(candidates, resolved...)
		}
	}
	if !hasCandidates {
		return nil, false
	}
	// candidates & SUB_OPTIONS[type] (set intersection preserving order, unique)
	var out []string
	for _, c := range candidates {
		if subOptions[typ][c] && !containsSub(out, c) {
			out = append(out, c)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out, true
}

func (b *Block) commitSubs() {
	defaultSubs := b.defaultSubs
	if !b.hasDefSubs {
		switch b.ContentModel {
		case "simple":
			defaultSubs = normalSubs
		case "verbatim":
			if b.Context == "verse" {
				defaultSubs = normalSubs
			} else {
				defaultSubs = verbatimSubs
			}
		case "raw":
			if b.Context == "stem" {
				defaultSubs = basicSubs
			} else {
				defaultSubs = nil
			}
		default:
			return
		}
	}
	if custom, ok := b.Attributes["subs"]; ok {
		// an empty (or unresolvable) subs attribute disables all substitutions
		b.subs, _ = resolveSubsOpt(custom, "block", defaultSubs)
	} else {
		b.subs = append([]string(nil), defaultSubs...)
	}
}

func (b *Block) extractAttributesFromText(text, defaultText string) (string, map[string]string) {
	t, attrs, ok := b.extractAttributesFromTextOpt(text)
	if !ok {
		return defaultText, attrs
	}
	return t, attrs
}

func (b *Block) extractAttributesFromTextOpt(text string) (string, map[string]string, bool) {
	attrlist := strings.ReplaceAll(text, "\n", " ")
	attrs := parseAttrList(attrlist, b, nil)
	if resolved, ok := attrs["$1"]; ok {
		if resolved == attrlist {
			return text, map[string]string{}, true
		}
		return resolved, attrs, true
	}
	return "", attrs, false
}

func (b *Block) convertQuotedText(m *rmatch, typ string, constrained bool) string {
	unescapedAttrs := ""
	if strings.HasPrefix(m.Group(0), `\`) {
		if constrained && m.Has(2) {
			unescapedAttrs = "[" + m.Group(2) + "]"
		} else {
			return m.Group(0)[1:]
		}
	}
	if constrained {
		if unescapedAttrs != "" {
			return unescapedAttrs + b.inline(&Inline{Context: "quoted", Text: m.Group(3), hasText: true, Type: typ})
		}
		var attributes map[string]string
		id := ""
		if m.Has(2) {
			attributes = b.parseQuotedTextAttributes(m.Group(2))
			id = attributes["id"]
			if typ == "mark" {
				typ = "unquoted"
			}
		}
		return m.Group(1) + b.inline(&Inline{Context: "quoted", Text: m.Group(3), hasText: true, Type: typ, ID: id, Attrs: attributes})
	}
	var attributes map[string]string
	id := ""
	if m.Has(1) {
		attributes = b.parseQuotedTextAttributes(m.Group(1))
		id = attributes["id"]
		if typ == "mark" {
			typ = "unquoted"
		}
	}
	return b.inline(&Inline{Context: "quoted", Text: m.Group(2), hasText: true, Type: typ, ID: id, Attrs: attributes})
}

func (b *Block) parseQuotedTextAttributes(str string) map[string]string {
	if strings.Contains(str, "{") {
		str = b.subAttributes(str, subAttrOpts{})
	}
	if i := strings.Index(str, ","); i >= 0 {
		str = str[:i]
	}
	str = strip(str)
	attrs := map[string]string{}
	if str == "" {
		return attrs
	}
	if strings.HasPrefix(str, ".") || strings.HasPrefix(str, "#") {
		before, after := str, ""
		if i := strings.Index(str, "#"); i >= 0 {
			before, after = str[:i], str[i+1:]
		}
		dotRole := func(s string) string { return lstrip(strings.ReplaceAll(s, ".", " ")) }
		if after == "" {
			if len(before) > 1 {
				attrs["role"] = dotRole(before)
			}
		} else {
			id, roles := after, ""
			if i := strings.Index(after, "."); i >= 0 {
				id, roles = after[:i], after[i+1:]
			}
			if id != "" {
				attrs["id"] = id
			}
			if roles == "" {
				if len(before) > 1 {
					attrs["role"] = dotRole(before)
				}
			} else if len(before) > 1 {
				attrs["role"] = dotRole(before + "." + roles)
			} else {
				attrs["role"] = strings.ReplaceAll(roles, ".", " ")
			}
		}
		return attrs
	}
	attrs["role"] = str
	return attrs
}

func splitSimpleCSV(str string) []string {
	if str == "" {
		return nil
	}
	var values []string
	if strings.Contains(str, `"`) {
		var accum strings.Builder
		quoteOpen := false
		for _, c := range str {
			switch c {
			case ',':
				if quoteOpen {
					accum.WriteRune(c)
				} else {
					values = append(values, strip(accum.String()))
					accum.Reset()
				}
			case '"':
				quoteOpen = !quoteOpen
			default:
				accum.WriteRune(c)
			}
		}
		values = append(values, strip(accum.String()))
		return values
	}
	for _, it := range rubySplit(str, ",") {
		values = append(values, strip(it))
	}
	return values
}
