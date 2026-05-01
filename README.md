# Architon CLI (rv)
[![CI](https://github.com/badimirzai/architon-cli/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/badimirzai/architon-cli/actions/workflows/ci.yaml) [![Release](https://img.shields.io/github/v/release/badimirzai/architon-cli?label=release)](https://github.com/badimirzai/architon-cli/releases)

Fail fast on hardware integration mistakes before you build the board.

Deterministic hardware architecture verification for robotics and embedded systems.
Runs before PCB fabrication and firmware bring-up to catch integration failures early.

Architon detects electrical compatibility, power, logic-level, and integration failures **before hardware is built or firmware runs**.
Run it locally or in CI to catch integration errors early and reduce costly board spins and bring-up churn.

⭐ If Architon helps you catch a hardware integration issue early, consider starring the repo.

---

## Why Architon exists

Software has compilers and static analysis.  
Hardware lacks a deterministic **system-level** verification step before fabrication.

Architon fills this gap by enforcing architecture contracts across **power, interfaces, and components**.  
It catches failures that typically appear during bring-up, after hardware has already been built.

**Where Architon fits in the hardware lifecycle:**

```text
Design              Verification        Build              Firmware              Physical
KiCad / Altium  →   Architon        →   PCB fabrication → STM32 / ESP32 / ROS → Hardware bring-up
```

---

## What Architon verifies

Architon validates system-level compatibility between components, including:

- Supply voltage compatibility
- Driver and motor electrical compatibility
- Power rail capacity and margin
- Logic voltage compatibility
- I2C address conflicts
- Current margin and stall load conditions

These checks are deterministic and reproducible across local development and CI.

---

## What Architon does
Architon runs after schematic design but before PCB fabrication and firmware bring-up (STM32, ESP32, ROS, etc).
Architon CLI validates hardware architecture from a specification (`.yaml`) and ingests KiCad BOM CSV files and KiCad `.net` netlists to produce a normalized, deterministic DesignIR JSON report.

**Inputs**
- YAML hardware architecture specification (`rv check`)
- KiCad BOM CSV, KiCad `.net` netlists, or a KiCad project directory (`rv scan`)

**Outputs**
- Deterministic exit codes for CI gating
- Machine-readable `report.json` for automation
- Stable normalized DesignIR representation

---

## Example
Run Architon on a real hardware design and detect an integration failure:

![Architon scanning KiCad project demo](docs/demo-readme.gif)
Architon detects integration failures deterministically before hardware is built.

---

## Quick Start

### Install

Requires Go **1.25.5** or newer (https://go.dev/dl/).

```bash
go install github.com/badimirzai/architon-cli/cmd/rv@latest
rv version
rv --help
```


---

## Scan a real KiCad project (30 seconds)
Try Architon on a real KiCad project:

```bash
git clone https://github.com/badimirzai/architon-kicad-demo.git demos
cd demos/demo1-esp32-motor/kicad
rv scan .
```

Expected output example:
```bash
ARCHITON SCAN
Target: .
Result: OK — no scan violations detected
Parts: 7
Nets: 59
Errors: 0
Warnings: 0
Rules: 0
Violations: 0
Inferred voltages: <n> Unknown voltage nets: <n> Rail coverage: <LEVEL> <PCT>%
Detected Netlist: <path>
Wrote architon-report.json
exit code: 0
```
Architon automatically detects KiCad .net netlists and BOM CSV files
and produces a deterministic DesignIR report.

### Try the architecture checker (YAML)


```bash
rv init --list
# templates:
# - 4wd-problem
# - 4wd-clean

rv init --template 4wd-problem
# Wrote robot.yaml (template: 4wd-problem)

rv check robot.yaml
# shows multiple ERROR/WARN findings, exit code 2

rv init --template 4wd-clean --out robot.yaml --force
# Wrote robot.yaml (template: 4wd-clean)

rv check robot.yaml
# clean or notes-only, exit code 0

rv scan examples/bom/bom.csv
# ARCHITON SCAN
# Target: examples/bom/bom.csv
# Result: OK — no scan violations detected
# Parts: 2
# Nets: 0
# Errors: 0
# Warnings: 0
# Rules: 0
# Violations: 0
# Inferred voltages: 0 Unknown voltage nets: 0 Rail coverage: UNKNOWN 0%
# Wrote architon-report.json
# exit code: 0

rv scan examples/bom/bom.csv --map examples/mapping.yaml
# Wrote architon-report.json

rv scan examples/bom/bom.csv --out my-report.json
# Wrote my-report.json

```



## CLI usage

`rv check` validates system architecture from YAML specification.  
`rv scan` imports KiCad BOM/netlist data and generates a normalized DesignIR report.

Core commands:

```text
rv check <file.yaml>       Run deterministic analysis
rv scan <path>             Import BOM CSV, KiCad .net, or project directory and emit DesignIR report JSON
rv init                    Create .architon metadata or write a starter robot spec
rv version                 Show installed version
rv check --output json     Emit JSON findings to stdout
rv --help                  Show all commands and flags
rv check --help            Show check command options
```

Findings severity:

- `INFO` context or non-blocking notes
- `WARN` risk indications
- `ERROR` rule violations

JSON output examples:

```bash
rv check specs/robot.yaml --output json
rv check specs/robot.yaml --output json --pretty
rv check specs/robot.yaml --output json --out-file report.json
rv check specs/robot.yaml --output json --pretty --out-file report.json
```

KiCad scan examples:

```bash
rv scan examples/bom/bom.csv
rv scan examples/bom/bom.csv --map examples/mapping.yaml
rv scan examples/bom/bom.csv --out my-report.json
rv scan exports/project.net --meta .architon/meta.yaml
rv scan exports/project.net --meta .architon/meta.yaml --rails

# KiCad project folder scan (demo repo)
git clone https://github.com/badimirzai/architon-kicad-demo.git demos
cd demos/demo1-esp32-motor/kicad
rv scan .
```



Architon automatically detects:
- KiCad .net netlists
- KiCad BOM CSV files
and produces a normalized deterministic DesignIR report. 

`rv scan .` supports three input modes:

- BOM CSV input such as `bom.csv`
- KiCad `.net` S-expression netlist input such as `exports/project.net`
- Project directory input such as `.`

When you scan a directory, Architon looks for:

- BOM candidates using the existing BOM detection rules: `bom/bom.csv`, `bom.csv`, `exports/bom.csv`, then lexical `*bom*.csv` matches in `bom/`, `exports/`, and the project root
- Netlist candidates in this order: lexical `exports/*.net`, then lexical `*.net` in the project root

If both a BOM and a netlist are found, Architon merges them deterministically into one DesignIR:

- BOM remains the base source of parts and raw `fields`
- Missing BOM `value` and `footprint` fields are filled from matching netlist parts
- Net connectivity is taken from the netlist and exported as `design_ir.nets`

You can override discovery explicitly:

```bash
rv scan . --bom bom/bom.csv --netlist exports/project.net
```

### Voltage rules for KiCad netlists

KiCad netlists provide connectivity, but they do not reliably provide electrical intent such as source voltages, regulator outputs, or component maximum voltage ratings. Architon uses `.architon/meta.yaml` or `--meta` for that data.

For a direct netlist scan, pass the metadata file explicitly:

```bash
rv scan exports/project.net --meta .architon/meta.yaml
```

For a project directory scan, Architon auto-discovers `.architon/meta.yaml`:

```bash
rv scan .
```

Architon deterministically infers obvious rail voltages from net names, then enriches that with metadata sources and regulator outputs from `meta.yaml`. Component limits still come from metadata.

Minimal voltage-rule metadata:

```yaml
version: "0"

sources:
  - net: /VBAT
    voltage: 24.0

regulators:
  - ref: U2
    in_pin: "1"
    out_pin: "3"
    out_voltage: 5.0

components:
  - ref: U1
    max_voltage: 3.3
```

Example overvoltage output:

```text
ARCHITON SCAN
Target: exports/project.net
Result: FAIL — scan violations detected
Parts: 3
Nets: 3
Errors: 0
Warnings: 0
Rules: 1
Violations: 1
Inferred voltages: 2 Unknown voltage nets: 0 Rail coverage: HIGH 100%
Rule findings:
- ERROR RULE_OVERVOLTAGE: U1 pin 1 on net /+5V is 5.00V (max 3.30V)
Wrote architon-report.json
exit code: 2
```

`Errors` are parse/import errors. Rule failures are reported as `Violations`.

Use `--rails` to print deterministic rail inference details, including voltage, confidence level, confidence score, source, and rail coverage. `--explain-rails` remains supported as a legacy alias.

Default output path: `architon-report.json`.

### Rail inference and coverage

Rail voltage inference is deterministic and transparent. Each inference includes source, confidence score, evidence, and warnings. Confidence score indicates the reliability of an inference. Coverage indicates how much of the design can be safely checked by voltage rules. Users can inspect details with `--rails`.

```bash
rv scan example.net
```

Expected output:

```text
Inferred voltages: 2 Unknown voltage nets: 1 Rail coverage: MEDIUM 67%
```

```bash
rv scan example.net --rails
```

Example detail:

```text
Rail inference:

- /+5V: 5.00V  HIGH   0.95  NET_NAME_EXACT
- VIN:  UNKNOWN LOW    0.35  HEURISTIC

Rail coverage:

- Total nets: 3
- Usable for rules: 2/3
- Coverage: MEDIUM 67%
```

High coverage does not guarantee correctness. Coverage indicates how many rails can be evaluated reliably. Low coverage means some checks may be skipped.

---

## Exit codes

`rv check` and `rv scan` return deterministic exit codes designed for CI and automation. Exit codes distinguish between rule findings and tool execution failures.

| Code | Meaning |
|-----:|---------|
| 0 | Clean or informational only. No warnings or violations. |
| 1 | Warnings detected, but no violations. |
| 2 | Rule violations detected. |
| 3 | Tool execution failure, including scan parse errors where analysis could not complete reliably. |

Warnings should be reviewed. CI may allow exit code 1 or treat it as failure using `--warn-as-error`.

For `rv scan`, malformed BOM rows and other parse failures still write a report when possible, then exit 3.

---

## CI integration

Many CI systems fail on any non-zero exit code. To allow warnings but fail on violations:

```yaml
- name: Architon check
  run: |
    rv check robot.yaml
    code=$?
    if [ "$code" -ge 2 ]; then exit "$code"; fi
```

Strict mode (fail on warnings):

```bash
rv check --warn-as-error robot.yaml
```

---

## Structured report output

Architon produces deterministic structured reports for automation.

Default scan output path:

```bash
architon-report.json
```

Custom output path:

```bash
rv check robot.yaml --output json --out-file report.json
rv scan bom.csv --out report.json
```

The report includes:
- summary counts
- findings for `rv check`, or scan `rules` for `rv scan`
- normalized DesignIR for scans, including nets when imported
- rail inference, rail coverage, and voltage-finding provenance for netlist-backed scans

Exit codes indicate pass/fail. The JSON report provides detailed structured results for CI integration and tooling.

For netlist-backed scans, `summary.nets` and `design_ir.nets` are populated. For BOM-only scans, these fields remain omitted to keep the JSON stable.

### Example: `report.json` from `rv scan bom.csv`

```json
{
  "report_version": "0",
  "summary": {
    "source": "kicad_bom_csv",
    "input_file": "bom.csv",
    "parts": 2,
    "rules": 0,
    "has_failures": false,
    "delimiter": ",",
    "parse_errors_count": 0,
    "parse_warnings_count": 0,
    "parse_errors": [],
    "parse_warnings": []
  },
  "design_ir": {
    "version": "0",
    "source": "kicad_bom_csv",
    "parts": [],
    "metadata": {
      "input_file": "bom.csv",
      "parsed_at": "2026-02-26T00:00:00Z"
    }
  },
  "rules": []
}
```

On parse failures, the report still includes `report_version`, `design_ir.version`, `delimiter`, and deterministic guidance in `summary.next_steps`.

### Example: `report.json` from `rv check robot.yaml`

```json
{
  "spec_file": "robot.yaml",
  "summary": {
    "errors": 1,
    "warnings": 2,
    "infos": 1,
    "exit_code": 2
  },
  "findings": [
    {
      "id": "DRV_SUPPLY_RANGE",
      "severity": "ERROR",
      "message": "battery voltage exceeds driver supply range",
      "path": "power.battery.voltage_v",
      "location": {
        "line": 5,
        "column": 5
      },
      "meta": {}
    }
  ]
}
```

---

## Parts lookup (quick reference)

You can reference built-in parts (`parts/`) and project-local parts (`./rv_parts`) with `part:`.

Resolver lookup order (earlier wins):

1. `./rv_parts`
2. `./parts`
3. `--parts-dir` (repeatable)
4. `RV_PARTS_DIRS` (OS path separator: `:` on Unix, `;` on Windows)

Detailed field-level behavior is documented in `docs/spec.md`.

---

## Full example spec

Create `spec.yaml`:

```yaml
name: "minimal-voltage-mismatch"

power:
  battery:
    voltage_v: 12
    max_current_a: 10
  logic_rail:
    voltage_v: 3.3
    max_current_a: 1

mcu:
  name: "Generic MCU"
  logic_voltage_v: 3.3
  max_gpio_current_ma: 12

motor_driver:
  name: "TB6612FNG-like"
  motor_supply_min_v: 18
  motor_supply_max_v: 24
  continuous_per_channel_a: 0.6
  peak_per_channel_a: 6
  channels: 1
  logic_voltage_min_v: 3.0
  logic_voltage_max_v: 5.5

motors:
  - name: "DC motor"
    count: 1
    voltage_min_v: 6
    voltage_max_v: 12
    stall_current_a: 5
    nominal_current_a: 1
```

Run:

```bash
rv check spec.yaml --style classic --no-color
```

Example output:

```text
rv check
--------------
ERROR DRV_SUPPLY_RANGE: spec.yaml:5 battery 12.00V outside motor_driver motor supply range [18.00, 24.00]V
WARN DRV_CONT_LOW_MARGIN: spec.yaml:20 driver continuous rating 0.60A is below recommended 1.25A for motor DC motor (nominal 1.00A). Risk of overheating or current limiting under sustained load.
WARN DRV_PEAK_MARGIN_LOW: spec.yaml:21 Total motor stall 5.00A is close to driver peak 6.00A

exit code: 2
```

---

## Deterministic by design

Architon performs deterministic analysis.
Rail voltage inference is deterministic and transparent.
Each inference includes source, confidence score, evidence, and warnings.
No probabilistic models or network calls are used.

Validation operates only on the specification and part data you provide.
The same input always produces the same result.

---

## Schema versioning

`rv scan` reports include `report_version` and `design_ir.version`. Both are currently `"0"`.

`summary.delimiter` is set for BOM scans and uses one of `","`, `";"`, or `"\t"`.
`summary.nets` is set when netlist data is present.
`summary.next_steps` appears only when parse failures are present.
Netlist-backed scan reports may include `derived.net_voltages`, `derived.inferred_net_voltages`, `derived.unknown_voltage_nets`, `derived.rail_inferences`, `derived.rail_coverage`, and optional `rules[].inference` provenance.

Human-readable output is colorized in TTY environments. Disable with `--no-color` or `NO_COLOR=1`.

---

## Documentation

Detailed technical documentation is available in `/docs`:

- `docs/architecture.md` — engine architecture and system design
- `docs/spec.md` — hardware specification format
- `docs/rules.md` — deterministic rule system and validation logic

---

## Supported configurations

Architon CLI currently focuses on deterministic verification of mobile robot electrical architecture and BOM integrity.

Supported:

Electrical architecture validation (`rv check`):
- DC motors (single motor per driver channel)
- H-bridge motor drivers (TB6612FNG, L298 class, and compatible)
- Battery supply and driver supply compatibility checks
- Logic rail voltage compatibility between MCU and drivers
- Driver continuous and peak current margin checks
- Power budget validation where current limits are specified
- YAML-based architecture specification
- Deterministic exit codes and CI integration

BOM ingestion and normalization (`rv scan`):
- KiCad BOM CSV import
- KiCad `.net` S-expression netlist import
- Deterministic project-folder scan (`rv scan .`) with BOM + netlist auto-detection
- Deterministic BOM + netlist merge into one DesignIR
- Metadata-backed voltage propagation and overvoltage rule findings for netlist scans
- Deterministic rail voltage inference, confidence reporting, and rail coverage metrics
- Automatic delimiter detection (comma, semicolon, tab)
- Deterministic DesignIR JSON generation
- Parse error reporting with remediation guidance
- Stable versioned report format (`report_version`, `design_ir.version`)

Not supported yet:
- BLDC and ESC validation
- Stepper motor driver validation
- Multi-rail power tree modeling
- Thermal and derating models
- Detailed signal integrity validation
- ROS URDF or firmware-level integration

Architon CLI is a deterministic architecture verifier, not a circuit simulator.

---

## Contributing

Open an issue before starting work so scope can be aligned.
By contributing you agree to the CLA in `CLA.md`.

---

## Status

Early alpha. Interfaces and rule coverage evolving toward `v1.0`.

---

## License

Apache 2.0

---

## Disclaimer

This tool does not replace datasheets or engineering judgement.
Not suitable for safety critical systems.
Use at your own risk.
