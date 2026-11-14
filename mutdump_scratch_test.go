package adocgo

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

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
		for j := 0; j < 2; j++ {
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
