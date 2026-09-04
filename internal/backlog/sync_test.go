package backlog

import (
	"strings"
	"testing"
)

func item(id, status, milestone string, labels ...string) Item {
	return Item{ID: id, Title: "title of " + id, Status: status, Milestone: milestone,
		Labels: labels, Body: "why\n\n**Done when:** it works", Priority: "high"}
}

func TestPlanDecidesOnePerItem(t *testing.T) {
	// The table this projection lives by. Matching is by id, never by title: an edited title must
	// update the issue, not create a second one.
	items := []Item{
		item("new-thing", "open", "v0.3.0"),
		item("already-open", "open", "v0.3.0"),
		item("was-closed", "open", ""),
		item("finished", "done", ""),
		item("finished-and-closed", "done", ""),
	}
	existing := []Issue{
		{Number: 10, ID: "already-open", Title: "title of already-open", State: "open",
			Body: RenderBody(item("already-open", "open", "v0.3.0")), Labels: []string{SyncLabel, "high"}, Milestone: "v0.3.0"},
		{Number: 11, ID: "was-closed", Title: "title of was-closed", State: "closed",
			Body: RenderBody(item("was-closed", "open", "")), Labels: []string{SyncLabel, "high"}},
		{Number: 12, ID: "finished", Title: "title of finished", State: "open",
			Body: RenderBody(item("finished", "done", "")), Labels: []string{SyncLabel, "high"}},
		{Number: 13, ID: "finished-and-closed", Title: "x", State: "closed", Labels: []string{SyncLabel}},
		{Number: 14, ID: "vanished-from-the-file", Title: "x", State: "open", Labels: []string{SyncLabel}},
	}
	got := map[string]Action{}
	for _, a := range Plan(items, existing) {
		got[a.IssueID] = a
	}
	want := map[string]Action{
		"new-thing":              {Kind: Create},
		"already-open":           {Kind: Skip},
		"was-closed":             {Kind: Reopen},
		"finished":               {Kind: Close},
		"vanished-from-the-file": {Kind: Close},
	}
	for key, w := range want {
		a, ok := got[key]
		if !ok {
			t.Errorf("%s: no action planned", key)
			continue
		}
		if a.Kind != w.Kind {
			t.Errorf("%s: action = %v, expected %v", key, a.Kind, w.Kind)
		}
	}
	if _, ok := got["finished-and-closed"]; ok {
		t.Error("a done item whose issue is already closed needs no action: the sync must be quiet")
	}
	if len(got) != 5 {
		t.Errorf("expected 5 actions, got %d: %v", len(got), got)
	}
}

func TestPlanUpdatesWhenTheContentChanged(t *testing.T) {
	// Editing the prose, the labels or the milestone must reach the issue — otherwise the issue
	// becomes a stale copy nobody trusts.
	it := item("thing", "open", "v0.3.0", "cli")
	existing := []Issue{{Number: 7, ID: "thing", Title: "an older title", State: "open",
		Body: RenderBody(it), Labels: []string{SyncLabel, "cli", "high"}, Milestone: "v0.3.0"}}
	acts := Plan([]Item{it}, existing)
	if len(acts) != 1 || acts[0].Kind != Update || acts[0].Number != 7 {
		t.Fatalf("a changed title must plan an Update on the same issue, got %+v", acts)
	}

	sameBodyDifferentMilestone := []Issue{{Number: 7, ID: "thing", Title: it.Title, State: "open",
		Body: RenderBody(it), Labels: []string{SyncLabel, "cli", "high"}, Milestone: "v0.2.0"}}
	if acts = Plan([]Item{it}, sameBodyDifferentMilestone); acts[0].Kind != Update {
		t.Errorf("a changed milestone must plan an Update, got %v", acts[0].Kind)
	}
}

func TestRenderBodyCarriesTheFingerprintAndTheMetadata(t *testing.T) {
	body := RenderBody(item("thing", "open", "v0.3.0", "cli"))
	if !strings.Contains(body, "<!-- backlog-id: thing -->") {
		t.Error("the fingerprint is how the issue is matched back to the item: it must be in the body")
	}
	if !strings.Contains(body, "BACKLOG.md") {
		t.Error("the body must point back at the source of truth, or people edit the issue instead")
	}
	if !strings.Contains(body, "**Done when:** it works") {
		t.Error("the acceptance criterion belongs in the issue")
	}
	if id := IDFromBody(body); id != "thing" {
		t.Errorf("IDFromBody = %q: rendering and parsing must round-trip", id)
	}
	if IDFromBody("a body written by a person") != "" {
		t.Error("an issue without a fingerprint is not ours: it must never be touched")
	}
}

func TestLabelsAlwaysCarryTheSyncMarkerAndThePriority(t *testing.T) {
	got := strings.Join(Labels(item("thing", "open", "", "cli", "ci")), ",")
	for _, want := range []string{SyncLabel, "cli", "ci", "high"} {
		if !strings.Contains(got, want) {
			t.Errorf("labels = %q, missing %q", got, want)
		}
	}
}
