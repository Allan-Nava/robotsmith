# Changelog

Every user-visible change lands here, newest first
([Keep a Changelog](https://keepachangelog.com/en/1.1.0/), [SemVer](https://semver.org/)).
A release cuts a tag `vX.Y.Z` and bumps the `version` constant in [main.go](main.go); `go install`
consumers get exactly what a tag says. Pushing is always the maintainer's call, never an agent's.

## [Unreleased]

### Added
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

[Unreleased]: https://github.com/Allan-Nava/robotsmith/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Allan-Nava/robotsmith/releases/tag/v0.1.0
