package advise

import (
	"strings"
	"testing"
)

func TestClassificationOrder(t *testing.T) {
	// "Googlebot" contains "bot": if the generic rule won, we would block Google.
	cases := map[string]Family{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)": Search,
		"facebookexternalhit/1.1": Social,
		"Mozilla/5.0 AppleWebKit (compatible; ChatGPT-User/1.0; +openai.com/bot)": AIUser,
		"Mozilla/5.0 (compatible; GPTBot/1.4; +openai.com/gptbot)":                AITrain,
		"Mozilla/5.0 (compatible; SemrushBot/7~bl)":                               SEO,
		"Mozilla/5.0 (compatible; YisouSpider/5.0; +http://www.yisou.com)":        Unknown,
		"okhttp/5.4.0":                                         Tool,
		"SomeApp/1 CFNetwork/3860 Darwin/25.6.0":               App,
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/148": Browser,
		"Amazon-Route53-Health-Check-Service":                  Tool,
	}
	for ua, want := range cases {
		if fam, _ := Classify(ua); fam != want {
			t.Errorf("%.44s → %v, expected %v", ua, fam, want)
		}
	}
}

func TestNameExtractedFromUnknownUA(t *testing.T) {
	_, name := Classify("Mozilla/5.0 (compatible; YisouSpider/5.0; +http://www.yisou.com/help)")
	if name != "YisouSpider" {
		t.Errorf("expected YisouSpider as the token to write in the file, got %q", name)
	}
}

func TestHeavyUnknownCrawlerBecomesCandidate(t *testing.T) {
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (compatible; YisouSpider/5.0)", Requests: 5000},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 95000},
	})
	var d *Decision
	for i := range a.Decisions {
		if a.Decisions[i].Name == "YisouSpider" {
			d = &a.Decisions[i]
		}
	}
	if d == nil {
		t.Fatal("YisouSpider must appear among the decisions")
	}
	if d.Policy != Candidate {
		t.Errorf("policy = %v, expected Candidate (unknown but at 5%%)", d.Policy)
	}
	// at 5% it is above AutoBlockShare, so the file must carry it ACTIVE (not commented out)
	out := Render(a, "", "example.com")
	if !strings.Contains(out, "\nUser-agent: YisouSpider\nDisallow: /\n") {
		t.Error("a candidate above 3% must be written active, with the number next to it")
	}
}

func TestSmallCandidateComesOutCommented(t *testing.T) {
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (compatible; SomeSpider/1.0)", Requests: 700},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 99300},
	})
	out := Render(a, "", "example.com")
	if !strings.Contains(out, "# User-agent: SomeSpider") {
		t.Error("below 3% the candidate must be proposed COMMENTED OUT, not applied by default")
	}
}

func TestExistingRulesKeptAndOrphansRecovered(t *testing.T) {
	existing := "User-agent: *\nDisallow: /admin/\n\nDisallow: /product/\n"
	a := Analyze([]Observation{{UA: "GPTBot/1.4", Requests: 100}})
	out := Render(a, existing, "example.com")
	if !strings.Contains(out, "Disallow: /admin/") {
		t.Error("existing rules must be carried over")
	}
	if !strings.Contains(out, "Disallow: /product/") {
		t.Error("the ORPHANED rules of the previous file must be recovered, not lost")
	}
	if !strings.Contains(out, "ORPHANED") {
		t.Error("the recovery must be flagged to the reader")
	}
}

func TestSitemapOnAnotherHostIsDropped(t *testing.T) {
	existing := "User-agent: *\nDisallow:\nSitemap: https://othersite.example/sitemap.xml\n"
	out := Render(Analyze(nil), existing, "example.com")
	if strings.Contains(out, "\nSitemap: https://othersite.example") {
		t.Error("a cross-domain sitemap must not be carried over")
	}
	if !strings.Contains(out, "Sitemap dropped") {
		t.Error("dropping it must be explained")
	}
}

func TestWarningStatesTheLimitOfTheMethod(t *testing.T) {
	// The warning must not ACCUSE browser UAs (one common string stands for thousands of people):
	// it must say that on that slice the method cannot reach a verdict.
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (X11; Linux x86_64) Firefox/147.0", Requests: 30000},
		{UA: "Mozilla/5.0 (Windows NT 10.0) Chrome/148", Requests: 70000},
	})
	if len(a.Warnings) != 1 {
		t.Fatalf("expected 1 overall warning, found %d", len(a.Warnings))
	}
	if !strings.Contains(a.Warnings[0], "per-IP rate") {
		t.Error("the warning must explain WHAT is missing to judge, not accuse")
	}
}

func TestGenericTokenIsDiscarded(t *testing.T) {
	// From "... crawler ..." we must not derive the token `crawler`: it blocks nothing in a robots.txt.
	_, name := Classify("Mozilla/5.0 (compatible; some crawler; +http://example.com)")
	if name == "crawler" {
		t.Error("`crawler` is a generic word, not the name of a crawler")
	}
}

func TestGeneratedFileHasNoDefectsTheLinterWouldFlag(t *testing.T) {
	// The generator must not produce a file its own linter rejects: no blank line inside a group,
	// and never `Allow: /` before the Disallow rules of the same group.
	a := Analyze([]Observation{
		{UA: "Mozilla/5.0 (compatible; Googlebot/2.1)", Requests: 1000},
		{UA: "Mozilla/5.0 (compatible; GPTBot/1.4)", Requests: 500},
	})
	out := Render(a, "User-agent: *\nDisallow: /admin/\n", "example.com")
	group := false
	for i, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		switch {
		case l == "":
			group = false
		case strings.HasPrefix(strings.ToLower(l), "user-agent:"):
			group = true
		case strings.HasPrefix(strings.ToLower(l), "allow:"), strings.HasPrefix(strings.ToLower(l), "disallow:"):
			if !group {
				t.Errorf("line %d is orphaned in the generated file: %q", i+1, l)
			}
		}
	}
}

// sampleFor derives a user-agent that must match `pattern`, so the test does not need a second
// hand-maintained list of examples (which would drift from the table it is meant to check).
// ⚠️ An anchored branch (`^node$`) means the whole UA is that literal: wrapping it in a Mozilla
// comment would make a correct rule look shadowed.
func sampleFor(pattern string) string {
	first := strings.Split(pattern, "|")[0]
	if strings.HasPrefix(first, "^") && strings.HasSuffix(first, "$") {
		return strings.Trim(first, "^$")
	}
	first = strings.NewReplacer("^", "", "$", "", "-?", "-").Replace(first)
	return "Mozilla/5.0 (compatible; " + first + "/1.0)"
}

func TestRulesExposesTheWholeTableAndNothingIsShadowed(t *testing.T) {
	// ⚠️ The whole table is order-dependent: a pattern added above a more specific one silently
	// swallows it, and the swallowed rule becomes dead code nobody notices. Every rule must still
	// be reachable through Classify.
	rules := Rules()
	if len(rules) < 40 {
		t.Fatalf("expected the full table, got %d rules", len(rules))
	}
	for _, r := range rules {
		ua := sampleFor(r.Pattern)
		fam, token := Classify(ua)
		if fam != r.Family {
			t.Errorf("%q (from %q) classifies as %v, but its own rule says %v: an earlier pattern is "+
				"shadowing it", ua, r.Pattern, fam, r.Family)
		}
		if token != r.Token {
			t.Errorf("%q: token = %q, its rule says %q", ua, token, r.Token)
		}
		if r.Policy == Block && r.Why == "" {
			t.Errorf("rule %q blocks with no reason given", r.Pattern)
		}
	}
}

func TestRulesSaysWhatHappensToAnUnknownCrawler(t *testing.T) {
	// The unknown catch-all has no single policy: it depends on volume. Saying "ignore" would be a
	// lie and saying "block" would be worse.
	for _, r := range Rules() {
		if r.Family != Unknown {
			continue
		}
		if !strings.Contains(r.Why, "%") {
			t.Errorf("the unknown rule must explain the volume threshold, got %q", r.Why)
		}
		if r.Policy != Candidate {
			t.Errorf("the unknown rule's policy = %v, expected Candidate (a person decides)", r.Policy)
		}
	}
}
