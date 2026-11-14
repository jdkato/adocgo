package rx

import (
	"strings"
	"testing"
)

func caps(m *Match, n int) string {
	if m == nil {
		return "nil"
	}
	var parts []string
	for i := 0; i <= n; i++ {
		if m.Has(i) {
			parts = append(parts, "<"+m.Group(i)+">")
		} else {
			parts = append(parts, "-")
		}
	}
	return strings.Join(parts, "")
}

func TestBasics(t *testing.T) {
	tests := []struct{ pat, flags, in, want string }{
		{`(^|[^\p{Word};:}])(?:\[([^\]]+)\])?\*(\S|\S.*?\S)\*(?!\p{Word})`, "m", "a *b* *c d*e* f", "< *b*>< >-<b>"},
		{`^(=={0,5})[ \t]+(.+?)(?:[ \t]+\1)?$`, "", "== Title ==", "<== Title ==><==><Title>"},
		{`((?://|#|--|;;) ?)?(\\)?<!?(|--)(\d+|\.)\3>(?=(?: ?\\?<!?\3(?:\d+|\.)\3>)*$)`, "", "puts 1 # <1> <2>", "<# <1>><# >-<><1>"},
		{`x*`, "", "ab", "<>"},
		{`a\b`, "", "aé a", "<a>"},
		{`(a|ab)(c|bcd)(d*)`, "", "abcd", "<abcd><a><bcd><>"},
		{`(?:(a)|b)+`, "", "ab", "<ab><a>"},
		{`^(.*) \+$`, "", "x\nfoo +\nbar", "<foo +><foo>"},
		{`\Apass:([a-z]+(?:,[a-z-]+)*)?\[(.*)\]\Z`, "m", "pass:q[a\nb]\n", "<pass:q[a\nb]><q><a\nb>"},
		{`[^\p{Word}-]`, "", "ab-é!", "<!>"},
		{`(a*)*b`, "", "aaac", "nil"},
		{`\\?(?:\[([^\]]+)\])?\^(\S+?)\^`, "", "x^2^", "<^2^>-<2>"},
	}
	for _, tt := range tests {
		re := MustCompile(tt.pat, tt.flags)
		got := caps(re.Find(tt.in), re.NumGroups())
		if got != tt.want {
			t.Errorf("%s on %q: got %s want %s", tt.pat, tt.in, got, tt.want)
		}
	}
	if got := MustCompile(`x*`).ReplaceAll("ab", "-"); got != "-a-b-" {
		t.Errorf("gsub: %q", got)
	}
	if got := MustCompile(`\b`).ReplaceAll("é a", "|"); got != "|é| |a|" {
		t.Errorf("gsub: %q", got)
	}
}
