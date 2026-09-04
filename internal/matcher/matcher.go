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
	Agents []string
	Rules  []Rule
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

// Allowed reports whether `path` is allowed for `ua`. Longest match wins; on equal length Allow
// wins (RFC 9309 § 2.2.2). No applicable rule ⇒ allowed.
func (r *RobotsTxt) Allowed(ua, path string) bool {
	g := r.groupFor(ua)
	if g == nil {
		return true
	}
	bestLen, allow := -1, true
	for _, rule := range g.Rules {
		if rule.Pattern == "" {
			continue // an empty `Disallow:` means "nothing is forbidden"
		}
		if !match(rule.Pattern, path) {
			continue
		}
		l := effLen(rule.Pattern)
		if l > bestLen || (l == bestLen && rule.Allow) {
			bestLen, allow = l, rule.Allow
		}
	}
	return allow
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
