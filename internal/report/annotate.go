package report

import (
	"fmt"
	"strings"
)

// Annotations renders a document as GitHub Actions workflow commands, one per finding.
//
// Why this lives in the binary and not in the action's YAML: an annotation lands ON the defective
// line in the diff view, which is the only place a reviewer actually reads it — and getting there
// depends on escaping rules that are easy to get wrong and impossible to unit-test in YAML. Written
// here, the format has tests; the action stays a thin wrapper.
//
// ⚠️ An unescaped newline ENDS a workflow command: the rest of the message vanishes and whatever
// follows is parsed as a new command. The messages in this tool are multi-line prose, so this is
// not a theoretical concern.
func Annotations(doc any) []string {
	switch d := doc.(type) {
	case LintDoc:
		return annotateFindings("robotsmith lint", d.Source, d.Findings)
	case CheckDoc:
		out := make([]string, 0, len(d.Problems)+len(d.Findings))
		for _, p := range d.Problems {
			// A problem is about what the file SAYS, not about a line of it: annotate the job.
			out = append(out, fmt.Sprintf("::error title=%s::%s", escapeProp("robotsmith check"), escapeData(p)))
		}
		out = append(out, annotateFindings("robotsmith check", d.URL, d.Findings)...)
		return out
	default:
		return nil
	}
}

func annotateFindings(title, file string, findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		level := "warning"
		if f.Severity == "ERROR" {
			level = "error"
		}
		props := []string{"file=" + escapeProp(file)}
		// ⚠️ No line is better than a wrong line: a finding about the file as a whole (an empty
		// file, a cross-domain sitemap) would send a reviewer to an innocent line 1.
		if f.Line > 0 {
			props = append(props, fmt.Sprintf("line=%d", f.Line))
		}
		props = append(props, "title="+escapeProp(title))
		out = append(out, fmt.Sprintf("::%s %s::%s", level, strings.Join(props, ","), escapeData(f.Message)))
	}
	return out
}

// escapeData escapes a workflow command's message, per GitHub's own rules.
func escapeData(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}

// escapeProp escapes a property VALUE, where a comma or a colon would close the list early.
func escapeProp(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(s)
}
