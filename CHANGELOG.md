# Changelog

Every user-visible change lands here, newest first
([Keep a Changelog](https://keepachangelog.com/en/1.1.0/), [SemVer](https://semver.org/)).
A release cuts a tag `vX.Y.Z` and bumps the `version` constant in [main.go](main.go); `go install`
consumers get exactly what a tag says. Pushing is always the maintainer's call, never an agent's.

## [Unreleased]

### Added
- **`advise --policy <file.json>`** ([internal/policy](internal/policy/)): a site's own answers where
  it disagrees with the built-in table, and the reasoning then names the rule that fired and the
  file it came from — a decision with invisible provenance cannot be argued with. First match wins;
  `family` is optional and only needed to reclassify. The parser is **strict**: an unknown key, an
  unknown value, a broken pattern or a rule an earlier one already covers is an error (exit 2), and
  a missing file is exit 4 — falling back to the defaults would quietly apply the opinion the site
  explicitly rejected. JSON, not YAML: a YAML parser would be a dependency.
- **`check --expect <file.json>`**: the same policy file, used to verify what got **deployed** —
  per-decision pass/fail with the served file's own line quoted, and an explicit note when an answer
  was merely *inherited from `*`* rather than written for that crawler. It **replaces** the built-in
  cases: if you stated your policy, yours is the contract, because otherwise a deliberate exception
  would fail a built-in case forever and a check that is red by design stops being read. `review`
  and `ignore` rules are not assertions and are skipped; so is a regex pattern, out loud.
- `matcher.Decide` returns a verdict with its reasoning — the deciding rule, its line, the group as
  spelled in the file, and whether it came from `*`. `Allowed` is now a thin wrapper over it, so the
  two cannot drift.
- **A GitHub Action** ([action.yml](action.yml)): three lines to put robotsmith in a pipeline.
  Composite, not Docker — it downloads the release binary for the runner and **verifies it against
  the release's own `checksums.txt`** before running it. Exposes `exit-code` and `output` so a later
  step can comment or open a ticket, and `fail-on-findings: false` reports without failing. This
  repo's CI uses it on `docs/robots.txt`, so a mistake in the action is a red check here first.
- **`--format text|json|github`** on `check` and `lint`. `github` emits workflow annotations **on
  the offending line**, which is the only place a reviewer reads them; the escaping lives in
  `internal/report.Annotations` with tests, because an unescaped newline silently truncates a
  workflow command. `--json` stays as shorthand for `--format json`: pipelines pin it.
- **The tool now eats its own cooking.** [docs/robots.txt](docs/robots.txt) is the file this project
  would publish for its own site, held to robotsmith's standard by robotsmith's tests on every run:
  clean lint, and all 32 expected cases satisfied. [docs/sitemap.xml](docs/sitemap.xml) ships too,
  because the host-root file that governs this site lists only sitemaps answering 200.
  [dogfood.yml](.github/workflows/dogfood.yml) runs weekly against the published site: it fails on
  what this repo can fix (our own file, our own sitemap, a *structural* defect in the shared
  host-root file) and only **reports** the host-root policy, which belongs to the account owner —
  a red build nobody here can act on is a red build everybody learns to ignore.

  ⚠️ Worth writing down: this is a project page, and crawlers read robots.txt **only from the host
  root**, so `allan-nava.github.io/robotsmith/robots.txt` governs nothing. The published copy says
  so in its own header rather than looking right and doing nothing — the exact trap this tool
  exists for.

### Changed
- **Homebrew is now `brew install --cask Allan-Nava/tap/robotsmith`**, published automatically to
  the shared tap at every tag instead of tapped from this repository. The self-tap formula built
  from source and needed a Go toolchain on the user's machine; the cask ships the release binary
  and installs on Linuxbrew too (its only artifact is `binary`, which Homebrew does not treat as
  macOS-only). Release artifacts are now `tar.gz` archives rather than bare binaries, which is what
  a cask can consume.

  ⚠️ If you installed the old way, `brew untap Allan-Nava/robotsmith` first.

### Removed
- **`Formula/robotsmith.rb`.** The tap is the only Homebrew path now: a formula here would be a
  second place for the version to go stale, and the tap already has the drift detection this repo
  would otherwise have had to grow.

### Added
- **The install is verified, and staleness is caught.** [brew.yml](.github/workflows/brew.yml)
  installs the cask on macOS and Linux after every release and every Monday, and asserts the
  installed version equals `releases/latest` — the one check that catches a tap left behind when
  the release's last step fails. It also re-checks the exit-code contract and that the quarantine
  attribute was stripped. Before the first tag it skips with a notice instead of failing: there is
  no cask to install yet, and a weekly red nobody can act on is one everybody learns to ignore.

- **The "document everything" rule is now a gate.** A test walks `.github/workflows/` and fails when
  a workflow has no row in CLAUDE.md's *Automation* table — `dogfood.yml` had none, which is exactly
  how automation ends up running unexplained. It joins the gates already covering flags, exit codes,
  JSON schemas, the Go version and the install methods.

_Next up: [v0.4.0 — Your policy, verified continuously](BACKLOG.md#v040--your-policy-verified-continuously)._

## [0.3.0] — 2026-09-04

**Sharper advice** — same inputs, more of the reasoning made visible. No change to the exit codes or
to the JSON schemas (only optional fields were added).

### Added
- **`advise --diff`**: reviews what applying the advice would change against `--current`, instead
  of printing a second file to compare by eye — added groups, groups the file contradicts (`!`,
  with the current directive quoted) and groups carried over verbatim (`~`). An empty diff says so
  in one line.
- **`robotsmith crawlers`** (`--json`, `robotsmith.crawlers/1`): publishes the classification table
  in evaluation order — family, policy, the token it writes, and why. The opinion this tool applies
  is now readable before running it on anyone's logs.
- **`check --sitemaps`**: asks every `Sitemap:` URL whether it actually answers, reporting status,
  content type and size. Off by default — a verification must not make network calls nobody asked
  for. A sitemap that 404s after a migration used to be invisible until Search Console complained.
- **Evidence behind a `REVIEW`**: from a `--log` input, an unrecognised crawler now comes with the
  paths it asked for and the time span it spread over — a crawl over eight hours is a different
  proposition from a burst, and a share alone cannot tell them apart. Also in the JSON document as
  an optional `evidence` field. A `--ua-counts` input claims nothing, because it cannot know.
- `matcher.Group.AgentsRaw`: the user-agent tokens as written, so anything rewriting a group keeps
  the spelling its author chose.

### Fixed
- `lint robots.txt --format github` parsed as `--format robots.txt`: a value-flag missing from the
  argument reordering swallowed the operand. Found by running it by hand, then closed as a class —
  a test now walks the real flag sets and fails when a value-flag is not declared to the reordering.
- **Hand-written groups are no longer dropped.** `advise` preserved only the `*` group: a group
  someone wrote for a crawler the logs never showed (`User-agent: YandexBot`) disappeared from the
  generated file. They are now carried over verbatim, in their own commented section, and named in
  the diff. This was the one damage the tool must never do
  ([INTENT.md](INTENT.md): *the worst damage this tool can do is lose a rule a person wrote*).

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

[Unreleased]: https://github.com/Allan-Nava/robotsmith/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Allan-Nava/robotsmith/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Allan-Nava/robotsmith/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Allan-Nava/robotsmith/releases/tag/v0.1.0
