package adocgo

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jdkato/adocgo/internal/rx"
)

// allPatterns returns every regular expression used by the package.
func allPatterns() []*rx.Regexp {
	res := []*rx.Regexp{
		authorInfoLineRx, authorDelimiterRx, revisionInfoLineRx, manpageTitleVolnumRx, manpageNamePurposeRx,
		conditionalDirectiveRx, evalExpressionRx, includeDirectiveRx, tagDirectiveRx, attributeEntryRx,
		invalidAttributeNameCharsRx, attributeEntryPassMacroRx, attributeReferenceRx, blockAnchorRx,
		blockAttributeListRx, blockAttributeLineRx, blockTitleRx, admonitionParagraphRx, literalParagraphRx,
		extAtxSectionTitleRx, setextSectionTitleRx, inlineSectionAnchorRx, invalidSectionIDCharsRx,
		sectionLevelStyleRx, anyListRx, unorderedListRx, orderedListRx, descriptionListRx, calloutListRx,
		calloutExtractRx, calloutScanRx, calloutSourceRx, columnSpecRx, cellSpecStartRx, cellSpecEndRx,
		blockMediaMacroRx, blockTocMacroRx, inlineAnchorRx, inlineAnchorScanRx, leadingInlineAnchorRx,
		inlineBiblioAnchorRx, inlineEmailRx, inlineFootnoteMacroRx, inlineImageMacroRx, inlineIndextermMacroRx,
		inlineKbdBtnMacroRx, inlineLinkRx, inlineLinkMacroRx, inlineStemMacroRx, inlineMenuMacroRx, inlineMenuRx,
		inlinePassRx, inlinePassCompatRx, inlinePassMacroRx, inlineXrefMacroRx, hardLineBreakRx,
		markdownThematicBreakRx, extLayoutBreakRx, blankLineRx, escapedSpaceRx, replaceableTextRx,
		spaceDelimiterRx, trailingDigitsRx, uriSniffRx, xmlSanitizeRx, dropAnchorRx, leadingAnchorsRx,
		stemBreakRx, svgPreambleRx, svgStartTagRx, dimensionAttributeRx, alBoundaryQuot, alBoundaryApos,
		alBoundaryComma, alNameRx, alBlankRx, alSkipRx, quotedTextSniffRx, quotedTextSniffCompatRx,
	}
	for _, k := range []string{"::", ":::", "::::", ";;"} {
		res = append(res, descriptionListSiblingRx[k])
	}
	for _, k := range orderedListStyles {
		res = append(res, orderedListMarkerRx[k])
	}
	for _, q := range quoteSubs {
		res = append(res, q.re)
	}
	for _, q := range quoteSubsCompat {
		res = append(res, q.re)
	}
	for _, r := range replacements {
		res = append(res, r.re)
	}
	return res
}

type rxCase struct {
	Pattern string   `json:"pattern"`
	Flags   string   `json:"flags"`
	Results []string `json:"results,omitempty"`
}

type rxCompat struct {
	Inputs []string `json:"inputs"`
	Cases  []rxCase `json:"cases"`
}

const rxCompatFile = "testdata/rx-compat.json"

// rxInputs picks a deterministic sample of lines (and a few multi-line
// chunks) from the feature fixtures to run every pattern against.
func rxInputs(t *testing.T) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && len(s) < 400 && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	files := corpusFiles(t)
	sort.Strings(files)
	for _, f := range files {
		if !strings.Contains(f, "/features/") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(data), "\n")
		for i, l := range lines {
			if i%3 == 0 {
				add(l)
				add(subSpecialchars(l))
			}
			if i%17 == 0 && i+2 < len(lines) {
				add(strings.Join(lines[i:i+3], "\n"))
			}
		}
	}
	return out
}

func rxResult(re *rx.Regexp, in string) string {
	var sb strings.Builder
	for n, m := range re.FindAll(in) {
		if n == 3 {
			break
		}
		if n > 0 {
			sb.WriteByte(';')
		}
		for i := 0; i <= re.NumGroups(); i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			if m.Has(i) {
				sb.WriteString(strconv.Itoa(m.Start(i)) + "-" + strconv.Itoa(m.End(i)))
			} else {
				sb.WriteByte('n')
			}
		}
	}
	return sb.String()
}

// TestRxCompat compares the regular expression engine with results recorded
// from Ruby (see scripts/regen_rx.rb) for every pattern used by the
// converter. Set ADOCGO_WRITE_RX=1 to (re)write the pattern and input lists
// before regenerating the expected results with Ruby.
func TestRxCompat(t *testing.T) {
	if os.Getenv("ADOCGO_WRITE_RX") != "" {
		c := rxCompat{Inputs: rxInputs(t)}
		seen := map[string]bool{}
		for _, re := range allPatterns() {
			key := re.Flags() + "/" + re.String()
			if !seen[key] {
				seen[key] = true
				c.Cases = append(c.Cases, rxCase{Pattern: re.String(), Flags: re.Flags()})
			}
		}
		data, _ := json.Marshal(c)
		if err := os.WriteFile(rxCompatFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d patterns and %d inputs; now run scripts/regen.sh rx", len(c.Cases), len(c.Inputs))
		return
	}
	data, err := os.ReadFile(rxCompatFile)
	if err != nil {
		t.Fatal(err)
	}
	var c rxCompat
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, re := range allPatterns() {
		have = append(have, re.Flags()+"/"+re.String())
	}
	var recorded []string
	checked := 0
	for _, cs := range c.Cases {
		recorded = append(recorded, cs.Flags+"/"+cs.Pattern)
		if len(cs.Results) != len(c.Inputs) {
			t.Fatalf("pattern %q: no recorded results (run scripts/regen.sh rx)", cs.Pattern)
		}
		re := rx.MustCompile(cs.Pattern, cs.Flags)
		for i, in := range c.Inputs {
			checked++
			if got := rxResult(re, in); got != cs.Results[i] {
				t.Errorf("pattern %q (flags %q) on %q:\n  got  %s\n  ruby %s", cs.Pattern, cs.Flags, in, got, cs.Results[i])
			}
		}
	}
	sort.Strings(have)
	have = uniqStrings(have)
	sort.Strings(recorded)
	if !reflect.DeepEqual(have, recorded) {
		t.Errorf("recorded pattern list is out of date (run ADOCGO_WRITE_RX=1 go test -run TestRxCompat . && scripts/regen.sh rx)")
	}
	t.Logf("checked %d pattern/input pairs (%d patterns, %d inputs)", checked, len(c.Cases), len(c.Inputs))
}

func uniqStrings(s []string) []string {
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}
