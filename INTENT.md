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
no shell, no package manager, `nobody` as the user. Homebrew goes through
`Allan-Nava/homebrew-tap`, as a cask carrying the release binary: the argument for self-tapping was
that a second repo is a second thing to keep alive, and that stopped being true once the tap was
already alive — five casks, its own CI, and a job that compares every cask against upstream's
latest release every six hours. A formula here would now be the *second* place a version can be
wrong, and the one without a watchdog. Nothing writes a cask by hand; the checksums come from the
real release assets, because a formula updated by hand goes stale after the first release nobody
remembers to edit.

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

### One layout model, two backends — and colour is never the channel
The report exists because the terminal output does not survive the trip to whoever signs off on the
change. Rendering it twice was the obvious risk: two renderers drift, and then the numbers in the
attached deck disagree with the numbers in the tool, which is precisely the credibility the report
was for. So the bars, labels and rows are computed once and two thin backends draw them, with a test
asserting every share in the HTML appears in the PDF.

Three constraints that came from the medium rather than from taste. **Self-contained**: the artifact
is emailed and opened on a laptop with no network, so inline CSS and inline SVG, no JavaScript and
nothing fetched — which also rules out a chart library, consistent with zero dependencies. **No pie
chart**: the argument being made is "this one is worth more than those ten", which a pie destroys at
exactly the sizes that matter; horizontal bars sorted by volume, each labelled at its own end.
**Colour is the second channel, never the first**: every bar carries its policy word and a glyph,
because red and green are one colour to a deuteranope and a printed page has none. The palette was
run through a validator rather than eyeballed; the light amber sits under 3:1 on the light surface,
whose documented relief — visible labels and a full table — is what the report already has.

For the PDF: base-14 fonts, so no embedding and no dependency, and the text stays selectable and
searchable instead of being a picture of a report. Uncompressed content streams, so the same input
produces byte-identical output and a report can be diffed in CI instead of eyeballed. And because a
PDF has no reflow, line widths are checked by arithmetic — Courier advances exactly 0.6 em — rather
than by looking at it once.

### A share is a snapshot; the decision is about the direction
The volume thresholds answer "how much does this cost today", which is the wrong question for
anything small. Two crawlers at 0.4% — one flat for a year, one quadrupled this month — got the same
"leave it commented" answer, and the second is the one worth acting on. Comparing needs no new file
format: the `--json` document already published everything a baseline requires, so the previous run
*is* the baseline. Two guards keep it honest: the movement threshold (±25%) exists so ordinary
wobble is not reported as a trend, because a report that cries trend teaches the reader to skim; and
the document is refused unless it is an advise document of a known schema, since comparing against
the wrong one would invent a trend, and a trend is what somebody acts on.

Vanished crawlers are reported for the opposite reason: a rule that no longer does anything costs
nothing today and is invisible for years. Nobody audits a robots.txt for rules that have stopped
mattering.

### Your expectations replace the defaults, they do not stack on them
`check --expect` verifies the decisions a team actually made. Making those *additional* to the
built-in 32 cases was the tempting design and the wrong one: a site that deliberately allows a
training crawler it has a deal with would fail a built-in case forever, and a check that is red by
design is one people stop reading — the same failure as a job that fails on something nobody can
fix. So stating a policy means adopting it as the contract. The report quotes the deployed file's own
line for every verdict, and says when an answer was merely inherited from `*`: "your rule matched"
and "you inherited the catch-all" are different facts, and only one of them is a decision.

### The policy file is strict, and says whose answer it is
Making the table overridable was the easy half. The half that matters: every decision now carries
the rule that produced it and whether that rule came from the site's file or from the defaults,
because the report exists to be argued with — and an answer whose origin is invisible cannot be. The
parser refuses an unknown key, an unknown value, a bad pattern and a rule an earlier one already
swallows: a policy file that half-works reads exactly like an applied decision and is not one, which
is the same failure mode as a robots.txt that looks right and does nothing. A broken file is exit 2
and a missing one exit 4, never a warning followed by the defaults: applying the opinion the site
explicitly rejected, quietly, would be the worst of both.

### The annotations live in the binary, the action stays thin
An annotation is only useful if it lands on the defective line in the diff view — a summary in the
job log is something nobody scrolls to. Getting there depends on GitHub's escaping rules (an
unescaped newline ends the workflow command, and the rest of a multi-line message vanishes), which
are easy to get wrong and impossible to unit-test in YAML. So the formatting is Go code with tests,
and the action passes `--format` through. It also means anyone can get annotations without the
action, in any CI.

The action downloads a released binary and checks it against the release's own `checksums.txt`
rather than requiring a Go toolchain on the runner. A CI step that curls an unverified binary and
executes it is a supply chain nobody audited; the checksum is the strongest claim available without
a signing key, and it catches the failure that actually happens — a truncated download or an asset
replaced by mistake.

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

### A warning threshold, not a warning on every run

HAProxy's default `httplog` carries no `User-Agent`, so `advise` on such a log used to produce a
short list that looked exactly like a complete one. The obvious fix — warn whenever a line yields no
user-agent — is the wrong one: a `-` user-agent is ordinary in any real log, the warning would fire
on every run, and a warning that always fires is one people learn to scroll past. So the threshold
is **half the lines**: the point where "some clients send no UA" stops being a plausible explanation
and "this parser does not understand this format" starts. The number is a named constant with that
argument next to it, because the right value is a judgement and the next person deserves to see the
judgement, not just the number.

### Provenance on the tables, and an honest "observed"

The tables in `internal/advise` and `internal/check` are the only part of this tool that encodes
facts about the **outside** world, and the outside world moves: a crawler is renamed, a vendor
splits its UA in two, a token is retired. Every rule and every expected case therefore carries the
day it was written and the source it came from, and `crawlers` prints both — because a rule that has
been wrong for a year otherwise looks exactly like one verified this morning.

The temptation was to cite a vendor page for all of them. Half of this table came from real access
logs (Bytespider, YisouSpider, the long tail of SEO crawlers), and inventing a URL for those would
make the field *worse* than absent: a dead link reads as authority. They say
`observed in production access logs`, which is both true and, as provenance goes, actually useful —
it tells the reader which half of the table is documented and which half is empirical.

The revisit cadence lives in code (`advise.TableReviewedEvery`, six months — roughly the interval at
which `Google-Extended`, `Applebot-Extended` and `OAI-SearchBot` appeared) and is printed in the
`crawlers` header. A cadence that lives only in a README is a cadence nobody honours.

### Dogfooding the action twice, and blocking on only one of them

The action was dogfooded here against `releases/latest`, which quietly coupled this repo's green
build to whatever was last published. Between 0.4.0 and 0.5.1 that produced six consecutive red
runs nobody acted on: `--format` had shipped in the branch, the published binary was 0.2.0 and did
not know the flag, and no commit here could fix it. That is the failure this repo already refuses
to tolerate in `dogfood.yml` — a red nobody can act on trains people to ignore red — and it had
grown in the main CI job unnoticed.

So the action now runs twice, because the two things worth catching are not the same thing. Against
the **branch's own binary** it checks `action.yml`'s logic, and that blocks: it is entirely within
this repo's control. Against the **published release** it checks the install path — asset discovery
and checksum verification, the half that had to be fixed once already — and that does not block,
because it depends on an artifact no commit here can change.

⚠️ The `binary` input that makes the first run possible skips the checksum verification along with
the download. That is acceptable for a binary this workflow just compiled from the tree it is
testing, and wrong for anybody else, which is why it is documented as having exactly one caller.

### `@v0`, not `@v1`, and a tag that moves

The documents advertised `@v1` while the newest release was `0.5.1`. The tempting fix is to create
a `v1` tag and be done: the example would start working, and it would be a lie. A `v1` claims a
stability contract this project has not earned, and it would point at a `0.x` release. So the
advertised reference is `@v0` — the major that actually ships — and the day `1.0.0` lands the gate
forces every document to `@v1`, because it derives the expectation from the CHANGELOG rather than
from a constant somebody has to remember.

The tag is **force-moved** on every stable release rather than created once. A tag that is only
created stays frozen on the release it was cut at while looking perfectly maintained, which is the
same failure as a stale Homebrew tap: the thing that is supposed to track the newest release quietly
stops, and nobody finds out until a user does. It moves last, so a release that died before
publishing cannot drag every consumer onto assets that do not exist.

⚠️ And the reason nobody noticed for as long as it existed: CI dogfooded the action as `./`. The
repo ran its own working copy and recommended a reference it had never executed. It now runs `@v0`
for real, in a **job of its own** — which is the part that was got wrong first time and is worth
writing down. `continue-on-error` on a step does not save you from an unresolvable `uses:`: GitHub
resolves every action during *Prepare all required actions*, before any step runs, so the job dies
at setup. Sitting inside the required `test` job, that took branch protection's own check down and
left nothing able to merge. In its own non-required job the reference is still executed for real,
and its failure costs a visible red instead of a frozen repository. It stays red until a release
first moves the tag — honest, and exactly the signal that was missing.

### `main` behind a pull request, with zero required approvals

The repository is one person plus agents, which is exactly the shape where "I will just push this
one" turns into the only review anything gets. `main` is now protected: a change arrives through a
pull request and the `test` check must be green.

Three choices inside that, each of which could have gone the other way:

- **Zero required approvals.** A solo maintainer cannot approve their own pull request, so
  requiring one review would mean nothing could ever merge. The gate here is CI, not a second pair
  of eyes that does not exist.
- **Administrators included.** Otherwise the protection is advice, and the one account that can
  ignore it is the account that writes everything. The escape hatch is turning protection off
  deliberately, which leaves a trace, rather than a habit of pushing past it.
- **Only `test` is required.** ⚠️ `image` and `sync` are path-filtered: they do not run on every
  pull request, and a required check that never starts leaves the PR waiting forever. This is the
  trap to remember before adding another required check.

⚠️ Tags are not covered by branch protection. `release.yml` force-moves `v0`, and a `git push
--tags` still publishes a release — the discipline there is still a human one.

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
- **2026-09-21** — `main` is protected: pull request required, `test` must be green, zero required
  approvals, administrators included. Reasoning above under *`main` behind a pull request*.
- **2026-09-18** — milestone *v0.6.0 — The three lines actually work* complete: the advertised
  action reference is `@v0` and the tag now exists and moves, and CI runs the reference it
  recommends instead of a local path. Reasoning above under *`@v0`, not `@v1`*.
- **2026-09-17** — milestone *v0.5.0 — Trust the input, date the opinion* complete: HAProxy's
  braced captures are read, a log the parser cannot read is said out loud instead of silently
  shortening the advice, and every rule and expected case carries its source and its date.
  Reasoning above under *A warning threshold, not a warning on every run* and *Provenance on the
  tables*.
- **2026-09-07** — milestone *v0.4.0 — Your policy, verified continuously* complete: `--policy`,
  `check --expect`, `advise --compare`, and the HTML/PDF report. Reasoning above.
- **2026-09-04** — shipped as a GitHub Action, with findings as annotations (first item of
  *v0.4.0*). Reasoning above.
- **2026-09-04** — the tool is now run against its own published site weekly, and the file it
  publishes is checked by its own tests. Reasoning above under *Dogfooding, without shipping a
  decoration*.
- **2026-09-04** — milestone *v0.3.0 — Sharper advice* implemented: `advise --diff`,
  `robotsmith crawlers`, `check --sitemaps`, evidence behind a REVIEW — and the fix for
  hand-written groups being dropped, which was the worst defect in the tool. Reasoning above.
- **2026-09-04** — the backlog is now projected onto GitHub issues automatically, reversing the
  "no sync on purpose" decision taken the same day (reasoning above), and the tool ships as a
  container image and a Homebrew formula.
- **2026-09-04** — the Homebrew formula is installed and tested on macOS by CI, not just written.
  The `url`/`sha256` in it are produced by a robot at tag time and consumed by people days later:
  the gap between those two moments is where a broken `brew install` lives, and nothing was closing
  it. The check runs *after* the release commit, on `main`, because the tag's tree still carries
  the previous checksum.
- **2026-09-04** — Homebrew moves to `Allan-Nava/homebrew-tap` as a cask, **reversing** the
  self-tap decision recorded above the same day. The reason it was taken (a second repo is a second
  thing to keep alive) no longer holds: that repo exists, carries four other CLIs, and already
  detects a tap left behind — which this repo did not. The cost is one CI tool, goreleaser, adopted
  because the tap's rule is "no formula, only casks, and nobody writes one by hand" and its
  `brew style` buckets key off goreleaser's own marker.

When you make a decision someone might want to reverse, add it here with the date and the reason.
One line is enough.
