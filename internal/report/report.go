// Package report renders the machine-readable form of what the commands found.
//
// Why it exists: the exit codes say *whether* something is wrong, the prose says *what* — and the
// prose is explicitly free to be reworded, so a pipeline that parsed it would break on a typo fix.
// This package is the stable half: one document per command, with a `schema` field a consumer can
// pin.
//
// ⚠️ The schema strings are a promise. A breaking change to a document — a field removed, a type
// changed, a meaning altered — means bumping its version, never editing the shape in place.
package report

import (
	"encoding/json"
	"io"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
)

const (
	SchemaCheck  = "robotsmith.check/1"
	SchemaLint   = "robotsmith.lint/1"
	SchemaAdvise = "robotsmith.advise/1"
)

// Finding is one structural defect, in the shape a CI annotation needs: severity, message, line.
type Finding struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
}

// Cache carries what the response said about its own freshness, when the CDN says anything at all.
type Cache struct {
	XCache string `json:"x_cache,omitempty"`
	Age    string `json:"age,omitempty"`
}

// CheckDoc is the document produced by `check --json`.
type CheckDoc struct {
	Schema   string    `json:"schema"`
	URL      string    `json:"url"`
	Path     string    `json:"path"`
	OK       bool      `json:"ok"`
	Cases    int       `json:"cases"`
	Failed   int       `json:"failed"`
	Deindex  bool      `json:"deindex_risk"`
	Problems []string  `json:"problems"`
	Findings []Finding `json:"findings"`
	Cache    *Cache    `json:"cache,omitempty"`
}

// LintDoc is the document produced by `lint --json`.
type LintDoc struct {
	Schema   string    `json:"schema"`
	Source   string    `json:"source"`
	OK       bool      `json:"ok"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Findings []Finding `json:"findings"`
}

// Decision is one crawler and what to do about it, with the reason in words.
type Decision struct {
	Name     string  `json:"name"`
	UA       string  `json:"user_agent"`
	Family   string  `json:"family"`
	Policy   string  `json:"policy"`
	Requests int64   `json:"requests"`
	Share    float64 `json:"share"`
	Why      string  `json:"why"`
}

// AdviseDoc is the document produced by `advise --json`. It carries the advised file too: a
// consumer must not have to run the command twice to get both the reasoning and the result.
type AdviseDoc struct {
	Schema      string     `json:"schema"`
	TotalReqs   int64      `json:"total_requests"`
	UserAgents  int        `json:"user_agents"`
	SavingShare float64    `json:"saving_share"`
	Decisions   []Decision `json:"decisions"`
	Warnings    []string   `json:"warnings"`
	RobotsTxt   string     `json:"robots_txt"`
}

// FromCheck builds the check document. `findings` is the structural pass run on the same body.
func FromCheck(res *check.Result, path string, findings []lint.Finding) CheckDoc {
	d := CheckDoc{
		Schema:   SchemaCheck,
		URL:      res.URL,
		Path:     path,
		OK:       len(res.Problems) == 0,
		Cases:    res.Cases,
		Failed:   res.Failed,
		Deindex:  res.Deindex,
		Problems: strings2(res.Problems),
		Findings: findings2(findings),
	}
	if res.Headers != nil {
		if x, age := res.Headers.Get("X-Cache"), res.Headers.Get("Age"); x != "" || age != "" {
			d.Cache = &Cache{XCache: x, Age: age}
		}
	}
	return d
}

// FromLint builds the lint document.
func FromLint(source string, findings []lint.Finding) LintDoc {
	d := LintDoc{Schema: SchemaLint, Source: source, Findings: findings2(findings)}
	for _, f := range findings {
		if f.Sev == lint.Error {
			d.Errors++
		} else {
			d.Warnings++
		}
	}
	d.OK = len(findings) == 0
	return d
}

// FromAdvise builds the advise document. `userAgents` is how many distinct UAs were observed —
// including the ones the algorithm decided to ignore, so the denominator stays honest.
func FromAdvise(a *advise.Advice, robotsTxt string, userAgents int) AdviseDoc {
	d := AdviseDoc{
		Schema:      SchemaAdvise,
		TotalReqs:   a.Total,
		UserAgents:  userAgents,
		SavingShare: a.Saving,
		Decisions:   make([]Decision, 0, len(a.Decisions)),
		Warnings:    strings2(a.Warnings),
		RobotsTxt:   robotsTxt,
	}
	for _, x := range a.Decisions {
		d.Decisions = append(d.Decisions, Decision{
			Name: x.Name, UA: x.UA, Family: x.Family.String(), Policy: policyName(x.Policy),
			Requests: x.Requests, Share: x.Share, Why: x.Why,
		})
	}
	return d
}

// policyName spells the policy out: a Go iota means nothing outside this binary.
func policyName(p advise.Policy) string {
	switch p {
	case advise.Allow:
		return "allow"
	case advise.Block:
		return "block"
	case advise.Candidate:
		return "review"
	default:
		return "ignore"
	}
}

func severityName(s lint.Severity) string {
	return s.String()
}

// findings2 and strings2 return an empty slice rather than nil: an absent list serialises as
// `null`, and a consumer iterating the field should not have to special-case it.
func findings2(in []lint.Finding) []Finding {
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		out = append(out, Finding{Severity: severityName(f.Sev), Message: f.Msg, Line: f.Line})
	}
	return out
}

func strings2(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// Write emits the document indented and newline-terminated: line-oriented tools and `jq` pipes
// both expect the trailing newline.
func Write(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
