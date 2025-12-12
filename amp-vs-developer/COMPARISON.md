# Gofsck Project Comparison Report

Cross-project metrics analysis across all workspace modules, measured using recursive package scanning (./...) with full symbol and test coverage detection.

## Test Coverage by Symbol (Coverage & Symbols)

Shows the percentage of exported symbols covered by tests. Covered count includes all symbols matched to test functions; Uncovered count shows symbols without test coverage. Standalone Tests are test functions with no matching symbol. Total Symbols is the sum of covered and uncovered.

| Project | Coverage % | Covered | Uncovered | Standalone Tests | Total Symbols |
|---------|---|---|---|---|---|
| etl | 5% | 131 | 146 | 124 | 277 |
| lessgo | 1% | 49 | 232 | 44 | 281 |
| mig | 5% | 3 | 41 | 1 | 44 |
| platform-app | 1% | 6 | 411 | 2 | 417 |
| platform-blog | 0% | 0 | 24 | 0 | 24 |
| platform-maillist | 0% | 0 | 55 | 0 | 55 |
| platform | 18% | 24 | 65 | 7 | 89 |
| vuego | 32% | 163 | 84 | 50 | 247 |
| yamlexpr | 41% | 122 | 57 | 20 | 179 |

## File-Test Pairing (File Organization)

Measures how many implementation files (.go) have corresponding test files (*_test.go). Paired files are counted when both exist; Unpaired Files and Unpaired Tests indicate orphaned code.

| Project | Files | Tests | Paired | Unpaired Files | Unpaired Tests |
|---------|---|---|---|---|---|
| etl | 69 | 20 | 15 | 54 | 5 |
| lessgo | 40 | 15 | 12 | 28 | 3 |
| mig | 35 | 3 | 3 | 32 | 0 |
| platform-app | 63 | 8 | 8 | 55 | 0 |
| platform-blog | 10 | 0 | 0 | 10 | 0 |
| platform-maillist | 6 | 0 | 0 | 6 | 0 |
| platform | 38 | 18 | 18 | 20 | 0 |
| vuego | 35 | 33 | 23 | 12 | 10 |
| yamlexpr | 23 | 17 | 14 | 9 | 3 |

## File Pairing Ratio

Percentage of implementation files paired with test files. Calculated as (paired / total implementation files) × 100.

| Project | Pairing % | Paired/Total | Unpaired Count |
|---------|---|---|---|
| etl | 21% | 15/69 | 59 |
| lessgo | 30% | 12/40 | 31 |
| mig | 8% | 3/35 | 32 |
| platform-app | 12% | 8/63 | 55 |
| platform-blog | 0% | 0/10 | 10 |
| platform-maillist | 0% | 0/6 | 6 |
| platform | 47% | 18/38 | 20 |
| vuego | 65% | 23/35 | 22 |
| yamlexpr | 60% | 14/23 | 12 |

## Aggregate Metrics Across All Projects

Conclusions from analysis across all projects.

**File-Test Pairing Coverage**

- Analyzed: 9 projects with 319 implementation files and 114 test files
- Paired files: 93 (29% of implementation files)
- Unpaired implementation files: 226 (lacking test counterparts)
- Unpaired test files: 21 (without implementation counterparts)

**Symbol-Test Coverage**

- Total exported symbols: 1613
- Covered by tests: 498 symbols
- Uncovered: 1115 symbols
- Average coverage ratio: 30%
- Test functions without matching symbols: 248 (general/integration tests)

## Coverage Distribution

Projects grouped by test coverage ratio.

- Zero coverage (0%): 2 projects
- Low coverage (1-25%): 5 projects
- Medium coverage (26-75%): 2 projects
- High coverage (76-100%): 0 projects

## File Organization Summary

- Total implementation files: 319
- Total paired: 93 (29% of implementation files)
- Unpaired files/tests: 247

## Projects Requiring Attention (Lowest Coverage)

Projects with the lowest test coverage, ordered by coverage ratio.

- **platform-blog**: 10 files, 0 tests, 0/24 symbols covered (0%)
- **platform-maillist**: 6 files, 0 tests, 0/55 symbols covered (0%)
- **lessgo**: 40 files, 15 tests, 49/281 symbols covered (1%)
- **platform-app**: 63 files, 8 tests, 6/417 symbols covered (1%)
- **etl**: 69 files, 20 tests, 131/277 symbols covered (5%)

---

Generated: Fri Dec 12 21:58:30 CET 2025
Source: Gofsck analysis reports (recursive package scanning)
