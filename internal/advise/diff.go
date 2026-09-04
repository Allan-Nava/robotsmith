package advise

import (
	"fmt"
	"strings"

	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// ChangeKind is what would happen to one group if the advice were applied.
type ChangeKind int

const (
	Kept    ChangeKind = iota // already says what the advice says
	Added                     // a group the file does not have yet
	Flipped                   // the file says the opposite: the dangerous case
	Carried                   // hand-written group for a crawler the advice knows nothing about
)

func (k ChangeKind) String() string {
	return [...]string{"kept", "added", "flipped", "carried"}[k]
}

// Change is one line of the review.
type Change struct {
	Kind      ChangeKind
	Agent     string
	Directive string // what the advised file would say
	Was       string // what the current file says, when that differs
	Why       string
	Share     float64
}

// Diff answers the question a reviewer actually has: what would applying this advice change?
//
// Why it exists: `advise --current` already reads the existing file, but printing a whole new one
// leaves the reader diffing two sixty-line files by eye — and the asymmetric risk (blocking
// Googlebot by accident costs weeks of traffic) means the review has to be easy, or it does not
// happen.
func Diff(a *Advice, existing, host string) []Change {
	cur := matcher.Parse(existing)
	byAgent := map[string]matcher.Group{}
	for _, g := range cur.Groups {
		for _, ag := range g.Agents {
			if ag != "*" {
				byAgent[ag] = g
			}
		}
	}

	var out []Change
	advised := map[string]bool{}
	for _, d := range a.Decisions {
		if d.Policy != Allow && d.Policy != Block && d.Policy != Candidate {
			continue
		}
		want := "Disallow: /"
		if d.Policy == Allow {
			want = "Allow: /"
		}
		key := strings.ToLower(d.Name)
		advised[key] = true
		g, found := byAgent[key]
		switch {
		case !found:
			out = append(out, Change{Kind: Added, Agent: d.Name, Directive: want, Why: d.Why, Share: d.Share})
		case saysTheSame(g, want):
			out = append(out, Change{Kind: Kept, Agent: d.Name, Directive: want, Share: d.Share})
		default:
			out = append(out, Change{Kind: Flipped, Agent: d.Name, Directive: want, Was: describe(g),
				Why: d.Why, Share: d.Share})
		}
	}
	// ⚠️ Groups written by a person for a crawler the logs never showed. They are not advice, they
	// are somebody's decision: they get carried over and SAID OUT LOUD, because silence is how a
	// rule gets lost.
	for _, g := range cur.Groups {
		for i, ag := range g.Agents {
			if ag == "*" || advised[ag] {
				continue
			}
			name := ag
			if i < len(g.AgentsRaw) {
				name = g.AgentsRaw[i] // the spelling its author used
			}
			out = append(out, Change{Kind: Carried, Agent: name, Directive: describe(g),
				Why: "written for this site, kept verbatim: this tool does not know why it is there"})
		}
	}
	return out
}

// AllKept reports whether applying the advice would change nothing, so the CLI can say it in one
// line instead of printing a file the reader then has to compare.
func AllKept(changes []Change) bool {
	for _, c := range changes {
		if c.Kind != Kept {
			return false
		}
	}
	return true
}

func saysTheSame(g matcher.Group, want string) bool {
	if len(g.Rules) != 1 {
		return false
	}
	return describe(g) == want
}

func describe(g matcher.Group) string {
	parts := make([]string, 0, len(g.Rules))
	for _, r := range g.Rules {
		verb := "Disallow: "
		if r.Allow {
			verb = "Allow: "
		}
		parts = append(parts, verb+r.Pattern)
	}
	if len(parts) == 0 {
		return "(no rule)"
	}
	return strings.Join(parts, ", ")
}

// carriedGroups returns the hand-written groups Render has to reproduce verbatim: every non-`*`
// group whose agent the advice does not cover.
func carriedGroups(a *Advice, cur *matcher.RobotsTxt) []matcher.Group {
	advised := map[string]bool{}
	for _, d := range a.Decisions {
		advised[strings.ToLower(d.Name)] = true
	}
	var out []matcher.Group
	for _, g := range cur.Groups {
		keep := false
		for _, ag := range g.Agents {
			if ag == "*" {
				keep = false
				break
			}
			if !advised[ag] {
				keep = true
			}
		}
		if keep {
			out = append(out, g)
		}
	}
	return out
}

// renderCarried writes those groups back out, with the spelling their author used.
func renderCarried(b *strings.Builder, groups []matcher.Group) {
	if len(groups) == 0 {
		return
	}
	b.WriteString("\n# ── Kept from the previous file: written for this site, not advised here ─\n")
	b.WriteString("#    This tool does not know why these are here, so it changes nothing about them.\n")
	for _, g := range groups {
		for i, ag := range g.Agents {
			if i < len(g.AgentsRaw) {
				ag = g.AgentsRaw[i]
			}
			fmt.Fprintf(b, "User-agent: %s\n", ag)
		}
		for _, r := range g.Rules {
			verb := "Disallow: "
			if r.Allow {
				verb = "Allow: "
			}
			b.WriteString(verb + r.Pattern + "\n")
		}
	}
}
