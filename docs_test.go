package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

func TestWorkflowInputsDoNotUseShellVariables(t *testing.T) {
	// ⚠️ This cost two failed releases. A value under `with:` is read by the Actions expression
	// engine, not by a shell: `${RUNNER_TEMP}` is passed through verbatim and the tool receives a
	// path with a literal dollar sign in it. Only `${{ runner.temp }}` is substituted. The failure
	// is invisible in review — the line looks like every shell line around it — and invisible until
	// a tag is pushed, which is the most expensive moment to find out.
	entries, err := os.ReadDir(".github/workflows")
	if err != nil {
		t.Fatal(err)
	}
	// A `with:` block runs until a line dedents out of it. Good enough for these files, and a
	// false positive here is a line worth looking at anyway.
	shellVar := regexp.MustCompile(`\$\{[^{]`)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(".github/workflows", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		inWith, indent := false, 0
		for i, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			lead := len(line) - len(trimmed)
			if inWith && lead <= indent {
				inWith = false
			}
			if trimmed == "with:" || strings.HasPrefix(trimmed, "with:") {
				inWith, indent = true, lead
				continue
			}
			if inWith && shellVar.MatchString(line) {
				t.Errorf("%s:%d passes a shell variable to an action input:\n\t%s\n"+
					"`with:` is not a shell — use ${{ runner.temp }}, ${{ github.ref_name }} and so on",
					e.Name(), i+1, strings.TrimSpace(line))
			}
		}
	}
}

// advertisedActionRef pulls the `uses: Allan-Nava/robotsmith@<ref>` that a document tells people to
// copy. Three files carry it and they must agree: two of them are the first thing a newcomer reads.
var reAdvertisedRef = regexp.MustCompile(`uses: Allan-Nava/robotsmith@(\S+)`)

func advertisedActionRefs(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range reAdvertisedRef.FindAllStringSubmatch(string(body), -1) {
		out = append(out, strings.TrimRight(m[1], "\"'`<"))
	}
	if len(out) == 0 {
		t.Fatalf("%s advertises no `uses: Allan-Nava/robotsmith@…` line: the three-line example is "+
			"the first thing a reader copies", path)
	}
	return out
}

// newestReleasedMajor reads the major version out of the newest released CHANGELOG section. The
// CHANGELOG is the only in-repo statement of what version this project is at, and deriving the
// expected reference from it is the same trick as counting the check cases from the tables rather
// than typing a number that goes stale.
func newestReleasedMajor(t *testing.T) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^## \[(\d+)\.\d+\.\d+\]`)
	m := re.FindStringSubmatch(readDoc(t, "CHANGELOG.md"))
	if m == nil {
		t.Fatal("CHANGELOG.md has no released `## [X.Y.Z]` section to take the major from")
	}
	return "v" + m[1]
}

func TestTheActionReferenceTheDocsAdvertiseIsTheOneWeShip(t *testing.T) {
	// ⚠️ This shipped broken: every document said `@v1` while the newest release was 0.5.1 and no
	// `v1` tag had ever existed, so the three-line example failed for anybody who copied it. The
	// repo could not notice because its own CI used `./` — it never ran the reference it recommends
	// to everyone else.
	want := newestReleasedMajor(t)
	// ⚠️ ci.yml is in the list because it is the one place that actually RUNS the reference: a
	// document and a workflow drifting apart is how the advertised one went untested.
	for _, path := range []string{"README.md", "docs/index.html", "action.yml", ".github/workflows/ci.yml"} {
		for _, got := range advertisedActionRefs(t, path) {
			if got != want {
				t.Errorf("%s advertises `@%s` but the newest release is %s.x, so the major tag we "+
					"maintain is `%s`: a reference that does not exist fails on first use",
					path, got, want, want)
			}
		}
	}
}

func TestTheAdvertisedMajorTagIsActuallyMaintained(t *testing.T) {
	// A moving major tag is a promise. If nothing moves it, the documents point at a tag that is
	// either missing or frozen on an old release — which is worse than pinning, because it looks
	// maintained.
	release := readDoc(t, ".github/workflows/release.yml")
	if !strings.Contains(release, "major") {
		t.Fatal("release.yml does not maintain the major tag the documents advertise: " +
			"either move it there, or stop advertising it")
	}
	for _, needed := range []string{"git tag -f", "git push"} {
		if !strings.Contains(release, needed) {
			t.Errorf("release.yml has no %q: the major tag has to be force-moved and pushed, "+
				"or it stays on the release it was first cut at", needed)
		}
	}
}
