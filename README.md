# Architon CLI (rv)
[![CI](https://github.com/badimirzai/architon-cli/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/badimirzai/architon-cli/actions/workflows/ci.yaml) [![Release](https://img.shields.io/github/v/release/badimirzai/architon-cli?label=release)](https://github.com/badimirzai/architon-cli/releases)

Deterministic hardware architecture verification for robotics and embedded systems.

Architon detects electrical compatibility, power, logic-level, and integration failures **before hardware is built**.
Run it locally or in CI to catch integration errors early and reduce costly board spins and bring-up churn.

---

## Why Architon exists

Software has compilers and static analysis.
Hardware lacks a deterministic **system-level** verification step before fabrication.

Architon fills this gap by enforcing architecture contracts across **power, interfaces, and components**.
It catches failures that typically appear during bring-up, after hardware has already been built.

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

Architon CLI validates hardware architecture from a specification (`.yaml`) and can ingest KiCad BOM CSV files to produce a normalized, deterministic **DesignIR** JSON report.

**Inputs**
- YAML hardware architecture specification (`rv check`)
- KiCad BOM CSV (`rv scan`)

**Outputs**
- Deterministic exit codes for CI gating
- Machine-readable `report.json` for automation
- Stable normalized DesignIR representation

---

## Quick Start

### Install

Requires Go **1.25.5** or newer (https://go.dev/dl/).

```bash
go install github.com/badimirzai/architon-cli/cmd/rv@latest
rv --help
```

### Try it in 30 seconds

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

rv scan bom.csv
# Wrote architon-report.json

rv scan bom.csv --map mapping.yaml
# Wrote architon-report.json

rv scan bom.csv --out my-report.json
# Wrote my-report.json
```

---

## Example

```bash
rv check robot.yaml
```

Example output:

```text
ERROR DRV_SUPPLY_RANGE: battery 16.8V exceeds motor_driver supply range [6.0V, 15.0V]
WARN RAIL_I_UNKNOWN: logic rail current capacity not specified
INFO DRV_CHANNELS_OK: driver channels correctly mapped

exit code: 2
```

Example run video:

https://github.com/user-attachments/assets/3c73410f-bda8-49a3-9171-b888dff7446e

---

## CLI usage

`rv check` validates system architecture from YAML specification.  
`rv scan` imports BOM data and generates a normalized DesignIR report.

Core commands:

```text
rv check <file.yaml>       Run deterministic analysis
rv scan <bom.csv>          Import BOM CSV and emit DesignIR report JSON
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

KiCad BOM scan examples:

```bash
rv scan bom.csv
rv scan bom.csv --map examples/mapping.yaml
rv scan bom.csv --out my-report.json
```

---

## Exit codes

`rv check` returns deterministic exit codes designed for CI and automation. Exit codes distinguish between architecture problems and tool execution failures.

| Code | Meaning |
|-----:|---------|
| 0 | Clean or informational only. No warnings or violations. |
| 1 | Warnings detected, but no violations. |
| 2 | Architecture violations detected. |
| 3 | Tool execution failure (analysis could not complete). |

### Exit code 1 — Warnings

Exit code 1 means Architon successfully analyzed the architecture and found one or more warnings, but no violations.

Warnings indicate elevated risk or incomplete constraints, such as:
- Missing current limits
- Low electrical margin conditions
- Incomplete architecture specification

The architecture may still function, but warnings should be reviewed.
CI may allow exit code 1 or treat it as failure using `--warn-as-error`.

### Exit code 2 — Violations

Exit code 2 means Architon successfully analyzed the architecture and found one or more violations (HARD STOPS / errors).
This indicates the architecture is invalid and must be fixed.

### Exit code 3 — Tool failure

Exit code 3 means Architon could not complete analysis. This is not an architecture violation.
It indicates an input or runtime problem, such as:
- Invalid YAML syntax
- Missing input file
- Schema validation failure
- Import or resolution failure
- Internal tool error

Exit codes 0–2 indicate successful analysis. Exit code 3 indicates analysis could not run.

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

Default output path:

```bash
architon-report.json
```

Custom output path:

```bash
rv check robot.yaml --out report.json
rv scan bom.csv --out report.json
```

The report includes:
- summary counts
- violations / warnings / notes
- normalized architecture model (DesignIR for scans)

Exit codes indicate pass/fail. The JSON report provides detailed structured results for CI integration and tooling.

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
  "report_version": "0",
  "summary": {
    "input_file": "robot.yaml",
    "violations": 1,
    "warnings": 2,
    "notes": 1,
    "has_failures": true
  },
  "violations": [
    {
      "rule": "DRV_SUPPLY_RANGE",
      "severity": "error",
      "message": "battery voltage exceeds driver supply range",
      "fix": "Use compatible driver or adjust battery voltage"
    }
  ],
  "warnings": [],
  "notes": []
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
INFO DRV_CHANNELS_OK: spec.yaml:22 driver channels OK: 1 motor(s) mapped to 1 available channel(s)
ERROR DRV_SUPPLY_RANGE: spec.yaml:5 battery 12.00V outside motor_driver motor supply range [18.00, 24.00]V
WARN DRV_CONT_LOW_MARGIN: spec.yaml:20 driver continuous rating 0.60A is below recommended 1.25A for motor DC motor (nominal 1.00A). Risk of overheating or current limiting under sustained load.
INFO RAIL_BUDGET_NOTE: spec.yaml:9 logic rail budget set to 1.00A. v1 does not estimate MCU and driver logic current yet.
WARN DRV_PEAK_MARGIN_LOW: spec.yaml:21 Total motor stall 5.00A is close to driver peak 6.00A

exit code: 2
```

---

## Deterministic by design

Architon is deterministic by design:
- No AI
- No guessing
- No probabilistic inference
- No hallucination

Validation operates only on the specification and part data you provide.
The same input always produces the same result.

---

## Schema versioning

`rv scan` reports include `report_version` and `design_ir.version`. Both are currently `"0"`.

`summary.delimiter` is set for BOM scans and uses one of `","`, `";"`, or `"\t"`.
`summary.next_steps` appears only when parse failures are present.

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
