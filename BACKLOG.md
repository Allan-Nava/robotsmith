# Backlog & roadmap — single source of truth

Every todo for this repository lives here, grouped by **milestone**. This file *is* the roadmap:
there is no generated roadmap page, on purpose — one file cannot drift out of sync with itself, and
this repo is small enough that a generator would cost more than it buys.

GitHub milestones mirror the headings below, character for character
([v0.2.0](https://github.com/Allan-Nava/robotsmith/milestone/1),
[v0.3.0](https://github.com/Allan-Nava/robotsmith/milestone/2)). Issues are optional; when one
exists it is linked in the item's `ref`.

## Writing an item

- An item starts with ``### `<stable-id>` — <Title>`` — the id is kebab-case and **never changes**,
  because it is what links the item to its issue and to the changelog entry.
- Metadata as bullets: **status** (`open` | `done`), **priority** (`low` | `medium` | `high`),
  **labels**, **milestone**, **ref** (issue, doc or code path).
- The prose says the **why** and, on the last line, **Done when:** — the observable condition that
  closes it. Since work here is test-first ([CLAUDE.md](CLAUDE.md)), "Done when" is almost always
  "a test that failed first now passes".
- Closing a todo: set `status: done` (keeps the history) and add the line to
  [CHANGELOG.md](CHANGELOG.md). Remove an item only if it was never released.

## Roadmap at a glance

| Milestone | Theme | Open | Done |
|---|---|---:|---:|
| [v0.2.0 — Fit for a pipeline](#v020--fit-for-a-pipeline) | make the tool consumable by machines, and cut real releases | 0 | 7 |
| [v0.3.0 — Sharper advice](#v030--sharper-advice) | better answers on the same input | 4 | 0 |
| [Backlog (unscheduled)](#backlog-unscheduled) | worth doing, not worth scheduling | 3 | 0 |

---

# v0.2.0 — Fit for a pipeline

> ✅ **Complete in the working tree** — every item below is implemented and tested. The milestone
> closes when the maintainer pushes and tags `v0.2.0`: the release workflow then builds the binaries
> and takes its notes from [CHANGELOG.md](CHANGELOG.md).

**Goal**: `robotsmith` runs unattended in someone else's CI. That means machine-readable output, a
strict mode, logs arriving the way logs actually arrive, and installable binaries — plus the tests
that make those promises checkable.

### `json-output` — `--json` on all three commands

- **status**: done
- **priority**: high
- **labels**: cli, ci
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: `internal/report`, `TestJSONOutputIsValidAndCarriesItsSchema`

Exit codes say *whether* something is wrong; a pipeline that wants to comment on a PR, open a
ticket or trend the numbers needs *what*. Today that means parsing prose that
[CLAUDE.md](CLAUDE.md) explicitly reserves the right to reword. One stable schema per command
(`check`: cases, failures, deindex flag, cache headers; `lint`: findings with severity and line;
`advise`: decisions with family, policy, share and reason), versioned by a `schema` field so a
future change does not break consumers silently. The human output stays the default.

**Done when:** a golden test per command asserts the exact JSON for a fixed input, and the file
still goes to stdout with the JSON on stdout only when `--json` is given.

### `lint-strict` — `--strict`: warnings fail too

- **status**: done
- **priority**: medium
- **labels**: cli, ci
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: `TestExitCodeContract` (case "lint --strict on a warning")

`lint` exits 1 only on `ERROR`, which is right by default: `Allow: /` in the wrong place is legal
and works with Google. But a team that has decided the file must be correct under *both* parsers
(the RFC one and a first-match one) has no way to enforce that decision in CI today.

**Done when:** `--strict` makes any finding exit 1, the default behaviour is unchanged, and a test
covers both.

### `logs-stdin-gzip` — read logs from stdin and from `.gz`

- **status**: done
- **priority**: high
- **labels**: cli, ux
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [logs_test.go](logs_test.go)

Real access logs are rotated and gzipped, and they usually arrive down a pipe
(`zcat access.log.*.gz | robotsmith advise --log -`). Today `--log` takes a path and the file has to
be decompressed first, which on a busy edge node means writing gigabytes to disk to answer a
question about user-agents.

**Done when:** `--log -` reads stdin, a `.gz` path is decompressed transparently (detected by the
gzip magic bytes, not by the extension — rotated names lie), and both are covered by a test on a
fixture built in `t.TempDir()`.

### `cli-integration-tests` — cover `cmdCheck` / `cmdLint` / `cmdAdvise`

- **status**: done
- **priority**: high
- **labels**: tests
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [cli_test.go](cli_test.go) — `main` coverage 84.5%

`main` sits at 33% coverage: the three `cmd*` functions — where exit codes, stdout/stderr split and
flag handling live, i.e. exactly the contract other people's pipelines depend on — are untested.
The exit codes are documented as a contract and nothing checks them.

**Done when:** each command has a table test asserting exit code, what lands on stdout and what
lands on stderr (against a local `httptest` server, never the network), `main` coverage ≥ 70%, and
the `0/1/2/4` contract is asserted case by case.

### `release-workflow` — tagged releases with binaries, version from the tag

- **status**: done
- **priority**: medium
- **labels**: ci, release
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [.github/workflows/release.yml](.github/workflows/release.yml), [version_test.go](version_test.go)

`version` is a constant in [main.go](main.go): a binary built from any commit claims to be `0.1.0`,
so a bug report cannot be tied to code. And there are no downloadable binaries, which forces a Go
toolchain on anyone who just wants to run the check in a container.

**Done when:** a `release` workflow triggered by `v*` tags publishes linux/darwin (amd64/arm64)
binaries with checksums, the version is injected with `-ldflags -X`, `robotsmith version` prints the
tag plus the commit, and a plain `go build` still reports a sensible development version.

### `docs-cli-sync-gate` — CI catches documentation that drifts from the CLI

- **status**: done
- **priority**: low
- **labels**: ci, docs
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [docs_test.go](docs_test.go)

The `31` vs `32` cases bug was exactly this class: the docs stated a number the code no longer
produced, and nothing noticed. The same exposure exists for the exit codes and the flag list, which
appear verbatim in [README.md](README.md) and in `docs/index.html`.

**Done when:** a test (or a small CI step) fails when the exit codes and the flags documented in
`README.md` / `docs/index.html` no longer match the ones the binary actually exposes.

### `check-case-count-from-data` — the case count comes from the tables

- **status**: done
- **priority**: medium
- **labels**: correctness, docs
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: `TestUsageStatesTheRealNumberOfCases`, [CHANGELOG.md](CHANGELOG.md)

The help text claimed 31 verified cases while the tables held 32. A literal count lies the first
time a crawler is added. Now derived from `len(check.MustPass)+len(check.MustBeBlocked)` and
guarded by a test.

---

# v0.3.0 — Sharper advice

**Goal**: better answers on the same input. Nothing here changes the contract; it changes how much
a person learns from one run.

### `advise-diff` — show the delta against the current file, not the whole file

- **status**: open
- **priority**: high
- **labels**: advise, ux
- **milestone**: v0.3.0 — Sharper advice

`advise --current` already reads the existing file, but it prints a whole new one: the reader has to
diff two ~60-line files by eye to see that the advice is "block two crawlers, keep everything else".
Reviewing a change is what decides whether the advice gets applied — and the asymmetric risk
([INTENT.md](INTENT.md)) means the review has to be easy, or it does not happen.

**Done when:** `--diff` prints only the added, removed and preserved groups with the reason for each
change, and a test asserts that a run whose advice changes nothing prints an empty diff.

### `crawlers-command` — `robotsmith crawlers` prints the classification table

- **status**: open
- **priority**: medium
- **labels**: advise, ux
- **milestone**: v0.3.0 — Sharper advice

The policy table is the opinionated core of the tool, and today the only way to see it is to read
`internal/advise/advise.go`. Someone deciding whether to trust `advise` needs to see what it
believes — family, policy and the token it would write — before running it on their logs.

**Done when:** the command prints family, policy, reason and robots.txt token for every rule (plus
`--json`), and a test asserts every rule in the table is reachable in the output — so a rule shadowed
by an earlier pattern becomes visible instead of silently dead.

### `sitemap-reachability` — check the `Sitemap:` actually answers

- **status**: open
- **priority**: low
- **labels**: lint, check
- **milestone**: v0.3.0 — Sharper advice

`lint` verifies the sitemap host, which catches the copy-paste-from-another-site case. It does not
catch the more common one: a sitemap that 404s or redirects after a migration, which is invisible
until Search Console complains weeks later.

**Done when:** `check` optionally fetches every `Sitemap:` URL and reports status, content type and
size, off by default (a lint must not make network calls without being asked) and tested against a
local server.

### `unknown-crawler-evidence` — say *what* an unknown crawler asked for

- **status**: open
- **priority**: medium
- **labels**: advise
- **milestone**: v0.3.0 — Sharper advice

For an unknown but heavy crawler the tool says "review this" and hands over a percentage. The
decision a person actually has to make — is this a channel or a leech? — needs the shape of the
traffic: which paths, spread over how many hours, always the same URLs or a crawl frontier.
`--log` already reads the lines that carry it, and throws them away.

**Done when:** for every `REVIEW` decision the report adds the top paths and the observed time span,
computed only when a `--log` is given (a `--ua-counts` input cannot know), with a test on a fixture
log.

---

# Backlog (unscheduled)

Worth doing, not worth scheduling. Not "next": pulling one of these in means moving it to a
milestone first.

### `haproxy-log-format-coverage` — validate the UA extraction against real formats

- **status**: open
- **priority**: medium
- **labels**: tests, advise

`readLog` takes the quoted fields and drops the request line and the referer. It is tested against
nginx `combined`; HAProxy's `httplog` puts captured headers in `{braces}`, and a custom
`log-format` can put the UA anywhere. The failure mode is silent: no crash, just user-agents that
never show up in the advice.

**Done when:** a fixture per real format (nginx combined, HAProxy httplog with captured headers,
HAProxy custom log-format) is asserted, and a format the parser cannot handle produces a warning
instead of a silently short list.

### `robots-txt-of-this-repo` — dogfood the tool on its own Pages site

- **status**: open
- **priority**: low
- **labels**: docs, ci

The site published at `allan-nava.github.io/robotsmith` has no `robots.txt`. A tool that advises
about robots.txt not shipping one is a small credibility hole, and running `check` against its own
site in CI turns the tool into its own smoke test.

**Done when:** `docs/robots.txt` exists, is generated by `advise` (or hand-written and passed by
`lint`), and a scheduled CI job runs `robotsmith check` against the published site.

### `crawler-table-provenance` — date and source every rule

- **status**: open
- **priority**: low
- **labels**: advise, docs

The tables in `internal/advise` and `internal/check` encode facts about the outside world that
change: a crawler is renamed, an AI vendor splits its UA in two, a token is retired. Nothing records
when a rule was added or where the claim came from, so nobody can tell a stale rule from a current
one.

**Done when:** every rule carries a source and a date (comment or struct field), and the
documentation says how often the table should be revisited.
