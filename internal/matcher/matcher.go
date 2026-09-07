// Package matcher implements robots.txt evaluation per RFC 9309.
//
// Why not reuse a library: the "historical" implementations (including Python's own stdlib
// robotparser) apply the FIRST matching rule. RFC 9309 — and Google — use the LONGEST match
// instead, with Allow winning ties. The difference is not academic:
//
//	User-agent: *
//	Allow: /
//	Disallow: /login
//
// With first-match, `/login` comes out ALLOWED (`Allow: /` wins); with the RFC it comes out
// DISALLOWED (`/login` is longer than `/`). A tool that advises what to write must model how real
// crawlers behave, not how a simplified parser does.
package matcher

import (
	"strings"
)

// Rule is an Allow/Disallow directive with its path pattern.
type Rule struct {
	Allow   bool
	Pattern string
	Line    int
}

// Group is one record of the file: one or more user-agents with their rules.
type Group struct {
	// Agents is lowercased, because user-agent matching is case-insensitive.
	Agents []string
	// AgentsRaw is the same list as written in the file. Anything that writes a group back out
	// uses it: rewriting "YandexBot" as "yandexbot" changes nothing for a crawler and reads, to a
	// person, like the tool mangled their file.
	AgentsRaw []string
	Rules     []Rule
	// StartLine is the line of the group's first `User-agent:` (used in diagnostics).
	StartLine int
}

// RobotsTxt is a parsed file.
type RobotsTxt struct {
	Groups   []Group
	Sitemaps []string
	// Orphans are rules found OUTSIDE any group: this happens when a blank line closes the record
	// and the following directives are left without a `User-agent:`. A strict parser ignores them,
	// Google tolerates them: the tool reports them instead of deciding on its own.
	Orphans []Rule
}

// Parse reads a robots.txt. Deliberately lenient, like real crawlers: it skips lines it does not
// recognise and never stops at the first error.
func Parse(body string) *RobotsTxt {
	r := &RobotsTxt{}
	var cur *Group
	// inGroup says whether we are collecting rules for an open group. A blank line closes it:
	// that is where orphan rules come from.
	inGroup := false

	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		// ⚠️ Order matters: a COMMENT-only line does not close the group, a BLANK line does.
		// Stripping the comment first would make the two indistinguishable (bug caught by the test
		// TestCommentsDoNotCloseTheGroup).
		if line == "" {
			inGroup = false
			continue
		}
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue // it was only a comment: the group stays open
		}
		key, val, ok := split(line)
		if !ok {
			continue
		}
		switch key {
		case "user-agent":
			if !inGroup || cur == nil {
				r.Groups = append(r.Groups, Group{StartLine: i + 1})
				cur = &r.Groups[len(r.Groups)-1]
				inGroup = true
			}
			cur.Agents = append(cur.Agents, strings.ToLower(val))
			cur.AgentsRaw = append(cur.AgentsRaw, val)
		case "allow", "disallow":
			rule := Rule{Allow: key == "allow", Pattern: val, Line: i + 1}
			if inGroup && cur != nil && len(cur.Agents) > 0 {
				cur.Rules = append(cur.Rules, rule)
			} else {
				r.Orphans = append(r.Orphans, rule)
			}
		case "sitemap":
			r.Sitemaps = append(r.Sitemaps, val)
		}
	}
	return r
}

func split(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])), strings.TrimSpace(line[i+1:]), true
}

// groupFor picks the group that applies to a user-agent: the one with the most SPECIFIC token
// (longest match, case-insensitive, on a substring as crawlers do), with `*` as the last resort.
// That is the RFC rule: exactly one group applies, not the union of all of them.
func (r *RobotsTxt) groupFor(ua string) *Group {
	ua = strings.ToLower(ua)
	var best *Group
	bestLen := -1
	var star *Group
	for i := range r.Groups {
		for _, a := range r.Groups[i].Agents {
			if a == "*" {
				if star == nil {
					star = &r.Groups[i]
				}
				continue
			}
			if strings.Contains(ua, a) && len(a) > bestLen {
				best, bestLen = &r.Groups[i], len(a)
			}
		}
	}
	if best != nil {
		return best
	}
	return star
}

// Verdict is an answer with its reasoning attached: which group applied, which rule decided, and
// whether the group was inherited from `*`. A verdict a reader cannot check is one they have to
// trust, and `check --expect` quotes the deployed file's own line.
type Verdict struct {
	Allowed bool
	Agent   string // the user-agent token as written in the file; "" when no group applied
	Rule    *Rule  // the deciding rule; nil when nothing matched
	ViaStar bool   // the group came from `*` rather than from a token for this crawler
}

// Allowed reports whether `path` is allowed for `ua`. Longest match wins; on equal length Allow
// wins (RFC 9309 § 2.2.2). No applicable rule ⇒ allowed.
//
// It is a thin wrapper over Decide on purpose: two code paths answering the same question drift.
func (r *RobotsTxt) Allowed(ua, path string) bool {
	return r.Decide(ua, path).Allowed
}

// Decide is Allowed with the reasoning kept.
func (r *RobotsTxt) Decide(ua, path string) Verdict {
	g := r.groupFor(ua)
	if g == nil {
		return Verdict{Allowed: true}
	}
	v := Verdict{Allowed: true, Agent: agentAsWritten(g, ua)}
	v.ViaStar = v.Agent == "*"
	bestLen := -1
	for i, rule := range g.Rules {
		if rule.Pattern == "" {
			continue // an empty `Disallow:` means "nothing is forbidden"
		}
		if !match(rule.Pattern, path) {
			continue
		}
		l := effLen(rule.Pattern)
		if l > bestLen || (l == bestLen && rule.Allow) {
			bestLen, v.Allowed, v.Rule = l, rule.Allow, &g.Rules[i]
		}
	}
	return v
}

// agentAsWritten picks the token of the group that matched this crawler, in the spelling the file
// uses: a report that renames somebody's token reads like the tool rewrote their file.
func agentAsWritten(g *Group, ua string) string {
	l := strings.ToLower(ua)
	best, bestLen := "", -1
	for i, a := range g.Agents {
		raw := a
		if i < len(g.AgentsRaw) {
			raw = g.AgentsRaw[i]
		}
		if a == "*" {
			if bestLen < 0 {
				best, bestLen = "*", 0
			}
			continue
		}
		if strings.Contains(l, a) && len(a) > bestLen {
			best, bestLen = raw, len(a)
		}
	}
	return best
}

// effLen is the "useful" length of a pattern when comparing specificity: wildcards do not count.
func effLen(p string) int {
	return len(strings.NewReplacer("*", "", "$", "").Replace(p))
}

// match applies the pattern with the RFC wildcards: `*` = any sequence, `$` = end of path.
func match(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	if anchored {
		pattern = strings.TrimSuffix(pattern, "$")
	}
	parts := strings.Split(pattern, "*")
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == 0 {
			if !strings.HasPrefix(path[pos:], part) {
				return false
			}
			pos += len(part)
			continue
		}
		k := strings.Index(path[pos:], part)
		if k < 0 {
			return false
		}
		pos += k + len(part)
	}
	if anchored {
		// with `$` the last chunk must reach the end of the path
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			return pos == len(path)
		}
		return true
	}
	return true
}
