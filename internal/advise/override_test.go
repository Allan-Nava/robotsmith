package advise

import (
	"strings"
	"testing"
)

// stubOverride stands in for a parsed policy file, so this package keeps knowing nothing about
// JSON or files.
type stubOverride map[string]struct {
	fam  Family
	pol  Policy
	why  string
	rule string
}

func (s stubOverride) Override(ua string) (Family, Policy, string, string, bool) {
	for needle, v := range s {
		if strings.Contains(strings.ToLower(ua), needle) {
			return v.fam, v.pol, v.why, v.rule, true
		}
	}
	return Unknown, Ignore, "", "", false
}

func TestAnOverrideBeatsTheBuiltInTable(t *testing.T) {
	// The point of the file: the site's answer wins, and the report says whose answer it was.
	ov := stubOverride{"bytespider": {pol: Allow, why: "we license our content to them", rule: "bytespider"}}
	a := AnalyzeWith([]Observation{
		{UA: "Mozilla/5.0 (compatible; Bytespider)", Requests: 1000},
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 1000},
	}, Options{Override: ov})

	byName := map[string]Decision{}
	for _, d := range a.Decisions {
		byName[d.Name] = d
	}
	got := byName["Bytespider"]
	if got.Policy != Allow {
		t.Errorf("Bytespider policy = %v, expected allow: the file overrides the default block", got.Policy)
	}
	if !strings.Contains(got.Why, "license") {
		t.Errorf("why = %q, expected the file's own reason", got.Why)
	}
	if got.Rule != "bytespider" || !got.FromPolicy {
		t.Errorf("decision = %+v: the report must say which rule fired, and that it came from the file", got)
	}
	// Untouched crawlers keep the built-in answer, and are marked as such.
	if gpt := byName["GPTBot"]; gpt.Policy != Block || gpt.FromPolicy {
		t.Errorf("GPTBot = %+v, expected the default block, not from the file", gpt)
	}
}

func TestAnOverrideCanReclassifyAnUnknownCrawler(t *testing.T) {
	// The useful case for a small site: "this thing nobody has heard of is our own indexer".
	ov := stubOverride{"internal-indexer": {fam: Search, pol: Allow, why: "our own", rule: "internal-indexer"}}
	a := AnalyzeWith([]Observation{
		{UA: "internal-indexer/1.0", Requests: 100},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 900},
	}, Options{Override: ov})
	var found bool
	for _, d := range a.Decisions {
		if d.Name != "internal-indexer" {
			continue
		}
		found = true
		if d.Family != Search || d.Policy != Allow {
			t.Errorf("decision = %+v, expected search/allow", d)
		}
	}
	if !found {
		t.Fatal("an overridden UA must produce a decision even when the table would ignore it")
	}
}

func TestAnOverrideToIgnoreDropsItFromTheFile(t *testing.T) {
	// "Stop telling me about this one" has to be expressible, or the file grows noise nobody reads.
	ov := stubOverride{"semrush": {pol: Ignore, rule: "semrush"}}
	a := AnalyzeWith([]Observation{
		{UA: "Mozilla/5.0 (compatible; SemrushBot/7~bl)", Requests: 500},
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 500},
	}, Options{Override: ov})
	for _, d := range a.Decisions {
		if strings.EqualFold(d.Name, "SemrushBot") {
			t.Errorf("an ignored crawler must not reach the file: %+v", d)
		}
	}
	if !strings.Contains(Render(a, "", "example.com"), "GPTBot") {
		t.Error("the rest of the advice must be unaffected")
	}
}

func TestAnalyzeStillWorksWithoutAPolicy(t *testing.T) {
	// Analyze is the call everything already makes: it must keep meaning "no overrides".
	a := Analyze([]Observation{{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 10}})
	if len(a.Decisions) != 1 || a.Decisions[0].Policy != Block || a.Decisions[0].FromPolicy {
		t.Errorf("decisions = %+v", a.Decisions)
	}
}
