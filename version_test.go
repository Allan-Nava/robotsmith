package main

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionStringCarriesTheCommitOutsideARelease(t *testing.T) {
	// A development build must never look like a release: a bug report has to be tied to a commit.
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
		{Key: "vcs.modified", Value: "true"},
	}}
	got := versionString("dev", info)
	if !strings.Contains(got, "0123456789ab") || !strings.Contains(got, "dirty") {
		t.Errorf("version = %q, expected the short commit and the dirty marker", got)
	}
	if strings.Contains(got, "0123456789abcdef0123") {
		t.Errorf("version = %q: the commit must be shortened, it goes in a one-line banner", got)
	}
}

func TestVersionStringOfATaggedBuild(t *testing.T) {
	// Release builds inject the tag with -ldflags: it must be reported verbatim.
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abcdef1234567890"},
		{Key: "vcs.modified", Value: "false"},
	}}
	got := versionString("v0.2.0", info)
	if !strings.HasPrefix(got, "v0.2.0 (abcdef123456") || strings.Contains(got, "dirty") {
		t.Errorf("version = %q", got)
	}
}

func TestVersionStringFallsBackToTheModuleVersion(t *testing.T) {
	// `go install module@v0.2.0` stamps nothing in vcs.*: the tag only shows up in Main.Version.
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}}
	if got := versionString("dev", info); got != "v0.2.0" {
		t.Errorf("version = %q, expected v0.2.0", got)
	}
}

func TestVersionStringWithoutBuildInfo(t *testing.T) {
	if got := versionString("dev", nil); got != "dev" {
		t.Errorf("version = %q, expected the bare version", got)
	}
}

func TestVersionCommandPrintsIt(t *testing.T) {
	code, stdout, _ := runCLI("version")
	if code != 0 || !strings.HasPrefix(stdout, "robotsmith ") {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestVersionStringIgnoresAPseudoVersionWhenTheCommitIsKnown(t *testing.T) {
	// ⚠️ Go stamps a pseudo-version in Main.Version for a plain `go build` inside a repo
	// (`v0.0.0-20260904085357-474c20152ff0+dirty`). Preferring it repeats the commit twice and
	// dresses a development build as something installable: when vcs info is there, it wins.
	info := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.0.0-20260904085357-474c20152ff0+dirty"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "474c20152ff0abcdef"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	got := versionString("dev", info)
	if got != "dev (474c20152ff0, dirty)" {
		t.Errorf("version = %q, expected \"dev (474c20152ff0, dirty)\"", got)
	}
}
