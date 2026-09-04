// Package backlog parses BACKLOG.md — the single source of truth for this repo's todos — and
// checks the things about it that a machine can check.
//
// Why a parser instead of a spreadsheet or an issue tracker as the source: the backlog lives next
// to the code, in the same review, in the same commit. The issues are a *projection* of it, so
// they can be recreated at any time; the file cannot be reconstructed from the issues.
//
// ⚠️ The `id` in backticks is the join key with the GitHub issues (it travels in a fingerprint
// comment in the issue body). Changing an id orphans an issue, which is why Parse refuses
// duplicates and the sync never matches on the title.
package backlog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Item is one todo.
type Item struct {
	ID        string
	Title     string
	Status    string // "open" | "done"
	Priority  string
	Labels    []string
	Milestone string
	Ref       string
	Body      string // the prose, metadata bullets removed
	Line      int    // where the item starts, so a lint message is clickable
}

// Done reports whether the item is history rather than work.
func (i Item) Done() bool { return i.Status == "done" }

var (
	reItem      = regexp.MustCompile("^### `([a-z0-9][a-z0-9-]*)`\\s*—\\s*(.+?)\\s*$")
	reMeta      = regexp.MustCompile(`^-\s+\*\*([a-z]+)\*\*:\s*(.+?)\s*$`)
	reHeading   = regexp.MustCompile(`^#\s+(.+?)\s*$`)
	reTableRow  = regexp.MustCompile(`^\|\s*\[([^\]]+)\]\([^)]*\)\s*\|[^|]*\|\s*(\d+)\s*\|\s*(\d+)\s*\|`)
	validStatus = map[string]bool{"open": true, "done": true}
)

// Parse reads the whole file. It is strict about the two things that break the projection into
// issues — a duplicate id and an unknown status — and lenient about everything else, because the
// rest is prose written for people.
func Parse(md string) ([]Item, error) {
	var items []Item
	var cur *Item
	var body []string
	seen := map[string]int{}

	flush := func() {
		if cur == nil {
			return
		}
		cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
		items = append(items, *cur)
		cur, body = nil, nil
	}

	for n, line := range strings.Split(md, "\n") {
		if m := reItem.FindStringSubmatch(line); m != nil {
			flush()
			if before, dup := seen[m[1]]; dup {
				return nil, fmt.Errorf("line %d: duplicate id %q (already used on line %d): the id is "+
					"the join key with the issue, it must be unique", n+1, m[1], before)
			}
			seen[m[1]] = n + 1
			cur = &Item{ID: m[1], Title: m[2], Status: "open", Line: n + 1}
			continue
		}
		// A top-level heading closes the current item: it is the next milestone section.
		if cur != nil && reHeading.MatchString(line) {
			flush()
			continue
		}
		if cur == nil {
			continue
		}
		if m := reMeta.FindStringSubmatch(line); m != nil {
			switch m[1] {
			case "status":
				if !validStatus[m[2]] {
					return nil, fmt.Errorf("line %d: item %q has status %q: only \"open\" and \"done\" exist",
						n+1, cur.ID, m[2])
				}
				cur.Status = m[2]
			case "priority":
				cur.Priority = m[2]
			case "labels":
				for _, l := range strings.Split(m[2], ",") {
					if l = strings.TrimSpace(l); l != "" {
						cur.Labels = append(cur.Labels, l)
					}
				}
			case "milestone":
				cur.Milestone = m[2]
			case "ref":
				cur.Ref = m[2]
			}
			continue
		}
		body = append(body, line)
	}
	flush()
	return items, nil
}

// Lint returns the problems worth failing a build over. `md` is the original document, needed for
// the checks that compare the items against the prose around them; pass "" to skip those.
func Lint(items []Item, md string) []string {
	var probs []string
	for _, it := range items {
		if !it.Done() && !strings.Contains(it.Body, "**Done when:**") {
			probs = append(probs, fmt.Sprintf("line %d: item `%s` is open with no \"**Done when:**\" "+
				"line: without an observable condition nobody can tell when it is finished", it.Line, it.ID))
		}
		if it.Body == "" {
			probs = append(probs, fmt.Sprintf("line %d: item `%s` has an empty body: the issue would "+
				"carry the title and nothing else", it.Line, it.ID))
		}
	}
	if md == "" {
		return probs
	}
	headings := map[string]bool{}
	for _, line := range strings.Split(md, "\n") {
		if m := reHeading.FindStringSubmatch(line); m != nil {
			headings[m[1]] = true
		}
	}
	// ⚠️ A milestone title with no matching section heading is almost always a typo — and a typo
	// here silently creates a second GitHub milestone at the next sync.
	for _, it := range items {
		if it.Milestone != "" && !headings[it.Milestone] {
			probs = append(probs, fmt.Sprintf("line %d: item `%s` names the milestone %q, which has no "+
				"section heading: a typo here creates a duplicate milestone on GitHub", it.Line, it.ID, it.Milestone))
		}
	}
	probs = append(probs, lintRoadmapTable(items, md)...)
	return probs
}

// lintRoadmapTable compares the summary table with the items it summarises. The table is
// hand-written prose about counted facts, which is the same class of defect as a help text stating
// a case count the code no longer produces.
func lintRoadmapTable(items []Item, md string) []string {
	type counts struct{ open, done int }
	real := map[string]*counts{}
	get := func(k string) *counts {
		if real[k] == nil {
			real[k] = &counts{}
		}
		return real[k]
	}
	for _, it := range items {
		key := it.Milestone
		if key == "" {
			key = Unscheduled
		}
		if it.Done() {
			get(key).done++
		} else {
			get(key).open++
		}
	}
	var probs []string
	for _, line := range strings.Split(md, "\n") {
		m := reTableRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimSpace(m[1])
		c := real[name]
		if c == nil {
			probs = append(probs, fmt.Sprintf("the roadmap table lists %q, which no item belongs to", name))
			continue
		}
		wantOpen, _ := strconv.Atoi(m[2])
		wantDone, _ := strconv.Atoi(m[3])
		if wantOpen != c.open || wantDone != c.done {
			probs = append(probs, fmt.Sprintf("the roadmap table says %q has %d open / %d done, the items "+
				"say %d open / %d done", name, wantOpen, wantDone, c.open, c.done))
		}
	}
	return probs
}

// Unscheduled is the bucket for items with no milestone. It matches the section heading so the
// summary table can count them like any other group.
const Unscheduled = "Backlog (unscheduled)"
