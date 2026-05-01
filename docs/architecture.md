# Architecture

Architon CLI (rv) is a deterministic verification engine for hardware architecture contracts.

## System flow

```text
spec.yaml
  -> YAML parser/decoder
  -> parts resolver
  -> deterministic rule engine
  -> findings report
  -> CLI renderer (human or JSON) + exit code

bom.csv / project.net / project directory
  -> KiCad BOM and/or netlist importer
  -> DesignIR (stable internal model)
  -> deterministic rail inference + metadata-backed voltage rules
  -> deterministic report payload
  -> architon-report.json
```

The implementation follows this sequence in `rv check`:

1. Read spec file bytes.
2. Parse YAML into a `yaml.Node` and decode into `model.RobotSpec`.
3. Build parts search paths and resolve `part:` references into concrete values.
4. Execute all deterministic validation rules.
5. Render findings (report/classic/json) and return a deterministic exit code.

## Components

### Spec parser

- Input format: YAML.
- Decoding target: `internal/model/RobotSpec`.
- Parser behavior:
  - Supports scalar I2C addresses in decimal or `0x` hex via `I2CAddress.UnmarshalYAML`.
  - Builds a YAML location map (`file`, `line`, `column`) for path-aware findings.

### Parts resolver

Resolver implementation: `internal/resolve.ResolveAll`.

Responsibilities:

- Resolve `part:` for:
  - `mcu`
  - `motor_driver`
  - `motors[]`
  - `i2c_buses[].devices[]`
- Merge strategy:
  - Explicit spec values override part defaults.
  - Missing zero-valued fields are filled from part definitions.
- Post-merge requirements enforced before rule evaluation:
  - `mcu.logic_voltage_v`
  - `motor_driver.channels`
  - `motor_driver.motor_supply_min_v`
  - `motor_driver.motor_supply_max_v`
  - `motor_driver.logic_voltage_min_v`
  - `motor_driver.logic_voltage_max_v`
  - `motor_driver.peak_per_channel_a`
  - `motors[].count`
  - `motors[].stall_current_a`

Parts search order (earlier wins):

1. `./rv_parts`
2. `./parts`
3. `--parts-dir` (repeatable)
4. `RV_PARTS_DIRS`

### Rule engine

Rule engine entrypoint: `internal/validate.RunAll`.

Execution is ordered and deterministic; each rule consumes resolved spec data and emits zero or more findings.

Current execution order:

1. Driver channel checks
2. Driver motor supply range checks
3. Driver current headroom checks
4. Logic rail vs driver logic range checks
5. Logic rail budget note checks
6. MCU vs driver logic level mismatch checks
7. Battery discharge/C-rate checks
8. Aggregate driver stall overload checks
9. I2C address conflict checks

### Findings system

Finding model (`internal/validate.Finding`):

- `severity`: `INFO | WARN | ERROR`
- `code`: stable deterministic rule identifier
- `message`: human-readable deterministic explanation
- `path`: YAML path for the relevant field
- `location`: optional file/line/column

Report model (`internal/validate.Report`) stores findings and computes whether any `ERROR` exists.

### CLI interface

CLI is implemented with Cobra under `cmd/`.

Primary command:

- `rv check <spec.yaml>`
- `rv scan <path>`
- `rv scan <bom.csv> --map mapping.yaml`
- `rv scan <project.net>`
- `rv scan .`
- `rv scan . --bom bom/bom.csv --netlist exports/project.net`
- `rv scan <bom.csv> --out my-report.json`
- `rv scan <netlist.net> --meta .architon/meta.yaml`
- `rv scan <netlist.net> --rails`

Output modes:

- Human report style (TTY default)
- Human classic style
- JSON (`--output json`, optional `--pretty`, optional `--out-file`)
- Scan JSON file output (`architon-report.json`) with:
  - `summary`
  - `design_ir`
  - `rules`
  - `derived` voltage inference and rail coverage data when netlist data is present

Exit behavior is documented in the canonical exit code table in `README.md`.

`rv scan` writes `architon-report.json` with `report_version`, `design_ir.version`, and:

- `summary.delimiter` for BOM-backed scans
- `summary.nets` and `design_ir.nets` for netlist-backed scans
- `summary.next_steps` only on parse failure
- `derived.net_voltages`, `derived.inferred_net_voltages`, `derived.unknown_voltage_nets`, `derived.rail_inferences`, and `derived.rail_coverage` when voltage inference data is present
- optional `rules[].inference` provenance for voltage-based findings

Successful CLI output also prints a short deterministic terminal summary with:

- `ARCHITON SCAN`
- `Target`
- `Result`
- `Parts`
- `Nets`
- `Errors`
- `Warnings`
- `Rules`
- `Violations`
- compact rail coverage counts and level
- `Detected BOM` / `Detected Netlist` for directory auto-detection

`--rails` prints the sorted rail inference table with voltage, confidence level, confidence score, source, warnings, and rail coverage summary. `--explain-rails` remains supported as a legacy alias.

Example success snippet:

```json
{
  "report_version": "0",
  "summary": {
    "delimiter": ","
  },
  "design_ir": {
    "version": "0"
  }
}
```

Example failure snippet:

```json
{
  "report_version": "0",
  "summary": {
    "delimiter": ",",
    "next_steps": [
      "Re-export BOM (CSV) and check missing delimiters/quotes",
      "Run rv scan <bom.csv> --out report.json and inspect summary.parse_errors"
    ]
  },
  "design_ir": {
    "version": "0"
  }
}
```

### Scan import path

`rv scan` implementation is intentionally separated:

- `internal/ir`: stable, input-agnostic `DesignIR` model
- `internal/importers`: importer adapter interface; KiCad, Altium, and future sources compile to DesignIR
- `internal/importers/kicad`: deterministic KiCad BOM CSV ingestion, header mapping, and KiCad `.net` S-expression parsing
- `internal/contracts`: importer-agnostic component, pin, and net contract schema
- `internal/enrichment`: pluggable contract sources such as `meta.yaml`, inferred rail voltages, future databases, or UI input
- `internal/rules`: contract-level rules that consume only DesignIR + ContractIR
- `cmd/scan.go`: deterministic single-file or project-directory scan input resolution
- `internal/ir`: deterministic BOM + netlist merge into one project-level DesignIR
- `internal/infer`: deterministic net-name and metadata-enriched rail voltage inference
- `internal/rails`: pure rail coverage summary and terminal explanation formatting
- `internal/report`: deterministic scan report JSON builder/writer

## Deterministic design philosophy

Architon performs deterministic analysis.
Rail voltage inference is deterministic and transparent.
Each inference includes source, confidence score, evidence, and warnings.
No probabilistic models or network calls are used.

Same input spec or scan input + same metadata/part library -> same output findings and exit code.

This allows CI gating, reproducible audits, and traceable engineering review.

## Integration model

Deterministic validation is the trust foundation for hardware contract enforcement.

Higher-level tooling can recommend options, but contract checks should remain explicit, reproducible, and testable.

## Diagram

Architecture diagram reference:

![Architon CLI (rv) architecture](../assets/rvcli_architecture.png)

This engine can be embedded into higher-level workflows.
