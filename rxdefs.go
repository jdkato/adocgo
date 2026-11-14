package adocgo

import "github.com/jdkato/adocgo/internal/rx"

// Regular expressions ported verbatim from Asciidoctor's rx.rb. In the Ruby
// source CC_ANY and CC_ALL are both "." (the latter combined with the m flag),
// CC_WORD/CG_WORD are \p{Word}, CC_ALPHA is \p{Alpha}, CC_ALNUM is \p{Alnum}
// and CG_BLANK is \p{Blank}.
var (
	authorInfoLineRx            = rx.MustCompile(`^(\p{Word}[\p{Word}\-'.]*)(?: +(\p{Word}[\p{Word}\-'.]*))?(?: +(\p{Word}[\p{Word}\-'.]*))?(?: +<([^>]+)>)?$`)
	authorDelimiterRx           = rx.MustCompile(`;(?: |$)`)
	revisionInfoLineRx          = rx.MustCompile(`^(?:[^\d{]*(.*?),)? *(?!:)(.*?)(?: *(?!^),?: *(.*))?$`)
	manpageTitleVolnumRx        = rx.MustCompile(`^(.+?) *\( *(.+?) *\)$`)
	manpageNamePurposeRx        = rx.MustCompile(`^(.+?) +- +(.+)$`)
	conditionalDirectiveRx      = rx.MustCompile(`^(\\)?(ifdef|ifndef|ifeval|endif)::(\S*?(?:([,+])\S*?)?)\[(.+)?\]$`)
	evalExpressionRx            = rx.MustCompile(`^(.+?) *([=!><]=|[><]) *(.+)$`)
	includeDirectiveRx          = rx.MustCompile(`^(\\)?include::([^\s\[](?:[^\[]*[^\s\[])?)\[(.+)?\]$`)
	tagDirectiveRx              = rx.MustCompile(`\b(?:tag|(e)nd)::(\S+?)\[\](?=$|[ \r])`, "m")
	attributeEntryRx            = rx.MustCompile(`^:(!?\p{Word}[^:]*):(?:[ \t]+(.*))?$`)
	invalidAttributeNameCharsRx = rx.MustCompile(`[^\p{Word}-]`)
	attributeEntryPassMacroRx   = rx.MustCompile(`\Apass:([a-z]+(?:,[a-z-]+)*)?\[(.*)\]\Z`, "m")
	attributeReferenceRx        = rx.MustCompile(`(\\)?\{(\p{Word}[\p{Word}-]*|(set|counter2?):.+?)(\\)?\}`)
	blockAnchorRx               = rx.MustCompile(`^\[\[(?:|([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+))?)\]\]$`)
	blockAttributeListRx        = rx.MustCompile(`^\[(|[\p{Word}.#%{,"'].*)\]$`)
	blockAttributeLineRx        = rx.MustCompile(`^\[(?:|[\p{Word}.#%{,"'].*|\[(?:|[\p{Alpha}_:][\p{Word}\-:.]*(?:, *.+)?)\])\]$`)
	blockTitleRx                = rx.MustCompile(`^\.(\.?[^ \t.].*)$`)
	admonitionParagraphRx       = rx.MustCompile(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION):[ \t]+`)
	literalParagraphRx          = rx.MustCompile(`^([ \t]+.*)$`)
	extAtxSectionTitleRx        = rx.MustCompile(`^(=={0,5}|##{0,5})[ \t]+(.+?)(?:[ \t]+\1)?$`)
	setextSectionTitleRx        = rx.MustCompile(`^((?!\.).*?\p{Alnum}.*)$`)
	inlineSectionAnchorRx       = rx.MustCompile(` (\\)?\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+))?\]\]$`)
	invalidSectionIDCharsRx     = rx.MustCompile(`<[^>]+>|&(?:[a-z][a-z]+\d{0,2}|#\d\d\d{0,4}|#x[\da-f][\da-f][\da-f]{0,3});|[^ \p{Word}\-.]+?`)
	sectionLevelStyleRx         = rx.MustCompile(`^sect\d$`)
	anyListRx                   = rx.MustCompile(`^(?:[ \t]*(?:-|\*\**|\.\.*|•|\d+\.|[a-zA-Z]\.|[IVXivx]+\))[ \t]|(?!//[^/])[ \t]*[^ \t].*?(?::::{0,2}|;;)(?:$|[ \t])|<(?:\d+|\.)>[ \t])`)
	unorderedListRx             = rx.MustCompile(`^[ \t]*(-|\*\**|•)[ \t]+(.*)$`)
	orderedListRx               = rx.MustCompile(`^[ \t]*(\.\.*|\d+\.|[a-zA-Z]\.|[IVXivx]+\))[ \t]+(.*)$`)
	descriptionListRx           = rx.MustCompile(`^(?!//[^/])[ \t]*([^ \t].*?)(:::{0,2}|;;)(?:$|[ \t]+(.*)$)`)
	calloutListRx               = rx.MustCompile(`^<(\d+|\.)>[ \t]+(.*)$`)
	calloutExtractRx            = rx.MustCompile(`((?://|#|--|;;) ?)?(\\)?<!?(|--)(\d+|\.)\3>(?=(?: ?\\?<!?\3(?:\d+|\.)\3>)*$)`)
	calloutScanRx               = rx.MustCompile(`\\?<!?(|--)(\d+|\.)\1>(?=(?: ?\\?<!?\1(?:\d+|\.)\1>)*$)`)
	calloutSourceRx             = rx.MustCompile(`((?://|#|--|;;) ?)?(\\)?&lt;!?(|--)(\d+|\.)\3&gt;(?=(?: ?\\?&lt;!?\3(?:\d+|\.)\3&gt;)*$)`)
	columnSpecRx                = rx.MustCompile(`^(?:(\d+)\*)?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?(\d+%?|~)?([a-z])?$`)
	cellSpecStartRx             = rx.MustCompile(`^[ \t]*(?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?([a-z])?$`)
	cellSpecEndRx               = rx.MustCompile(`[ \t]+(?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?([a-z])?$`)
	blockMediaMacroRx           = rx.MustCompile(`^(image|video|audio)::(\S|\S.*?\S)\[(.+)?\]$`)
	blockTocMacroRx             = rx.MustCompile(`^toc::\[(.+)?\]$`)
	inlineAnchorRx              = rx.MustCompile(`(\\)?(?:\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\]))`)
	inlineAnchorScanRx          = rx.MustCompile(`(?:^|[^\\\[])\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|(?:^|[^\\])anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\])`)
	leadingInlineAnchorRx       = rx.MustCompile(`^\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))?\]\]`)
	inlineBiblioAnchorRx        = rx.MustCompile(`^\[\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))?\]\]\]`)
	inlineEmailRx               = rx.MustCompile(`([\\>:/])?\p{Word}(?:&amp;|[\p{Word}\-.%+])*@\p{Alnum}[\p{Alnum}_\-.]*\.[a-zA-Z]{2,5}\b`)
	inlineFootnoteMacroRx       = rx.MustCompile(`\\?footnote(?:(ref):|:([\p{Word}-]+)?)\[(?:|(.*?[^\\]))\](?!</a>)`, "m")
	inlineImageMacroRx          = rx.MustCompile(`\\?i(?:mage|con):([^:\s\[](?:[^\n\[]*[^\s\[])?)\[(|.*?[^\\])\]`, "m")
	inlineIndextermMacroRx      = rx.MustCompile(`\\?(?:(indexterm2?):\[(.*?[^\\])\]|\(\((.+?)\)\)(?!\)))`, "m")
	inlineKbdBtnMacroRx         = rx.MustCompile(`(\\)?(kbd|btn):\[(.*?[^\\])\]`, "m")
	inlineLinkRx                = rx.MustCompile(`(^|link:|\p{Blank}|\\?&lt;()|[>\(\)\[\];"'])(\\?(?:https?|file|ftp|irc)://)(?:([^\s\[\]]+)\[(|.*?[^\\])\]|\2([^\s]+?)&gt;|([^\s\[\]<]*([^\s,.?!\[\]<\)])))`, "m")
	inlineLinkMacroRx           = rx.MustCompile(`\\?(?:link|(mailto)):(|[^:\s\[][^\s\[]*)\[(|.*?[^\\])\]`, "m")
	inlineStemMacroRx           = rx.MustCompile(`\\?(stem|(?:latex|ascii)math):([a-z]+(?:,[a-z-]+)*)?\[(.*?[^\\])\]`, "m")
	inlineMenuMacroRx           = rx.MustCompile(`\\?menu:(\p{Word}|[\p{Word}&][^\n\[]*[^\s\[])\[ *(?:|(.*?[^\\]))\]`, "m")
	inlineMenuRx                = rx.MustCompile(`\\?"([\p{Word}&][^"]*?[ \n]+&gt;[ \n]+[^"]*)"`)
	inlinePassRx                = rx.MustCompile(`((?:^|[^\p{Word};:\\])(?=(\[)|\+)|\\(?=\[)|(?=\\\+))(?:\2(x-|[^\]]+ x-)\]|(?:\[([^\]]+)\])?(?=(\\)?\+))(\5?(\+|`+"`"+`)(\S|\S.*?\S)\7)(?!\p{Word})`, "m")
	inlinePassCompatRx          = rx.MustCompile("(^|[^`\\p{Word}])(?:(\\Z)()|\\[([^\\]]+)\\](?=(\\\\))?)?(\\5?(`)([^`\\s]|[^`\\s].*?\\S)\\7)(?![`\\p{Word}])", "m")
	inlinePassMacroRx           = rx.MustCompile(`(?:(?:(\\?)\[([^\]]+)\])?(\\{0,2})(\+\+\+?|\$\$)(.*?)\4|(\\?)pass:([a-z]+(?:,[a-z-]+)*)?\[(|.*?[^\\])\])`, "m")
	inlineXrefMacroRx           = rx.MustCompile(`\\?(?:&lt;&lt;([\p{Word}#/.:{].*?)&gt;&gt;|xref:([\p{Word}#/.:{].*?)\[(?:\]|(.*?[^\\])\])))`, "m")
	hardLineBreakRx             = rx.MustCompile(`^(.*) \+$`)
	markdownThematicBreakRx     = rx.MustCompile(`^ {0,3}([-*_])( *)\1\2\1$`)
	extLayoutBreakRx            = rx.MustCompile(`^(?:'{3,}|<{3,}|([-*_])( *)\1\2\1)$`)
	blankLineRx                 = rx.MustCompile(`\n{2,}`)
	escapedSpaceRx              = rx.MustCompile(`\\([ \t\n])`)
	replaceableTextRx           = rx.MustCompile(`[&']|--|\.\.\.|\([CRT]M?\)`)
	spaceDelimiterRx            = rx.MustCompile(`([^\\])[ \t\n]+`)
	trailingDigitsRx            = rx.MustCompile(`\d+$`)
	uriSniffRx                  = rx.MustCompile(`\A\p{Alpha}[\p{Alnum}.+-]+:/{0,2}`)
	xmlSanitizeRx               = rx.MustCompile(`<[^>]+>`)
	passSlotRx                  = rx.MustCompile("\u0096(\\d+)\u0097")
)

var descriptionListSiblingRx = map[string]*rx.Regexp{
	"::":   rx.MustCompile(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(::)(?:$|[ \t]+(.*)$)`),
	":::":  rx.MustCompile(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(:::)(?:$|[ \t]+(.*)$)`),
	"::::": rx.MustCompile(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(::::)(?:$|[ \t]+(.*)$)`),
	";;":   rx.MustCompile(`^(?!//[^/])[ \t]*([^ \t].*?)(;;)(?:$|[ \t]+(.*)$)`),
}

var orderedListStyles = []string{"arabic", "loweralpha", "lowerroman", "upperalpha", "upperroman"}

var orderedListMarkerRx = map[string]*rx.Regexp{
	"arabic":     rx.MustCompile(`\d+\.`),
	"loweralpha": rx.MustCompile(`[a-z]\.`),
	"lowerroman": rx.MustCompile(`[ivx]+\)`),
	"upperalpha": rx.MustCompile(`[A-Z]\.`),
	"upperroman": rx.MustCompile(`[IVX]+\)`),
}

var listRxMap = map[string]*rx.Regexp{
	"ulist": unorderedListRx, "olist": orderedListRx, "dlist": descriptionListRx, "colist": calloutListRx,
}

type quoteSub struct {
	typ         string
	constrained bool
	re          *rx.Regexp
}

var quoteSubs, quoteSubsCompat []quoteSub

func init() {
	q := func(typ string, constrained bool, pat string, flags ...string) quoteSub {
		return quoteSub{typ, constrained, rx.MustCompile(pat, flags...)}
	}
	quoteSubs = []quoteSub{
		q("strong", false, `\\?(?:\[([^\]]+)\])?\*\*(.+?)\*\*`, "m"),
		q("strong", true, `(^|[^\p{Word};:}])(?:\[([^\]]+)\])?\*(\S|\S.*?\S)\*(?!\p{Word})`, "m"),
		q("double", true, "(^|[^\\p{Word};:}])(?:\\[([^\\]]+)\\])?\"`(\\S|\\S.*?\\S)`\"(?!\\p{Word})", "m"),
		q("single", true, "(^|[^\\p{Word};:`}])(?:\\[([^\\]]+)\\])?'`(\\S|\\S.*?\\S)`'(?!\\p{Word})", "m"),
		q("monospaced", false, "\\\\?(?:\\[([^\\]]+)\\])?``(.+?)``", "m"),
		q("monospaced", true, "(^|[^\\p{Word};:\"'`}])(?:\\[([^\\]]+)\\])?`(\\S|\\S.*?\\S)`(?![\\p{Word}\"'`])", "m"),
		q("emphasis", false, `\\?(?:\[([^\]]+)\])?__(.+?)__`, "m"),
		q("emphasis", true, `(^|[^\p{Word};:}])(?:\[([^\]]+)\])?_(\S|\S.*?\S)_(?!\p{Word})`, "m"),
		q("mark", false, `\\?(?:\[([^\]]+)\])?##(.+?)##`, "m"),
		q("mark", true, `(^|[^\p{Word}&;:}])(?:\[([^\]]+)\])?#(\S|\S.*?\S)#(?!\p{Word})`, "m"),
		q("superscript", false, `\\?(?:\[([^\]]+)\])?\^(\S+?)\^`),
		q("subscript", false, `\\?(?:\[([^\]]+)\])?~(\S+?)~`),
	}
	c := append([]quoteSub(nil), quoteSubs...)
	c[2] = q("double", true, "(^|[^\\p{Word};:}])(?:\\[([^\\]]+)\\])?``(\\S|\\S.*?\\S)''(?!\\p{Word})", "m")
	c[3] = q("single", true, "(^|[^\\p{Word};:}])(?:\\[([^\\]]+)\\])?`(\\S|\\S.*?\\S)'(?!\\p{Word})", "m")
	c[4] = q("monospaced", false, `\\?(?:\[([^\]]+)\])?\+\+(.+?)\+\+`, "m")
	c[5] = q("monospaced", true, `(^|[^\p{Word};:}])(?:\[([^\]]+)\])?\+(\S|\S.*?\S)\+(?!\p{Word})`, "m")
	emph := q("emphasis", true, `(^|[^\p{Word};:}])(?:\[([^\]]+)\])?'(\S|\S.*?\S)'(?!\p{Word})`, "m")
	quoteSubsCompat = append(append(append([]quoteSub(nil), c[:3]...), emph), c[3:]...)
}

type replacement struct {
	re      *rx.Regexp
	repl    string
	restore string
}

var replacements = []replacement{
	{rx.MustCompile(`\\?\(C\)`), "&#169;", "none"},
	{rx.MustCompile(`\\?\(R\)`), "&#174;", "none"},
	{rx.MustCompile(`\\?\(TM\)`), "&#8482;", "none"},
	{rx.MustCompile(`(?: |\n|^|\\)--(?: |\n|$)`), "&#8201;&#8212;&#8201;", "none"},
	{rx.MustCompile(`(\p{Word})\\?--(?=\p{Word})`), "&#8212;&#8203;", "leading"},
	{rx.MustCompile(`\\?\.\.\.`), "&#8230;&#8203;", "none"},
	{rx.MustCompile("\\\\?`'"), "&#8217;", "none"},
	{rx.MustCompile(`(\p{Alnum})\\?'(?=\p{Alpha})`), "&#8217;", "leading"},
	{rx.MustCompile(`\\?-&gt;`), "&#8594;", "none"},
	{rx.MustCompile(`\\?=&gt;`), "&#8658;", "none"},
	{rx.MustCompile(`\\?&lt;-`), "&#8592;", "none"},
	{rx.MustCompile(`\\?&lt;=`), "&#8656;", "none"},
	{rx.MustCompile(`\\?(&)amp;((?:[a-zA-Z][a-zA-Z]+\d{0,2}|#\d\d\d{0,4}|#x[\da-fA-F][\da-fA-F][\da-fA-F]{0,3});)`), "", "bounding"},
}

var intrinsicAttributes = map[string]string{
	"startsb": "[", "endsb": "]", "vbar": "|", "caret": "^", "asterisk": "*", "tilde": "~",
	"plus": "&#43;", "backslash": `\`, "backtick": "`", "blank": "", "empty": "", "sp": " ",
	"two-colons": "::", "two-semicolons": ";;", "nbsp": "&#160;", "deg": "&#176;", "zwsp": "&#8203;",
	"quot": "&#34;", "apos": "&#39;", "lsquo": "&#8216;", "rsquo": "&#8217;", "ldquo": "&#8220;",
	"rdquo": "&#8221;", "wj": "&#8288;", "brvbar": "&#166;", "pp": "&#43;&#43;", "cpp": "C&#43;&#43;",
	"amp": "&", "lt": "<", "gt": ">",
}

var defaultAttributes = map[string]string{
	"appendix-caption": "Appendix", "appendix-refsig": "Appendix", "caution-caption": "Caution",
	"chapter-refsig": "Chapter", "example-caption": "Example", "figure-caption": "Figure",
	"important-caption": "Important", "last-update-label": "Last updated", "note-caption": "Note",
	"part-refsig": "Part", "prewrap": "", "sectids": "", "section-refsig": "Section",
	"table-caption": "Table", "tip-caption": "Tip", "toc-placement": "auto",
	"toc-title": "Table of Contents", "untitled-label": "Untitled", "version-label": "Version",
	"warning-caption": "Warning",
}
