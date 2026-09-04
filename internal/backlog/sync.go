package backlog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SyncLabel marks the issues this projection owns. An issue without it — or without a fingerprint
// in its body — was written by a person and is never touched.
const SyncLabel = "backlog-sync"

// Issue is the part of a GitHub issue this package reasons about.
type Issue struct {
	Number    int
	ID        string // from the fingerprint in the body; empty means "not ours"
	Title     string
	Body      string
	State     string // "open" | "closed"
	Labels    []string
	Milestone string
}

// Kind is what the sync will do about one item.
type Kind int

const (
	Create Kind = iota
	Update
	Reopen
	Close
	Skip
)

func (k Kind) String() string {
	return [...]string{"CREATE", "UPDATE", "REOPEN", "CLOSE", "SKIP"}[k]
}

// Action is one planned change. `Item` is zero for a Close of an item that vanished from the file.
type Action struct {
	Kind    Kind
	Item    Item
	IssueID string // the id the issue carries, for a Close with no item behind it
	Number  int    // the issue to act on; 0 for a Create
	Why     string
}

var reFingerprint = regexp.MustCompile(`<!-- backlog-id: ([a-z0-9][a-z0-9-]*) -->`)

// IDFromBody extracts the fingerprint. Matching on it — never on the title — is what makes the
// sync idempotent: editing a title updates an issue instead of opening a twin.
func IDFromBody(body string) string {
	if m := reFingerprint.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	return ""
}

// RenderBody is the issue body: the prose, the metadata, a pointer back to the source of truth and
// the fingerprint. It has to be deterministic, or every sync would look like a change.
func RenderBody(it Item) string {
	var b strings.Builder
	b.WriteString(it.Body)
	b.WriteString("\n\n---\n\n")
	if it.Priority != "" {
		fmt.Fprintf(&b, "- priority: **%s**\n", it.Priority)
	}
	if it.Milestone != "" {
		fmt.Fprintf(&b, "- milestone: **%s**\n", it.Milestone)
	}
	if it.Ref != "" {
		fmt.Fprintf(&b, "- ref: %s\n", it.Ref)
	}
	b.WriteString("\n🤖 Projected from [`BACKLOG.md`](../blob/main/BACKLOG.md) — **edit the file, not " +
		"this issue**: the next sync overwrites anything changed here.\n")
	fmt.Fprintf(&b, "\n<!-- backlog-id: %s -->\n", it.ID)
	return b.String()
}

// Labels are the issue labels: the sync marker, the item labels and the priority (so a board can
// be sorted without opening anything).
func Labels(it Item) []string {
	out := []string{SyncLabel}
	out = append(out, it.Labels...)
	if it.Priority != "" {
		out = append(out, it.Priority)
	}
	sort.Strings(out)
	return out
}

// Plan works out the changes. One action per item at most, plus a Close for every owned issue whose
// item disappeared from the file — otherwise a deleted item would leave an issue open forever.
//
//	item open + no issue      → CREATE
//	item open + issue open    → SKIP, unless the content changed → UPDATE
//	item open + issue closed  → REOPEN
//	item done + issue open    → CLOSE
//	item done + issue closed  → nothing (a quiet sync is what makes it safe to run often)
//	no item   + issue open    → CLOSE
func Plan(items []Item, existing []Issue) []Action {
	byID := map[string]Issue{}
	for _, is := range existing {
		if is.ID == "" {
			is.ID = IDFromBody(is.Body)
		}
		if is.ID == "" || !hasLabel(is.Labels, SyncLabel) {
			continue // written by a person: not ours to touch
		}
		byID[is.ID] = is
	}

	var acts []Action
	inFile := map[string]bool{}
	for _, it := range items {
		inFile[it.ID] = true
		is, found := byID[it.ID]
		switch {
		case it.Done() && !found:
			// history with no issue: nothing to do
		case it.Done() && is.State == "open":
			acts = append(acts, Action{Kind: Close, Item: it, IssueID: it.ID, Number: is.Number,
				Why: "item marked done in BACKLOG.md"})
		case it.Done():
			// already closed
		case !found:
			acts = append(acts, Action{Kind: Create, Item: it, IssueID: it.ID, Why: "new item"})
		case is.State == "closed":
			acts = append(acts, Action{Kind: Reopen, Item: it, IssueID: it.ID, Number: is.Number,
				Why: "item is open again in BACKLOG.md"})
		case changed(it, is):
			acts = append(acts, Action{Kind: Update, Item: it, IssueID: it.ID, Number: is.Number,
				Why: "title, body, labels or milestone changed"})
		default:
			acts = append(acts, Action{Kind: Skip, Item: it, IssueID: it.ID, Number: is.Number})
		}
	}
	// Owned issues with no item left in the file.
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		is := byID[id]
		if !inFile[id] && is.State == "open" {
			acts = append(acts, Action{Kind: Close, IssueID: id, Number: is.Number,
				Why: "item removed from BACKLOG.md"})
		}
	}
	return acts
}

func changed(it Item, is Issue) bool {
	if is.Title != it.Title || is.Body != RenderBody(it) || is.Milestone != it.Milestone {
		return true
	}
	want, got := Labels(it), append([]string(nil), is.Labels...)
	sort.Strings(got)
	return strings.Join(want, ",") != strings.Join(got, ",")
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}
