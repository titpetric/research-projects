# <Project title: the question as a statement>

<!--
Filled from the discovery interview in ../AGENTS.md before any source is read,
then confirmed by the user. The confirmation is a line in CHATLOG.md.
Delete every comment block once the section under it is written.
-->

## Question

<!-- One sentence, phrased so that an answer to it can be wrong. -->

## Why now

<!-- The decision or implementation that waits on this. Name the repository, the
file, the commit or the task that waits. -->

## What a finished project contains

<!-- Pick what applies and say which: a recommendation among options, a
benchmark table, a working prototype under artifacts/, a patch, a measurement
that settles a disagreement. This sets the stop condition. -->

## Criteria

<!-- In descending order of weight. FINDINGS.md scores the shortlist against
this table, in this order. A criterion needs a way to tell two options apart.
Write how to measure it. -->

| # | Criterion | How to measure it | Weight |
|---|-----------|-------------------|--------|
| 1 |           |                   |        |
| 2 |           |                   |        |
| 3 |           |                   |        |

## Out of scope

<!-- Languages, platforms, licences, versions, hosted services, cost ceilings.
An exclusion here means a candidate is dropped without being appraised. -->

## Already tried

<!-- What the user brought in: what they tried, what they ruled out and why.
Each of these becomes a seed row in REFERENCES.md. -->

## Evidence bar

<!-- Which grades may support the recommendation. The default admits A and B,
and holds C to seeding a search. Raise or lower it here and say why. -->

| Grade                                                             | Admitted         | Note |
|-------------------------------------------------------------------|------------------|------|
| A primary: specification, source, a measurement this project runs | yes              |      |
| B secondary with a stated method                                  | yes              |      |
| C unverified: blog, forum, release note, recollection             | search seed only |      |

## Status

| Field             | Value                                                                      |
|-------------------|----------------------------------------------------------------------------|
| State             | discovery / searching / appraising / shortlisted / recommended / abandoned |
| Opened            | YYYY-MM-DD                                                                 |
| Last touched      | YYYY-MM-DD                                                                 |
| Sources appraised | 0                                                                          |
| Recommendation    | none yet                                                                   |

## Files

| File                           | Holds                                               |
|--------------------------------|-----------------------------------------------------|
| [CHATLOG.md](CHATLOG.md)       | the discussion, verbatim                            |
| [REFERENCES.md](REFERENCES.md) | every source read, appraised, including the dropped |
| [FINDINGS.md](FINDINGS.md)     | the shortlist of three and the recommendation       |
| `artifacts/`                   | code, data, benchmark output, charts                |
| `notes/`                       | per-source extracts                                 |
