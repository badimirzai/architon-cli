# Deterministic Rule System

Architon CLI (rv) uses a deterministic rule engine implemented in `internal/validate/rules.go`.

Rules evaluate only the resolved input specification and emit findings with explicit IDs.

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

## Exit codes (`rv check`)

- `0`: analysis completed with no `ERROR` or `WARN` findings (`INFO` notes are allowed)
- `1`: one or more `WARN` findings and no `ERROR` findings
- `2`: one or more `ERROR` findings, regardless of warnings
- `3`: parse/decode/resolve/import/schema/IO failure path

With `--warn-as-error`, warning-only results also return `2`.

## Rule catalog

### Driver channel allocation

- `DRV_CHANNELS_INVALID` (`ERROR`): `motor_driver.channels <= 0`
- `DRV_CHANNELS_INSUFFICIENT` (`ERROR`): total motor count exceeds channel count
- `DRV_CHANNELS_OK` (`INFO`): channels sufficient

### Motor supply compatibility

- `BAT_V_INVALID` (`ERROR`): negative battery voltage
- `DRV_SUPPLY_RANGE` (`ERROR`): battery voltage outside driver motor supply range

### Driver current headroom

- `DRV_PEAK_LT_STALL` (`ERROR`): driver peak per channel below motor stall current
- `DRV_CONT_LOW_MARGIN` (`WARN`): driver continuous per channel below recommended `1.25 * nominal_current`

### Logic voltage compatibility

- `RAIL_V_INVALID` (`ERROR`): negative logic rail voltage
- `LOGIC_V_DRIVER_MISMATCH` (`ERROR`): logic rail outside driver logic range
- `LOGIC_V_MCU_MISMATCH` (`WARN`): MCU logic voltage differs from logic rail by more than `0.25V`

### Logic rail budget signal

- `RAIL_I_UNKNOWN` (`WARN`): `power.logic_rail.max_current_a` missing or `<= 0`
- `RAIL_BUDGET_NOTE` (`INFO`): rail current budget present (v1 note, not full logic-current model)

### MCU-driver logic level checks

- `MCU_LOGIC_V_INVALID` (`ERROR`): negative MCU logic voltage
- `DRV_LOGIC_MIN_V_INVALID` (`ERROR`): negative driver logic min voltage
- `DRV_LOGIC_MAX_V_INVALID` (`ERROR`): negative driver logic max voltage
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

## Determinism guarantees

Given the same:

- Spec file content
- Part files resolved by search order
- CLI options

the engine returns the same findings and exit code.

No network access or probabilistic scoring is part of rule evaluation.

## Extensibility

To add a new rule safely:

1. Add a pure rule function in `internal/validate/rules.go` that reads `model.RobotSpec` and returns `[]Finding`.
2. Register it in `RunAll` in a deterministic position.
3. Assign a stable, descriptive rule code.
4. Add tests in `internal/validate/rules_test.go` for pass/fail and edge cases.
5. Keep rule logic side-effect free and independent of external state.
