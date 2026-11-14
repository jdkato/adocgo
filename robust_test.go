package adocgo

import (
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

// convertWithTimeout converts src and fails the test if conversion returns
// an error (which includes recovered panics) or does not finish in time.
func convertWithTimeout(t *testing.T, name string, src []byte, opts Options) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := Convert(src, opts)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("%s: %v\ninput:\n%q", name, err, truncateBytes(src, 2000))
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("%s: conversion did not finish within 30s\ninput:\n%q", name, truncateBytes(src, 2000))
	}
}

func truncateBytes(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

var mutationTokens = []string{
	"\n", "\n\n", "----\n", "====\n", "|===\n", "****\n", "____\n", "++++\n", "--\n", "....\n", "////\n", "```\n",
	"* ", "** ", ". ", "+\n", "::", ":: ", "[[", "]]", "<<", ">>", "[", "]", "{", "}", "|", "\\", "`", "*", "_", "#", "+",
	"= ", "== ", "ifdef::x[]\n", "endif::[]\n", "include::x[]\n", "image::", "footnote:[", "pass:[", "a|", "2+|", "\t", " ",
	"<1>", "[source]\n", "[quote]\n", "[%header,cols=\"1,a\"]\n", ":x: y\n", "xref:", "link:", "((", "))", "\x00", "\xff",
}

func mutate(r *rand.Rand, src []byte) []byte {
	out := append([]byte(nil), src...)
	for n := r.Intn(6) + 1; n > 0; n-- {
		if len(out) == 0 {
			out = append(out, mutationTokens[r.Intn(len(mutationTokens))]...)
			continue
		}
		pos := r.Intn(len(out))
		switch r.Intn(6) {
		case 0: // truncate
			out = out[:pos]
		case 1: // delete a span
			end := pos + r.Intn(40)
			if end > len(out) {
				end = len(out)
			}
			out = append(out[:pos:pos], out[end:]...)
		case 2: // insert a token
			tok := mutationTokens[r.Intn(len(mutationTokens))]
			out = append(out[:pos:pos], append([]byte(tok), out[pos:]...)...)
		case 3: // duplicate a span
			end := pos + r.Intn(80)
			if end > len(out) {
				end = len(out)
			}
			span := append([]byte(nil), out[pos:end]...)
			out = append(out[:end:end], append(span, out[end:]...)...)
		case 4: // flip a byte
			out[pos] = byte(r.Intn(256))
		case 5: // drop the head
			out = out[pos:]
		}
	}
	return out
}

// TestMutatedCorpus feeds truncated and garbled variants of the corpus
// inputs through the converter. Conversion must neither fail nor hang.
func TestMutatedCorpus(t *testing.T) {
	files := corpusFiles(t)
	r := rand.New(rand.NewSource(20240229))
	rounds := 3
	if testing.Short() {
		rounds = 1
	}
	optSets := []Options{
		{Safe: SafeSecure},
		{Safe: SafeSecure, Standalone: true, Attributes: map[string]string{"toc": "", "sectnums": "", "experimental": "", "icons": "font"}},
		{Safe: SafeSecure, Doctype: "book", Attributes: map[string]string{"compat-mode": "", "hardbreaks": ""}},
	}
	for i, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(src) > 20000 {
			src = src[:20000]
		}
		for j := 0; j < rounds; j++ {
			convertWithTimeout(t, path, mutate(r, src), optSets[(i+j)%len(optSets)])
		}
	}
}

// TestPathological checks inputs designed to stress nesting and unterminated
// constructs.
func pathologicalCases() map[string]string {
	return map[string]string{
		"deep ulist":          deepList("*", 60),
		"deep olist":          deepList(".", 60),
		"deep quote":          strings.Repeat("> ", 200) + "x\n",
		"nested open blocks":  strings.Repeat("--\n", 501),
		"nested examples":     nestedDelims('=', 80),
		"unterminated blocks": "----\n====\n|===\n****\n____\n++++\n....\n////\n",
		"unterminated table":  "|===\n|a |b\n|c",
		"many stars":          strings.Repeat("*", 5000) + "\n",
		"many underscores":    strings.Repeat("_a", 3000) + "\n",
		"many backticks":      strings.Repeat("`+", 3000) + "\n",
		"many brackets":       strings.Repeat("[", 3000) + strings.Repeat("]", 3000) + "\n",
		"many xrefs":          strings.Repeat("<<a,", 2000) + "\n",
		"many footnotes":      strings.Repeat("footnote:[", 1000) + "\n",
		"many xref macros":    strings.Repeat("xref:a[", 3000) + "\n",
		"many image macros":   strings.Repeat("image:a[", 3000) + "\n",
		"many link macros":    strings.Repeat("link:a[", 3000) + "\n",
		"many links":          strings.Repeat("https://a.b[", 1500) + "\n",
		"many attrs":          strings.Repeat("{a", 3000) + strings.Repeat("}", 3000) + "\n",
		"attr self reference": ":a: {a}\n\n{a}\n",
		"attr cycle":          ":a: {b}\n:b: {a}\n\n{a} {b}\n",
		"long line":           strings.Repeat("word *bold* _em_ `code` ", 5000) + "\n",
		"many sections":       strings.Repeat("== S\n\n", 2000),
		"pass slot self ref":  "pass:[\u00960\u0097] and \u00961\u0097 +x+\n\n+++\u00960\u0097+++ pass:n[\u00960\u0097 +a+ \u00961\u0097]\n",
		"stray pass slots":    "\u0096\u0097 \u00969\u0097 \u0096999999999999999999999\u0097 +a+ \u0018 \u007f {x}\n",
		"deep sections":       "= T\n\n== a\n\n=== b\n\n==== c\n\n===== d\n\n====== e\n\n======= f\n",
		"table spans":         "|===\n" + strings.Repeat("99+|x 3.99+|y\n", 50) + "|===\n",
		"table many cols":     "[cols=\"100000*\"]\n|===\n|a\n|===\n",
		"cols number":         "[cols=99999999]\n|===\n|a\n|===\n",
		"nested a cells":      nestedTables(12),
		"list continuations":  "* a\n" + strings.Repeat("+\n", 500) + "b\n",
		"dlist chain":         strings.Repeat("a::\n", 2000),
		"include loop":        "include::self.adoc[]\n",
		"conditionals":        strings.Repeat("ifdef::a[]\n", 1000) + "x\n",
		"endif only":          strings.Repeat("endif::[]\n", 1000),
		"ifeval garbage":      "ifeval::[\"a\" < ]\nifeval::[{x} == {y}]\nifeval::[1 <=> 2]\nx\n",
		"counter garbage":     "{counter:}{counter::}{counter2:a:}{set:}{set::}{set:a:b:c}\n",
		"callouts":            "----\n" + strings.Repeat("<1> <.> <!--2-->", 500) + "\n----\n" + strings.Repeat("<1> x\n", 500),
		"pass nesting":        strings.Repeat("pass:[+++$$", 800) + "\n",
		"kbd garbage":         ":experimental:\n\nkbd:[,] kbd:[+] kbd:[++] kbd:[,,] menu:[] menu:a[>] btn:[]\n",
		"setext confusion":    "a\n-\nb\n--\nc\n---\nd\n----\ne\n",
		"only delimiters":     "|===\n|===\n|===\n,===\n:===\n!===\n",
		"binary":              "\x00\x01\x02\xff\xfe\n= \x80\n\n* \xc3\n",
		"bom":                 "\xef\xbb\xbf= Title\n\ntext\n",
		"crlf":                "= Title\r\nAuthor\r\n\r\n== S\r\n\r\n* a\r\n* b\r\n",
		"no trailing newline": "= T\n\n|===\n|a",
		"author garbage":      "= T\n;;;; <<>> <a@b>;\nv, : :\n",
		"rev garbage":         "= T\nA B\n:,:,:\n",
		"manpage garbage":     "= x\n\n== NAME\n\n-\n",
		"video garbage":       "video::/[youtube]\n\nvideo::,[youtube]\n\nvideo::/[vimeo]\n",
		"stem garbage":        ":stem:\n\nstem:[] latexmath:[$] asciimath:[$$]\n\n[stem]\n++++\n\\\n\n\n++++\n",
		"leveloffset huge":    ":leveloffset: 99999999999999999999\n\n= T\n\n== S\n",
		"toc levels":          ":toc:\n:toclevels: -5\n:sectnumlevels: 99\n:sectnums:\n\n== a\n\n=== b\n",
	}
}

// TestPathological checks inputs designed to stress nesting and unterminated
// constructs.
func TestPathological(t *testing.T) {
	cases := pathologicalCases()
	for _, doctype := range []string{"article", "book", "manpage", "inline"} {
		for name, src := range cases {
			opts := Options{Safe: SafeSecure, Doctype: doctype, Standalone: doctype == "book"}
			convertWithTimeout(t, name+" ("+doctype+")", []byte(src), opts)
		}
	}
}

// TestNestingLimit checks that absurdly deep nesting yields an error (as
// with the reference implementation) rather than a stack overflow.
func TestNestingLimit(t *testing.T) {
	cases := map[string]string{
		"quotes":   strings.Repeat("> ", 20000) + "x\n",
		"sections": strings.Repeat(":leveloffset: +1\n\n== S\n\n", 20000),
	}
	for name, src := range cases {
		done := make(chan error, 1)
		go func() {
			_, err := Convert([]byte(src), Options{Safe: SafeSecure})
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Logf("%s: converted without hitting the nesting limit", name)
			} else if !strings.Contains(err.Error(), "nesting") {
				t.Errorf("%s: unexpected error: %v", name, err)
			}
		case <-time.After(60 * time.Second):
			t.Fatalf("%s: did not finish", name)
		}
	}
}

func deepList(marker string, depth int) string {
	var sb strings.Builder
	for i := 1; i <= depth; i++ {
		sb.WriteString(strings.Repeat(marker, i) + " item\n")
	}
	return sb.String()
}

func nestedDelims(ch byte, depth int) string {
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		sb.WriteString(strings.Repeat(string(ch), 4+i) + "\n")
	}
	sb.WriteString("x\n")
	return sb.String()
}

func nestedTables(depth int) string {
	// only two delimiter characters are available (| and !), so deeper
	// levels simply become cell text; this still exercises the nesting path
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		if i%2 == 0 {
			sb.WriteString("|===\na|\n")
		} else {
			sb.WriteString("!===\na!\n")
		}
	}
	sb.WriteString("x\n")
	return sb.String()
}

// FuzzConvert is a native fuzz target; run it with
//
//	go test -run '^$' -fuzz FuzzConvert -fuzztime 60s
func FuzzConvert(f *testing.F) {
	seeds := []string{
		"= Title\nAuthor <a@b.c>\nv1.0, 2020-01-01: r\n:toc:\n\npreamble\n\n== S\n\n* a\n** b\n+\nc\n",
		"[source,ruby]\n----\nputs 1 # <1>\n----\n<1> x\n",
		"|===\n|a |b\n\n2+|c\na|* x\n|===\n",
		"term:: def\n[horizontal]\na::: b\n",
		"*b* _i_ `m` #h# ^s^ ~s~ +p+ `+l+` pass:[x] https://x.y[z] <<a,b>> footnote:[f] ((i)) kbd:[a]\n",
		"ifdef::a[]\nx\nendif::[]\nifeval::[1 < 2]\ny\nendif::[]\n",
		"NOTE: n\n\n[quote,a,b]\n____\nq\n____\n\n====\ne\n====\n",
		":a: {b}\n:b: 1 \\\n2\n\n{a} {counter:c} {set:d:e}\n",
		"image::a.png[b,1,2,link=c]\n\nvideo::d[youtube]\n\ntoc::[]\n",
		",===\na,\"b\nc\",d\n,===\n",
		"pass:[\u00960\u0097] +x+ \u00961\u0097 \u0018\u007f{x}\n",
		"> > > q\n\n:leveloffset: +1\n\n== S\n\nxref:a[b] <<c>> kbd:[d] menu:e[f > g] image:h[i] icon:j[k]\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s), uint8(0))
	}
	f.Fuzz(func(t *testing.T, src []byte, mode uint8) {
		opts := Options{Safe: SafeSecure, Standalone: mode&1 != 0}
		switch (mode >> 1) & 3 {
		case 1:
			opts.Doctype = "book"
		case 2:
			opts.Doctype = "manpage"
		case 3:
			opts.Doctype = "inline"
		}
		if mode&8 != 0 {
			opts.Attributes = map[string]string{"experimental": "", "sectnums": "", "toc": "", "icons": "font", "compat-mode": ""}
		}
		if _, err := Convert(src, opts); err != nil {
			t.Fatalf("Convert failed: %v", err)
		}
	})
}
