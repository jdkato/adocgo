package adocgo

import (
	"strings"
	"unicode/utf8"

	"github.com/jdkato/adocgo/internal/rx"
)

const rubySpace = " \t\n\v\f\r"

func rstrip(s string) string { return strings.TrimRight(s, rubySpace+"\x00") }
func lstrip(s string) string { return strings.TrimLeft(s, rubySpace) }
func strip(s string) string  { return lstrip(rstrip(s)) }

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func lastRune(s string) (rune, int) { return utf8.DecodeLastRuneInString(s) }

// rubySplit emulates String#split(sep) which drops trailing empty fields.
func rubySplit(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// rubySplitN emulates String#split(sep, n) for n > 0.
func rubySplitN(s, sep string, n int) []string {
	if s == "" {
		return nil
	}
	return strings.SplitN(s, sep, n)
}

// rubySplitRx emulates String#split(regexp), dropping trailing empty fields.
func rubySplitRx(s string, re *rx.Regexp) []string {
	if s == "" {
		return nil
	}
	var parts []string
	last := 0
	for _, m := range re.FindAll(s) {
		if m.End(0) == m.Start(0) {
			continue
		}
		parts = append(parts, s[last:m.Start(0)])
		last = m.End(0)
	}
	parts = append(parts, s[last:])
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// splitWhitespaceN emulates String#split(nil, n): split on runs of
// whitespace, ignoring leading whitespace, into at most n fields.
func splitWhitespaceN(s string, n int) []string {
	var out []string
	s = lstrip(s)
	for s != "" {
		if len(out) == n-1 {
			out = append(out, s)
			return out
		}
		i := strings.IndexAny(s, rubySpace)
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i])
		s = lstrip(s[i:])
	}
	return out
}

func isUriish(s string) bool {
	return strings.Contains(s, ":") && uriSniffRx.MatchString(s)
}

func encodeSpacesInURI(s string) string { return strings.ReplaceAll(s, " ", "%20") }

func hasExtname(path string) bool {
	i := strings.LastIndex(path, ".")
	return i >= 0 && !strings.Contains(path[i:], "/")
}

func extname(path string) string {
	i := strings.LastIndex(path, ".")
	if i >= 0 && !strings.Contains(path[i:], "/") {
		return path[i:]
	}
	return ""
}

func rootname(filename string) string {
	i := strings.LastIndex(filename, ".")
	if i >= 0 && !strings.Contains(filename[i:], "/") {
		return filename[:i]
	}
	return filename
}

// fileBasename emulates File.basename.
func fileBasename(p string) string {
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	if p == "/" {
		return p
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return p
}

func basenameNoExt(target string) string {
	base := fileBasename(target)
	ext := extname(target)
	if ext != "" && strings.HasSuffix(base, ext) && len(base) > len(ext) {
		return base[:len(base)-len(ext)]
	}
	return base
}

func fileDirname(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "."
	}
	if i == 0 {
		return "/"
	}
	return p[:i]
}

// encodeURIComponent emulates CGI.escapeURIComponent.
func encodeURIComponent(s string) string {
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' || c == '*' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// path resolution (port of the relevant parts of PathResolver)

func partitionPath(path string, web bool) ([]string, string) {
	root := ""
	if web {
		if strings.HasPrefix(path, "/") {
			root = "/"
		} else if strings.HasPrefix(path, "./") {
			root = "./"
		}
	} else if strings.HasPrefix(path, "/") {
		if strings.HasPrefix(path, "//") {
			root = "//"
		} else {
			root = "/"
		}
	} else if strings.HasPrefix(path, "./") {
		root = "./"
	}
	var segs []string
	for _, s := range rubySplit(path[len(root):], "/") {
		if s != "." {
			segs = append(segs, s)
		}
	}
	return segs, root
}

func joinPath(segs []string, root string) string { return root + strings.Join(segs, "/") }

func expandPath(path string) string {
	segs, root := partitionPath(path, false)
	if strings.Contains(path, "..") {
		var res []string
		for _, s := range segs {
			if s == ".." {
				if len(res) > 0 {
					res = res[:len(res)-1]
				}
			} else {
				res = append(res, s)
			}
		}
		return joinPath(res, root)
	}
	return joinPath(segs, root)
}

func descendsFrom(path, base string) bool {
	if base == path {
		return true
	}
	if base == "/" {
		return strings.HasPrefix(path, "/")
	}
	return strings.HasPrefix(path, base+"/")
}

// systemPath resolves target relative to start, optionally confined to jail.
func systemPath(target, start, jail, workingDir string) string {
	isRoot := func(p string) bool { return strings.HasPrefix(p, "/") }
	var targetSegs []string
	if target != "" {
		if isRoot(target) {
			tp := expandPath(target)
			if jail != "" && !descendsFrom(tp, jail) {
				ts, _ := partitionPath(tp, false)
				js, jr := partitionPath(jail, false)
				return joinPath(append(js, ts...), jr)
			}
			return tp
		}
		targetSegs, _ = partitionPath(target, false)
	}
	fallback := jail
	if fallback == "" {
		fallback = workingDir
	}
	if len(targetSegs) == 0 {
		if start == "" {
			return fallback
		} else if isRoot(start) {
			if jail == "" {
				return expandPath(start)
			}
		} else {
			targetSegs, _ = partitionPath(start, false)
			start = fallback
		}
	} else if start == "" {
		start = fallback
	} else if !isRoot(start) {
		start = strings.TrimSuffix(fallback, "/") + "/" + start
	}
	recheck := jail != "" && !descendsFrom(start, jail)
	startSegs, jailRoot := partitionPath(start, false)
	resolved := append(append([]string(nil), startSegs...), targetSegs...)
	hasDotDot := false
	for _, s := range resolved {
		if s == ".." {
			hasDotDot = true
		}
	}
	var jailSegs []string
	if jail != "" {
		jailSegs, _ = partitionPath(jail, false)
	}
	if hasDotDot {
		unresolved := resolved
		resolved = nil
		for _, s := range unresolved {
			if s == ".." {
				if jail != "" {
					if len(resolved) > len(jailSegs) {
						resolved = resolved[:len(resolved)-1]
					}
				} else if len(resolved) > 0 {
					resolved = resolved[:len(resolved)-1]
				}
			} else {
				resolved = append(resolved, s)
			}
		}
	}
	if recheck {
		tp := joinPath(resolved, jailRoot)
		if descendsFrom(tp, jail) {
			return tp
		}
		return joinPath(append(jailSegs, targetSegs...), jailRoot)
	}
	return joinPath(resolved, jailRoot)
}

// webPath ports PathResolver#web_path.
func webPath(target, start string) string {
	uriPrefix := ""
	if start != "" && !strings.HasPrefix(target, "/") {
		sep := "/"
		if strings.HasSuffix(start, "/") {
			sep = ""
		}
		target = start + sep + target
		if strings.Contains(target, ":") {
			if m := uriSniffRx.Find(target); m != nil {
				uriPrefix = m.Group(0)
				target = target[len(uriPrefix):]
			}
		}
	}
	segs, root := partitionPath(target, true)
	var resolved []string
	for _, s := range segs {
		if s == ".." {
			if len(resolved) == 0 {
				if !(root != "" && root != "./") {
					resolved = append(resolved, s)
				}
			} else if resolved[len(resolved)-1] == ".." {
				resolved = append(resolved, s)
			} else {
				resolved = resolved[:len(resolved)-1]
			}
		} else {
			resolved = append(resolved, s)
		}
	}
	p := joinPath(resolved, root)
	if strings.Contains(p, " ") {
		p = strings.ReplaceAll(p, " ", "%20")
	}
	return uriPrefix + p
}

func relativePath(path, base string) string {
	if strings.HasPrefix(path, "/") {
		if base == path {
			return ""
		}
		if base == "/" {
			return path[1:]
		}
		if strings.HasPrefix(path, base+"/") {
			return path[len(base)+1:]
		}
	}
	return path
}

func normalizeText(text string, normalizeWhitespace, unescapeClosingSB bool) string {
	if text != "" {
		if normalizeWhitespace {
			text = strings.ReplaceAll(strip(text), "\n", " ")
		}
		if unescapeClosingSB && strings.Contains(text, "]") {
			text = strings.ReplaceAll(text, `\]`, "]")
		}
	}
	return text
}

// prepareSourceString splits data into lines, removing a BOM and trailing
// whitespace (or just the line ending when trimEnd is false).
func prepareSourceString(data string, trimEnd bool) []string {
	if data == "" {
		return nil
	}
	data = strings.TrimPrefix(data, "\xef\xbb\xbf")
	if data == "" {
		return nil
	}
	var lines []string
	for len(data) > 0 {
		i := strings.IndexByte(data, '\n')
		var line string
		if i < 0 {
			line, data = data, ""
		} else {
			line, data = data[:i+1], data[i+1:]
		}
		if trimEnd {
			line = rstrip(line)
		} else {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
		}
		lines = append(lines, line)
	}
	return lines
}
