// Command adocgo converts AsciiDoc files to HTML5. It accepts the most
// commonly used flags of the asciidoctor command line interface.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdkato/adocgo"
)

const usage = `Usage: adocgo [OPTION]... FILE...
Convert the AsciiDoc source FILE or FILE(s) to HTML5 (use - for stdin).

  -a, --attribute name[=value]  set a document attribute (name!, name@ supported)
  -b, --backend BACKEND         output format: html5 (default) or xhtml5
  -B, --base-dir DIR            base directory containing the document and resources
  -d, --doctype DOCTYPE         document type: article, book, manpage, inline
  -e, --embedded                output an embedded document (no header/footer)
  -s, --no-header-footer        same as --embedded
  -o, --out-file FILE           output file (default: based on input; - for stdout)
  -S, --safe-mode MODE          safe mode: unsafe, safe, server, secure (default: unsafe)
      --safe                    set safe mode to safe
  -n, --section-numbers         auto-number section titles
  -h, --help                    show this message
  -V, --version                 display the version
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts := adocgo.Options{Standalone: true, Safe: adocgo.SafeUnsafe, Attributes: map[string]string{}}
	outFile := ""
	var files []string
	fail := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "adocgo: "+format+"\n", a...)
		return 1
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		// value returns the option value for a flag that takes one
		value := func(short, long string) (string, bool, error) {
			switch {
			case arg == short || arg == long:
				if i+1 >= len(args) {
					return "", true, fmt.Errorf("option requires an argument: %s", arg)
				}
				i++
				return args[i], true, nil
			case strings.HasPrefix(arg, long+"="):
				return arg[len(long)+1:], true, nil
			case len(short) == 2 && strings.HasPrefix(arg, short) && !strings.HasPrefix(arg, "--") && len(arg) > 2:
				return arg[2:], true, nil
			}
			return "", false, nil
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		switch arg {
		case "-h", "--help":
			fmt.Fprint(stdout, usage)
			return 0
		case "-V", "--version":
			fmt.Fprintf(stdout, "adocgo (compatible with Asciidoctor %s)\n", adocgo.Version)
			return 0
		case "-e", "--embedded", "-s", "--no-header-footer":
			opts.Standalone = false
			continue
		case "--safe":
			opts.Safe = adocgo.SafeSafe
			continue
		case "-n", "--section-numbers":
			opts.Attributes["sectnums"] = ""
			continue
		case "-q", "--quiet", "-v", "--verbose", "-w", "--warnings", "-t", "--timings", "--trace":
			continue
		}
		if v, ok, err := value("-a", "--attribute"); ok {
			if err != nil {
				return fail("%v", err)
			}
			name, val, _ := strings.Cut(v, "=")
			opts.Attributes[name] = val
			continue
		}
		if v, ok, err := value("-o", "--out-file"); ok {
			if err != nil {
				return fail("%v", err)
			}
			outFile = v
			continue
		}
		if v, ok, err := value("-d", "--doctype"); ok {
			if err != nil {
				return fail("%v", err)
			}
			opts.Doctype = v
			continue
		}
		if v, ok, err := value("-B", "--base-dir"); ok {
			if err != nil {
				return fail("%v", err)
			}
			opts.BaseDir = v
			continue
		}
		if v, ok, err := value("-S", "--safe-mode"); ok {
			if err != nil {
				return fail("%v", err)
			}
			mode, valid := adocgo.ParseSafeMode(v)
			if !valid {
				return fail("invalid safe mode: %s", v)
			}
			opts.Safe = mode
			continue
		}
		if v, ok, err := value("-b", "--backend"); ok {
			if err != nil {
				return fail("%v", err)
			}
			switch v {
			case "html5", "html", "xhtml5", "xhtml":
				opts.Attributes["backend"] = v
			default:
				return fail("unsupported backend: %s (only html5 and xhtml5 are available)", v)
			}
			continue
		}
		return fail("invalid option: %s", arg)
	}
	if len(files) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}
	status := 0
	for _, file := range files {
		o := opts
		o.Attributes = map[string]string{}
		for k, v := range opts.Attributes {
			o.Attributes[k] = v
		}
		var out []byte
		var err error
		dest := outFile
		if file == "-" {
			var src []byte
			src, err = io.ReadAll(stdin)
			if err == nil {
				if dest == "" {
					dest = "-"
				}
				if dest != "-" {
					o.OutFile = dest
				}
				out, err = adocgo.Convert(src, o)
			}
		} else {
			if dest == "" {
				ext := filepath.Ext(file)
				dest = strings.TrimSuffix(file, ext) + ".html"
			}
			if dest != "-" {
				o.OutFile = dest
			}
			out, err = adocgo.ConvertFile(file, o)
		}
		if err != nil {
			fmt.Fprintf(stderr, "adocgo: %v\n", err)
			status = 1
			continue
		}
		// Asciidoctor ends stream output with a newline but writes files
		// without one.
		if dest == "-" {
			stdout.Write(out)
		} else if err := os.WriteFile(dest, bytes.TrimSuffix(out, []byte("\n")), 0o644); err != nil {
			fmt.Fprintf(stderr, "adocgo: %v\n", err)
			status = 1
		}
	}
	return status
}
