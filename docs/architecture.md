# Architecture

Robotics Verifier (`rv-cli`) is a deterministic verification engine for hardware architecture contracts.

## System flow

```text
spec.yaml
  -> YAML parser/decoder
  -> parts resolver
  -> deterministic rule engine
  -> findings report
  -> CLI renderer (human or JSON) + exit code
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

Output modes:

- Human report style (TTY default)
- Human classic style
- JSON (`--output json`, optional `--pretty`, optional `--out-file`)

Exit behavior for `rv check`:

- `0`: no `ERROR` findings
- `2`: one or more `ERROR` findings
- `3`: parse/decode/resolve/internal failures

## Deterministic design philosophy

The engine is intentionally non-probabilistic.

- No model inference
- No remote lookups during validation
- No hidden heuristics
- Same input spec + same part library -> same output findings and exit code

This allows CI gating, reproducible audits, and traceable engineering review.

## Integration model

Deterministic validation is the trust foundation for hardware contract enforcement.

Higher-level tooling can recommend options, but contract checks should remain explicit, reproducible, and testable.

## Diagram

Architecture diagram reference:

![rv-cli architecture](../assets/rvcli_architecture.png)

This engine can be embedded into higher-level workflows.
