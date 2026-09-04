# Backlog & roadmap — single source of truth

Every todo for this repository lives here, grouped by **milestone**. This file *is* the roadmap:
there is no generated roadmap page, on purpose — one file cannot drift out of sync with itself.

**The GitHub issues are a projection of this file, kept in sync automatically.**
[`cmd/backlog-sync`](cmd/backlog-sync/) parses it and opens, updates, reopens or closes one issue
per item; [.github/workflows/backlog.yml](.github/workflows/backlog.yml) runs it as a **dry run** on
every pull request and applies it on a push to `main`. The join key is the item's `id`, carried in a
fingerprint comment inside the issue body — so editing a title updates the issue instead of opening
a twin, and the projection can be thrown away and rebuilt at any time. The file can never be
reconstructed from the issues, which is why it is the source and they are not.

Edit the file, not the issue: the next sync overwrites anything changed on the GitHub side.
Milestones mirror the headings below, character for character
([v0.2.0](https://github.com/Allan-Nava/robotsmith/milestone/1),
[v0.3.0](https://github.com/Allan-Nava/robotsmith/milestone/2)) — the sync matches them by exact
title and creates a missing one, which is why a title with no matching heading fails the lint.

## Writing an item

- An item starts with ``### `<stable-id>` — <Title>`` — the id is kebab-case and **never changes**,
  because it is what links the item to its issue and to the changelog entry.
- Metadata as bullets: **status** (`open` | `done`), **priority** (`low` | `medium` | `high`),
  **labels**, **milestone**, **ref** (issue, doc or code path).
- The prose says the **why** and, on the last line, **Done when:** — the observable condition that
  closes it. Since work here is test-first ([CLAUDE.md](CLAUDE.md)), "Done when" is almost always
  "a test that failed first now passes".
- Closing a todo: set `status: done` (keeps the history) and add the line to
  [CHANGELOG.md](CHANGELOG.md). Remove an item only if it was never released — in both cases the
  sync closes the issue, with a comment saying why.

The file is linted by `go test ./internal/backlog/`: duplicate ids, unknown statuses, an open item
with no **Done when:**, a milestone that matches no heading, and a summary table whose counts
disagree with the items all fail the build. The counts below are therefore checked, not trusted.

## Roadmap at a glance

| Milestone | Theme | Open | Done |
|---|---|---:|---:|
| [v0.2.0 — Fit for a pipeline](#v020--fit-for-a-pipeline) ✅ | make the tool consumable by machines, and cut real releases | 0 | 10 |
| [v0.3.0 — Sharper advice](#v030--sharper-advice) | better answers on the same input | 4 | 0 |
| [v0.4.0 — Your policy, verified continuously](#v040--your-policy-verified-continuously) | overridable policy, a closed loop, one-line CI, a report you can send | 6 | 0 |
| [Backlog (unscheduled)](#backlog-unscheduled) | worth doing, not worth scheduling | 3 | 0 |

---

# v0.2.0 — Fit for a pipeline

> ✅ **Closed** — every item below is implemented, tested and written up in
> [CHANGELOG.md](CHANGELOG.md) under `0.2.0`. The binaries, the image and the formula bump are
> produced by [release.yml](.github/workflows/release.yml) when the maintainer pushes the `v0.2.0`
> tag; nothing in this milestone is waiting on code.

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

### `backlog-to-issues` — project the backlog onto GitHub issues

- **status**: done
- **priority**: medium
- **labels**: ci, automation
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [cmd/backlog-sync](cmd/backlog-sync/), [internal/backlog](internal/backlog/), [.github/workflows/backlog.yml](.github/workflows/backlog.yml)

A todo that lives only in a file gets read when someone opens the file; one that lives only in a
tracker loses the reasoning that came with it. The file stays the source — it travels with the code,
in the same review — and the issues become a projection that can be rebuilt at any time. Idempotent
by construction: matching is by the item `id` carried in a fingerprint comment, never by title.
Dry run is the default, and the sync refuses to run on a file that does not lint.

### `docker-image` — publish a container image

- **status**: done
- **priority**: medium
- **labels**: packaging, ci
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [Dockerfile](Dockerfile), [.github/workflows/docker.yml](.github/workflows/docker.yml)

The most likely place this tool runs is somebody's CI, where a Go toolchain is an unwanted
dependency. `scratch` plus the static binary and the CA bundle: no shell, no package manager, runs
as `nobody`, and the exit-code contract survives the trip. Built (not pushed) on every pull request
so a broken Dockerfile costs a red check, not a failed release.

### `homebrew-formula` — installable with brew

- **status**: done
- **priority**: low
- **labels**: packaging, ci
- **milestone**: v0.2.0 — Fit for a pipeline
- **ref**: [Formula/robotsmith.rb](Formula/robotsmith.rb)

Tapped straight from this repository, so there is no second repo to keep alive. The formula's
`url` and `sha256` are rewritten by the release workflow at tag time — a formula updated by hand
goes stale after the first release nobody remembers to edit. Its `test do` block checks both ends of
the contract (a clean file lints clean, a defect exits 1) at install time.

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

# v0.4.0 — Your policy, verified continuously

**Goal**: stop being a tool someone remembers to run, and stop being a tool only its operator can
read. The opinion baked into the tables is a good default, not everyone's policy — so make it
overridable, make the loop close (advise → deploy → verify), make it trivial to run on every push,
and make the result something you can hand to whoever actually signs off on it.

### `github-action` — a composite action, so adding this to a pipeline is three lines

- **status**: open
- **priority**: high
- **labels**: ci, ux
- **milestone**: v0.4.0 — Your policy, verified continuously

The tool is CI-shaped — machine-readable output, an exit-code contract, a container image — and
still every team has to write the same twenty lines of workflow to use it. An action that wraps
`check`/`lint`, turns `--json` findings into GitHub annotations on the changed lines and fails the
job on the documented codes removes the only remaining excuse not to run it.

**Done when:** `uses: Allan-Nava/robotsmith@v1` with `command: lint` annotates a defective
`robots.txt` on the right line, a test asserts the annotation format against a fixture document, and
the action is exercised by this repo's own CI (dogfooding, so a broken action is a red build here
before it is a red build for anyone else).

### `policy-config` — let a site override the built-in opinion

- **status**: open
- **priority**: high
- **labels**: advise, ux
- **milestone**: v0.4.0 — Your policy, verified continuously

The policy table is an opinion — a defensible one, and documented as such in
[INTENT.md](INTENT.md) — but a news site and a shop do not owe each other the same answer about
`Bytespider`, and a team that disagrees today has to fork the binary. A policy file makes
disagreement cheap and, more importantly, **written down**: the file says who decided what, and the
advice explains itself against it.

⚠️ JSON, not YAML: a YAML parser is a dependency, and zero dependencies is an invariant
([CLAUDE.md](CLAUDE.md)).

**Done when:** `--policy policy.json` can move a UA pattern to another family or force
allow/block/review, the reasoning line says which rule fired and whether it came from the file or
the defaults, an unknown key or an unreachable pattern is an error rather than silence, and a test
covers an override that contradicts the built-in table.

### `check-expect` — verify what got deployed is what was advised

- **status**: open
- **priority**: medium
- **labels**: check, ci
- **milestone**: v0.4.0 — Your policy, verified continuously

Today `advise` produces a file and `check` verifies a fixed set of expectations. Nothing verifies
the *specific* decisions a team made: block these four, keep these two — which is exactly what
someone silently reverts when they regenerate the file from a copied list, and exactly what a CDN
serves a stale copy of. This closes the loop advise → deploy → verify.

**Done when:** `check --expect policy.json` (the same file as `--policy`) reports per-decision
pass/fail with the deployed file's own lines quoted, exits 1 on any divergence, and is covered
against a local server serving a file that satisfies some decisions and not others.

### `crawler-mix-diff` — compare two observations and say what changed

- **status**: open
- **priority**: medium
- **labels**: advise
- **milestone**: v0.4.0 — Your policy, verified continuously

A share is a snapshot; the decision to block usually hinges on a direction. A crawler at 0.4% that
tripled this month is a different problem from one that has sat at 0.4% for a year, and today the
tool cannot tell them apart — so it says "below the threshold, leave it commented" to both.

**Done when:** `advise --compare previous.json` (a stored `--json` document) reports new, grown,
shrunk and vanished crawlers with the deltas, a crawler under `MinCandidateShare` that grew sharply
is promoted to `REVIEW` with the growth as its reason, and a test covers all four transitions.

### `report-html` — a self-contained visual report

- **status**: open
- **priority**: high
- **labels**: report, ux
- **milestone**: v0.4.0 — Your policy, verified continuously

The decision to block a crawler is rarely taken by the person running the command: it is taken by
whoever reads the report attached to a ticket, and a 60-line terminal dump does not survive that
trip. Today the only shareable artifact is a screenshot of a scrolling table — the volumes, which
are the whole argument, have to be reconstructed in the reader's head.

What it has to show, in the order a reader needs it: how much of the traffic is judged and how much
is not (the browser slice is not a footnote — it is the honesty of the whole method), the decisions
ranked by volume with the reason next to each, the share the advised blocks would remove, and the
generated file itself. Bars sorted by volume with direct labels — **no pie chart**: the argument is
"this one is worth more than those ten", which a pie makes unreadable at exactly the sizes that
matter.

⚠️ **Self-contained**: inline SVG and inline CSS, no JavaScript and no CDN. The artifact gets
emailed, attached to a ticket and opened on a laptop with no network — a report that needs a
CDN is a broken report. Which also rules out a chart library, consistent with the zero-dependency
invariant.

**Done when:** `advise --report report.html` writes one file that opens offline with no console
errors and no external requests (asserted by a test that greps the output for `http://`, `https://`
and `<script`), the numbers in the chart come from the same document `--json` emits (asserted
against the same fixture, so the two renderings cannot disagree), the page prints to one or two
tidy pages via a print stylesheet, and it reads correctly in both light and dark.

### `report-pdf` — the same report as a PDF, from the same layout

- **status**: open
- **priority**: medium
- **labels**: report, ux
- **milestone**: v0.4.0 — Your policy, verified continuously

HTML covers the reader who opens a link; a PDF covers the one who gets it in an attachment, files
it, or puts it in front of someone who signs off on blocking a third of the crawler traffic. The
trap is producing it a second way: two renderers drift, and then the numbers in the deck disagree
with the numbers in the tool — which is exactly the credibility this report exists to have.

So: **one layout model, two backends**. The bars, labels and tables are computed once; the HTML
backend emits SVG, the PDF backend emits PDF drawing operators. No headless browser and no external
converter — a report that only builds where Chrome is installed does not build in CI. Text stays
text (base-14 fonts, no embedding, no dependency), so it is selectable and searchable rather than a
picture of a report.

**Done when:** `advise --report report.pdf` writes a valid PDF that opens in Preview and Acrobat
with selectable text, the same input produces byte-identical output twice (so a report can be
diffed in CI instead of eyeballed), a test asserts the header, the xref table and the trailer plus
that every number present in the HTML report is present in the PDF, and a run with no
`--report` flag behaves exactly as it does today.

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
