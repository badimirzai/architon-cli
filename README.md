# Robotics Verifier (rv-cli) [![CI](https://github.com/badimirzai/robotics-verifier-cli/actions/workflows/ci.yaml/badge.svg?branch=main)](https://github.com/badimirzai/robotics-verifier-cli/actions/workflows/ci.yaml) [![Release](https://img.shields.io/github/v/release/badimirzai/robotics-verifier-cli?label=release)](https://github.com/badimirzai/robotics-verifier-cli/releases)

Deterministic hardware architecture verification engine.

Detects electrical compatibility, power, logic-level, and bus integration failures before hardware is built.

Run deterministic validation locally or in CI to catch integration errors early and prevent hardware damage.

Used as the verification core for Architon (under development).

## What this engine does

`rv-cli` runs deterministic validation over a robot hardware spec (`.yaml`) and can scan KiCad BOM CSV input into a stable DesignIR JSON report.

It is designed to prevent common pre-build integration failures, including:

- Voltage mismatches
- Current overload and low headroom conditions
- Driver incompatibility with selected motors
- Logic level conflicts between MCU, rail, and motor driver
- I2C address conflicts on shared buses
- Battery discharge and C-rate violations under peak motor stall load

Deterministic validation matters because architecture verification must be reproducible in local development, CI, and release gates. A contract either passes or fails given the same input, with no probabilistic behavior.

## Quick Start

### Install

Requires Go **1.25.5** or newer (https://go.dev/dl/).

```bash
go install github.com/badimirzai/robotics-verifier-cli/cmd/rv@latest
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
# clean (or WARN-only if intentionally retained), exit code 0

rv scan bom.csv
# Wrote architon-report.json

rv scan bom.csv --out my-report.json
# Wrote my-report.json
```

### Parts lookup (quick reference)

You can reference built-in parts (`parts/`) and project-local parts (`./rv_parts`) with `part:`.

Resolver lookup order (earlier wins):

1. `./rv_parts`
2. `./parts`
3. `--parts-dir` (repeatable)
4. `RV_PARTS_DIRS` (OS path separator: `:` on Unix, `;` on Windows)

Detailed field-level behavior is documented in `docs/spec.md`.

## Example

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

Example run video:

https://github.com/user-attachments/assets/3c73410f-bda8-49a3-9171-b888dff7446e

## CLI usage

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

## Output Control

Use `--out` to override the default `architon-report.json` output path:

```bash
rv scan bom.csv --out my-report.json
```

## Exit Codes

For `rv scan`:

- `0` = success
- `1` = rule violations
- `2` = parse errors

Mapping file shape (`examples/mapping.yaml`):

```yaml
ref: Designator
value: Component
footprint: Package
mpn: Part Number
manufacturer: Mfr
```

`rv scan` writes `architon-report.json` with this schema:

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

On parse failures, the report still includes `report_version`, `design_ir.version`, `delimiter`, and deterministic guidance in `summary.next_steps`:

```json
{
  "report_version": "0",
  "summary": {
    "source": "kicad_bom_csv",
    "input_file": "bom.csv",
    "parts": 1,
    "rules": 0,
    "has_failures": true,
    "delimiter": ",",
    "parse_errors_count": 1,
    "parse_warnings_count": 0,
    "parse_errors": [
      "row 3: malformed CSV row: expected 3 columns from header, got 1"
    ],
    "parse_warnings": [],
    "next_steps": [
      "Re-export BOM (CSV) and check missing delimiters/quotes",
      "Run rv scan <bom.csv> --out report.json and inspect summary.parse_errors"
    ]
  },
  "design_ir": {
    "version": "0",
    "source": "kicad_bom_csv",
    "parts": [
      {
        "ref": "R1",
        "value": "10k",
        "footprint": "Resistor_SMD:R_0603_1608Metric",
        "fields": {
          "Footprint": "Resistor_SMD:R_0603_1608Metric",
          "Reference": "R1",
          "Value": "10k"
        }
      }
    ],
    "metadata": {
      "input_file": "bom.csv",
      "parsed_at": "2026-02-28T00:00:00Z"
    }
  },
  "rules": []
}
```

## Schema Versioning

`rv scan` reports include `report_version` and `design_ir.version`. Both are currently `"0"`.

`summary.delimiter` is set for BOM scans and uses one of `","`, `";"`, or `"\\t"`. `summary.next_steps` appears only when parse failures are present.

CI integration example:

```yaml
steps:
  - name: Verify hardware spec
    run: rv check specs/robot.yaml
```

Human-readable output is colorized in TTY environments. Disable with `--no-color` or `NO_COLOR=1`.

## Determinism and Trust

This engine is deterministic by design:

- No AI
- No guessing
- No inference
- No hallucination

Validation operates only on the specification and part data you provide.

## Documentation

Detailed technical documentation is available in the /docs directory:

- docs/architecture.md — engine architecture and system design
- docs/spec.md — hardware specification format
- docs/rules.md — deterministic rule system and validation logic

## Supported configurations (v0.1)

Focused on early-stage mobile robot electrical architecture checks.

Supported:

- DC motors (one motor per driver channel)
- TB6612FNG and L298 class H-bridge driver classes
- Single logic rail
- YAML part-based configuration with deterministic merge behavior

Not supported yet:

- Stepper, BLDC, ESC powertrain validation
- Multi-rail power tree validation
- Thermal derating models
- Serial/IO arbitration models

This is a linter and contract verifier, not a simulator or optimizer.

## Versioning and stability

The interface is still evolving before `v1.0`.

Exit behavior used by `rv check`:

- `0` clean or WARN-only
- `2` deterministic rule violations (`ERROR` findings)
- `3` parse/decode/resolve/internal failures

## Contributing

Open an issue before starting work so scope can be aligned.

By contributing you agree to the CLA in `CLA.md`.

## Status

Early alpha. Interfaces and rule coverage are still evolving before `v1.0`.

## License

Apache 2.0. See `LICENSE`.

## Disclaimer

This tool does not replace datasheets or engineering judgement.
Not suitable for safety critical systems.
Use at your own risk.
