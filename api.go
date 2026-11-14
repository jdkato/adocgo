// Package adocgo converts AsciiDoc to HTML5. It is a native Go port of
// Asciidoctor's parser, substitutors and html5 converter and aims to produce
// byte-identical output.
package adocgo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func generateSectionID(title string, doc *Document) string {
	attrs := doc.Attributes
	pre, ok := attrs["idprefix"]
	if !ok {
		pre = "_"
	}
	sep, sepSub := "_", " _.-"
	noSep := false
	if s, ok := attrs["idseparator"]; ok {
		sep = s
		if sep == "" {
			noSep = true
		} else {
			if runeLen(sep) != 1 {
				sep = firstChar(sep)
				attrs["idseparator"] = sep
			}
			if sep == "-" || sep == "." {
				sepSub = " .-"
			} else {
				sepSub = " " + sep + ".-"
			}
		}
	}
	genID := pre + invalidSectionIDCharsRx.ReplaceAll(strings.ToLower(title), "")
	if noSep {
		genID = strings.ReplaceAll(genID, " ", "")
	} else {
		var sb strings.Builder
		prev := false
		for _, r := range genID {
			if strings.ContainsRune(sepSub, r) {
				if !prev {
					sb.WriteString(sep)
				}
				prev = true
			} else {
				sb.WriteRune(r)
				prev = false
			}
		}
		genID = sb.String()
		if strings.HasSuffix(genID, sep) {
			genID = genID[:len(genID)-len(sep)]
		}
		if pre == "" && strings.HasPrefix(genID, sep) {
			genID = genID[len(sep):]
		}
	}
	if _, ok := doc.refs[genID]; ok {
		cnt := 2
		for {
			candidate := genID + sep + strconv.Itoa(cnt)
			if _, ok := doc.refs[candidate]; !ok {
				return candidate
			}
			cnt++
		}
	}
	return genID
}

// Load parses AsciiDoc source into a Document without converting it.
func Load(src []byte, opts Options) (doc *Document, err error) {
	defer func() {
		if r := recover(); r != nil {
			doc = nil
			err = fmt.Errorf("adocgo: failed to load AsciiDoc document: %v", r)
		}
	}()
	doc = newDocument(opts)
	doc.srcLen = len(src)
	doc.reader = newPreprocessorReader(doc, string(src), doc.Attributes["docfile"])
	if opts.OutFile != "" {
		if p, err := filepath.Abs(opts.OutFile); err == nil {
			doc.outFile = filepath.ToSlash(p)
		}
	}
	parseDocument(doc.reader, doc, false)
	doc.restoreAttributes()
	doc.parsed = true
	return doc, nil
}

// Convert converts the document to HTML5.
func (d *Document) Convert() (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out = nil
			err = fmt.Errorf("adocgo: failed to convert document: %v", r)
		}
	}()
	if d.Safe < SafeServer && d.outFile != "" {
		d.Attributes["outfile"] = d.outFile
		d.Attributes["outdir"] = fileDirname(d.outFile)
	}
	var w bytes.Buffer
	w.Grow(2*d.srcLen + 1024)
	d.convertDocumentTo(&w)
	if n := w.Len(); n > 0 && w.Bytes()[n-1] != '\n' {
		w.WriteByte('\n')
	}
	out = w.Bytes()
	return out, nil
}

// DocTitle returns the document title with substitutions applied, if any.
func (d *Document) DocTitle() string {
	t, _ := d.doctitle(false)
	return t
}

// Convert converts AsciiDoc source to HTML5.
func Convert(src []byte, opts Options) ([]byte, error) {
	doc, err := Load(src, opts)
	if err != nil {
		return nil, err
	}
	return doc.Convert()
}

// ConvertFile reads the named file and converts it to HTML5. Unless set in
// opts, the input file name, base directory and modification time are
// derived from the file.
func ConvertFile(name string, opts Options) ([]byte, error) {
	src, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if opts.InputFile == "" {
		opts.InputFile = name
	}
	if opts.ModTime.IsZero() {
		if st, err := os.Stat(name); err == nil {
			opts.ModTime = st.ModTime()
		}
	}
	return Convert(src, opts)
}
