package advise

import (
	"strings"
	"testing"
)

func TestComparisonNamesWhatChanged(t *testing.T) {
	prev := Snapshot{Total: 100000, Shares: map[string]float64{
		"GPTBot":       2.0, // shrunk
		"Bytespider":   1.0, // grew
		"OldBot":       3.0, // vanished
		"SteadySpider": 5.0, // unchanged
	}}
	a := AnalyzeWith([]Observation{
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 1000},       // 1%
		{UA: "Mozilla/5.0 (compatible; Bytespider)", Requests: 4000},       // 4%
		{UA: "Mozilla/5.0 (compatible; SteadySpider/1.0)", Requests: 5000}, // 5%
		{UA: "Mozilla/5.0 (compatible; NewSpider/1.0)", Requests: 2000},    // 2%, new
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 88000},
	}, Options{Previous: &prev})

	if a.Comparison == nil {
		t.Fatal("a comparison was asked for: it must be part of the advice")
	}
	byName := map[string]Movement{}
	for _, m := range a.Comparison.Movements {
		byName[m.Name] = m
	}
	for name, want := range map[string]Direction{
		"Bytespider":   Grew,
		"GPTBot":       Shrank,
		"NewSpider":    Appeared,
		"OldBot":       Vanished,
		"SteadySpider": Steady,
	} {
		got, ok := byName[name]
		if !ok {
			t.Errorf("%s: no movement reported", name)
			continue
		}
		if got.Direction != want {
			t.Errorf("%s: direction = %v, expected %v (was %.1f%%, now %.1f%%)",
				name, got.Direction, want, got.Was, got.Now)
		}
	}
	if got := byName["Bytespider"]; got.Factor < 3.9 || got.Factor > 4.1 {
		t.Errorf("Bytespider factor = %.2f, expected ×4 (1%% → 4%%)", got.Factor)
	}
	// A crawler that vanished has a `Now` of zero and must not be presented as advice: there is
	// nothing to block any more.
	if got := byName["OldBot"]; got.Now != 0 || got.Was != 3.0 {
		t.Errorf("OldBot = %+v", got)
	}
}

func TestASmallButGrowingUnknownCrawlerIsPromotedToReview(t *testing.T) {
	// ⚠️ The reason this feature exists: 0.4% flat for a year and 0.4% tripling this month get the
	// same answer from a share alone — and the second one is the decision worth taking.
	prev := Snapshot{Total: 100000, Shares: map[string]float64{"CreepSpider": 0.1}}
	a := AnalyzeWith([]Observation{
		{UA: "Mozilla/5.0 (compatible; CreepSpider/1.0)", Requests: 400}, // 0.4%: below MinCandidateShare
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 99600},
	}, Options{Previous: &prev})

	var d *Decision
	for i := range a.Decisions {
		if a.Decisions[i].Name == "CreepSpider" {
			d = &a.Decisions[i]
		}
	}
	if d == nil {
		t.Fatalf("CreepSpider grew ×4 and must be raised for review even at %.1f%%: %+v",
			0.4, a.Decisions)
	}
	if d.Policy != Candidate {
		t.Errorf("policy = %v, expected Candidate", d.Policy)
	}
	if !strings.Contains(d.Why, "×4") && !strings.Contains(d.Why, "x4") {
		t.Errorf("why = %q: the growth is the reason, so it must be the reason given", d.Why)
	}
}

func TestASmallSteadyCrawlerStaysIgnored(t *testing.T) {
	// The other half of the same rule: without growth, a tiny unknown crawler is still not worth
	// a line in the file.
	prev := Snapshot{Total: 100000, Shares: map[string]float64{"TinySpider": 0.35}}
	a := AnalyzeWith([]Observation{
		{UA: "Mozilla/5.0 (compatible; TinySpider/1.0)", Requests: 400},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 99600},
	}, Options{Previous: &prev})
	for _, d := range a.Decisions {
		if d.Name == "TinySpider" {
			t.Errorf("TinySpider is flat at 0.4%%: it must stay out of the advice, got %+v", d)
		}
	}
}

func TestComparisonWithoutAPreviousSnapshotIsAbsent(t *testing.T) {
	a := Analyze([]Observation{{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 10}})
	if a.Comparison != nil {
		t.Error("nothing to compare against: the field must be absent, not empty")
	}
}
