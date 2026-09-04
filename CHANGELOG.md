# Changelog

Every user-visible change lands here, newest first
([Keep a Changelog](https://keepachangelog.com/en/1.1.0/), [SemVer](https://semver.org/)).
A release cuts a tag `vX.Y.Z` and bumps the `version` constant in [main.go](main.go); `go install`
consumers get exactly what a tag says. Pushing is always the maintainer's call, never an agent's.

## [Unreleased]

_Nothing yet. Next up: [v0.3.0 — Sharper advice](BACKLOG.md#v030--sharper-advice)._

## [0.2.0] — 2026-09-04

**Fit for a pipeline** — machine-readable output, a strict mode, logs the way logs actually arrive,
installable everywhere, and the automation that keeps all of it honest.

### Added
- **`--json` on `check`, `lint` and `advise`** (`internal/report`): one document on stdout, nothing
  else there, with a pinned `schema` field (`robotsmith.check/1`, `robotsmith.lint/1`,
  `robotsmith.advise/1`). The exit code is unchanged; the advised file travels inside the advise
  document so a consumer never has to run the command twice.
- **`lint --strict`**: warnings fail too, for a team that wants the file correct under a first-match
  parser as well. The default stays "only an ERROR fails".
- **`advise --log -`** reads stdin, and **gzipped logs are decompressed transparently** — detected
  by the gzip magic bytes, not by the file name, because rotated names lie.
- **Release automation**: a `v*` tag cross-compiles linux/darwin (amd64/arm64) with the version
  stamped in via `-ldflags`, takes the notes from this file (and refuses to publish when the section
  is missing) and publishes binaries with checksums. CI cross-compiles the same targets on every
  pull request; Dependabot keeps the actions current.
- **Backlog → GitHub issues, automatically**: [internal/backlog](internal/backlog/) parses and lints
  `BACKLOG.md`, [cmd/backlog-sync](cmd/backlog-sync/) projects it onto issues (create / update /
  reopen / close), matched by the item `id` in a fingerprint comment so an edited title updates the
  issue instead of opening a twin. Dry run is the default; the workflow runs it as a dry run on
  every pull request and applies it on `main`. The lint also fails when the roadmap table's counts
  disagree with the items.
- **Container image**: `scratch` + static binary + CA bundle, running as `nobody`
  ([Dockerfile](Dockerfile)), published to `ghcr.io/allan-nava/robotsmith` on tags (`X.Y.Z`, `X.Y`,
  `latest`) and on `main` (`edge`), multi-arch amd64/arm64, built on every pull request and
  smoke-tested after each push.
- **Homebrew formula** ([Formula/robotsmith.rb](Formula/robotsmith.rb)), tapped straight from this
  repo — `brew tap Allan-Nava/robotsmith https://github.com/Allan-Nava/robotsmith`. Its `url` and
  `sha256` are rewritten by the release workflow at tag time.
- **A documentation gate**: tests assert that every flag the binary exposes is documented (and that
  the docs expose no flag that does not exist), that the exit-code contract is repeated correctly in
  `README.md` and `docs/index.html`, and that the JSON schemas are documented. It also asserts that
  the Go version agrees across `go.mod`, both workflows and the `Dockerfile`, that every documented
  install method has a file behind it, and that the image stays `scratch`-based and non-root.
- Integration tests for the whole CLI (`run()` with injected streams): the `0/1/2/4` contract is now
  asserted case by case — coverage of `main` 33% → 84.5%.
- Tests for `internal/check` on a local `httptest` server (URL normalisation, healthy file, empty
  file, deindexing flag, CDN/origin divergence, 404): coverage 0 → 88.9%.
- Tests for the CLI plumbing in `main` (argument reordering, `uniq -c` parsing, access-log parsing):
  coverage 0 → 33.1%.
- Test asserting the generator never emits a file its own linter would reject (no orphan rules).
- [CLAUDE.md](CLAUDE.md), [AGENTS.md](AGENTS.md), [INTENT.md](INTENT.md), [BACKLOG.md](BACKLOG.md)
  and this changelog.
- Logo (`docs/logo.svg`, `docs/logo-dark.svg`, `docs/mark.svg`) and the GitHub Pages site
  (`docs/index.html`), deployed by [.github/workflows/pages.yml](.github/workflows/pages.yml).

### Changed
- `robotsmith version` reports the tag *and* the commit (dirty marker included); the version is no
  longer a constant, so a development build cannot claim to be a release.
- `main()` is one line: the CLI lives in `run(args, stdout, stderr)`, which is what made the exit
  codes testable.
- **Whole repository translated to English** — identifiers, comments, CLI output, generated file and
  documentation. Rationale in [INTENT.md](INTENT.md) under *English everywhere*.
- `lint` severities are printed as `ERROR` / `WARNING`; thousands in the generated file are grouped
  with a comma.

### Fixed
- The help text claimed **31** verified cases while the tables hold **32** (17 must-pass + 15
  must-be-blocked). The number is now derived from the data, so it cannot drift when a crawler is
  added — guarded by `TestUsageStatesTheRealNumberOfCases`.

### Removed
- Dead helper `short()` in `internal/advise`.

## [0.1.0] — 2026-09-04

### Added
- `check`: fetches the public `robots.txt`, optionally compares it with `--origin` (the only
  reliable way to catch a stale CDN copy), and runs the expected cases.
- `lint`: structural defects — empty file, orphan rules after a blank line, `Allow: /` before the
  prohibitions, cross-domain `Sitemap:`, `Disallow: /` for everyone.
- `advise`: classifies the crawlers observed in the logs (or in a `uniq -c` count) and generates the
  advised file, preserving the existing rules and recovering the orphaned ones.
- `internal/matcher`: RFC 9309 parser and evaluation (longest match, `Allow` wins ties).
- Exit codes `0` / `1` / `2` / `4` as a CI contract.

[Unreleased]: https://github.com/Allan-Nava/robotsmith/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Allan-Nava/robotsmith/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Allan-Nava/robotsmith/releases/tag/v0.1.0
