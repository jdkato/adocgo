package adocgo

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixtureOptions derives the conversion options for a fixture: fixtures
// under a "standalone" directory are converted as full documents, and an
// optional sidecar NAME.flags file holds asciidoctor-style command line
// flags (-a, -b, -d, -S, -s).
func fixtureOptions(t testing.TB, path string) Options {
	opts := Options{Safe: SafeUnsafe, Attributes: map[string]string{}}
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == "standalone" {
			opts.Standalone = true
		}
	}
	data, err := os.ReadFile(strings.TrimSuffix(path, ".adoc") + ".flags")
	if err != nil {
		return opts
	}
	args := splitFlags(string(data))
	for i := 0; i < len(args); i++ {
		next := func() string {
			i++
			if i < len(args) {
				return args[i]
			}
			t.Fatalf("%s: missing flag value", path)
			return ""
		}
		switch args[i] {
		case "-a":
			name, val, _ := strings.Cut(next(), "=")
			opts.Attributes[name] = val
		case "-d":
			opts.Doctype = next()
		case "-b":
			opts.Backend = next()
		case "-S":
			opts.Safe, _ = ParseSafeMode(next())
		case "-s", "-e":
			opts.Standalone = false
		case "-n":
			opts.Attributes["sectnums"] = ""
		default:
			t.Fatalf("%s: unsupported flag %q", path, args[i])
		}
	}
	return opts
}

// splitFlags splits a flags line into words, honouring single and double
// quotes.
func splitFlags(s string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// expectedPath returns the file holding the reference output of a fixture:
// NAME.xml for fixtures converted with a DocBook backend, else NAME.html.
func expectedPath(path string, opts Options) string {
	ext := ".html"
	if strings.HasPrefix(opts.Backend, "docbook") {
		ext = ".xml"
	}
	return strings.TrimSuffix(path, ".adoc") + ext
}

func corpusFiles(t testing.TB) []string {
	var files []string
	err := filepath.WalkDir(corpusRoot(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "include" || d.Name() == "fuzz" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".adoc") {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

// corpusRoot returns the directory holding the fixtures. It can be pointed
// at another tree with ADOCGO_CORPUS_DIR, which is handy for ad-hoc
// differential runs against a scratch corpus.
func corpusRoot() string {
	if dir := os.Getenv("ADOCGO_CORPUS_DIR"); dir != "" {
		return dir
	}
	return "testdata"
}

func knownFailures(t testing.TB) map[string]bool {
	known := map[string]bool{}
	f, err := os.Open(filepath.Join(corpusRoot(), "known-failures.txt"))
	if err != nil {
		return known
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		known[strings.Fields(line)[0]] = true
	}
	return known
}

// TestCorpus converts every fixture under testdata/ and compares the result
// byte-for-byte with the output of the reference implementation. Fixtures
// listed in testdata/known-failures.txt are expected to differ; the test
// fails if one of them unexpectedly passes so the list stays accurate.
func TestCorpus(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	known := knownFailures(t)
	var pass, fail, knownFail, missing int
	for _, path := range corpusFiles(t) {
		opts := fixtureOptions(t, path)
		want, err := os.ReadFile(expectedPath(path, opts))
		if err != nil {
			missing++
			t.Errorf("%s: missing expected output (run scripts/regen.sh)", path)
			continue
		}
		got, err := ConvertFile(path, opts)
		ok := err == nil && bytes.Equal(got, want)
		switch {
		case ok && known[path]:
			fail++
			t.Errorf("%s: listed in known-failures.txt but passes; remove it from the list", path)
		case ok:
			pass++
		case known[path]:
			knownFail++
		default:
			fail++
			if err != nil {
				t.Errorf("%s: %v", path, err)
			} else {
				t.Errorf("%s: output differs from asciidoctor%s", path, firstDiff(got, want))
			}
		}
	}
	t.Logf("corpus: %d fixtures, %d pass, %d known failures, %d unexpected failures, %d missing", pass+fail+knownFail+missing, pass, knownFail, fail, missing)
}

func firstDiff(got, want []byte) string {
	g := strings.Split(string(got), "\n")
	w := strings.Split(string(want), "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			if len(gl) > 200 {
				gl = gl[:200] + "..."
			}
			if len(wl) > 200 {
				wl = wl[:200] + "..."
			}
			return "\n  line " + itoa(i+1) + "\n  got:  " + gl + "\n  want: " + wl
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
