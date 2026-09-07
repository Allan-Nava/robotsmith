// Package policy reads the optional policy file: a site's own answer where it disagrees with the
// built-in table.
//
// Why it exists: the classification table is an opinion — a defensible one, documented as such —
// but a news site and a shop do not owe each other the same answer about a given crawler, and a
// team that disagrees should not have to fork the binary. A file also makes the disagreement
// *written down*: it says who decided what, and the advice explains itself against it.
//
// ⚠️ JSON and not YAML: a YAML parser would be a dependency, and zero dependencies outside the
// stdlib is an invariant of this repo.
//
// This package does no I/O: the caller reads the bytes. It is strict on purpose — an unknown key,
// an unknown value or a rule that can never match is an error, because a policy file that
// half-works reads like an applied decision and is not one.
package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Allan-Nava/robotsmith/internal/advise"
)

// Schema is the only version this parser accepts. A breaking change bumps it.
const Schema = "robotsmith.policy/1"

// Rule is one override: the first pattern that matches a user-agent wins, as in the built-in table.
type Rule struct {
	Pattern string
	Family  advise.Family // Unknown means "not stated here": keep the built-in classification
	HasFam  bool
	Policy  advise.Policy
	Why     string
	re      *regexp.Regexp
}

// Policy is a parsed policy file.
type Policy struct {
	Source string
	Rules  []Rule
}

// wire mirrors the file exactly, so unknown keys can be rejected rather than ignored.
type wire struct {
	Schema string `json:"schema"`
	Rules  []struct {
		Pattern string `json:"pattern"`
		Family  string `json:"family"`
		Policy  string `json:"policy"`
		Why     string `json:"why"`
	} `json:"rules"`
}

var (
	families = map[string]advise.Family{
		"search": advise.Search, "social": advise.Social, "ai-user": advise.AIUser,
		"ai-training": advise.AITrain, "seo": advise.SEO, "tool": advise.Tool,
		"app": advise.App, "browser": advise.Browser, "unknown": advise.Unknown,
	}
	policies = map[string]advise.Policy{
		"allow": advise.Allow, "block": advise.Block, "review": advise.Candidate,
		"ignore": advise.Ignore,
	}
)

// Parse validates and compiles the file. `source` is only used in the error messages, which name
// the file and the rule index so whoever wrote it does not have to hunt.
func Parse(b []byte, source string) (*Policy, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields() // an ignored typo is a decision that never took effect
	var w wire
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	if w.Schema != Schema {
		return nil, fmt.Errorf("%s: schema is %q, this build understands %q", source, w.Schema, Schema)
	}
	p := &Policy{Source: source}
	for i, r := range w.Rules {
		where := fmt.Sprintf("%s: rules[%d]", source, i)
		if strings.TrimSpace(r.Pattern) == "" {
			return nil, fmt.Errorf("%s: needs a `pattern`", where)
		}
		if r.Policy == "" {
			return nil, fmt.Errorf("%s (%s): needs a `policy` (%s)", where, r.Pattern, keys(policies))
		}
		pol, ok := policies[r.Policy]
		if !ok {
			return nil, fmt.Errorf("%s (%s): unknown policy %q, valid: %s", where, r.Pattern, r.Policy, keys(policies))
		}
		rule := Rule{Pattern: r.Pattern, Policy: pol, Why: r.Why}
		if r.Family != "" {
			fam, ok := families[r.Family]
			if !ok {
				return nil, fmt.Errorf("%s (%s): unknown family %q, valid: %s", where, r.Pattern, r.Family, keys(families))
			}
			rule.Family, rule.HasFam = fam, true
		}
		re, err := regexp.Compile(strings.ToLower(r.Pattern))
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", where, r.Pattern, err)
		}
		rule.re = re
		// ⚠️ First match wins, so a rule an earlier pattern already swallows can never fire. Dead
		// config reads like an applied decision, which is the dangerous kind of wrong.
		for j, earlier := range p.Rules {
			if earlier.re.MatchString(strings.ToLower(r.Pattern)) {
				return nil, fmt.Errorf("%s: rule %q can never match: rules[%d] (%q) already covers it — "+
					"put the specific pattern first", source, r.Pattern, j, earlier.Pattern)
			}
		}
		p.Rules = append(p.Rules, rule)
	}
	return p, nil
}

// Lookup returns the override for a user-agent, if the file has one. `ok` false means "the file has
// nothing to say about this one": the built-in table decides.
func (p *Policy) Lookup(ua string) (advise.Family, advise.Policy, string, bool) {
	fam, pol, why, _, ok := p.Override(ua)
	return fam, pol, why, ok
}

// Override implements advise.Override: same answer, plus the pattern that produced it so the
// report can say which line of the file fired.
func (p *Policy) Override(ua string) (advise.Family, advise.Policy, string, string, bool) {
	if p == nil {
		return advise.Unknown, advise.Ignore, "", "", false
	}
	l := strings.ToLower(ua)
	for _, r := range p.Rules {
		if r.re.MatchString(l) {
			return r.Family, r.Policy, r.Why, r.Pattern, true
		}
	}
	return advise.Unknown, advise.Ignore, "", "", false
}

// Patterns lists the rules in order, for the report that says which one fired.
func (p *Policy) Patterns() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Rules))
	for _, r := range p.Rules {
		out = append(out, r.Pattern)
	}
	return out
}

func keys[V any](m map[string]V) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// sorted, so an error message is stable and diffable
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return strings.Join(out, ", ")
}
