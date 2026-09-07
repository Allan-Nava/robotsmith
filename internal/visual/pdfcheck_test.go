package visual

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var reXrefEntry = regexp.MustCompile(`(?m)^(\d{10}) 00000 n $`)

// checkXref verifies that every cross-reference offset points at the byte where its object starts.
// ⚠️ This is the classic hand-rolled-PDF bug: a wrong offset produces a file that opens in one
// viewer and silently fails in another, and "it opened on my machine" is not a test.
func checkXref(b []byte) error {
	i := bytes.LastIndex(b, []byte("startxref"))
	if i < 0 {
		return fmt.Errorf("no startxref")
	}
	var start int
	if _, err := fmt.Sscanf(string(b[i:]), "startxref\n%d", &start); err != nil {
		return fmt.Errorf("unreadable startxref: %w", err)
	}
	if start <= 0 || start >= len(b) || !bytes.HasPrefix(b[start:], []byte("xref")) {
		return fmt.Errorf("startxref points at %d, which is not the xref table", start)
	}
	for n, m := range reXrefEntry.FindAllSubmatch(b, -1) {
		off, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return err
		}
		want := []byte(fmt.Sprintf("%d 0 obj", n+1))
		if off < 0 || off >= len(b) || !bytes.HasPrefix(b[off:], want) {
			return fmt.Errorf("xref entry %d points at byte %d, where %q does not start", n+1, off, want)
		}
	}
	return nil
}

var reCourierLine = regexp.MustCompile(`BT /F3 ([\d.]+) Tf ([\d.]+) ([\d.]+) Td \((.*)\) Tj ET`)

// checkLineWidths verifies that no monospaced line runs past the right margin. Courier's advance
// width is exactly 0.6 em, so this is arithmetic rather than eyeballing — and a line that overflows
// is text the reader silently does not get, on a medium with no reflow.
func checkLineWidths(b []byte) []string {
	var over []string
	for _, m := range reCourierLine.FindAllSubmatch(b, -1) {
		size, _ := strconv.ParseFloat(string(m[1]), 64)
		x, _ := strconv.ParseFloat(string(m[2]), 64)
		// ⚠️ Count GLYPHS, not bytes: `\(` is two bytes and one character, and measuring the escaped
		// form would report an overflow that does not exist.
		text := strings.NewReplacer(`\(`, "(", `\)`, ")", `\\`, `\`).Replace(string(m[4]))
		w := x + float64(len([]rune(text)))*size*0.6
		if w > pageW-marginX+0.5 {
			over = append(over, fmt.Sprintf("%.0fpt past the margin: %q", w-(pageW-marginX), text))
		}
	}
	return over
}
