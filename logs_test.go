package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const nginxLine = `1.2.3.4 - - [01/Sep/2026:10:00:00 +0000] "GET /a HTTP/1.1" 200 12 "-" "Mozilla/5.0 (compatible; GPTBot/1.4)"`

func gzipped(t *testing.T, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReadLogFromStdin(t *testing.T) {
	// Rotated logs arrive down a pipe: `zcat access.log.*.gz | robotsmith advise --log -`.
	obs, err := readLog("-", strings.NewReader(nginxLine+"\n"+nginxLine+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].Requests != 2 {
		t.Fatalf("expected one user-agent seen twice, got %+v", obs)
	}
}

func TestReadLogDecompressesGzipFromStdin(t *testing.T) {
	obs, err := readLog("-", bytes.NewReader(gzipped(t, nginxLine+"\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs) != 1 || obs[0].UA != "Mozilla/5.0 (compatible; GPTBot/1.4)" {
		t.Fatalf("a gzipped stream must be decompressed transparently, got %+v", obs)
	}
}

func TestReadLogDetectsGzipByContentNotByName(t *testing.T) {
	// ⚠️ Rotated names lie: `access.log.3`, `access.log-20260904`, `access.log` fed by a pipe. The
	// magic bytes are the only thing that tells the truth.
	dir := t.TempDir()
	misnamed := filepath.Join(dir, "access.log") // gzip content, no .gz extension
	if err := os.WriteFile(misnamed, gzipped(t, nginxLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	obs, err := readLog(misnamed, nil)
	if err != nil {
		t.Fatalf("a gzipped file without the .gz extension must still be read: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 user-agent, got %+v", obs)
	}

	plainNamedGz := filepath.Join(dir, "access.log.gz") // plain content, .gz extension
	if err := os.WriteFile(plainNamedGz, []byte(nginxLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if obs, err = readLog(plainNamedGz, nil); err != nil {
		t.Fatalf("a plain file named .gz must not be treated as compressed: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 user-agent, got %+v", obs)
	}
}

func TestReadLogOnAnEmptyStreamIsNotAnError(t *testing.T) {
	// An empty rotated log is normal; it must produce no advice, not a failure.
	obs, err := readLog("-", strings.NewReader(""))
	if err != nil {
		t.Fatalf("an empty stream is not an error: %v", err)
	}
	if len(obs) != 0 {
		t.Errorf("expected no observation, got %+v", obs)
	}
}

func TestAdviseReadsAGzippedLogEndToEnd(t *testing.T) {
	p := filepath.Join(t.TempDir(), "access.log.gz")
	if err := os.WriteFile(p, gzipped(t, strings.Repeat(nginxLine+"\n", 40)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI("advise", "--log", p)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "User-agent: GPTBot") {
		t.Errorf("the advice must reach the generated file:\n%s", stdout)
	}
}
