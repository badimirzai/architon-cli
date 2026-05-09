# Offline HTML Report

`rv report` creates a static, local HTML artifact from the same deterministic
pipeline used by `rv scan` and `rv graph`.

```bash
rv report <path> --format html --out architon-report.html
```

The command runs scan internally, builds GraphIR internally, embeds both JSON
payloads in the HTML, and writes a report that works without a network
connection, external fonts, CDN assets, or frontend frameworks.

## Use Cases

- CI artifacts for pull requests and releases
- Sharing a single visual proof file with teammates
- Inspecting scan findings, contract coverage, components, and rails offline
- Keeping the raw scan report and GraphIR attached to the same artifact

This is not Studio. It is a static report optimized for local viewing and
artifact preview.

## Command

```bash
rv report examples/custom-contracts/exports/custom_contracts.net --format html --out architon-report.html
rv report . --format html --out architon-report.html
rv report . --contracts .architon/contracts.yaml --format html --out report.html
rv report exports/project.net --meta .architon/meta.yaml --format html --out report.html
```

`--format` currently supports only `html`. If `--out` is omitted, the default
output path is:

```text
architon-report.html
```

The command accepts the same scan inputs as `rv scan`: KiCad BOM CSV, KiCad
`.net` netlist, or a project directory with discoverable scan inputs. It also
supports the same project input overrides as scan and graph:

```text
--map
--bom
--netlist
--meta
--contracts
--no-kicad-cli
--kicad-cli
```

## Contracts

For project directories, `rv report` uses the same default contract discovery as
`rv scan` and `rv graph`:

```bash
rv report . --format html --out report.html
```

If `.architon/contracts.yaml` exists in the project, it is loaded. You can also
pin the exact contract file explicitly:

```bash
rv report . --contracts .architon/contracts.yaml --format html --out report.html
```

When the input is a generated netlist under `.architon`, such as
`.architon/generated.net`, the project root is inferred as the parent directory
of `.architon`, so `.architon/contracts.yaml` is still discovered. Standalone
`.net` files outside a project do not guess a contract file; pass `--contracts`
when you want user policies applied.

## Companion Artifacts

The HTML embeds the canonical scan JSON and GraphIR JSON used to render the
page. To write those exact payloads next to the report, use:

```bash
rv report . \
  --contracts .architon/contracts.yaml \
  --format html \
  --out report.html \
  --scan-out report-scan.json \
  --graph-out report-graph.json
```

`--scan-out` writes the exact embedded scan JSON. `--graph-out` writes the exact
embedded GraphIR JSON. If either flag is omitted, only the HTML is written.

Avoid comparing an HTML report with stale JSON files produced by a different
command, different input path, or different flags. Use `--scan-out` and
`--graph-out` when you need artifacts that are guaranteed to match the HTML.

## CI Example

```bash
rv report . \
  --contracts .architon/contracts.yaml \
  --format html \
  --out artifacts/report.html \
  --scan-out artifacts/report-scan.json \
  --graph-out artifacts/report-graph.json
```

The command writes artifacts before returning the scan-style exit code, so CI can
upload `artifacts/report.html`, `artifacts/report-scan.json`, and
`artifacts/report-graph.json` even when violations fail the job.

## Contents

The HTML report includes:

- Header with project path, PASS/WARN/FAIL status, rv version, and report version
- Summary cards for violations, warnings, loaded contracts, contract coverage, and rail coverage
- Findings table with severity, contract ID, source, component, net, message, why it matters, and fix
- Contracts table with loaded contracts, source, severity, and requirement types
- Components table with component reference, value, type, contract coverage, and finding count
- Rails table with rail name, voltage, source, consumers, and finding count
- Embedded scan JSON and GraphIR JSON

The embedded payloads are available in the document as:

```html
<script type="application/json" id="architon-scan-json">...</script>
<script type="application/json" id="architon-graph-json">...</script>
```

## Exit Codes

`rv report` follows `rv scan` exit behavior after the HTML file is written:

```text
0 clean or info-only findings
1 warnings
2 violations
3 internal, import, parse, or tool failure
```

This means CI can upload the report artifact even when violations are present,
then fail the job with exit code `2`.

After writing the report, stdout includes the embedded scan finding count,
embedded graph finding count, user contract count, and exit code. This makes
artifact mismatches visible immediately.

## Offline Guarantees

The report is a single HTML file. It uses Go `html/template`, inline CSS, and
embedded JSON only. It does not load external JavaScript, external CSS, CDN
resources, web fonts, or remote images.
