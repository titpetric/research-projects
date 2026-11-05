# References: <project slug>

Append-only. Every source read gets a row, including the ones that lost, and the reason they lost. `FINDINGS.md` cites rows by `ID`. [../AGENTS.md](../AGENTS.md#2-appraise-before-extracting) defines the grades and the verdicts.

| Grade | Means                                                                                                  |
|-------|--------------------------------------------------------------------------------------------------------|
| A     | Primary: a specification, the source code, or a measurement this project runs                          |
| B     | Secondary with a stated method: a paper, or a vendor document that states numbers and their conditions |
| C     | Unverified: a blog post, a forum answer, a release note, a model's recollection                        |

`Verdict` is `shortlisted`, `background`, or `rejected: <concrete reason>`. A reason names what the source or candidate cannot do.

## Register

| ID | Source                     | Type                                                    | Date       | Grade | Claim taken                                       | Verified by                                                | Verdict            |
|----|----------------------------|---------------------------------------------------------|------------|-------|---------------------------------------------------|------------------------------------------------------------|--------------------|
| R1 | <title, with the full URL> | spec / source / paper / vendor doc / post / measurement | YYYY-MM-DD | A     | <the figure or statement taken, in its own units> | <the A or B source, the run, or the file that confirms it> | shortlisted        |
| R2 |                            |                                                         |            |       |                                                   |                                                            | rejected: <reason> |

## Seeds

<!-- Rows from the README's "Already tried" section. The user supplied these
claims; grade them like any other source before using one. -->

## Promotions

<!-- A grade C claim that mattered, chased down to its A or B source or measured
here. One line each: the C row, the claim, the row that now holds it. -->

| From | Claim | Now held by |
|------|-------|-------------|

## Searches run

<!-- The queries, in order, copied from the `* claude searches` lines of the chat
log. A query is reproducible; a description of one is not. Keep this list so a
later pass can tell which angles were already swept. -->

| # | Query or source consulted | New rows |
|---|---------------------------|----------|
| 1 |                           |          |
