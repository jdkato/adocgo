package adocgo

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// TestMutDump is a helper for differential fuzzing against the reference
// implementation (see scripts/difffuzz.sh). When MUTDIR is set it writes
// mutated variants of the corpus inputs into $MUTDIR/x; otherwise it is
// skipped. MUTSEED selects the random seed and MUTROUNDS the number of
// variants per fixture.
func TestMutDump(t *testing.T) {
	dir := os.Getenv("MUTDIR")
	if dir == "" {
		t.Skip()
	}
	seed := int64(1)
	fmt.Sscan(os.Getenv("MUTSEED"), &seed)
	r := rand.New(rand.NewSource(seed))
	os.MkdirAll(filepath.Join(dir, "x"), 0o755)
	n := 0
	for _, path := range corpusFiles(t) {
		src, _ := os.ReadFile(path)
		if len(src) > 6000 {
			continue
		}
		rounds := 2
		fmt.Sscan(os.Getenv("MUTROUNDS"), &rounds)
		for j := 0; j < rounds; j++ {
			m := mutate(r, src)
			if !utf8.Valid(m) {
				continue
			}
			n++
			os.WriteFile(filepath.Join(dir, "x", fmt.Sprintf("m%05d.adoc", n)), m, 0o644)
		}
	}
	t.Logf("wrote %d", n)
}
