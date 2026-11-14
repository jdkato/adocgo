package adocgo

// Shared definitions for the scanners (see scan*.go), which replace the
// regular expressions of Asciidoctor's rx.rb.

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
