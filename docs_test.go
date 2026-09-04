package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// reFlagToken matches a real flag mention and not the markdown around it (`---`, `-->`).
var reFlagToken = regexp.MustCompile(`^--[a-z][a-z0-9-]*$`)

// docFiles are the places that promise things about this CLI to people who never run --help.
var docFiles = []string{"README.md", "docs/index.html"}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(b)
}

// allFlagSets is what the binary really exposes. The docs are checked against this, never against
// a second hand-maintained list — that is the drift the "31 cases" bug came from.
func allFlagSets() map[string]*flag.FlagSet {
	var sink strings.Builder
	c, _ := checkFlags(&sink)
	l, _ := lintFlags(&sink)
	a, _ := adviseFlags(&sink)
	return map[string]*flag.FlagSet{"check": c, "lint": l, "advise": a}
}

func TestEveryFlagIsDocumented(t *testing.T) {
	// A flag nobody documents is a flag nobody uses — and one that gets removed by accident.
	for cmd, fs := range allFlagSets() {
		fs.VisitAll(func(f *flag.Flag) {
			needle := "--" + f.Name
			if !strings.Contains(usageText(), needle) {
				t.Errorf("%s: %s is missing from the usage text", cmd, needle)
			}
			if !strings.Contains(readDoc(t, "README.md"), needle) {
				t.Errorf("%s: %s is missing from README.md", cmd, needle)
			}
		})
	}
}

func TestReadmeDoesNotDocumentFlagsThatDoNotExist(t *testing.T) {
	// The other direction: a removed flag must not keep living in the docs.
	known := map[string]bool{}
	for _, fs := range allFlagSets() {
		fs.VisitAll(func(f *flag.Flag) { known["--"+f.Name] = true })
	}
	readme := readDoc(t, "README.md")
	start := strings.Index(readme, "<!-- flags:start -->")
	end := strings.Index(readme, "<!-- flags:end -->")
	if start < 0 || end < 0 {
		t.Fatal("README.md must delimit its flag list with <!-- flags:start --> / <!-- flags:end -->")
	}
	for _, word := range strings.Fields(strings.NewReplacer("`", " ", "|", " ", ",", " ").Replace(readme[start:end])) {
		if reFlagToken.MatchString(word) && !known[word] {
			t.Errorf("README.md documents %s, which the CLI does not expose", word)
		}
	}
}

func TestExitCodeContractIsDocumentedEverywhere(t *testing.T) {
	// The exit codes are the contract other people's pipelines depend on. One source of truth in
	// the code, checked against every document that repeats it.
	for _, doc := range docFiles {
		content := readDoc(t, doc)
		for _, ec := range exitCodes {
			if !strings.Contains(content, ec.meaning) {
				t.Errorf("%s does not document exit code %d (%q)", doc, ec.code, ec.meaning)
			}
			if !strings.Contains(content, fmt.Sprintf("%d", ec.code)) {
				t.Errorf("%s does not mention the number %d", doc, ec.code)
			}
		}
	}
	if !strings.Contains(usageText(), "Exit codes:") {
		t.Error("the usage text must state the exit codes")
	}
}

func TestJSONSchemasAreDocumented(t *testing.T) {
	// A consumer pins the schema string: it has to be findable without reading the source.
	readme := readDoc(t, "README.md")
	for _, s := range []string{"robotsmith.check/1", "robotsmith.lint/1", "robotsmith.advise/1"} {
		if !strings.Contains(readme, s) {
			t.Errorf("README.md does not document the schema %q", s)
		}
	}
}

func TestEveryWorkflowIsDocumented(t *testing.T) {
	// ⚠️ Automation nobody documented is automation nobody knows runs — and when it fails, nobody
	// knows what it was for. This is the gate for the rule "document everything": adding a workflow
	// without a row in CLAUDE.md's Automation table fails the build, which is more reliable than
	// remembering. (dogfood.yml and brew.yml were both added without one.)
	entries, err := os.ReadDir(".github/workflows")
	if err != nil {
		t.Fatal(err)
	}
	claude := readDoc(t, "CLAUDE.md")
	var seen int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		seen++
		if !strings.Contains(claude, e.Name()) {
			t.Errorf("CLAUDE.md does not mention %s: every workflow needs a row in the Automation "+
				"table saying what triggers it and what it does", e.Name())
		}
	}
	if seen < 5 {
		t.Errorf("expected the workflows to be there, found %d", seen)
	}
}
