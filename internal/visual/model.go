// Package visual turns an advise document into a report a person can send.
//
// ⚠️ ONE layout model, TWO backends. The bars, labels and rows are computed once here; html.go
// emits SVG and pdf.go emits PDF drawing operators. Producing the second rendering a second way is
// the trap: two renderers drift, and then the numbers in the deck disagree with the numbers in the
// tool — which is exactly the credibility this report exists to have.
//
// The model is built from the same document `--json` emits, for the same reason.
package visual

import (
	"fmt"
	"sort"

	"github.com/Allan-Nava/robotsmith/internal/report"
)

// Bar is one crawler, sized against the heaviest one.
type Bar struct {
	Name        string
	Requests    int64
	Share       float64
	Fraction    float64 // width relative to the largest bar, 0..1
	Policy      string  // allow | block | review
	PolicyLabel string  // ALLOW | BLOCK | REVIEW — identity is never colour alone
	Glyph       string  // ✓ ✗ ? — the second channel, for print and for colour-blind readers
	Why         string
	FromPolicy  bool
	Rule        string
}

// Row is one line of the decisions table: the table IS the accessibility relief for the chart.
type Row struct {
	Bar
	Family string
}

// Movement is one crawler's change against an earlier run, when there was one.
type Movement struct {
	Name           string
	Direction      string
	Was, Now       float64
	Factor         float64
	FactorReadable string
}

// Model is everything both backends draw.
type Model struct {
	Host          string
	TotalRequests int64
	TotalReadable string
	UserAgents    int
	SavingShare   float64
	Judged        float64 // share of traffic the method could judge
	Unjudged      float64 // ...and the share it could not: not a footnote, the honesty of the method
	Bars          []Bar
	Rows          []Row
	Movements     []Movement
	Warnings      []string
	RobotsTxt     string
	Schema        string
}

// From builds the model. It is deterministic: no clock, no map iteration, so the same document
// always produces the same report — that is what lets a report be diffed instead of eyeballed.
func From(d report.AdviseDoc, host string) Model {
	m := Model{
		Host: host, TotalRequests: d.TotalReqs, TotalReadable: grouped(d.TotalReqs),
		UserAgents: d.UserAgents, SavingShare: d.SavingShare, Warnings: d.Warnings,
		RobotsTxt: d.RobotsTxt, Schema: d.Schema,
	}
	var maxReq int64
	for _, dec := range d.Decisions {
		if dec.Requests > maxReq {
			maxReq = dec.Requests
		}
		m.Judged += dec.Share
	}
	// What the decisions do not cover is what the method cannot judge — browser user-agents,
	// monitors, apps. Stating it is the difference between a report and a sales pitch.
	m.Unjudged = 100 - m.Judged
	if m.Unjudged < 0 {
		m.Unjudged = 0
	}
	for _, dec := range d.Decisions {
		b := Bar{
			Name: dec.Name, Requests: dec.Requests, Share: dec.Share, Policy: dec.Policy,
			PolicyLabel: label(dec.Policy), Glyph: glyph(dec.Policy), Why: dec.Why,
			FromPolicy: dec.FromPolicyFile, Rule: dec.Rule,
		}
		if maxReq > 0 {
			b.Fraction = float64(dec.Requests) / float64(maxReq)
		}
		m.Bars = append(m.Bars, b)
		m.Rows = append(m.Rows, Row{Bar: b, Family: dec.Family})
	}
	// ⚠️ The chart is sorted by VOLUME and the table is left in the document's order (policy first,
	// then volume). They answer different questions: the chart argues "this one is worth more than
	// those ten", the table is read decision by decision.
	sort.SliceStable(m.Bars, func(i, j int) bool { return m.Bars[i].Requests > m.Bars[j].Requests })

	if c := d.Comparison; c != nil {
		for _, mv := range c.Movements {
			if mv.Direction == "steady" {
				continue
			}
			f := ""
			if mv.Factor > 0 {
				f = fmt.Sprintf("×%.3g", mv.Factor)
			}
			m.Movements = append(m.Movements, Movement{Name: mv.Name, Direction: mv.Direction,
				Was: mv.Was, Now: mv.Now, Factor: mv.Factor, FactorReadable: f})
		}
	}
	return m
}

func label(policy string) string {
	switch policy {
	case "allow":
		return "ALLOW"
	case "block":
		return "BLOCK"
	case "review":
		return "REVIEW"
	default:
		return "IGNORE"
	}
}

// glyph is the second channel. A status colour never carries meaning alone: red and green are the
// same colour to a deuteranope, and a printed report has no colour at all.
func glyph(policy string) string {
	switch policy {
	case "allow":
		return "✓"
	case "block":
		return "✗"
	case "review":
		return "?"
	default:
		return "·"
	}
}

func grouped(n int64) string {
	s := fmt.Sprintf("%d", n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
