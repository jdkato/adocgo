package adocgo

import (
	"strconv"
	"strings"
)

var (
	alBoundaryQuot  = rxScan(`.*?[^\\](?=")`)
	alBoundaryApos  = rxScan(`.*?[^\\](?=')`)
	alBoundaryComma = rxScan(`.*?(?=[ \t]*(,|$))`)
	alNameRx        = rxScan(`\p{Word}[\p{Word}\-.]*`)
	alBlankRx       = rxScan(`[ \t]+`)
	alSkipRx        = rxScan(`[ \t]*(,|$)`)
)

// posKey returns the attribute key used for the positional attribute at the
// given 1-based index.
func posKey(i int) string { return "$" + strconv.Itoa(i) }

// attributeList parses an AsciiDoc attribute list such as
// `quote, Author, role=lead`. It is a port of Asciidoctor's AttributeList.
type attributeList struct {
	src   string
	pos   int
	block *Block
	attrs map[string]string
}

func (a *attributeList) eos() bool { return a.pos >= len(a.src) }

func (a *attributeList) scan(re *scanner) (string, bool) {
	m := re.MatchAt(a.src, a.pos)
	if m == nil {
		return "", false
	}
	a.pos = m.End(0)
	return m.Group(0), true
}

func (a *attributeList) skipBlank() int {
	s, _ := a.scan(alBlankRx)
	return len(s)
}

func (a *attributeList) getByte() (byte, bool) {
	if a.eos() {
		return 0, false
	}
	c := a.src[a.pos]
	a.pos++
	return c, true
}

func (a *attributeList) scanToDelimiter() string {
	s, _ := a.scan(alBoundaryComma)
	return s
}

func parseAttrList(src string, block *Block, posattrs []string) map[string]string {
	a := &attributeList{src: src, block: block, attrs: map[string]string{}}
	index := 0
	for a.parseAttribute(index, posattrs) {
		if a.eos() {
			break
		}
		a.scan(alSkipRx)
		index++
	}
	return a.attrs
}

func (a *attributeList) parseAttribute(index int, posattrs []string) bool {
	cont := true
	a.skipBlank()
	var name, value string
	var hasName, hasValue, singleQuoted bool
	var peek byte
	if !a.eos() {
		peek = a.src[a.pos]
	}
	switch {
	case !a.eos() && peek == '"':
		a.pos++
		name, hasName = a.parseAttributeValue('"'), true
	case !a.eos() && peek == '\'':
		a.pos++
		name, hasName = a.parseAttributeValue('\''), true
		if !strings.HasPrefix(name, "'") {
			singleQuoted = true
		}
	default:
		skipped := 0
		name, hasName = a.scan(alNameRx)
		if hasName {
			skipped = a.skipBlank()
		}
		if a.eos() {
			if !hasName && !strings.HasSuffix(strings.TrimRight(a.src, " \t\n\v\f\r\x00"), ",") {
				return false
			}
			cont = false
		} else if c, _ := a.getByte(); c == ',' {
			a.pos--
		} else if hasName {
			if c == '=' {
				a.skipBlank()
				c2, ok := a.getByte()
				switch {
				case !ok:
					value, hasValue = "", true
				case c2 == '"':
					value, hasValue = a.parseAttributeValue('"'), true
				case c2 == '\'':
					value, hasValue = a.parseAttributeValue('\''), true
					if !strings.HasPrefix(value, "'") {
						singleQuoted = true
					}
				case c2 == ',':
					value, hasValue = "", true
					a.pos--
				default:
					value, hasValue = string([]byte{c2})+a.scanToDelimiter(), true
					if value == "None" {
						return true
					}
				}
			} else {
				name = name + strings.Repeat(" ", skipped) + string([]byte{c}) + a.scanToDelimiter()
			}
		} else {
			name, hasName = string([]byte{c})+a.scanToDelimiter(), true
		}
	}
	if hasValue {
		switch name {
		case "options", "opts":
			if strings.Contains(value, ",") {
				value = strings.ReplaceAll(value, " ", "")
				for _, opt := range strings.Split(value, ",") {
					if opt != "" {
						a.attrs[opt+"-option"] = ""
					}
				}
			} else if value != "" {
				a.attrs[value+"-option"] = ""
			}
		default:
			if singleQuoted && a.block != nil {
				switch name {
				case "title", "reftext":
					a.attrs[name] = value
				default:
					a.attrs[name] = a.block.applySubs(value, normalSubs)
				}
			} else {
				a.attrs[name] = value
			}
		}
	} else {
		if singleQuoted && a.block != nil && hasName {
			name = a.block.applySubs(name, normalSubs)
		}
		if hasName {
			if index < len(posattrs) && posattrs[index] != "" {
				a.attrs[posattrs[index]] = name
			}
			a.attrs[posKey(index+1)] = name
		}
	}
	return cont
}

func (a *attributeList) parseAttributeValue(quote byte) string {
	if !a.eos() && a.src[a.pos] == quote {
		a.pos++
		return ""
	}
	re := alBoundaryQuot
	if quote == '\'' {
		re = alBoundaryApos
	}
	if value, ok := a.scan(re); ok {
		a.pos++
		if strings.Contains(value, `\`) {
			return strings.ReplaceAll(value, `\`+string(quote), string(quote))
		}
		return value
	}
	return string(quote) + a.scanToDelimiter()
}

// rekey copies positional attributes to the given names.
func rekey(attrs map[string]string, posattrs []string) {
	for i, key := range posattrs {
		if key == "" {
			continue
		}
		if v, ok := attrs[posKey(i+1)]; ok {
			attrs[key] = v
		}
	}
}

type parseAttrOpts struct {
	unescapeInput bool
	subInput      bool
	subResult     bool
	into          map[string]string
}

// parseAttributes ports Substitutors#parse_attributes.
func (b *Block) parseAttributes(attrlist string, posattrs []string, o parseAttrOpts) map[string]string {
	if attrlist == "" {
		return map[string]string{}
	}
	if o.unescapeInput {
		attrlist = normalizeText(attrlist, true, true)
	}
	if o.subInput && strings.Contains(attrlist, "{") {
		attrlist = b.Document.subAttributes(attrlist, subAttrOpts{})
	}
	var blk *Block
	if o.subResult {
		blk = b
	}
	res := parseAttrList(attrlist, blk, posattrs)
	if o.into != nil {
		for k, v := range res {
			o.into[k] = v
		}
		return o.into
	}
	return res
}
