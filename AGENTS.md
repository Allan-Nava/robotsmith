# AGENTS.md — instructions for automated agents

This file applies to any coding agent (Claude Code, Codex, Copilot Agent, Cursor, Aider…). The full
conventions are in [CLAUDE.md](CLAUDE.md), the reasoning behind the choices in
[INTENT.md](INTENT.md). What follows is the minimum needed to avoid doing damage.

## Context in one line

A Go CLI with no external dependencies that verifies a `robots.txt` and advises how to write it
from the site's logs. `main.go` does I/O and printing; the logic lives in
`internal/{matcher,lint,check,advise}`.

## Working loop

```bash
go test ./...        # 1. start here: if it is red before you touch anything, the red is not yours
# 2. write the TEST for the wanted behaviour
go test ./...        # 3. it MUST fail, for the right reason
# 4. write the minimum code
go test -race ./...  # 5. the whole suite, not just the package you touched
gofmt -l .           # 6. must print nothing
go vet ./...
```

**Do not deliver code without a test that failed first.** This holds for one-line fixes too: in this
repo every interesting bug has been an *ordering* bug between lines that looked equivalent, and a
test found them, not a re-read.

## Five things that look harmless and are not

1. **Reordering the `rules` table** in `internal/advise/advise.go`. The first matching pattern wins
   and `"Googlebot"` contains `"bot"`: an "alphabetical" cleanup blocks Google. Specific always
   above generic.
2. **Simplifying `matcher.Allowed`** into a loop that returns on the first match. RFC 9309 wants the
   **longest** match with `Allow` winning ties; first-match gives the opposite answer on
   `Allow: /` + `Disallow: /login`.
3. **Moving the comment stripping before the blank-line check** in `matcher.Parse`. A comment-only
   line does **not** close the group, a blank line **does**: swapping the order makes them
   indistinguishable.
4. **Changing the exit codes** (`0/1/2/4`). They are a contract with the CI pipelines of whoever
   uses the tool.
5. **Moving output between stdout and stderr.** The generated file goes to stdout, messages to
   stderr: that is what makes `robotsmith advise ... > robots.txt` work.

## Boundaries

- **Do not add dependencies.** Stdlib only. If a parser is needed, write it (there is one already).
- **Do not touch `LICENSE`** or the documented exit codes.
- **No network in tests**: use `httptest.NewServer`. No real domains, not even in runnable examples.
- **Do not introduce a toolchain** for the site or the logo: `docs/` is hand-written HTML and SVG.
- **Keep everything in English**: code, comments, messages and documentation.
- **Do not "clean up" the `⚠️` comments**: they mark pitfalls that have already cost a bug.

## What to deliver

A change is done when: the suite is green with `-race`, `gofmt -l .` is silent, the new behaviour
has its own test, and the documentation is updated (`README.md` for usage, `INTENT.md` for a
debatable choice with a date, `CLAUDE.md` for thresholds/exit codes/formats, `docs/index.html` for
the public page). In the commit message: what changes and **why**, without listing the files.
