# INTENT.md — why robotsmith exists, and why it is built this way

This file is the memory of the decisions. The README says **what** the tool does, CLAUDE.md **how**
to work on it; here is the **why**. It exists so old debates are not re-run and so nobody
"simplifies" something that is complicated for a reason.

## The starting problem

On a real site, some crawlers were taking about **5% of the requests without bringing a single
visit**. The operational question was one: *what do I put in `robots.txt`?* — and it had no
data-backed answer. The user-agent lists you find online are generic; a site's traffic is specific.
A copied list blocks crawlers that site has never seen and ignores the one costing it 6% of its
bandwidth.

Hence the three commands, which answer three different questions:

| Command | Question |
|---|---|
| `advise` | what **should** I write, given my logs? |
| `lint` | does the file I wrote say what I think it says? |
| `check` | is the file crawlers **actually see** the one I wrote? |

## The asymmetry that governs everything

The risk is not symmetric, and that determines nearly every choice in the tool:

- accidentally blocking a scraper → **no visible effect**;
- accidentally blocking Googlebot → the site drops out of the index within weeks, and **you notice
  once the traffic is already gone**.

Concrete consequences in the code:

- the search-engine allowlist is **explicit** in the generated file, even though it is technically
  unnecessary (whatever is not forbidden is already allowed): it makes it obvious to the reader that
  those crawlers are wanted, and it prevents the "block everything except two" that ends in
  deindexing;
- `check` has a dedicated flag (`Deindex`) and an all-caps message for the one urgent case;
- an **unknown** crawler is not blocked by default: below a volume threshold it comes out
  **commented**.

## The non-obvious decisions

### An in-house parser instead of a library
The historical implementations (including Python's stdlib `urllib.robotparser`) apply the **first**
matching rule. RFC 9309 and Google use the **longest** match, with `Allow` winning ties. On

```
User-agent: *
Allow: /
Disallow: /login
```

first-match says `/login` is **allowed**, the RFC says **disallowed**. A tool that advises what to
write must model how real crawlers behave. `lint` still flags that layout, because not every crawler
is compliant: the file has to be correct under **both** parsers.

### `ChatGPT-User` and `OAI-SearchBot` are ALLOW
The name says "AI" and instinct says block. But the first is a fetch **triggered by a person** who
is looking at that page, and the second feeds search results. Blocking them costs visibility
**without removing load**: the worst trade there is. The question the tool asks is not "is this AI?"
but **"does this traffic bring me anything?"**.

### `Google-Extended` and `Applebot-Extended` are not user-agents
They exist **only** as `robots.txt` tokens (they mean «do not use my content for training»). They
never appear in logs and a UA rule looking for them does nothing. That is why the tool **emits**
them into the file instead of hunting for them among the observations.

### The double threshold on unknown crawlers
Two constants, two different reasons:

- `MinCandidateShare` = **0.5%** — below this share an unknown crawler is not even worth a line: it
  adds maintenance without removing load.
- `AutoBlockShare` = **3%** — above this share the line is written **active**, with the number next
  to it. This is the burden of proof flipping: below 3% whoever blocks has to justify it (a wrong
  block is invisible), above 3% the cost is such that whoever does *not* block has to justify it.

### Pre-existing rules are carried over unchanged
They were written for that site by a person who knew something the algorithm does not (admin areas,
login flows, paths that generate load). The worst damage this tool can do is **lose one of them**.
**Orphaned** rules — the ones left after a blank line inside a group, which a strict parser ignored
— are **recovered** and flagged: they were probably intended and simply not working.

### `--origin` instead of a cache-buster
To find out whether a CDN still serves an old version, the instinctive move is `?cb=<timestamp>`. It
does not work: if the query string is not part of the cache key — the **normal** setup for a static
file — both fetches return the same copy, and the comparison says "up to date" while crawlers see
the old one. The only reliable comparison is **public against origin**.

### The closing warning accuses nobody
A browser user-agent with a high share is **normal**: behind one string there are thousands of
people. The tool only says that on that slice it **cannot reach a verdict**, because telling a
person from a disguised scraper needs the **per-IP rate**, which `advise` does not look at. Stating
a limit is information; insinuating a suspicion is not.

### Zero dependencies
The tool is meant to run inside a CI pipeline. Every extra library is attack and maintenance surface
on a binary whose whole job is reading text and making two GET requests.

### Exit codes as a contract
`0` everything as it should be · `1` something is not · `2` usage error · `4` file unreachable.
Telling `1` from `4` matters: "the file says the wrong thing" and "the file is not there" call for
different actions, and in CI the difference between a failing test and downed infrastructure must
not be lost.

### English everywhere
The first version was written in Italian (code, comments, output). It was made English on
2026-09-04: the audience for a robots.txt tool is international, and a mixed-language repository —
Italian identifiers with English Go APIs — costs a translation step to every reader and every
contributor. One language, all the way through: code, comments, CLI output, documentation.

### `--json` is the contract, the prose is not
Exit codes say *whether* something is wrong; a pipeline that wants to annotate a pull request, open
a ticket or trend the numbers needs *what*. Parsing the human output was the only way to get that,
and [CLAUDE.md](CLAUDE.md) explicitly reserves the right to reword it — so the first typo fix would
have broken every consumer. Hence one document per command with a pinned `schema` field: a breaking
change bumps its number (`robotsmith.lint/1` → `/2`) and never edits the shape in place. The advised
file travels *inside* the advise document on purpose: making a consumer run the command twice to get
both the reasoning and the result is how the two drift apart.

### `lint` stays lenient by default, `--strict` is opt-in
`Allow: /` before the prohibitions is legal, and with Google it changes nothing — failing the build
on it by default would make the tool cry wolf, and a tool that cries wolf gets `|| true` appended to
it. But a team that has decided the file must be correct under a first-match parser too had no way
to enforce that decision. `--strict` is that decision, made explicitly by whoever runs it.

### The version comes from the tag, not from the source
A `version` constant means every development build claims to be the last release, and a bug report
cannot be tied to code. It is now stamped in with `-ldflags -X` at release time, and a non-release
build reports its commit plus a dirty marker. Consequence worth stating: the CHANGELOG section is
the *only* thing that has to be written by hand before a tag, and the release workflow refuses to
publish without it.

### Gzip is detected by content, not by file name
Rotated logs are called `access.log.3.gz`, `access.log-20260904`, or nothing at all when they arrive
down a pipe. Trusting the extension means either failing on a compressed file with the wrong name or
mangling a plain file with a `.gz` suffix — both observed in the wild. The magic bytes are the only
thing that tells the truth.

### The documentation is verified, not trusted
The `31` vs `32` cases bug was a document stating a number the code no longer produced, with nothing
to notice. A test now walks the real flag sets and the exit-code table and checks every document
that repeats them. This is deliberately narrow: it covers the facts a machine can check (flags,
codes, schema strings), not the prose — the prose is reviewed by people.

### The issues are a projection, and this reverses an earlier decision
When the backlog was written it explicitly said no automatic sync to issues: 14 items are not 140,
and the machinery looked like it would cost more than it bought. That was wrong in one respect — a
todo that lives only in a file gets read when someone opens the file, and the items most worth doing
are the ones nobody opens the file for. So the file stays the source of truth (it travels with the
code, in the same review, and it can rebuild the issues at any time — the reverse is not true) and
the issues became a projection.

What keeps it safe rather than clever: matching is by a stable `id` carried in a fingerprint comment,
never by title, so editing prose cannot spawn a twin; dry run is the default, because a tool that
writes to a tracker by accident gets run once and never again; and the sync refuses to run on a file
that does not lint, because a typo in a milestone title would silently create a second milestone.

### Packaging: brew and a scratch image
The most likely place this tool runs is somebody's CI, where a Go toolchain is an unwanted
dependency — hence a container image, and hence `scratch` plus the static binary and the CA bundle:
no shell, no package manager, `nobody` as the user. The Homebrew formula is tapped from this
repository instead of a second `homebrew-tap` repo, because a second repo is a second thing to keep
alive; its checksum is rewritten by the release workflow, because a formula updated by hand goes
stale after the first release nobody remembers to edit.

### A diff, because the review is where the decision happens
`advise` was printing a whole new file next to the old one and leaving the reader to compare them.
Given the asymmetry — nobody notices a wrongly blocked scraper, everybody notices a wrongly blocked
Googlebot — a review that is hard simply does not happen, and the mistake it would have caught is
the expensive one. So the diff shows what moves, marks the dangerous case separately (`!`: the file
currently says the opposite of the advice), and says "nothing to change" in one line when that is
the answer.

### Hand-written groups were being lost, and that was the worst bug in the tool
This file said, from the first day, that the worst damage the tool can do is lose a rule a person
wrote — and `advise` preserved only the `*` group, silently dropping a group written for a crawler
the logs never showed. Two things fixed it: the groups are carried over verbatim in their own
section, and the diff **names** them (`~`), because silence is the mechanism by which a rule gets
lost. The parser also keeps the original spelling of a user-agent token now: rewriting `YandexBot`
as `yandexbot` is identical to a crawler and reads, to a person, like the tool mangled their file.

### The policy table is published, not hidden
The classification table is the opinionated core of the tool, and it was readable only by opening
the source. `robotsmith crawlers` prints it in evaluation order — the order is part of the answer,
since the first match wins — and the test that backs it derives a sample user-agent from each
pattern and checks it still classifies as its own rule says. That is what makes a rule shadowed by
an earlier pattern fail the build instead of becoming dead code.

### Fetching a sitemap is opt-in
`lint` checks the sitemap's host, which catches a copy-paste from another site. Checking that it
*answers* catches the more common failure — a 404 after a migration, invisible for weeks — but it
means making a network call, and the sitemap of a large site is a big file. A verification tool that
generates traffic nobody asked for is a tool people stop running, so `--sitemaps` is a request, not
a default.

### Evidence only where it exists
For an unrecognised crawler the decision hinges on the shape of the traffic, not the share alone:
0.4% spread over a crawl frontier is a different proposition from 0.4% in one burst. The log already
carries the paths and the timestamps, and the tool was throwing them away. It shows them **only**
for the decisions a person has to take, and **only** from a real log — a `uniq -c` count cannot know
them, and inventing them would be worse than omitting them.

### Dogfooding, without shipping a decoration
The plan was "publish a robots.txt for our own site". The site is a *project* page, and crawlers read
robots.txt only from the host root — so a file at `…/robotsmith/robots.txt` would have been exactly
the artifact this tool warns about: correct, present, and governing nothing. It ships anyway, for two
reasons that survive the objection: it is what this project would publish if it owned an origin, and
it is held to the tool's own standard by the tool's own tests on every run (clean lint, all 32
expected cases) — plus it says in its own header that only the host-root copy is read. The
`Sitemap:` line is the part with an actual effect: the host-root file lists only sitemaps that
answer 200, which is why `docs/sitemap.xml` now exists.

The scheduled job draws the same line. It fails on what this repo can fix and only *reports* the
policy of the shared host root, which is the account owner's call. A check that goes red for
something nobody here can act on does not create pressure to fix it — it trains people to ignore
red, and then the next real failure is invisible too.

## Non-goals

Stated, not forgotten:

- **It does not tell you whether crawlers obey.** `robots.txt` is a request, not a control. That is
  measured from the logs over the following days.
- **It does not stop whoever disguises itself as a browser.** By construction: there is no token to
  write. That calls for a per-IP request cap or a WAF.
- **It does not look at the per-IP rate.** That would be another tool, with different inputs.
- **It does no fingerprinting** and does not verify the reverse DNS of declared UAs.
- **It does not handle `Crawl-delay`**: Google ignores it, and suggesting it would give false
  confidence.
- **It has no server, daemon or database.** It is a command that reads, decides and prints.

## Log

- **2026-09-04** — first version: `check`, `lint`, `advise`, in-house RFC 9309 parser.
- **2026-09-04** — tests added for `internal/check` (with `httptest`, no network) and for log/count
  parsing in `main`; plus a test asserting the generator never emits a file its own linter would
  reject. Documentation: `CLAUDE.md`, `AGENTS.md`, this file, the logo and the GitHub Pages site.
- **2026-09-04** — whole repository translated to English (identifiers, comments, CLI output,
  documentation). Rationale above under *English everywhere*.
- **2026-09-04** — milestone *v0.2.0 — Fit for a pipeline* implemented: `--json` with pinned
  schemas, `lint --strict`, logs from stdin and gzip, CLI integration tests on the exit-code
  contract, release automation with the version from the tag, and a gate that keeps the docs
  honest. Each decision above; backlog in [BACKLOG.md](BACKLOG.md).
- **2026-09-04** — the tool is now run against its own published site weekly, and the file it
  publishes is checked by its own tests. Reasoning above under *Dogfooding, without shipping a
  decoration*.
- **2026-09-04** — milestone *v0.3.0 — Sharper advice* implemented: `advise --diff`,
  `robotsmith crawlers`, `check --sitemaps`, evidence behind a REVIEW — and the fix for
  hand-written groups being dropped, which was the worst defect in the tool. Reasoning above.
- **2026-09-04** — the backlog is now projected onto GitHub issues automatically, reversing the
  "no sync on purpose" decision taken the same day (reasoning above), and the tool ships as a
  container image and a Homebrew formula.

When you make a decision someone might want to reverse, add it here with the date and the reason.
One line is enough.
