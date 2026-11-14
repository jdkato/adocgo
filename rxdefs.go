package adocgo

// Regular expressions ported verbatim from Asciidoctor's rx.rb. In the Ruby
// source CC_ANY and CC_ALL are both "." (the latter combined with the m flag),
// CC_WORD/CG_WORD are \p{Word}, CC_ALPHA is \p{Alpha}, CC_ALNUM is \p{Alnum}
// and CG_BLANK is \p{Blank}.
var (
	calloutExtractRx       = rxScan(`((?://|#|--|;;) ?)?(\\)?<!?(|--)(\d+|\.)\3>(?=(?: ?\\?<!?\3(?:\d+|\.)\3>)*$)`)
	calloutScanRx          = rxScan(`\\?<!?(|--)(\d+|\.)\1>(?=(?: ?\\?<!?\1(?:\d+|\.)\1>)*$)`)
	calloutSourceRx        = rxScan(`((?://|#|--|;;) ?)?(\\)?&lt;!?(|--)(\d+|\.)\3&gt;(?=(?: ?\\?&lt;!?\3(?:\d+|\.)\3&gt;)*$)`)
	inlineAnchorRx         = rxScan(`(\\)?(?:\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\]))`)
	inlineAnchorScanRx     = rxScan(`(?:^|[^\\\[])\[\[([\p{Alpha}_:][\p{Word}\-:.]*)(?:, *(.+?))? ?\]\]|(?:^|[^\\])anchor:([\p{Alpha}_:][\p{Word}\-:.]*)\[(?:\]|(.*?[^\\])\])`)
	inlineEmailRx          = rxScan(`([\\>:/])?\p{Word}(?:&amp;|[\p{Word}\-.%+])*@\p{Alnum}[\p{Alnum}_\-.]*\.[a-zA-Z]{2,5}\b`)
	inlineFootnoteMacroRx  = rxScan(`\\?footnote(?:(ref):|:([\p{Word}-]+)?)\[(?:|(.*?[^\\]))\](?!</a>)`, "m")
	inlineImageMacroRx     = rxScan(`\\?i(?:mage|con):([^:\s\[](?:[^\n\[]*[^\s\[])?)\[(|.*?[^\\])\]`, "m")
	inlineIndextermMacroRx = rxScan(`\\?(?:(indexterm2?):\[(.*?[^\\])\]|\(\((.+?)\)\)(?!\)))`, "m")
	inlineKbdBtnMacroRx    = rxScan(`(\\)?(kbd|btn):\[(.*?[^\\])\]`, "m")
	inlineLinkRx           = rxScan(`(^|link:|\p{Blank}|\\?&lt;()|[>\(\)\[\];"'])(\\?(?:https?|file|ftp|irc)://)(?:([^\s\[\]]+)\[(|.*?[^\\])\]|\2([^\s]+?)&gt;|([^\s\[\]<]*([^\s,.?!\[\]<\)])))`, "m")
	inlineLinkMacroRx      = rxScan(`\\?(?:link|(mailto)):(|[^:\s\[][^\s\[]*)\[(|.*?[^\\])\]`, "m")
	inlineStemMacroRx      = rxScan(`\\?(stem|(?:latex|ascii)math):([a-z]+(?:,[a-z-]+)*)?\[(.*?[^\\])\]`, "m")
	inlineMenuMacroRx      = rxScan(`\\?menu:(\p{Word}|[\p{Word}&][^\n\[]*[^\s\[])\[ *(?:|(.*?[^\\]))\]`, "m")
	inlineMenuRx           = rxScan(`\\?"([\p{Word}&][^"]*?[ \n]+&gt;[ \n]+[^"]*)"`)
	inlinePassRx           = rxScan(`((?:^|[^\p{Word};:\\])(?=(\[)|\+)|\\(?=\[)|(?=\\\+))(?:\2(x-|[^\]]+ x-)\]|(?:\[([^\]]+)\])?(?=(\\)?\+))(\5?(\+|`+"`"+`)(\S|\S.*?\S)\7)(?!\p{Word})`, "m")
	inlinePassCompatRx     = rxScan("(^|[^`\\p{Word}])(?:(\\Z)()|\\[([^\\]]+)\\](?=(\\\\))?)?(\\5?(`)([^`\\s]|[^`\\s].*?\\S)\\7)(?![`\\p{Word}])", "m")
	inlinePassMacroRx      = rxScan(`(?:(?:(\\?)\[([^\]]+)\])?(\\{0,2})(\+\+\+?|\$\$)(.*?)\4|(\\?)pass:([a-z]+(?:,[a-z-]+)*)?\[(|.*?[^\\])\])`, "m")
	inlineXrefMacroRx      = rxScan(`\\?(?:&lt;&lt;([\p{Word}#/.:{].*?)&gt;&gt;|xref:([\p{Word}#/.:{].*?)\[(?:\]|(.*?[^\\])\]))`, "m")
)

var orderedListStyles = []string{"arabic", "loweralpha", "lowerroman", "upperalpha", "upperroman"}

var listRxMap = map[string]*scanner{
	"ulist": unorderedListRx, "olist": orderedListRx, "dlist": descriptionListRx, "colist": calloutListRx,
}

type quoteSub struct {
	typ         string
	constrained bool
	re          *scanner
}

var quoteSubs, quoteSubsCompat []quoteSub

type replacement struct {
	re      *scanner
	repl    string
	restore string
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
