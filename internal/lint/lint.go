// Package lint finds the STRUCTURAL defects of a robots.txt: the ones that make the file behave
// differently from what its author thinks it says. They are the most common cause of "I wrote it
// and it does not work".
package lint

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/Allan-Nava/robotsmith/internal/matcher"
)

// Severity separates what is broken from what is fragile.
type Severity int

const (
	Warn Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "ERROR"
	}
	return "WARNING"
}

// Finding is one defect found.
type Finding struct {
	Sev  Severity
	Msg  string
	Line int
}

// Check analyses the content. `host` is the host the file was downloaded from (for the Sitemap).
func Check(body, host string) []Finding {
	var out []Finding
	r := matcher.Parse(body)

	if strings.TrimSpace(body) == "" {
		return append(out, Finding{Error, "EMPTY file: to a crawler this means \"everything is " +
			"allowed\", not \"everything is forbidden\"", 0})
	}
	if len(r.Groups) == 0 {
		out = append(out, Finding{Error, "no `User-agent:` group: every directive is ignored", 0})
	}
	// 1) orphan rules: the number one cause of rules that "do not apply"
	for _, o := range r.Orphans {
		verb := "Disallow"
		if o.Allow {
			verb = "Allow"
		}
		out = append(out, Finding{Error, fmt.Sprintf(
			"`%s: %s` comes after a BLANK line inside a group: the blank line closes the record, so "+
				"a strict parser ignores this rule (Google honours it anyway). Remove the blank line",
			verb, o.Pattern), o.Line})
	}
	// 2) `Allow: /` before the Disallow rules: harmless with Google (longest match wins), fatal
	//    with first-match parsers
	for _, g := range r.Groups {
		for i, rule := range g.Rules {
			if !rule.Allow || rule.Pattern != "/" {
				continue
			}
			for _, later := range g.Rules[i+1:] {
				if !later.Allow {
					out = append(out, Finding{Warn, fmt.Sprintf(
						"`Allow: /` comes BEFORE `Disallow: %s`: with Google nothing changes (the "+
							"longest match wins) but a first-match parser voids every prohibition. Move it "+
							"to the end of the group, or drop the line: whatever is not forbidden is "+
							"already allowed", later.Pattern), rule.Line})
					break
				}
			}
			break
		}
	}
	// 3) Sitemap on another host
	for _, sm := range r.Sitemaps {
		if host == "" {
			continue
		}
		if u, err := url.Parse(sm); err == nil {
			a := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			b := strings.TrimPrefix(strings.ToLower(host), "www.")
			if a != "" && a != b {
				out = append(out, Finding{Warn, fmt.Sprintf(
					"the `Sitemap:` points to another host (%s): a cross-domain sitemap is not "+
						"considered", a), 0})
			}
		}
	}
	// 4) a group that blocks everything for everyone
	for _, g := range r.Groups {
		for _, ag := range g.Agents {
			if ag != "*" {
				continue
			}
			for _, rule := range g.Rules {
				if !rule.Allow && rule.Pattern == "/" {
					out = append(out, Finding{Error, "`User-agent: *` with `Disallow: /`: the site " +
						"drops out of the search indexes. If that is intended (a staging environment) " +
						"it is fine, otherwise it is the most expensive defect there is", rule.Line})
				}
			}
		}
	}
	return out
}
