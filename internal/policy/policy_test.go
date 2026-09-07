package policy

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/robotsmith/internal/advise"
)

const valid = `{
  "schema": "robotsmith.policy/1",
  "rules": [
    {"pattern": "bytespider", "policy": "allow", "why": "we license our content to them"},
    {"pattern": "internal-indexer", "family": "search", "policy": "allow"},
    {"pattern": "scrapy", "policy": "block", "why": "no user behind it"}
  ]
}`

func TestParseReadsTheOverrides(t *testing.T) {
	p, err := Parse([]byte(valid), "policy.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(p.Rules))
	}
	fam, pol, why, ok := p.Lookup("Mozilla/5.0 (compatible; Bytespider)")
	if !ok {
		t.Fatal("Bytespider must match the first rule")
	}
	if pol != advise.Allow {
		t.Errorf("policy = %v, expected allow: the file is the whole point of overriding", pol)
	}
	if why == "" || !strings.Contains(why, "license") {
		t.Errorf("why = %q: the file's own reason must reach the report", why)
	}
	// No family stated: the default classification stands, only the policy is overridden.
	if fam != advise.Unknown {
		t.Errorf("family = %v, expected it left alone (Unknown means \"not stated here\")", fam)
	}
	if fam2, _, _, _ := p.Lookup("internal-indexer/1.0"); fam2 != advise.Search {
		t.Errorf("family = %v, expected search: a stated family must be honoured", fam2)
	}
	if _, _, _, ok := p.Lookup("Mozilla/5.0 (compatible; Googlebot/2.1)"); ok {
		t.Error("a UA no rule mentions must fall through to the built-in table")
	}
}

func TestParseRejectsWhatWouldFailSilently(t *testing.T) {
	cases := map[string]string{
		"unknown top-level key": `{"schema":"robotsmith.policy/1","rulez":[]}`,
		"unknown rule key":      `{"schema":"robotsmith.policy/1","rules":[{"pattern":"x","polcy":"allow"}]}`,
		"unknown policy":        `{"schema":"robotsmith.policy/1","rules":[{"pattern":"x","policy":"maybe"}]}`,
		"unknown family":        `{"schema":"robotsmith.policy/1","rules":[{"pattern":"x","family":"robots","policy":"allow"}]}`,
		"missing pattern":       `{"schema":"robotsmith.policy/1","rules":[{"policy":"allow"}]}`,
		"missing policy":        `{"schema":"robotsmith.policy/1","rules":[{"pattern":"x"}]}`,
		"broken regex":          `{"schema":"robotsmith.policy/1","rules":[{"pattern":"x(","policy":"allow"}]}`,
		"wrong schema":          `{"schema":"robotsmith.policy/2","rules":[]}`,
		"missing schema":        `{"rules":[{"pattern":"x","policy":"allow"}]}`,
		"not json":              `pattern: x`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body), "policy.json"); err == nil {
				t.Error("must be rejected: a policy file that half-works is worse than none")
			}
		})
	}
}

func TestParseRejectsAnUnreachableRule(t *testing.T) {
	// ⚠️ First match wins here as it does in the built-in table, so a rule hidden behind an earlier
	// one is dead config — and dead config reads like an applied decision.
	shadowed := `{"schema":"robotsmith.policy/1","rules":[
	  {"pattern":"bot","policy":"block"},
	  {"pattern":"googlebot","policy":"allow"}
	]}`
	_, err := Parse([]byte(shadowed), "policy.json")
	if err == nil {
		t.Fatal("a rule shadowed by an earlier pattern must be an error")
	}
	if !strings.Contains(err.Error(), "googlebot") || !strings.Contains(err.Error(), "bot") {
		t.Errorf("the error must name both patterns so it is actionable: %v", err)
	}
}

func TestErrorsCarryTheFileAndTheRuleIndex(t *testing.T) {
	// A message with no location sends whoever wrote the file hunting through it.
	_, err := Parse([]byte(`{"schema":"robotsmith.policy/1","rules":[{"pattern":"x","policy":"nope"}]}`), "my-policy.json")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "my-policy.json") || !strings.Contains(err.Error(), "rules[0]") {
		t.Errorf("error = %v, expected the file name and the rule index", err)
	}
}
