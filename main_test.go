package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Allan-Nava/robotsmith/internal/check"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReorderArgsMovesFlagsFirst(t *testing.T) {
	// `check domain --quiet` must behave like `check --quiet domain`: the stdlib flag package stops
	// at the first operand, which for a CLI is a needless stumble.
	got := reorderArgs([]string{"example.com", "--origin", "https://origin/robots.txt", "--quiet"})
	want := []string{"--origin", "https://origin/robots.txt", "--quiet", "example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs = %v, expected %v", got, want)
	}
}

func TestReorderArgsDoesNotStealTheOperandForBooleanFlags(t *testing.T) {
	// `--quiet` takes no value: `example.com` must stay an operand, not become its value.
	got := reorderArgs([]string{"--quiet", "example.com"})
	want := []string{"--quiet", "example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs = %v, expected %v", got, want)
	}
}

func TestReorderArgsAcceptsTheEqualsForm(t *testing.T) {
	got := reorderArgs([]string{"example.com", "--origin=https://origin/robots.txt"})
	want := []string{"--origin=https://origin/robots.txt", "example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs = %v, expected %v", got, want)
	}
}

func TestReadCounts(t *testing.T) {
	// the shape of `sort | uniq -c` output: variable leading spaces, UAs containing spaces
	p := writeTemp(t, "ua.txt", "  1204 Mozilla/5.0 (compatible; GPTBot/1.4)\n 12 okhttp/5.4.0\nbroken line\n\n")
	obs, err := readCounts(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations (malformed lines are dropped), got %d: %+v", len(obs), obs)
	}
	if obs[0].Requests != 1204 || obs[0].UA != "Mozilla/5.0 (compatible; GPTBot/1.4)" {
		t.Errorf("first observation = %+v", obs[0])
	}
	if obs[1].Requests != 12 || obs[1].UA != "okhttp/5.4.0" {
		t.Errorf("second observation = %+v", obs[1])
	}
}

func TestReadLogExtractsOnlyTheUserAgents(t *testing.T) {
	// nginx `combined`: the quoted fields are request line, referer and user-agent. The first two
	// must be dropped, otherwise the counts get polluted with URLs.
	log := `1.2.3.4 - - [01/Sep/2026:10:00:00 +0000] "GET /a HTTP/1.1" 200 12 "https://ref.example/x" "Mozilla/5.0 (compatible; GPTBot/1.4)"
1.2.3.5 - - [01/Sep/2026:10:00:01 +0000] "GET /b HTTP/1.1" 200 12 "-" "Mozilla/5.0 (compatible; GPTBot/1.4)"
1.2.3.6 - - [01/Sep/2026:10:00:02 +0000] "POST /c HTTP/1.1" 200 12 "-" "okhttp/5.4.0"
`
	obs, err := readLog(writeTemp(t, "access.log", log), nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int64{}
	for _, o := range obs {
		counts[o.UA] = o.Requests
	}
	if len(counts) != 2 {
		t.Fatalf("expected 2 distinct user-agents, got %d: %+v", len(counts), counts)
	}
	if counts["Mozilla/5.0 (compatible; GPTBot/1.4)"] != 2 {
		t.Errorf("expected GPTBot twice, got %d", counts["Mozilla/5.0 (compatible; GPTBot/1.4)"])
	}
	if counts["okhttp/5.4.0"] != 1 {
		t.Errorf("expected okhttp once, got %d", counts["okhttp/5.4.0"])
	}
	// descending order: the list is what tells you what is worth blocking first
	if obs[0].Requests < obs[len(obs)-1].Requests {
		t.Error("observations must be sorted by descending volume")
	}
}

func TestReadLocalFile(t *testing.T) {
	p := writeTemp(t, "robots.txt", "User-agent: *\nDisallow:\n")
	body, host, err := read(p)
	if err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Error("the content of the local file must be returned")
	}
	if host != "" {
		t.Errorf("a local file has no host (the Sitemap cannot be validated), got %q", host)
	}
}

func TestUsageStatesTheRealNumberOfCases(t *testing.T) {
	// The number of verified cases must come from the data, not from a literal in the help text:
	// a hardcoded count silently lies the first time a crawler is added to the tables.
	want := fmt.Sprintf("%d cases", len(check.MustPass)+len(check.MustBeBlocked))
	if !strings.Contains(usageText(), want) {
		t.Errorf("the usage text must state %q, got:\n%s", want, usageText())
	}
}

func TestEveryValueFlagIsKnownToTheArgumentReordering(t *testing.T) {
	// ⚠️ `reorderArgs` moves operands to the end, and to do that it must know which flags carry a
	// value: one that is missing gets the operand as its value instead. That is how
	// `lint file --format github` became `--format file`. A new value-flag must be added to
	// `takesValue`, and this is the check that says so.
	for cmd, fs := range allFlagSets() {
		fs.VisitAll(func(f *flag.Flag) {
			_, isBool := f.Value.(interface{ IsBoolFlag() bool })
			if isBool {
				return // booleans take no value: nothing to reorder
			}
			if !takesValue(f.Name) {
				t.Errorf("%s: --%s takes a value but takesValue() does not list it, so "+
					"`%s <operand> --%s <value>` misparses", cmd, f.Name, cmd, f.Name)
			}
		})
	}
}

func TestFlagsAndOperandInEitherOrder(t *testing.T) {
	// The whole point of the reordering: both spellings must behave the same.
	got := reorderArgs([]string{"robots.txt", "--strict", "--format", "github"})
	want := []string{"--strict", "--format", "github", "robots.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reorderArgs = %v, expected %v", got, want)
	}
}
