package adocgo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jdkato/adocgo/internal/rx"
)

// scanOracle compares every scan performed while it is installed with the
// regular expression the scanner replaces.
type scanOracle struct {
	mu       sync.Mutex
	res      map[*scanner]*rx.Regexp
	checked  map[string]int
	failures []string
}

func (o *scanOracle) check(sc *scanner, s string, from int, anchored bool, c []int, ok bool) {
	if !sc.hand {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	re := o.res[sc]
	if re == nil {
		re = rx.MustCompile(sc.pat, sc.flags)
		if re.NumGroups() != sc.n {
			o.failures = append(o.failures, fmt.Sprintf("%s: scanner declares %d groups, pattern has %d", sc.pat, sc.n, re.NumGroups()))
		}
		o.res[sc] = re
	}
	o.checked[sc.pat]++
	var m *rx.Match
	if anchored {
		m = re.MatchAt(s, from)
	} else {
		m = re.FindAt(s, from)
	}
	same := (m != nil) == ok
	if same && ok {
		for i := 0; i <= sc.n && i <= re.NumGroups(); i++ {
			ws, we := -1, -1
			if m.Has(i) {
				ws, we = m.Start(i), m.End(i)
			}
			if c[2*i] != ws || c[2*i+1] != we {
				same = false
			}
		}
	}
	if !same && len(o.failures) < 40 {
		want := "no match"
		if m != nil {
			want = ""
			for i := 0; i <= re.NumGroups(); i++ {
				if m.Has(i) {
					want += fmt.Sprintf(" %d-%d", m.Start(i), m.End(i))
				} else {
					want += " n"
				}
			}
		}
		got := "no match"
		if ok {
			got = fmt.Sprint(c)
		}
		in := s
		if len(in) > 300 {
			lo := from - 20
			if lo < 0 {
				lo = 0
			}
			hi := lo + 300
			if hi > len(in) {
				hi = len(in)
			}
			in = fmt.Sprintf("...[%d:]%s", lo, in[lo:hi])
		}
		o.failures = append(o.failures, fmt.Sprintf("%s (flags %q) from %d on %q:\n  scanner: %s\n  regexp: %s", sc.pat, sc.flags, from, in, got, want))
	}
}

func installOracle(t *testing.T) *scanOracle {
	o := &scanOracle{res: map[*scanner]*rx.Regexp{}, checked: map[string]int{}}
	scanCheck = o.check
	t.Cleanup(func() { scanCheck = nil })
	return o
}

func (o *scanOracle) report(t *testing.T) {
	t.Helper()
	for _, f := range o.failures {
		t.Error(f)
	}
	total := 0
	for _, n := range o.checked {
		total += n
	}
	t.Logf("compared %d scans of %d hand-written scanners with their regular expressions", total, len(o.checked))
	if testing.Verbose() {
		for pat, n := range o.checked {
			if n < 200 {
				t.Logf("  only %d scans: %s", n, pat)
			}
		}
	}
}

// TestScannersAgainstRegexps converts the whole corpus, mutated variants of
// it and the adversarial inputs while comparing the result of every single
// scan with the regular expression (run by internal/rx, which TestRxCompat
// in turn validates against Ruby) that the scanner replaces.
func TestScannersAgainstRegexps(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	o := installOracle(t)
	r := rand.New(rand.NewSource(7))
	optSets := []Options{
		{Safe: SafeSecure},
		{Safe: SafeSecure, Standalone: true, Attributes: map[string]string{"toc": "", "sectnums": "", "experimental": "", "icons": "font", "stem": ""}},
		{Safe: SafeSecure, Doctype: "book", Attributes: map[string]string{"compat-mode": "", "hardbreaks": "", "experimental": ""}},
		{Safe: SafeSecure, Doctype: "manpage", Attributes: map[string]string{"hide-uri-scheme": "", "sectanchors": "", "sectlinks": ""}},
		{Safe: SafeSecure, Backend: "docbook", Standalone: true, Attributes: map[string]string{"experimental": "", "copyright": "ACME 2020", "front-cover-image": "image:c.png[]"}},
	}
	for i, path := range corpusFiles(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ConvertFile(path, fixtureOptions(t, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		if len(src) > 30000 {
			continue
		}
		if _, err := Convert(src, optSets[i%len(optSets)]); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		rounds := 2
		if testing.Short() {
			rounds = 1
		}
		for j := 0; j < rounds; j++ {
			if _, err := Convert(mutate(r, src), optSets[(i+j)%len(optSets)]); err != nil {
				t.Errorf("%s (mutated): %v", path, err)
			}
		}
	}
	for name, src := range pathologicalCases() {
		if len(src) > 20000 {
			src = src[:20000]
		}
		if _, err := Convert([]byte(src), optSets[1]); err != nil && !strings.Contains(err.Error(), "nesting") {
			t.Errorf("%s: %v", name, err)
		}
	}
	o.report(t)
}

// scanSamples returns the inputs for TestScannersOnSamples: the lines and
// multi-line chunks recorded for the Ruby compatibility test, their
// special-character-escaped forms and mutated variants of both.
func scanSamples(t *testing.T) []string {
	data, err := os.ReadFile(rxCompatFile)
	if err != nil {
		t.Fatal(err)
	}
	var c rxCompat
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(11))
	var out []string
	for _, in := range c.Inputs {
		out = append(out, in)
		for j := 0; j < 6; j++ {
			m := string(mutate(r, []byte(in)))
			if len(m) < 600 {
				out = append(out, m)
			}
		}
	}
	extra := []string{"", "\n", "a\n", "\na", " ", "[[a]]", "[[a, b]]\n[[c]]", "x::\ny:: z", "* a\n* b\n", "= T\n\n== S", "a *b* c\n*d*", "x\n\n\ny"}
	return append(out, extra...)
}

// TestScannersOnSamples runs every hand-written scanner and the regular
// expression it replaces over a fixed set of sample strings (including
// multi-line and garbled ones that the converter itself may never pass to
// a given scanner) and requires identical matches.
func TestScannersOnSamples(t *testing.T) {
	o := &scanOracle{res: map[*scanner]*rx.Regexp{}, checked: map[string]int{}}
	samples := scanSamples(t)
	scs := append([]*scanner(nil), allScanners...)
	for _, lc := range []string{"", "%", "--", "#!", "//"} {
		scs = append(scs, lineCommentCalloutScanner(lc))
	}
	for _, in := range append([]string(nil), samples...) {
		if strings.Contains(in, "&lt;") {
			samples = append(samples, strings.ReplaceAll(in, "//", "%"), strings.ReplaceAll(in, " &lt;", "#!&lt;"))
		}
	}
	for _, sc := range scs {
		for _, in := range samples {
			pos := 0
			for n := 0; n < 4 && pos <= len(in); n++ {
				var m rmatch
				c := m.c[:2*(sc.n+1)]
				for i := range c {
					c[i] = -1
				}
				ok := sc.find(in, pos, c)
				o.check(sc, in, pos, false, c, ok)
				if !ok {
					break
				}
				if c[1] == c[0] {
					pos = c[1] + 1
				} else {
					pos = c[1]
				}
			}
		}
	}
	for _, sc := range allAtScanners {
		for _, in := range samples {
			for pos := 0; pos <= len(in) && pos < 24; pos++ {
				var m rmatch
				c := m.c[:2*(sc.n+1)]
				for i := range c {
					c[i] = -1
				}
				o.check(sc, in, pos, true, c, sc.at(in, pos, c))
			}
		}
	}
	o.report(t)
}

// FuzzScanners compares every hand-written scanner with the regular
// expression it replaces on arbitrary input:
//
//	go test -run '^$' -fuzz FuzzScanners -fuzztime 60s
func FuzzScanners(f *testing.F) {
	for _, s := range []string{
		"a *b* _c_ `d` #e# ^f^ ~g~ **h** __i__ ``j`` ##k##\n", "[.r]*x* [#i]_y_ \\*z* +p+ `+q+` ++r++ +++s+++ $$t$$ pass:q[u]",
		"https://a.b/c[d] <https://e.f> link:g[h] mailto:i@j.kl[m] n@o.pq xref:r#s[t] <<u,v>> &lt;&lt;w&gt;&gt;",
		"term:: def\n* item\n. item\n<1> co\n[[id,ref]]\n[quote,a,b]\n.Title\n:attr: val\n== Sect [[x]]\n",
		"image:a.png[b] icon:c[d] kbd:[e] btn:[f] menu:g[h > i] \"j &gt; k\" footnote:[l] ((m)) (((n))) indexterm:[o]",
		"x -- y (C) (R) (TM) ... -> => <- <= it's &amp;amp; &amp;#167; a--b `'", "code &lt;1&gt; // &lt;2&gt; &lt;!--3--&gt;\n# <1> <2>",
		"{a} \\{b} {set:c:d} {counter:e} {counter2:f:1}\\}", "|a 2+|b .3+^.>s|c 2*d|e\n\\<1> [x-]`y` [z]\\+w+",
	} {
		f.Add(s)
	}
	scs := append([]*scanner(nil), allScanners...)
	for _, lc := range []string{"", "%", "--"} {
		scs = append(scs, lineCommentCalloutScanner(lc))
	}
	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 400 {
			in = in[:400]
		}
		o := &scanOracle{res: scanFuzzOracles, checked: map[string]int{}}
		for _, sc := range scs {
			pos := 0
			for n := 0; n < 3 && pos <= len(in); n++ {
				var m rmatch
				c := m.c[:2*(sc.n+1)]
				for i := range c {
					c[i] = -1
				}
				ok := sc.find(in, pos, c)
				o.check(sc, in, pos, false, c, ok)
				if !ok {
					break
				}
				if c[1] == c[0] {
					pos = c[1] + 1
				} else {
					pos = c[1]
				}
			}
		}
		for _, sc := range allAtScanners {
			for pos := 0; pos <= len(in) && pos < 8; pos++ {
				var m rmatch
				c := m.c[:2*(sc.n+1)]
				for i := range c {
					c[i] = -1
				}
				o.check(sc, in, pos, true, c, sc.at(in, pos, c))
			}
		}
		for _, fl := range o.failures {
			t.Error(fl)
		}
	})
}

// scanFuzzOracles caches the compiled oracle expressions across fuzz
// iterations (each worker process runs iterations sequentially).
var scanFuzzOracles = map[*scanner]*rx.Regexp{}
