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
	"time"

	"github.com/Allan-Nava/robotsmith/internal/advise"
	"github.com/Allan-Nava/robotsmith/internal/check"
	"github.com/Allan-Nava/robotsmith/internal/lint"
)

const (
	SchemaCheck    = "robotsmith.check/1"
	SchemaLint     = "robotsmith.lint/1"
	SchemaAdvise   = "robotsmith.advise/1"
	SchemaCrawlers = "robotsmith.crawlers/1"
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
	// Sitemaps appears only when --sitemaps asked for it. Adding an optional field is not a
	// breaking change, so the schema stays at /1.
	Sitemaps []Sitemap `json:"sitemaps,omitempty"`
}

// Sitemap is one `Sitemap:` line and what it answered.
type Sitemap struct {
	URL         string `json:"url"`
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Bytes       int64  `json:"bytes"`
	Problem     string `json:"problem,omitempty"`
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
	// Evidence is present only when the input was a real log (a `uniq -c` count cannot know it).
	Evidence *Evidence `json:"evidence,omitempty"`
}

// Evidence is the shape of the traffic behind a decision: which paths, over how long.
type Evidence struct {
	TopPaths  []PathCount `json:"top_paths"`
	FirstSeen string      `json:"first_seen,omitempty"`
	LastSeen  string      `json:"last_seen,omitempty"`
}

// PathCount is one path and how often it was asked for.
type PathCount struct {
	Path     string `json:"path"`
	Requests int64  `json:"requests"`
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

// Rule is one classification rule as published by `crawlers`.
type Rule struct {
	Pattern string `json:"pattern"`
	Family  string `json:"family"`
	Policy  string `json:"policy"`
	Token   string `json:"token"`
	Why     string `json:"why"`
}

// CrawlersDoc is the document produced by `crawlers --json`. The rules stay in EVALUATION order:
// the first matching pattern wins, so the order is part of the answer.
type CrawlersDoc struct {
	Schema string `json:"schema"`
	Rules  []Rule `json:"rules"`
}

// FromRules builds the crawlers document.
func FromRules(in []advise.RuleInfo) CrawlersDoc {
	d := CrawlersDoc{Schema: SchemaCrawlers, Rules: make([]Rule, 0, len(in))}
	for _, r := range in {
		d.Rules = append(d.Rules, Rule{Pattern: r.Pattern, Family: r.Family.String(),
			Policy: policyName(r.Policy), Token: r.Token, Why: r.Why})
	}
	return d
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
	for _, sm := range res.Sitemaps {
		d.Sitemaps = append(d.Sitemaps, Sitemap{URL: sm.URL, Status: sm.Status,
			ContentType: sm.ContentType, Bytes: sm.Bytes, Problem: sm.Problem})
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
		dec := Decision{
			Name: x.Name, UA: x.UA, Family: x.Family.String(), Policy: policyName(x.Policy),
			Requests: x.Requests, Share: x.Share, Why: x.Why,
		}
		if x.Evidence != nil {
			ev := &Evidence{TopPaths: make([]PathCount, 0, len(x.Evidence.TopPaths))}
			for _, p := range x.Evidence.TopPaths {
				ev.TopPaths = append(ev.TopPaths, PathCount{Path: p.Path, Requests: p.Requests})
			}
			if !x.Evidence.First.IsZero() {
				ev.FirstSeen = x.Evidence.First.UTC().Format(time.RFC3339)
				ev.LastSeen = x.Evidence.Last.UTC().Format(time.RFC3339)
			}
			dec.Evidence = ev
		}
		d.Decisions = append(d.Decisions, dec)
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
