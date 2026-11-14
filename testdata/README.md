# Test corpus

Every `NAME.adoc` below this directory is a fixture. `NAME.html` (or
`NAME.xml` for fixtures converted with `-b docbook`) holds the output of the
reference implementation (Asciidoctor 2.0.26) for it, and
`go test` compares adocgo's output with that file byte-for-byte.

| Directory     | Contents |
|---------------|----------|
| `features/`   | Hand-written fixtures, one feature area per file |
| `standalone/` | Fixtures converted as full documents (no `-s`) |
| `upstream/`   | AsciiDoc snippets extracted from the heredocs of Asciidoctor's Ruby test suite |
| `docs/`       | Pages of the Asciidoctor documentation |
| `docbook/`    | DocBook 5 fixtures: focused ones, the feature fixtures again (`features/`) and full documents (`standalone/`) |
| `realworld/`  | README, CHANGELOG, man page and test fixtures of Asciidoctor, and the Asciidoctor PDF docs |

Conventions:

* Fixtures are converted in embedded mode (`asciidoctor -s`), except those in
  a directory named `standalone`.
* An optional `NAME.flags` file holds extra asciidoctor command line flags
  for the fixture (`-a name=value`, `-b backend`, `-d doctype`, `-S safe-mode`, `-s`, `-n`).
* Directories named `include` hold files pulled in by fixtures; they are not
  fixtures themselves.
* `known-failures.txt` lists fixtures whose output is known to differ.
* `rx-compat.json` records what Ruby's regular expression engine matches for
  every Asciidoctor pattern that adocgo's scanners replace (see `rxcompat_test.go`).
* `SOURCE_DATE_EPOCH=1700000000` is set while generating and testing so that
  date attributes are stable.

Regenerate the expected output with `scripts/regen.sh` (needs Ruby and the
asciidoctor gem).
