# adocgo

A Go port of [Asciidoctor](https://asciidoctor.org) 2.0.26. It converts
AsciiDoc to HTML5, XHTML5, and DocBook 5, and its output matches
Asciidoctor's byte for byte. It uses only the Go standard library.

```console
$ go install github.com/jdkato/adocgo/cmd/adocgo@latest
$ adocgo -s -o - README.adoc
```

## Library

```go
import "github.com/jdkato/adocgo"

html, err := adocgo.Convert(src, adocgo.Options{
    Attributes: map[string]string{"toc": "", "icons": "font"}, // like -a
    Standalone: false,             // true writes a full document
    Safe:       adocgo.SafeSecure, // zero value is SafeUnsafe
    BaseDir:    "docs",            // where include:: resolves from
    Doctype:    "book",            // article, book, manpage, inline
    Backend:    "html5",           // html5, xhtml5, docbook5
})

html, err = adocgo.ConvertFile("guide.adoc", adocgo.Options{Standalone: true})
```

`ConvertFile` also sets `docfile`, `docdir`, `docname`, `docdate`, and
`doctime`, as `asciidoctor FILE` does. `Load` parses without converting and
returns the document tree, which is exposed but not yet stable.

`Attributes` follows the `-a` conventions: `"name!"` unsets, and a trailing
`@` makes the value a default that the document may override.

Use `SafeSecure` or `SafeSafe` for input you don't trust. In unsafe mode a
document can include any readable file.

Conversion returns an error instead of panicking. Input nested more than
3000 blocks deep is rejected.

## Command line

```
adocgo [OPTION]... FILE...

  -a, --attribute name[=value]  set a document attribute
  -b, --backend BACKEND         html5 (default), xhtml5, docbook5
  -B, --base-dir DIR            base directory for the document and includes
  -d, --doctype DOCTYPE         article, book, manpage, inline
  -e, --embedded                no header or footer
  -s, --no-header-footer        same as --embedded
  -o, --out-file FILE           output file; - for stdout
  -S, --safe-mode MODE          unsafe (default), safe, server, secure
      --safe                    set safe mode to safe
  -n, --section-numbers         number section titles
  -V, --version                 display the version
```

Use `-` as FILE to read from stdin. `-q`, `-v`, `-w`, `-t`, and `--trace`
are accepted and ignored.

## Compatibility

The test suite converts 1976 fixtures and compares each with the output of
Asciidoctor 2.0.26. All of them match.

| Corpus | Fixtures | Source |
|--------|---------:|--------|
| `testdata/upstream` | 1459 | every snippet in Asciidoctor's test suite |
| `testdata/realworld` | 167 | Asciidoctor's README, changelog, and man page; Asciidoctor PDF docs |
| `testdata/docs` | 99 | the Asciidoctor documentation |
| `testdata/features` | 97 | one feature per fixture |
| `testdata/standalone` | 27 | full HTML documents |
| `testdata/docbook` | 127 | DocBook output, embedded and standalone |

Not supported:

* The `manpage` backend. An unknown backend name renders as HTML5.
* `rouge`, `pygments`, and `coderay`. Output is what Asciidoctor writes
  when the highlighter isn't installed. `highlight.js` works.
* AsciiMath to MathML in DocBook.
* Warnings and errors. The output is the same, but nothing is logged.
* Remote includes and remote `data-uri` images. Remote includes render as
  links.
* Extensions, custom templates and converters, `sourcemap`, and diagrams.
* The `-D`, `-r`, `-I`, `-T`, and `-E` flags, and stylesheet copying for
  `linkcss`.
* Input that isn't UTF-8.
* Windows paths. Nothing is tested there.

When `showtitle` and `notitle` are both passed through the API, `showtitle`
wins.

## Performance

`benchmarks/` converts the same generated document, written in AsciiDoc and
in Markdown, with adocgo and with
[goldmark](https://github.com/yuin/goldmark). Median of three runs on a
4-core x86-64 VM:

| Document | Converter | Time | Throughput | Allocated | Allocations |
|----------|-----------|-----:|-----------:|----------:|------------:|
| 8 sections (15 KB)    | goldmark | 1.08 ms | 13.5 MB/s | 413 KB  | 2,714   |
|                       | adocgo   | 0.93 ms | 15.5 MB/s | 283 KB  | 1,951   |
| 80 sections (150 KB)  | goldmark | 10.1 ms | 14.4 MB/s | 3.98 MB | 26,910  |
|                       | adocgo   | 8.3 ms  | 17.2 MB/s | 2.66 MB | 18,842  |
| 800 sections (1.5 MB) | goldmark | 96.9 ms | 15.0 MB/s | 38.8 MB | 268,843 |
|                       | adocgo   | 81.1 ms | 17.7 MB/s | 26.3 MB | 187,645 |

```console
$ cd benchmarks && go test -run xxx -bench Convert -benchmem .
```

## Testing

```console
$ go test ./...
$ go test -run '^$' -fuzz FuzzConvert -fuzztime 60s .
$ scripts/regen.sh                  # needs Ruby and asciidoctor
$ scripts/difffuzz.sh 7 "-d book"   # differential fuzzing against asciidoctor
```

Asciidoctor finds its syntax with about 120 regular expressions, and many
of them need backreferences or lookahead that Go's `regexp` doesn't have.
adocgo replaces each one with a hand-written scanner. The tests run every
scanner against the expression it replaces, using a Ruby-compatible engine
in `internal/rx` that only the tests import.

The files follow Asciidoctor's layout (`reader.go` for `reader.rb`,
`parser.go` for `parser.rb`, and so on), so a change upstream is easy to
find here. See [`testdata/README.md`](testdata/README.md) for the fixture
layout.

## AI attribution

adocgo was written by [Claude Code](https://claude.com/claude-code),
Anthropic's AI coding agent, working under the direction of the repository
owner. The code is a port of the Asciidoctor source, and its correctness
rests on the differential test corpus described above (byte-for-byte
comparison with Asciidoctor 2.0.26) rather than on line-by-line human
authorship. Commits produced this way carry a `Co-Authored-By: Claude`
trailer.

## License

Apache-2.0. See [LICENSE](LICENSE). adocgo is derived from Asciidoctor,
which is MIT licensed, and embeds its default stylesheet. See
[NOTICE](NOTICE).
