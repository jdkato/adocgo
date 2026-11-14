package adocgo

import (
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
