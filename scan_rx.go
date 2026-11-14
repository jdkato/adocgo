package adocgo

import "github.com/jdkato/adocgo/internal/rx"

// rxScan wraps a regular expression as a scanner. It is the transitional
// fallback for constructs that do not have a hand-written scanner yet.
func rxScan(pattern string, flags ...string) *scanner {
	re := rx.MustCompile(pattern, flags...)
	sc := &scanner{pat: pattern}
	if len(flags) > 0 {
		sc.flags = flags[0]
	}
	fill := func(m *rx.Match, c []int) bool {
		if m == nil {
			return false
		}
		for i := 0; i*2 < len(c); i++ {
			if m.Has(i) {
				c[2*i], c[2*i+1] = m.Start(i), m.End(i)
			}
		}
		return true
	}
	sc.find = func(s string, from int, c []int) bool { return fill(re.FindAt(s, from), c) }
	sc.at = func(s string, pos int, c []int) bool { return fill(re.MatchAt(s, pos), c) }
	sc.n = re.NumGroups()
	return sc
}
