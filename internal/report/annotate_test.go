package report

import (
	"strings"
	"testing"

	"github.com/Allan-Nava/robotsmith/internal/lint"
)

func TestAnnotationsAreWorkflowCommandsOnTheRightLine(t *testing.T) {
	// The point of an annotation is that it lands ON the defective line in the diff view: a summary
	// in the log is something nobody scrolls to.
	doc := FromLint("docs/robots.txt", []lint.Finding{
		{Sev: lint.Error, Msg: "orphan rule after a blank line", Line: 3},
		{Sev: lint.Warn, Msg: "`Allow: /` comes first", Line: 2},
	})
	got := Annotations(doc)
	if len(got) != 2 {
		t.Fatalf("expected one annotation per finding, got %d: %v", len(got), got)
	}
	if got[0] != "::error file=docs/robots.txt,line=3,title=robotsmith lint::orphan rule after a blank line" {
		t.Errorf("annotation = %q", got[0])
	}
	if !strings.HasPrefix(got[1], "::warning file=docs/robots.txt,line=2,") {
		t.Errorf("a WARNING must map to ::warning, not ::error: %q", got[1])
	}
}

func TestAnnotationsEscapeWhatWouldTruncateThem(t *testing.T) {
	// ⚠️ An unescaped newline ends the workflow command: the rest of the message disappears and,
	// worse, whatever follows is parsed as a new command. Our messages are multi-line prose.
	doc := FromLint("robots.txt", []lint.Finding{
		{Sev: lint.Error, Msg: "first line\nsecond line: 100% of it, and a comma", Line: 1},
	})
	got := Annotations(doc)[0]
	if strings.Count(got, "\n") != 0 {
		t.Fatalf("an annotation must be a single line: %q", got)
	}
	for _, want := range []string{"%0A", "%25"} {
		if !strings.Contains(got, want) {
			t.Errorf("annotation = %q, missing the escape %s", got, want)
		}
	}
	// The message part keeps commas and colons — only property VALUES need those escaped.
	if !strings.Contains(got, "and a comma") {
		t.Errorf("the message must survive intact: %q", got)
	}
}

func TestAnnotationsEscapeThePropertyValues(t *testing.T) {
	// A path with a comma or a colon in it would otherwise close the property list early.
	doc := FromLint("weird,dir/rob:ots.txt", []lint.Finding{{Sev: lint.Error, Msg: "x", Line: 1}})
	got := Annotations(doc)[0]
	if !strings.Contains(got, "file=weird%2Cdir/rob%3Aots.txt,line=1") {
		t.Errorf("annotation = %q: a comma or colon in the path must be escaped", got)
	}
}

func TestAnnotationsWithoutALineStillPointAtTheFile(t *testing.T) {
	// Some findings are about the file as a whole (an empty file, a cross-domain sitemap). Faking
	// line 1 would send a reviewer to the wrong place, so the line is simply omitted.
	doc := FromLint("robots.txt", []lint.Finding{{Sev: lint.Error, Msg: "EMPTY file", Line: 0}})
	got := Annotations(doc)[0]
	if strings.Contains(got, "line=") {
		t.Errorf("annotation = %q: no line is better than a wrong line", got)
	}
	if !strings.Contains(got, "file=robots.txt") {
		t.Errorf("annotation = %q must still name the file", got)
	}
}

func TestAnnotationsFromACheckDocument(t *testing.T) {
	// `check` runs against a URL, so there is no file to annotate: the problems become log-level
	// annotations, which is what a CI job needs in order to show them at all.
	doc := CheckDoc{Schema: SchemaCheck, URL: "https://example.com/robots.txt",
		Problems: []string{"`Googlebot` is BLOCKED but must get through"},
		Findings: []Finding{{Severity: "ERROR", Message: "orphan rule", Line: 4}}}
	got := Annotations(doc)
	if len(got) != 2 {
		t.Fatalf("expected the problem and the finding, got %v", got)
	}
	if !strings.HasPrefix(got[0], "::error title=robotsmith check::") || !strings.Contains(got[0], "Googlebot") {
		t.Errorf("annotation = %q", got[0])
	}
	if !strings.Contains(got[1], "file=https%3A//example.com/robots.txt,line=4") {
		t.Errorf("a structural finding on a fetched file must still carry its line: %q", got[1])
	}
}

func TestAnnotationsOfACleanRunAreEmpty(t *testing.T) {
	if got := Annotations(FromLint("robots.txt", nil)); len(got) != 0 {
		t.Errorf("a clean run must annotate nothing, got %v", got)
	}
}
