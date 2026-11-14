package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

func TestStdinToStdout(t *testing.T) {
	out, _, code := runCLI(t, "= Title\n\nHello *world*.\n", "-s", "-")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	want := "<div class=\"paragraph\">\n<p>Hello <strong>world</strong>.</p>\n</div>\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStandaloneByDefault(t *testing.T) {
	out, _, code := runCLI(t, "Hello.\n", "-a", "reproducible", "-")
	if code != 0 || !strings.HasPrefix(out, "<!DOCTYPE html>") || !strings.HasSuffix(out, "</html>\n") {
		t.Errorf("unexpected output (code %d): %.80q", code, out)
	}
}

func TestFlags(t *testing.T) {
	src := "= T\n:b: doc\n\n{a} {b} {doctype} {safe-mode-name}\n\ninclude::x.adoc[]\n"
	out, _, code := runCLI(t, src, "-e", "-a", "a=1", "-ab=cli", "--attribute=c", "-d", "book", "-S", "secure", "-o", "-", "-")
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	for _, want := range []string{"<p>1 cli book secure</p>", `<a href="x.adoc" class="bare include">x.adoc</a>`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	out, _, _ = runCLI(t, "== S\n", "-s", "-n", "-")
	if !strings.Contains(out, ">1. S</h2>") {
		t.Errorf("-n did not number sections: %s", out)
	}
}

func TestFileOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "doc.adoc")
	if err := os.WriteFile(in, []byte("= Doc\n\n{docname}{outfilesuffix} include::part.adoc[]\n\ninclude::part.adoc[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "part.adoc"), []byte("included text\n"), 0o644)
	if _, stderr, code := runCLI(t, "", "-s", in); code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "doc.html") || !strings.Contains(string(got), "<p>included text</p>") {
		t.Errorf("unexpected output: %s", got)
	}
	other := filepath.Join(dir, "out", "x.html")
	os.Mkdir(filepath.Dir(other), 0o755)
	if _, _, code := runCLI(t, "", "-s", "-o", other, "-B", dir, in); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if _, err := os.Stat(other); err != nil {
		t.Error(err)
	}
}

func TestErrors(t *testing.T) {
	if _, _, code := runCLI(t, "", "--bogus", "-"); code == 0 {
		t.Error("expected failure for unknown option")
	}
	if _, _, code := runCLI(t, "", "-S", "nope", "-"); code == 0 {
		t.Error("expected failure for invalid safe mode")
	}
	if _, _, code := runCLI(t, "", "no-such-file.adoc"); code == 0 {
		t.Error("expected failure for missing input")
	}
	if _, _, code := runCLI(t, ""); code == 0 {
		t.Error("expected failure without input")
	}
	if out, _, code := runCLI(t, "", "--version"); code != 0 || !strings.Contains(out, "2.0.26") {
		t.Errorf("--version: %q", out)
	}
}
