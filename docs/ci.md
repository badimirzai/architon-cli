# CI and PR Review Output

`rv scan` is deterministic and keeps the same exit-code contract in CI:

| Exit code | Meaning |
| --- | --- |
| 0 | Clean scan or informational findings only |
| 1 | Warnings were found |
| 2 | Contract violations were found |
| 3 | Tool, import, or internal failure |

`--format json`, `--format markdown`, and `--format github` do not emit ANSI color. For human-readable logs, use `--no-color` or `NO_COLOR=1` when your runner needs plain text.

## GitHub Actions Annotations

Use `--format github` to emit GitHub Actions annotations while preserving the scan exit code. Exit code `2` fails the PR when contract violations are present.

```yaml
name: Architon Hardware Review

on:
  pull_request:
  push:
    branches: [main]

jobs:
  architon:
    runs-on: ubuntu-latest
    permissions:
      contents: read

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Install rv
        run: go install ./cmd/rv

      - name: Run Architon scan
        run: rv scan . --format github

      - name: Upload JSON report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: architon-report
          path: architon-report.json
          if-no-files-found: ignore
```

The scan step does not need `continue-on-error`. GitHub Actions fails the job on exit code `2`, and exit code `1` also marks the step as failed if you choose to treat warnings as blocking.

## Fail PRs on Violations Only

To allow warnings but fail on contract violations or tool failures, capture the scan status and exit only for codes `2` and `3`.

```yaml
- name: Run Architon scan
  shell: bash
  run: |
    set +e
    rv scan . --format github
    status=$?
    set -e
    if [ "$status" -ge 2 ]; then
      exit "$status"
    fi
```

## Machine-Readable JSON

Use `--format json` when another tool needs a stable CI schema on stdout.

```bash
rv scan . --format json > architon-ci-report.json
```

`rv scan` also writes the full deterministic scan report to `architon-report.json` by default. Override that path with `--out` when needed.

```bash
rv scan . --format json --out architon-full-report.json > architon-ci-report.json
```

## PR Comment Markdown

Use `--format markdown` to generate a PR-comment-ready review. Capture the exit code, post the comment, then exit with the original status so PR failure behavior is preserved.

```yaml
- name: Generate Architon PR review
  id: architon
  shell: bash
  run: |
    set +e
    rv scan . --format markdown > architon-review.md
    status=$?
    set -e
    echo "status=$status" >> "$GITHUB_OUTPUT"

- name: Post Architon PR comment
  if: always() && github.event_name == 'pull_request'
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const body = fs.readFileSync('architon-review.md', 'utf8');
      await github.rest.issues.createComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        issue_number: context.issue.number,
        body
      });

- name: Preserve Architon exit code
  if: always()
  run: exit "${{ steps.architon.outputs.status }}"
```

For the comment step, add `pull-requests: write` or `issues: write` permissions according to your repository policy.

## Artifact Upload

`rv scan . --format github` writes `architon-report.json` unless `--out` is set. Upload it with `if: always()` so the report is available even when the scan fails.

```yaml
- name: Upload Architon report
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: architon-report
    path: architon-report.json
    if-no-files-found: ignore
```
