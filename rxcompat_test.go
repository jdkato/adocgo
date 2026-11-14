package adocgo

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jdkato/adocgo/internal/rx"
)

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

// TestRxCompat compares the regular expression engine (which the tests use
// as the oracle for the hand-written scanners) with results recorded from
// Ruby by scripts/regen_rx.rb for every pattern of Asciidoctor's rx.rb.
func TestRxCompat(t *testing.T) {
	data, err := os.ReadFile(rxCompatFile)
	if err != nil {
		t.Fatal(err)
	}
	var c rxCompat
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, cs := range c.Cases {
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
	t.Logf("checked %d pattern/input pairs (%d patterns, %d inputs)", checked, len(c.Cases), len(c.Inputs))
}
