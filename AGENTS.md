# AGENTS.md

This repository holds research projects. A project is one directory: a question, the sources that settle it, the discussion that got there, and the code or data that checks the result. The directory is the unit of work, the unit of review and the unit of archival.

Read this file before creating, continuing or archiving a project.

## Gates

These decisions already hold. When a prompt names something a gate forbids, name the gate back and stop.

1. **A project starts with an agreed README, never with a search.** A new research area begins with the discovery interview in [Starting a project](#starting-a-project) and ends it by writing `inbox/<slug>/README.md`. The user confirms that file before the first source. A project without a confirmed README collects a reading list: no part of it states which findings count.
2. **`CHATLOG.md` is verbatim and append-only.** Copy every message the user writes in as typed: typos, half sentences, corrections, reversals. Leave every line above the last one alone. A correction becomes a new line further down.
3. **A claim with no reference is not a finding.** Every sentence in `FINDINGS.md` cites a row of `REFERENCES.md` by its identifier. Delete a sentence with no citation, or move it into the chat log, where unsourced opinion belongs.
4. **The shortlist is three.** `FINDINGS.md` holds three candidates and one recommendation among them. Under three means the search stayed too narrow. Over three means the README's criteria fail to separate the candidates. Fix the criteria before growing the shortlist.
5. **Record what lost.** Every candidate the project read and dropped gets a row in `REFERENCES.md` with the reason. A reader who cannot see the dropped options repeats the search to trust the recommendation.
6. **The root `README.md` is the owner's hand-written text.** Edit it on request, never as a side effect of other work.

## Layout

```
README.md               the owner's text about the repository
AGENTS.md               this file
templates/              the four per-project files, unfilled
inbox/<slug>/           a project in flight
<year>/<slug>/          an archived project
```

`inbox/` holds as many projects at once as are open. A `<year>/` directory archives the projects whose first commit landed in that year.

A project directory:

```
inbox/<slug>/
  README.md             scope and expectations, agreed with the user up front
  CHATLOG.md            the discussion, verbatim, IRC format
  REFERENCES.md         every source read, appraised, including the dropped
  FINDINGS.md           the shortlist of three and the recommendation
  artifacts/            code, data, benchmark output, charts, captures
  notes/                per-source extracts and working notes
```

Create `artifacts/` and `notes/` when something goes in them. The four markdown files exist from the first commit, with `README.md` filled and the other three holding their headers.

Slugs are lowercase, hyphen-separated, and name the subject of the question: `ebpf-command-tracing`. A slug that names the method, `research-tracing-options`, tells a later reader less.

## Starting a project

When the user requests a new research project area, hold off on creating the directory. Run the discovery interview first. The interview produces the question, and a question written before the interview is the wrong one.

Put these seven to the user in a single round, as a numbered list. Accept short answers, infer the rest, and state each inference in the README for them to correct.

| # | Question                                                                                                            | What it sets                                |
|---|---------------------------------------------------------------------------------------------------------------------|---------------------------------------------|
| 1 | What is the question, in one sentence, phrased so that an answer to it can be wrong?                                | The title and the stop condition            |
| 2 | What decision or implementation waits on this?                                                                      | Research or reading                         |
| 3 | What does a finished project contain: a recommendation, a benchmark table, a working prototype, a patch?            | The shape of `FINDINGS.md` and `artifacts/` |
| 4 | What makes one option better than another, and which of those weighs most?                                          | The criteria table and its order            |
| 5 | What falls out of scope: languages, platforms, licences, versions, hosted services?                                 | The exclusion list                          |
| 6 | What have you already tried or already ruled out, and why?                                                          | The seed rows of `REFERENCES.md`            |
| 7 | What counts as evidence: a measurement this project runs, a primary specification, a vendor's number, a forum post? | The evidence bar                            |

The interview is the first entry in `CHATLOG.md`, verbatim, including the questions in the form they went out. Write the log and the README in the same commit.

Then restate the scope as `README.md` from [`templates/README.template.md`](templates/README.template.md) and request a confirmation or an amendment. The confirmation lands as a line in the chat log. Research begins after that line.

## Running a project

The loop runs search, appraise, extract, shortlist, check. Each pass leaves a trace in a file. The project then explains how it reached its result as well as the result itself.

### 1. Search broad, then narrow

Search for synthesised evidence before primary material: a specification, a maintained comparison, a paper with a method section, a benchmark published alongside its harness. A synthesised source costs less to appraise and names the primary ones worth reading.

Record every search as a `* claude` action line in the chat log with the exact query. A query reproduces; a description of a query does not.

### 2. Appraise before extracting

Grade each source on the way into `REFERENCES.md`:

| Grade | Means                                                                                                  |
|-------|--------------------------------------------------------------------------------------------------------|
| A     | Primary: a specification, the source code, or a measurement this project runs                          |
| B     | Secondary with a stated method: a paper, or a vendor document that states numbers and their conditions |
| C     | Unverified: a blog post, a forum answer, a release note, a model's recollection                        |

A grade C source seeds a search and cannot support a recommendation. When a C claim matters, promote it: find the A or B source behind it, or measure it here and file the measurement as grade A.

### 3. Extract into the register

A source's row holds the claim the project took from it. A long extract goes into `notes/<reference-id>.md`. Copy a figure with its units and the conditions of its measurement; never paraphrase one.

### 4. Shortlist three

Score the surviving candidates against the README's criteria, in the README's order. Three go into `FINDINGS.md`. A candidate that got as far as a read and lost stays in `REFERENCES.md` with the reason. A reader then audits the shortlist without repeating the search.

### 5. Check the recommendation against the question

Before reporting, read `README.md` again and answer its question directly. A recommendation that answers a nearby question is the common failure of a long research pass. Where the project changed the question, amend the README and note the change in the chat log. Where it answered a different question, say so in `FINDINGS.md` under `Scope drift`.

## The chat log

`CHATLOG.md` is an IRC log of the project, and the only place the discussion lives. The log holds the reasoning no artifact records: the user's request, their rejections, and the turn that changed the direction.

Format, one line per utterance:

```
[2026-11-05 14:22] <black> can we trace every exec on the box without a kernel module?
[2026-11-05 14:23] <claude> ebpf tracepoints reach it. three ways in, all needing CAP_BPF.
* claude searches "ebpf exec tracing CO-RE libbpf vs cilium/ebpf"
* claude reads man 7 bpf-helpers
[2026-11-05 14:31] <claude> cilium/ebpf is the only pure-go loader. see REFERENCES.md R4.
--- topic: narrowing to CO-RE portability
```

Rules:

- The timestamp is local time at minute resolution, in brackets.
- A nick in angle brackets opens an utterance. The user's nick is the local part of the git author email, `black` here. The agent's nick is `claude`.
- A line starting `* <nick>` is an action: a search, a file read, a build, a measurement. The action names the exact query, path or command.
- A line starting `--- topic:` marks a change of direction.
- A line starting `--- note:` holds an out-of-band fact: a link pasted without comment, an interruption, a session resuming days later.
- A fenced code block inside an utterance holds what someone pasted or what a command printed.
- An agent line is the substantive reply: one utterance per turn, trimmed to the facts and the question it sends back. Tool work goes in action lines.

Copy user lines; never compose one. Where the user spoke outside the session and no record holds the exact words, write a `--- note:` line with the decision and the fact that it fell outside the session.

## The references register

One append-only table, with an identifier per row that findings cite:

| ID | Source | Type | Date | Grade | Claim taken | Verified by | Verdict |
|----|--------|------|------|-------|-------------|-------------|---------|

`Verdict` is one of `shortlisted`, `rejected: <reason>`, or `background`. The reason names what the candidate cannot do: `rejected: needs a kernel module` over `rejected: not suitable`.

A URL goes in whole. A page that matters gets a markdown copy under `notes/`, because a grade C source disappears.

## The findings

`FINDINGS.md` answers the README's question in its first paragraph, then scores the three candidates in a single table against the README's criteria, then opens one section per candidate: its behaviour, its cost, the options it rules out later, and the reference identifiers behind each. The recommendation names one of the three and the condition that would reverse it.

A reversal condition is mandatory. Without one, a reader cannot revisit the recommendation when the inputs move, and repeats the project to replace it.

## Archival

`git mv` moves a project from `inbox/<slug>/` to `<year>/<slug>/`, where `<year>` is the year of the first commit that touched it:

```sh
git log --reverse --format='%ad' --date=format:'%Y' -- inbox/<slug> | head -1
```

The leaf project is the unit. Split a directory that holds more than one project and file each part under its own first-commit year. Where the split loses the grouping, the old parent becomes a prefix on the slug: `golang/parsing` archives as `2018/golang-parsing`.

A project archives once `FINDINGS.md` holds a recommendation, or once work on it stops. An abandoned project archives with its question, its log, and a `FINDINGS.md` naming what stopped it. A later pass then skips the searches already run.

A project whose code outgrew this repository leaves a tombstone `README.md` naming the new location. The chat log and the references stay behind. `2026/go-callback-overheads` is one.

The archive pass runs around January and files the previous year. No schedule drives it today.

## Conventions this repository inherits

- Prose follows `/docs:tone`: plain ASCII punctuation, no AI register, measured numbers in tables.
- Markdown formatting follows `atkins mdox:fmt` where a pipeline exists. This repository has no root pipeline. Hand-format to the same shape: single spaces in table cells.
- Commits follow `/git:commit`: `type(scope): subject`, one line, with the project slug as the scope.
- `/discover` reads this file as the gate list for this repository.
