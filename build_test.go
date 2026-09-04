package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Go version is stated in four places that must agree: a Docker image built against an older
// toolchain than go.mod requires fails at build time, and a release built against a different one
// than CI tested is a different binary.
func TestGoVersionIsTheSameEverywhere(t *testing.T) {
	goMod := readDoc(t, "go.mod")
	m := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindStringSubmatch(goMod)
	if m == nil {
		t.Fatal("go.mod has no go directive")
	}
	want := m[1]
	for _, f := range []string{".github/workflows/ci.yml", ".github/workflows/release.yml", "Dockerfile"} {
		if !strings.Contains(readDoc(t, f), want) {
			t.Errorf("%s does not use Go %s (go.mod does)", f, want)
		}
	}
}

func TestEveryDocumentedInstallMethodExists(t *testing.T) {
	// The README promises four ways in. A promise with no file behind it is the worst kind.
	readme := readDoc(t, "README.md")
	for _, c := range []struct{ promise, file string }{
		{"brew ", "Formula/robotsmith.rb"},
		{"docker run", "Dockerfile"},
		{"Releases", ".github/workflows/release.yml"},
		{"go install", "main.go"},
	} {
		if !strings.Contains(readme, c.promise) {
			t.Errorf("README.md does not document %q", c.promise)
			continue
		}
		if _, err := os.Stat(c.file); err != nil {
			t.Errorf("README.md documents %q but %s does not exist", c.promise, c.file)
		}
	}
}

func TestHomebrewFormulaIsShaped(t *testing.T) {
	f := readDoc(t, "Formula/robotsmith.rb")
	for _, want := range []string{
		"class Robotsmith < Formula",
		"github.com/Allan-Nava/robotsmith",
		`depends_on "go" => :build`,
		"test do", // brew audit refuses a formula with no test block
		"main.version=",
	} {
		if !strings.Contains(f, want) {
			t.Errorf("the formula is missing %q", want)
		}
	}
	// The release automation rewrites the version and the checksum: both need a line it can find.
	if !regexp.MustCompile(`(?m)^\s+url ".*/v[0-9.]+\.tar\.gz"`).MatchString(f) {
		t.Error("the url must carry a v-tag the release workflow can rewrite")
	}
	if !regexp.MustCompile(`(?m)^\s+sha256 "`).MatchString(f) {
		t.Error("the formula needs a sha256 line for the release workflow to fill in")
	}
	if !strings.Contains(readDoc(t, ".github/workflows/release.yml"), "Formula/robotsmith.rb") {
		t.Error("the release workflow must update the formula, or it goes stale after the first tag")
	}
}

func TestDockerImageRunsAsNobodyAndFromScratch(t *testing.T) {
	// This binary reads a file and makes two GET requests: it has no business owning a shell, a
	// package manager or root inside a container.
	d := readDoc(t, "Dockerfile")
	for _, want := range []string{"FROM scratch", "ca-certificates.crt", "USER 65534", "ENTRYPOINT"} {
		if !strings.Contains(d, want) {
			t.Errorf("Dockerfile is missing %q", want)
		}
	}
	if !strings.Contains(d, "CGO_ENABLED=0") {
		t.Error("a scratch image needs a static binary: CGO_ENABLED=0")
	}
	if _, err := os.Stat(".dockerignore"); err != nil {
		t.Error("without .dockerignore the build context carries the git history into the image build")
	}
}
