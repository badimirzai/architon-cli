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

## Contents

The HTML report includes:

- Header with project path, PASS/WARN/FAIL status, rv version, and report version
- Summary cards for violations, warnings, loaded contracts, contract coverage, and rail coverage
- Findings table with severity, contract ID, source, component, net, message, and fix
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

## Offline Guarantees

The report is a single HTML file. It uses Go `html/template`, inline CSS, and
embedded JSON only. It does not load external JavaScript, external CSS, CDN
resources, web fonts, or remote images.
