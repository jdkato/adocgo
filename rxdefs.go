package adocgo

// Regular expressions ported verbatim from Asciidoctor's rx.rb. In the Ruby
// source CC_ANY and CC_ALL are both "." (the latter combined with the m flag),
// CC_WORD/CG_WORD are \p{Word}, CC_ALPHA is \p{Alpha}, CC_ALNUM is \p{Alnum}
// and CG_BLANK is \p{Blank}.
var (
	authorInfoLineRx            = rxScan(`^(\p{Word}[\p{Word}\-'.]*)(?: +(\p{Word}[\p{Word}\-'.]*))?(?: +(\p{Word}[\p{Word}\-'.]*))?(?: +<([^>]+)>)?$`)
	authorDelimiterRx           = rxScan(`;(?: |$)`)
	revisionInfoLineRx          = rxScan(`^(?:[^\d{]*(.*?),)? *(?!:)(.*?)(?: *(?!^),?: *(.*))?$`)
	manpageTitleVolnumRx        = rxScan(`^(.+?) *\( *(.+?) *\)$`)
	manpageNamePurposeRx        = rxScan(`^(.+?) +- +(.+)$`)
	conditionalDirectiveRx      = rxScan(`^(\\)?(ifdef|ifndef|ifeval|endif)::(\S*?(?:([,+])\S*?)?)\[(.+)?\]$`)
	evalExpressionRx            = rxScan(`^(.+?) *([=!><]=|[><]) *(.+)$`)
	includeDirectiveRx          = rxScan(`^(\\)?include::([^\s\[](?:[^\[]*[^\s\[])?)\[(.+)?\]$`)
	tagDirectiveRx              = rxScan(`\b(?:tag|(e)nd)::(\S+?)\[\](?=$|[ \r])`, "m")
	attributeEntryRx            = rxScan(`^:(!?\p{Word}[^:]*):(?:[ \t]+(.*))?$`)
	invalidAttributeNameCharsRx = rxScan(`[^\p{Word}-]`)
	attributeEntryPassMacroRx   = rxScan(`\Apass:([a-z]+(?:,[a-z-]+)*)?\[(.*)\]\Z`, "m")
	attributeReferenceRx        = rxScan(`(\\)?\{(\p{Word}[\p{Word}-]*|(set|counter2?):.+?)(\\)?\}`)
	blockAnchorRx               = rxScan(`^\[\[(?:|([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+))?)\]\]$`)
	blockAttributeListRx        = rxScan(`^\[(|[\p{Word}.#%{,"'].*)\]$`)
	blockAttributeLineRx        = rxScan(`^\[(?:|[\p{Word}.#%{,"'].*|\[(?:|[\p{Alpha}_:][\p{Word}\-:.]*(?:, *.+)?)\])\]$`)
	blockTitleRx                = rxScan(`^\.(\.?[^ \t.].*)$`)
	admonitionParagraphRx       = rxScan(`^(NOTE|TIP|IMPORTANT|WARNING|CAUTION):[ \t]+`)
	literalParagraphRx          = rxScan(`^([ \t]+.*)$`)
	extAtxSectionTitleRx        = rxScan(`^(=={0,5}|##{0,5})[ \t]+(.+?)(?:[ \t]+\1)?$`)
	setextSectionTitleRx        = rxScan(`^((?!\.).*?\p{Alnum}.*)$`)
	inlineSectionAnchorRx       = rxScan(` (\\)?\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+))?\]\]$`)
	invalidSectionIDCharsRx     = rxScan(`<[^>]+>|&(?:[a-z][a-z]+\d{0,2}|#\d\d\d{0,4}|#x[\da-f][\da-f][\da-f]{0,3});|[^ \p{Word}\-.]+?`)
	sectionLevelStyleRx         = rxScan(`^sect\d$`)
	anyListRx                   = rxScan(`^(?:[ \t]*(?:-|\*\**|\.\.*|•|\d+\.|[a-zA-Z]\.|[IVXivx]+\))[ \t]|(?!//[^/])[ \t]*[^ \t].*?(?::::{0,2}|;;)(?:$|[ \t])|<(?:\d+|\.)>[ \t])`)
	unorderedListRx             = rxScan(`^[ \t]*(-|\*\**|•)[ \t]+(.*)$`)
	orderedListRx               = rxScan(`^[ \t]*(\.\.*|\d+\.|[a-zA-Z]\.|[IVXivx]+\))[ \t]+(.*)$`)
	descriptionListRx           = rxScan(`^(?!//[^/])[ \t]*([^ \t].*?)(:::{0,2}|;;)(?:$|[ \t]+(.*)$)`)
	calloutListRx               = rxScan(`^<(\d+|\.)>[ \t]+(.*)$`)
	calloutExtractRx            = rxScan(`((?://|#|--|;;) ?)?(\\)?<!?(|--)(\d+|\.)\3>(?=(?: ?\\?<!?\3(?:\d+|\.)\3>)*$)`)
	calloutScanRx               = rxScan(`\\?<!?(|--)(\d+|\.)\1>(?=(?: ?\\?<!?\1(?:\d+|\.)\1>)*$)`)
	calloutSourceRx             = rxScan(`((?://|#|--|;;) ?)?(\\)?&lt;!?(|--)(\d+|\.)\3&gt;(?=(?: ?\\?&lt;!?\3(?:\d+|\.)\3&gt;)*$)`)
	columnSpecRx                = rxScan(`^(?:(\d+)\*)?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?(\d+%?|~)?([a-z])?$`)
	cellSpecStartRx             = rxScan(`^[ \t]*(?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?([a-z])?$`)
	cellSpecEndRx               = rxScan(`[ \t]+(?:(\d+(?:\.\d*)?|(?:\d*\.)?\d+)([*+]))?([<^>](?:\.[<^>]?)?|(?:[<^>]?\.)?[<^>])?([a-z])?$`)
	blockMediaMacroRx           = rxScan(`^(image|video|audio)::(\S|\S.*?\S)\[(.+)?\]$`)
	blockTocMacroRx             = rxScan(`^toc::\[(.+)?\]$`)
	inlineAnchorRx              = rxScan(`(\\)?(?:\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\]))`)
	inlineAnchorScanRx          = rxScan(`(?:^|[^\\\[])\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|(?:^|[^\\])anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\])`)
	leadingInlineAnchorRx       = rxScan(`^\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))?\]\]`)
	inlineBiblioAnchorRx        = rxScan(`^\[\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))?\]\]\]`)
	inlineEmailRx               = rxScan(`([\\>:/])?\p{Word}(?:&amp;|[\p{Word}\-.%+])*@\p{Alnum}[\p{Alnum}_\-.]*\.[a-zA-Z]{2,5}\b`)
	inlineFootnoteMacroRx       = rxScan(`\\?footnote(?:(ref):|:([\p{Word}-]+)?)\[(?:|(.*?[^\\]))\](?!</a>)`, "m")
	inlineImageMacroRx          = rxScan(`\\?i(?:mage|con):([^:\s\[](?:[^\n\[]*[^\s\[])?)\[(|.*?[^\\])\]`, "m")
	inlineIndextermMacroRx      = rxScan(`\\?(?:(indexterm2?):\[(.*?[^\\])\]|\(\((.+?)\)\)(?!\)))`, "m")
	inlineKbdBtnMacroRx         = rxScan(`(\\)?(kbd|btn):\[(.*?[^\\])\]`, "m")
	inlineLinkRx                = rxScan(`(^|link:|\p{Blank}|\\?&lt;()|[>\(\)\[\];"'])(\\?(?:https?|file|ftp|irc)://)(?:([^\s\[\]]+)\[(|.*?[^\\])\]|\2([^\s]+?)&gt;|([^\s\[\]<]*([^\s,.?!\[\]<\)])))`, "m")
	inlineLinkMacroRx           = rxScan(`\\?(?:link|(mailto)):(|[^:\s\[][^\s\[]*)\[(|.*?[^\\])\]`, "m")
	inlineStemMacroRx           = rxScan(`\\?(stem|(?:latex|ascii)math):([a-z]+(?:,[a-z-]+)*)?\[(.*?[^\\])\]`, "m")
	inlineMenuMacroRx           = rxScan(`\\?menu:(\p{Word}|[\p{Word}&][^\n\[]*[^\s\[])\[ *(?:|(.*?[^\\]))\]`, "m")
	inlineMenuRx                = rxScan(`\\?"([\p{Word}&][^"]*?[ \n]+&gt;[ \n]+[^"]*)"`)
	inlinePassRx                = rxScan(`((?:^|[^\p{Word};:\\])(?=(\[)|\+)|\\(?=\[)|(?=\\\+))(?:\2(x-|[^\]]+ x-)\]|(?:\[([^\]]+)\])?(?=(\\)?\+))(\5?(\+|`+"`"+`)(\S|\S.*?\S)\7)(?!\p{Word})`, "m")
	inlinePassCompatRx          = rxScan("(^|[^`\\p{Word}])(?:(\\Z)()|\\[([^\\]]+)\\](?=(\\\\))?)?(\\5?(`)([^`\\s]|[^`\\s].*?\\S)\\7)(?![`\\p{Word}])", "m")
	inlinePassMacroRx           = rxScan(`(?:(?:(\\?)\[([^\]]+)\])?(\\{0,2})(\+\+\+?|\$\$)(.*?)\4|(\\?)pass:([a-z]+(?:,[a-z-]+)*)?\[(|.*?[^\\])\])`, "m")
	inlineXrefMacroRx           = rxScan(`\\?(?:&lt;&lt;([\p{Word}#/.:{].*?)&gt;&gt;|xref:([\p{Word}#/.:{].*?)\[(?:\]|(.*?[^\\])\]))`, "m")
	hardLineBreakRx             = rxScan(`^(.*) \+$`)
	markdownThematicBreakRx     = rxScan(`^ {0,3}([-*_])( *)\1\2\1$`)
	extLayoutBreakRx            = rxScan(`^(?:'{3,}|<{3,}|([-*_])( *)\1\2\1)$`)
	blankLineRx                 = rxScan(`\n{2,}`)
	escapedSpaceRx              = rxScan(`\\([ \t\n])`)
	replaceableTextRx           = rxScan(`[&']|--|\.\.\.|\([CRT]M?\)`)
	spaceDelimiterRx            = rxScan(`([^\\])[ \t\n]+`)
	trailingDigitsRx            = rxScan(`\d+$`)
	uriSniffRx                  = rxScan(`\A\p{Alpha}[\p{Alnum}.+-]+:/{0,2}`)
	xmlSanitizeRx               = rxScan(`<[^>]+>`)
	passSlotRx                  = rxScan("\u0096(\\d+)\u0097")
)

var descriptionListSiblingRx = map[string]*scanner{
	"::":   rxScan(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(::)(?:$|[ \t]+(.*)$)`),
	":::":  rxScan(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(:::)(?:$|[ \t]+(.*)$)`),
	"::::": rxScan(`^(?!//[^/])[ \t]*([^ \t].*?[^:]|[^ \t:])(::::)(?:$|[ \t]+(.*)$)`),
	";;":   rxScan(`^(?!//[^/])[ \t]*([^ \t].*?)(;;)(?:$|[ \t]+(.*)$)`),
}

var orderedListStyles = []string{"arabic", "loweralpha", "lowerroman", "upperalpha", "upperroman"}

var orderedListMarkerRx = map[string]*scanner{
	"arabic":     rxScan(`\d+\.`),
	"loweralpha": rxScan(`[a-z]\.`),
	"lowerroman": rxScan(`[ivx]+\)`),
	"upperalpha": rxScan(`[A-Z]\.`),
	"upperroman": rxScan(`[IVX]+\)`),
}

var listRxMap = map[string]*scanner{
	"ulist": unorderedListRx, "olist": orderedListRx, "dlist": descriptionListRx, "colist": calloutListRx,
}

type quoteSub struct {
	typ         string
	constrained bool
	re          *scanner
}

var quoteSubs, quoteSubsCompat []quoteSub

func init() {
	q := func(typ string, constrained bool, pat string, flags ...string) quoteSub {
		return quoteSub{typ, constrained, rxScan(pat, flags...)}
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
	re      *scanner
	repl    string
	restore string
}

var replacements = []replacement{
	{rxScan(`\\?\(C\)`), "&#169;", "none"},
	{rxScan(`\\?\(R\)`), "&#174;", "none"},
	{rxScan(`\\?\(TM\)`), "&#8482;", "none"},
	{rxScan(`(?: |\n|^|\\)--(?: |\n|$)`), "&#8201;&#8212;&#8201;", "none"},
	{rxScan(`(\p{Word})\\?--(?=\p{Word})`), "&#8212;&#8203;", "leading"},
	{rxScan(`\\?\.\.\.`), "&#8230;&#8203;", "none"},
	{rxScan("\\\\?`'"), "&#8217;", "none"},
	{rxScan(`(\p{Alnum})\\?'(?=\p{Alpha})`), "&#8217;", "leading"},
	{rxScan(`\\?-&gt;`), "&#8594;", "none"},
	{rxScan(`\\?=&gt;`), "&#8658;", "none"},
	{rxScan(`\\?&lt;-`), "&#8592;", "none"},
	{rxScan(`\\?&lt;=`), "&#8656;", "none"},
	{rxScan(`\\?(&)amp;((?:[a-zA-Z][a-zA-Z]+\d{0,2}|#\d\d\d{0,4}|#x[\da-fA-F][\da-fA-F][\da-fA-F]{0,3});)`), "", "bounding"},
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
