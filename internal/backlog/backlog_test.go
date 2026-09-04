package backlog

import (
	"os"
	"strings"
	"testing"
)

const sample = "# Backlog\n\n" +
	"## Roadmap at a glance\n\n" +
	"| Milestone | Theme | Open | Done |\n|---|---|---:|---:|\n" +
	"| [v0.2.0 — Ship it](#x) | theme | 1 | 1 |\n" +
	"| [Backlog (unscheduled)](#y) | theme | 1 | 0 |\n\n" +
	"# v0.2.0 — Ship it\n\n" +
	"### `json-output` — `--json` on all three commands\n\n" +
	"- **status**: open\n" +
	"- **priority**: high\n" +
	"- **labels**: cli, ci\n" +
	"- **milestone**: v0.2.0 — Ship it\n\n" +
	"Exit codes say *whether*, a pipeline needs *what*.\n\n" +
	"**Done when:** a golden test per command passes.\n\n" +
	"### `case-count` — count from the data\n\n" +
	"- **status**: done\n" +
	"- **priority**: medium\n" +
	"- **milestone**: v0.2.0 — Ship it\n" +
	"- **ref**: `TestUsage`\n\n" +
	"A literal count lies.\n\n" +
	"# Backlog (unscheduled)\n\n" +
	"### `dogfood-robots-txt` — ship our own robots.txt\n\n" +
	"- **status**: open\n" +
	"- **priority**: low\n\n" +
	"Credibility.\n\n" +
	"**Done when:** docs/robots.txt exists.\n"

func TestParseReadsIdsMetadataAndBody(t *testing.T) {
	items, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	first := items[0]
	if first.ID != "json-output" {
		t.Errorf("id = %q", first.ID)
	}
	if first.Title != "`--json` on all three commands" {
		t.Errorf("title = %q: the id is the key, the title is what follows the dash", first.Title)
	}
	if first.Status != "open" || first.Priority != "high" {
		t.Errorf("status/priority = %q/%q", first.Status, first.Priority)
	}
	if strings.Join(first.Labels, "|") != "cli|ci" {
		t.Errorf("labels = %v", first.Labels)
	}
	if first.Milestone != "v0.2.0 — Ship it" {
		t.Errorf("milestone = %q", first.Milestone)
	}
	if !strings.Contains(first.Body, "a pipeline needs") || strings.Contains(first.Body, "**status**") {
		t.Errorf("the body is the prose, without the metadata bullets:\n%s", first.Body)
	}
	if items[1].Status != "done" || items[1].Ref != "`TestUsage`" {
		t.Errorf("second item = %+v", items[1])
	}
	// An item with no `milestone` bullet belongs to no milestone: it must not inherit the heading,
	// or everything in the unscheduled section would silently become scheduled work.
	if items[2].Milestone != "" {
		t.Errorf("unscheduled item milestone = %q, expected empty", items[2].Milestone)
	}
	if items[2].Line == 0 {
		t.Error("items must carry their line, so a lint error is clickable")
	}
}

func TestParseRejectsADuplicateID(t *testing.T) {
	// The id is what links an item to its issue: two items sharing one would fight over it forever.
	_, err := Parse(sample + "\n### `json-output` — again\n\n- **status**: open\n\nBody.\n")
	if err == nil || !strings.Contains(err.Error(), "json-output") {
		t.Errorf("expected a duplicate-id error, got %v", err)
	}
}

func TestParseRejectsAnUnknownStatus(t *testing.T) {
	_, err := Parse("### `x` — t\n\n- **status**: maybe\n\nBody.\n")
	if err == nil || !strings.Contains(err.Error(), "maybe") {
		t.Errorf("expected an unknown-status error, got %v", err)
	}
}

func TestLintRequiresDoneWhenOnOpenItems(t *testing.T) {
	items, err := Parse("### `x` — t\n\n- **status**: open\n\nNo acceptance criterion here.\n")
	if err != nil {
		t.Fatal(err)
	}
	probs := Lint(items, "")
	if !containsSubstring(probs, "Done when") {
		t.Errorf("an open item without an acceptance criterion must be flagged, got %v", probs)
	}
	// A done item does not need one: it is history.
	done, _ := Parse("### `x` — t\n\n- **status**: done\n\nHistory.\n")
	if len(Lint(done, "")) != 0 {
		t.Errorf("a done item needs no Done-when, got %v", Lint(done, ""))
	}
}

func TestLintCatchesARoadmapTableThatDisagreesWithTheItems(t *testing.T) {
	// ⚠️ The summary table is hand-written prose about counted facts: exactly the drift that the
	// "31 cases" bug was. Here it is checked.
	items, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	if probs := Lint(items, sample); len(probs) != 0 {
		t.Fatalf("the sample table is correct, got %v", probs)
	}
	wrong := strings.Replace(sample, "| [v0.2.0 — Ship it](#x) | theme | 1 | 1 |",
		"| [v0.2.0 — Ship it](#x) | theme | 5 | 1 |", 1)
	if probs := Lint(mustParse(t, wrong), wrong); !containsSubstring(probs, "v0.2.0 — Ship it") {
		t.Errorf("a wrong open count must be flagged, got %v", probs)
	}
}

func TestLintCatchesAMilestoneThatIsNotAHeading(t *testing.T) {
	// A typo in a milestone title means a second GitHub milestone gets created silently.
	md := strings.Replace(sample, "- **milestone**: v0.2.0 — Ship it\n\nExit codes",
		"- **milestone**: v0.2.0 - Ship it\n\nExit codes", 1)
	if probs := Lint(mustParse(t, md), md); !containsSubstring(probs, "v0.2.0 - Ship it") {
		t.Errorf("a milestone with no matching heading must be flagged, got %v", probs)
	}
}

func TestTheRealBacklogFileIsValid(t *testing.T) {
	// The gate itself: CI parses and lints the file people actually edit.
	b, err := os.ReadFile("../../BACKLOG.md")
	if err != nil {
		t.Fatal(err)
	}
	items, err := Parse(string(b))
	if err != nil {
		t.Fatalf("BACKLOG.md does not parse: %v", err)
	}
	if len(items) < 5 {
		t.Errorf("expected the real items, got %d", len(items))
	}
	for _, p := range Lint(items, string(b)) {
		t.Errorf("BACKLOG.md: %s", p)
	}
}

func mustParse(t *testing.T, md string) []Item {
	t.Helper()
	items, err := Parse(md)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func containsSubstring(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
