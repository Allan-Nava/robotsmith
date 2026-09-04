# CLAUDE.md — how to work on robotsmith

Instructions for Claude Code (and for anyone else) working on this repository.
The **why** behind the choices lives in [INTENT.md](INTENT.md): read it before changing the
algorithm. Conventions for automated agents live in [AGENTS.md](AGENTS.md).

## What this is

A Go CLI, zero external dependencies, that **verifies** a `robots.txt` and **advises how to write
it** starting from the site's real traffic. Three commands: `check` (the file as served), `lint`
(structural defects), `advise` (the file derived from the logs).

## Working rules (ALWAYS)

- **Every user-visible change gets a [CHANGELOG.md](CHANGELOG.md) entry** under `## [Unreleased]`
  (Keep a Changelog sections: Added / Changed / Fixed / Removed), **without being asked**. A
  release then moves that block under `vX.Y.Z` and tags `git tag -a vX.Y.Z`: **minor** for new
  behaviour or removals, **patch** for fixes. The version is **not** a constant — it is stamped in
  from the tag with `-ldflags -X main.version=`, so nothing has to be kept in sync by hand; the
  release workflow refuses to publish a tag whose CHANGELOG section is missing.
- **Commit your own work**, in logical commits, with a message saying what changes and **why** (no
  file lists, no `Co-Authored-By`). A commit is local and revertible: leaving changes uncommitted
  just means somebody else has to write your message for you.
- **NEVER `git push`.** That is the maintainer's call, always. Same for `gh release`: the tag is
  what triggers the public release, so an agent creates the annotated tag **locally** only when
  asked, and never pushes it.
- **Document always, without asking.** A behaviour change touches [README.md](README.md) (usage) and
  `docs/index.html` (the public page); a debatable decision gets a dated line in
  [INTENT.md](INTENT.md); a threshold, exit code or output format change gets one in the
  *Invariants* section below.
- **Todos live only in [BACKLOG.md](BACKLOG.md)** — single source, stable kebab-case ids, grouped by
  milestone (that file *is* the roadmap; its headings mirror the GitHub milestones character for
  character). Do not scatter `TODO` comments in the code and do not open a second list.
- **An item without a milestone is not "next".** The *Backlog (unscheduled)* section is explicitly
  not a queue: pulling something out of it means moving it into a milestone first, deliberately.
- **Keep everything aligned.** One factual change propagates to: code, tests, `README.md`,
  `docs/index.html`, `CHANGELOG.md`, `BACKLOG.md`. A number stated in prose that the code no longer
  produces is a bug — that is exactly how the "31 cases" defect survived (see
  `check-case-count-from-data`).

## Commands

```bash
go build ./...                 # build
go test ./...                  # tests (must be green before every commit)
go test -race ./...            # as in CI
go test -cover ./...           # coverage per package
gofmt -l .                     # must print NOTHING: CI fails if it prints anything
go vet ./...
go run . advise --ua-counts ua.txt --host example.com   # try it by hand
zcat access.log.*.gz | go run . advise --log -          # rotated logs, off the pipe
go run ./cmd/backlog-sync --repo Allan-Nava/robotsmith  # dry run of the issue projection
docker build -t robotsmith . && docker run --rm robotsmith version
```

CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs exactly: `gofmt -l`, `go vet`,
`go test -race`, `go build`. If it passes locally it passes there.

## TDD: mandatory, no exceptions

**Every** behaviour change starts from a failing test, in this order:

1. Write the test that describes the wanted behaviour. The name states the behaviour, not the
   function: `TestSmallCandidateComesOutCommented`, not `TestRender2`.
2. Run `go test ./...` and **check that it fails** for the right reason. A test that passes
   immediately is not testing what you think it is.
3. Write the minimum code that makes it pass.
4. Re-run the whole suite, not just the package you touched: the classification tables are shared
   between `advise` and `check`.

Rules that hold for the tests in this repo:

- **One bug found = one test before the fix.** [matcher.go](internal/matcher/matcher.go) has a
  comment citing `TestCommentsDoNotCloseTheGroup`: that is the model. The test stays as a guard,
  and the comment in the code explains why that line is in that order.
- **No network in tests.** `check` is exercised with `httptest.NewServer`
  ([check_test.go](internal/check/check_test.go)), never against real domains: a test that depends
  on `example.com` goes red for reasons that have nothing to do with the code.
- **No files outside `t.TempDir()`.**
- Failure messages must state **got and expected**, so whoever reads CI understands without opening
  an editor.
- Assertions on `advise`/`lint` output check **substance** (a line present, a policy chosen), not
  character-by-character formatting: the prose of the messages will change.

## Layout

| Path | Responsibility | Boundary not to cross |
|---|---|---|
| [main.go](main.go) | CLI: argument parsing, reading logs/files, printing | No policy decision here |
| [internal/matcher](internal/matcher/) | parser + RFC 9309 evaluation | Does not know what "a good crawler" is |
| [internal/lint](internal/lint/) | **structural** defects of the file | Does not judge which bots to block |
| [internal/check](internal/check/) | fetch, comparison with the origin, expected cases | The case list is data, not logic |
| [internal/advise](internal/advise/) | UA classification → policy → file | Does no I/O |
| [internal/report](internal/report/) | the `--json` documents | Decides nothing; only renders |
| [internal/backlog](internal/backlog/) | parses and lints `BACKLOG.md`, plans the issue sync | Does no I/O and knows no HTTP |
| [cmd/backlog-sync](cmd/backlog-sync/) | the GitHub side of that projection | Decides nothing the package did not plan |

The parser is kept separate from the policy **on purpose**: matching correctness is verifiable
against the RFC, policy is an opinion. Do not merge them into one function.

## Invariants not to break

These are the things that, changed by accident, break the tool silently.

1. **The order of `rules` in [advise.go](internal/advise/advise.go) is significant.** The first
   matching pattern wins. `"Googlebot"` contains `"bot"`: a generic rule placed at the top would
   block Google. Add specific patterns **above** generic ones, and add a case to
   `TestClassificationOrder`.
2. **`ChatGPT-User` and `OAI-SearchBot` must come before `gptbot`**: their UAs contain `openai`.
3. **Longest match, not first match** (RFC 9309 § 2.2.2), with `Allow` winning ties. Do not
   "simplify" `Allowed` into a loop that returns on the first match.
4. **The generated file contains no blank line inside a group** and never puts `Allow: /` before
   the `Disallow` rules: those are the two defects `lint` reports. The generator must not produce a
   file its own linter rejects — `TestGeneratedFileHasNoDefectsTheLinterWouldFlag` guards this.
5. **Pre-existing rules are carried over unchanged**, and orphaned ones are **recovered** with a
   warning. Losing a rule a person wrote is the worst damage this tool can do.
6. **Thresholds are named, documented constants**: `MinCandidateShare` (0.5%) and `AutoBlockShare`
   (3%). If you change them, update the comment with the *why*, not just the number.
7. **Process exit codes**: `0` everything as it should be · `1` something is not · `2` usage error ·
   `4` file unreachable. They are a contract for CI pipelines: do not reassign them.
8. **The generated file goes to stdout, messages to stderr**, so
   `robotsmith advise ... > robots.txt` produces a clean file.

## Style

- **Code, comments, messages and documentation are in English.** One language across the whole
  repository, including user-facing output.
- A comment explains **why**, not what. If a line has to be in a precise position, the comment says
  what breaks when it moves. Use `⚠️` for pitfalls that have already cost a bug.
- Package APIs stay conventional Go (`Classify`, `Analyze`, `Render`, `Allowed`); internal helpers
  read as plain English (`reorderArgs`, `readCounts`, `isRequestLine`).
- Zero dependencies outside the stdlib. An extra library in a tool that runs in CI is attack and
  maintenance surface: if a parser is needed, write it.
- No accusatory messages: when the tool cannot know something (browser traffic), it **states** the
  limit instead of insinuating.

## Adding a crawler to the table

1. Add the case to `TestClassificationOrder` with the **real** UA copied from the logs.
2. Check that the test fails.
3. Insert the `rule` at the right place in the order (specific before generic).
4. If it is a crawler that *must* pass or *must* be blocked in `check` too, add it to `MustPass` /
   `MustBeBlocked` in [check.go](internal/check/check.go) — and remember that
   `TestRunOnHealthyFileFindsNoProblem` uses the length of those lists, so update the test's
   `healthy` file accordingly.
5. If it is a token that exists **only** for `robots.txt` (`Google-Extended`,
   `Applebot-Extended`), it will never show up in the logs: it has to be emitted into the file, not
   looked for among the UAs.

## Documenting: always

No change is finished without its documentation. Concretely:

- new or changed behaviour → [README.md](README.md) (usage) and, if it is a debatable choice,
  [INTENT.md](INTENT.md) (the why, with a date);
- changed threshold, exit code or output format → this file, *Invariants* section;
- public page → [docs/index.html](docs/index.html), the source of the GitHub Pages site.

## Site and logo

- [docs/](docs/) is published to GitHub Pages by the
  [pages.yml](.github/workflows/pages.yml) workflow on every push to `main`. It is static HTML with
  no Jekyll (`.nojekyll`): it opens locally by double-clicking, no build step.
- The logo ships as two deterministic files — [docs/logo.svg](docs/logo.svg) (dark ink, for light
  backgrounds) and [docs/logo-dark.svg](docs/logo-dark.svg) (cream ink) — picked by a `<picture>`
  element in the README and in the page. Do **not** put a `prefers-color-scheme` query back inside
  them: a renderer that disagrees with the page theme makes the logo invisible.
  [docs/mark.svg](docs/mark.svg) is the square variant (favicon, avatar) and carries its own panel,
  so it flips background and ink together. Edit them by hand: do not introduce a toolchain to
  generate an image.

## Skills checked into the repo

[.claude/skills/](.claude/skills/) vendors three agent skills so anyone who clones robotsmith gets
them, with no global install and no plugin marketplace:

| Skill | Invoke | What it is for here |
|---|---|---|
| [graphify](.claude/skills/graphify/) | `/graphify` | Turns the repo (or `docs/`, or a log corpus) into a navigable knowledge graph. Once `graphify-out/` exists, questions about architecture and file relationships go through it first instead of a fresh grep sweep. |
| [qrspi](.claude/skills/qrspi/) | `/qrspi` | Questions → Research → Spec → Plan → Implement, one self-contained artifact per phase under `thoughts/<task-id>/`. Use it for changes that span the parser *and* the policy tables, where a single session would drift. |
| [token-efficiency](.claude/skills/token-efficiency/) | `/token-efficiency` | Diagnoses why a session got expensive or started re-reading files it already had. |

They are **copies**, pinned at the version vendored: they do not update when the global skill or the
`qrspi` plugin does. Refresh them deliberately with a commit that says which version came in.
`thoughts/` is a QRSPI working directory, not a documentation surface — the durable decisions still
land in [INTENT.md](INTENT.md), and the todos still live only in [BACKLOG.md](BACKLOG.md).

## Pointers

| File | Question it answers |
|---|---|
| [INTENT.md](INTENT.md) | **Why** the repo exists, what it deliberately does not do |
| [README.md](README.md) | **What** it does (usage, commands, exit codes) |
| this file / [AGENTS.md](AGENTS.md) | **How** work is done here (people / coding agents) |
| [BACKLOG.md](BACKLOG.md) | **What is missing**, by milestone — the roadmap |
| [CHANGELOG.md](CHANGELOG.md) | **What changed**, release by release |
| [.claude/skills/](.claude/skills/) | **Which skills** ship with the repo (`graphify`, `qrspi`, `token-efficiency`) |

## Automation

Nothing here needs a person to remember it:

| Trigger | Workflow | What it does |
|---|---|---|
| push / PR | [ci.yml](.github/workflows/ci.yml) | `gofmt -l`, `go vet`, `go test -race`, `go build`, cross-compile of the four release targets, coverage summary |
| after every release · weekly | [brew.yml](.github/workflows/brew.yml) | `brew install --cask Allan-Nava/tap/robotsmith` on macOS **and** Linux, then asserts the installed version equals `releases/latest` — the only check that catches a tap left behind — plus the exit codes and the quarantine attribute |
| push / PR touching the image | [docker.yml](.github/workflows/docker.yml) | builds the image on a PR, pushes it to GHCR on `main` (`edge`) and on a tag (`X.Y.Z`, `X.Y`, `latest`), multi-arch, then smoke-tests it |
| PR / push touching `BACKLOG.md` | [backlog.yml](.github/workflows/backlog.yml) | **dry run** on a PR, applies on `main`: projects the backlog onto GitHub issues via [cmd/backlog-sync](cmd/backlog-sync/) |
| push to `main` touching `docs/` | [pages.yml](.github/workflows/pages.yml) | publishes `docs/` to GitHub Pages (no Jekyll, no build step) |
| tag `v*` | [release.yml](.github/workflows/release.yml) | runs the suite, extracts the notes from `CHANGELOG.md` (**fails if the section is missing**), then goreleaser cross-compiles linux/darwin × amd64/arm64, publishes the archives + `checksums.txt`, and pushes `Casks/robotsmith.rb` to `Allan-Nava/homebrew-tap` |
| monthly | [dependabot.yml](.github/dependabot.yml) | bumps the workflow actions (Go modules are not listed: zero dependencies is an invariant) |

Cutting a release is therefore: write the CHANGELOG section, commit, `git tag -a vX.Y.Z` (the tag
message is that section), and let the maintainer `git push --follow-tags`. The cask is a side effect
of the tag — ⚠️ it needs the `HOMEBREW_TAP_GITHUB_TOKEN` secret (a PAT that can write to
`Allan-Nava/homebrew-tap`); without it goreleaser publishes the archives and then fails on its last
step, and the tap quietly keeps serving the previous version. The push is the release:
until it happens nothing is public and every step is revertible.
