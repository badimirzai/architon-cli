# Deterministic Rule System

Architon CLI (rv) uses deterministic rule engines for YAML specs and KiCad scan data. `rv check` rules live in `internal/validate/rules.go`; scan voltage rules live under `internal/rules`.

Rules evaluate only resolved local input data and emit findings with explicit IDs.

## Rule philosophy

- Rules are deterministic.
- Rules never guess.
- Rules never infer missing hidden data.
- Rules operate only on provided and resolved values.
- Rules may skip when required inputs are unknown (`0` or unset), unless the rule explicitly validates that field.

## Execution model

Rule engine entrypoint: `validate.RunAll(spec, locs)`.

Rules execute in a fixed order:

1. `ruleDriverChannels`
2. `ruleMotorSupplyVoltage`
3. `ruleDriverCurrentHeadroom`
4. `ruleLogicVoltageCompat`
5. `ruleRailCurrentBudget`
6. `ruleLogicLevelMisMatch`
7. `ruleBatteryCRate`
8. `ruleDriverStallOverload`
9. `ruleI2CAddressConflict`

This order is stable and produces reproducible findings for identical input.

## Severity levels

- `INFO`: context and non-blocking notes
- `WARN`: elevated risk or low margin
- `ERROR`: deterministic contract violation

Exit behavior is documented in `README.md`.

## Rule catalog

The catalog below covers `rv check`.

### Driver channel allocation

- `DRV_CHANNELS_INVALID` (`ERROR`): `motor_driver.channels <= 0`
- `DRV_CHANNELS_INSUFFICIENT` (`ERROR`): total motor count exceeds channel count

When channels are sufficient, this rule emits no finding.

### Motor supply compatibility

- `BAT_V_INVALID` (`ERROR`): negative battery voltage; `0` means unset and the rule skips
- `DRV_SUPPLY_RANGE` (`ERROR`): battery voltage outside driver motor supply range

### Driver current headroom

- `DRV_PEAK_LT_STALL` (`ERROR`): driver peak per channel below motor stall current
- `DRV_CONT_LOW_MARGIN` (`WARN`): driver continuous per channel below recommended `1.25 * nominal_current`

### Logic voltage compatibility

- `RAIL_V_INVALID` (`ERROR`): negative logic rail voltage; `0` means unset and the rule skips
- `LOGIC_V_DRIVER_MISMATCH` (`ERROR`): logic rail outside driver logic range
- `LOGIC_V_MCU_MISMATCH` (`WARN`): MCU logic voltage differs from logic rail by more than `0.25V`

### Logic rail budget signal

- `RAIL_I_UNKNOWN` (`WARN`): `power.logic_rail.max_current_a` missing or `<= 0`

When `power.logic_rail.max_current_a > 0`, this rule emits no finding.

### MCU-driver logic level checks

- `MCU_LOGIC_V_INVALID` (`ERROR`): negative MCU logic voltage; `0` means unset and the rule skips
- `DRV_LOGIC_MIN_V_INVALID` (`ERROR`): negative driver logic min voltage; `0` means unset and the rule skips
- `DRV_LOGIC_MAX_V_INVALID` (`ERROR`): negative driver logic max voltage; `0` means unset and the rule skips
- `DRV_LOGIC_RANGE_INVALID` (`ERROR`): driver logic min > max
- `LOGIC_LEVEL_MISMATCH` (`ERROR`): MCU logic outside driver logic window

### Battery discharge / C-rate validation

Battery max current source precedence:

1. `power.battery.max_discharge_a`
2. `power.battery.capacity_ah * power.battery.c_rating`
3. `power.battery.max_current_a`

Rules:

- `BATT_PEAK_OVER_C` (`ERROR`): total motor stall current exceeds battery max derived from precedence
- `BATT_PEAK_MARGIN_LOW` (`WARN`): total motor stall current is `>= 80%` of battery max

### Aggregate driver peak overload

- `DRV_PEAK_OVERLOAD` (`ERROR`): total motor stall current exceeds `driver_peak_per_channel * channels`
- `DRV_PEAK_MARGIN_LOW` (`WARN`): total motor stall current is `>= 80%` of driver total peak

### I2C conflict detection

- `I2C_ADDR_CONFLICT` (`ERROR`): duplicate non-zero I2C address on the same bus

`address_hex` accepts decimal or `0x`-prefixed hex values.

## Scan voltage rules (`rv scan`)

Netlist-backed scans can run deterministic contract rules when rail voltages are inferred from net names and/or supplied by `.architon/meta.yaml` / `--meta`. Project scans can generate a temporary KiCad netlist from one root `*.kicad_sch` when no `.net` file exists.

- `RULE_SUPPLY_CONTRACT` (`ERROR`): provider voltage on a net is outside a consumer pin's contracted voltage range
- `RULE_LOGIC_LEVEL_CONTRACT` (`ERROR`): output logic voltage exceeds an input pin's contracted tolerance
- `RULE_BUS_ROLE_CONTRACT` (`WARNING`/`ERROR`): obvious I2C role/direction conflicts without guessing when data is missing
- `RULE_VOLTAGE_CONFLICT` (`ERROR`): metadata, inferred, or propagated voltage evidence conflicts on a net
- `supply_abs_max` (`ERROR`): powered supply pin exceeds a matched part's absolute maximum supply voltage
- `supply_recommended_range` (`WARNING`): powered supply pin is outside the recommended operating range but within absolute maximum
- `gpio_abs_max` (`ERROR`): GPIO-like pin is on a net above its absolute maximum voltage
- `motor_driver_vm_range` (`ERROR`): motor-driver VM pin is outside the supported motor-supply voltage range
- `regulator_output_current` (`ERROR`): known downstream load current exceeds a regulator output-current contract

Voltage-based findings include inference provenance when available: net name, source, confidence score, confidence level, and reason.

Contract source precedence is deterministic: explicit `.architon/meta.yaml`, then schematic/BOM contract fields, then the curated built-in contract source, then explicit custom contracts from `.architon/contracts.yaml` or `--contracts`, then inferred net names. Built-ins are intentionally small and local; there is no network lookup, datasheet scraping, or generic parts database.

Custom contracts are deterministic explicit YAML. `rv contracts validate` validates schema only; use `rv scan --contracts <path>` to enforce a contract file against a design.

`pullup_ohms` is resistance-only in v0.4.0. I2C capacitance and rise-time validation require physical bus data and are future work.

## Determinism contract

Given the same:

- Spec file content, or scan input netlist/BOM and metadata
- Part files resolved by search order
- CLI options

the engine returns the same findings and exit code.

Architon performs deterministic analysis.
Rail voltage inference is deterministic and transparent.
Each inference includes source, confidence score, reason, evidence, and warnings.
No probabilistic models or network calls are used.

## Extensibility

To add a new rule safely:

1. Add a pure rule function in `internal/validate/rules.go` that reads `model.RobotSpec` and returns `[]Finding`.
2. Register it in `RunAll` in a deterministic position.
3. Assign a stable, descriptive rule code.
4. Add tests in `internal/validate/rules_test.go` for pass/fail and edge cases.
5. Keep rule logic side-effect free and independent of external state.
