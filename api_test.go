package adocgo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConvertEmbedded(t *testing.T) {
	out, err := Convert([]byte("= Title\n\nHello *world*.\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "<div class=\"paragraph\">\n<p>Hello <strong>world</strong>.</p>\n</div>\n"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestConvertStandalone(t *testing.T) {
	out, err := Convert([]byte("= Title\n\ntext\n"), Options{Standalone: true, Attributes: map[string]string{"reproducible": "", "stylesheet!": ""}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"<!DOCTYPE html>", "<title>Title</title>", "<h1>Title</h1>", "</html>\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(s, "<style>") {
		t.Error("stylesheet should have been disabled")
	}
}

func TestConvertEmpty(t *testing.T) {
	for _, src := range [][]byte{nil, {}, []byte("\n\n")} {
		out, err := Convert(src, Options{Standalone: false})
		if err != nil || len(out) != 0 {
			t.Errorf("Convert(%q) = %q, %v", src, out, err)
		}
	}
}

func TestOptions(t *testing.T) {
	src := []byte("{a} {b} {doctype} {safe-mode-name}\n")
	out, err := Convert(src, Options{Attributes: map[string]string{"a": "1", "b": "2"}, Doctype: "book", Safe: SafeSafe})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<p>1 2 book safe</p>") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestBaseDirAndSafeModes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.adoc"), []byte("included\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := []byte("include::inc.adoc[]\n")
	out, err := Convert(src, Options{BaseDir: dir, Safe: SafeSafe})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<p>included</p>") {
		t.Errorf("include not resolved against BaseDir: %s", out)
	}
	out, _ = Convert(src, Options{BaseDir: dir, Safe: SafeSecure})
	if !strings.Contains(string(out), `<a href="inc.adoc" class="bare include">inc.adoc</a>`) {
		t.Errorf("include should become a link in secure mode: %s", out)
	}
	// in safe mode an include may not escape the base directory
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	out, _ = Convert([]byte("include::../inc.adoc[]\n"), Options{BaseDir: sub, Safe: SafeSafe})
	if strings.Contains(string(out), "<p>included</p>") {
		t.Errorf("include escaped the jail in safe mode: %s", out)
	}
	out, _ = Convert([]byte("include::../inc.adoc[]\n"), Options{BaseDir: sub, Safe: SafeUnsafe})
	if !strings.Contains(string(out), "<p>included</p>") {
		t.Errorf("include outside base dir should work in unsafe mode: %s", out)
	}
}

func TestLoadAST(t *testing.T) {
	doc, err := Load([]byte("= Title\n\n== Section\n\n* a\n* b\n\n|===\n|x |y\n|===\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if doc.DocTitle() != "Title" {
		t.Errorf("title = %q", doc.DocTitle())
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].Context != "section" {
		t.Fatalf("unexpected top-level blocks: %d", len(doc.Blocks))
	}
	sect := doc.Blocks[0]
	if sect.ID != "_section" || sect.Level != 1 || len(sect.Blocks) != 2 {
		t.Errorf("unexpected section: id=%q level=%d blocks=%d", sect.ID, sect.Level, len(sect.Blocks))
	}
	if list := sect.Blocks[0]; list.Context != "ulist" || len(list.Blocks) != 2 {
		t.Errorf("unexpected list: %s with %d items", list.Context, len(list.Blocks))
	}
	if tbl := sect.Blocks[1]; tbl.Context != "table" || tbl.Table == nil || len(tbl.Table.Columns) != 2 || len(tbl.Table.Body) != 1 {
		t.Errorf("unexpected table")
	}
}

// TestConcurrent converts several fixtures from many goroutines at once to
// make sure conversions do not share mutable state (run with -race).
func TestConcurrent(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	files, _ := filepath.Glob("testdata/features/*.adoc")
	if len(files) > 40 {
		files = files[:40]
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, f := range files {
				want, err := os.ReadFile(strings.TrimSuffix(f, ".adoc") + ".html")
				if err != nil {
					continue
				}
				got, err := ConvertFile(f, fixtureOptions(t, f))
				if err != nil || !bytes.Equal(got, want) {
					if !knownFailures(t)[filepath.ToSlash(f)] {
						t.Errorf("%s: concurrent conversion differs (err=%v)", f, err)
					}
				}
			}
		}()
	}
	wg.Wait()
}
