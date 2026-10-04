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

## Architon Studio

Architon Studio imports two files from the hardware project. It does not read `rv scan --format json` stdout. From the KiCad project directory, `rv export .` runs the scan pipeline once and writes:

- `.architon/studio/report.json` — the verification report (`report_version`, `design_ir`, `findings`)
- `.architon/studio/graph.json` — the GraphIR (`graph_version`, `nodes`, `edges`, `findings_index`)

Exit codes match `rv scan`. Exit `2` still writes both files. Exit `3` writes nothing new when the pipeline fails before a report exists. Upload both files as one artifact named `architon-studio`. Use `if: always()` so the artifact is kept when violations fail the export step.

The workflow below runs the [container image](CLI.md#container-image), so the job installs neither Go nor KiCad. GitHub's Ubuntu runners already have Docker. `--user` lets the container write to the checkout, and `--network none` keeps the export offline. Use `ghcr.io/badimirzai/architon:v0.15.0` in place of `ghcr.io/badimirzai/architon` to pin a release. If the KiCad project is in a subfolder, mount that folder: `-v "$GITHUB_WORKSPACE/hardware":/project`, and update the artifact paths to match.

```yaml
name: Architon Studio

on:
  pull_request:
  push:
    branches: [main]

jobs:
  architon-studio:
    runs-on: ubuntu-latest
    permissions:
      contents: read

    steps:
      - uses: actions/checkout@v4

      - name: Export Architon Studio files
        run: |
          docker run --rm --pull always --network none \
            --user "$(id -u):$(id -g)" \
            -v "$GITHUB_WORKSPACE":/project \
            ghcr.io/badimirzai/architon

      - name: Upload Architon Studio files
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: architon-studio
          path: |
            .architon/studio/report.json
            .architon/studio/graph.json
          if-no-files-found: ignore
```

This example is for a hardware project that contains a discoverable BOM, netlist, or root KiCad schematic. It is separate from the `--format github` workflow above. If the job installs `rv` with Go instead, `rv export .` writes the same two files.

## Testing This Source Repository

This repository is the `rv` source tree, not a KiCad project, so `rv scan .` and `rv export .` are expected to exit `3` here. Do not add `rv export .` to this repository's own CI. The checked-in example workflow uses deterministic fixtures instead:

- `internal/importers/kicad/testdata/bom_minimal.csv` must scan cleanly.
- `testdata/esp32_overvoltage/netlist.net` with `testdata/esp32_overvoltage/meta.yaml` must emit a GitHub error annotation and exit `2`.

The `Container Image` workflow builds the Docker image and runs `scripts/docker-smoke.sh`, which exports `examples/agent-loop/broken` with `--network none` and expects exit `2`. On a `vX.Y.Z` tag it pushes `ghcr.io/badimirzai/architon:<tag>` and `:latest`. It never runs `rv export .` on this repository. GHCR creates a new package as private. After the first publish, set the `architon` package visibility to Public in its GitHub package settings, or `docker pull` fails with `denied`.

That keeps pushes and PRs green when behavior is correct while still proving that GitHub annotation output works. For a hardware project repository, use `rv scan . --format github` once the project root contains a discoverable BOM, netlist, or root KiCad schematic.

For an external demo project, [badimirzai/architon-kicad-demo](https://github.com/badimirzai/architon-kicad-demo) is a BOM CSV demo. Its README documents `rv scan bom/bom.csv`; it is not intended to prove `rv scan .` project auto-discovery.

## Fail PRs on Violations Only

To allow warnings but fail on contract violations or tool failures, capture the scan status and exit only for codes `2` and `3`.

```yaml
- name: Run Architon scan
  shell: bash
  run: |
    set +e
    rv scan . --format github
    scan_status=$?
    set -e
    if [ "$scan_status" -ge 2 ]; then
      exit "$scan_status"
    fi
```

## Machine-Readable JSON

Use `--format json` when another tool needs a stable CI schema on stdout.

```bash
rv scan . --format json > architon-ci-report.json
```

`rv scan` also writes the full deterministic scan report to `architon-report.json` by default. Override that path with `--out` when needed.

JSON findings for a wrong interface pin include `expected` and `observed`. `interface_pin_mismatch` puts the contract pin token in `expected.text` and the netlist pin name, or pin number, in `observed.text`. `interface_pin_conflict` puts the other signal in `expected.text` and that pin in `observed.text`. Markdown and GitHub annotation output print the finding message.

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
    scan_status=$?
    set -e
    echo "status=$scan_status" >> "$GITHUB_OUTPUT"

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
