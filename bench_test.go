package adocgo

import (
	"os"
	"path/filepath"
	"testing"
)

// benchInputs are corpus documents of increasing size.
var benchInputs = []struct{ name, path string }{
	{"README", "testdata/realworld/README.adoc"},
	{"WhatsNew", "testdata/docs/ROOT__whats-new.adoc"},
	{"Changelog", "testdata/realworld/CHANGELOG.adoc"},
}

func BenchmarkConvert(b *testing.B) {
	for _, in := range benchInputs {
		src, err := os.ReadFile(filepath.FromSlash(in.path))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(in.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := Convert(src, Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
