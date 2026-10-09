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
- `pin_function_mismatch` (`ERROR`): a cited dedicated SDA pin is on a net named `SCL` or `I2C_SCL`, or a cited dedicated SCL pin is on a net named `SDA` or `I2C_SDA`. A leading `/` is ignored. `expected.text` is the function signal. `observed.text` is the net. `citations` is the datasheet citation. A `gpio_candidate` does not produce this finding.
- `pin_bus_short` (`ERROR`): cited dedicated SDA and SCL of the same part land on one net. `expected.text` is `SDA and SCL on different nets`. `observed.text` is that net. A `gpio_candidate` does not produce this finding.
- `motor_driver_vm_range` (`ERROR`): motor-driver VM pin is outside the supported motor-supply voltage range
- `regulator_output_current` (`ERROR`): known downstream load current exceeds a regulator output-current contract
- `interface_component_missing` (`ERROR`): a `connected` or `power_budget` contract names a component ref that is not in the design
- `interface_net_missing` (`ERROR`): a `connected` or `terminated` contract names a net that is not in the design, including a chip-select net
- `interface_not_connected` (`ERROR`): a participant has no pin on a net named by a `connected` contract, including its chip-select net
- `interface_pin_mismatch` (`ERROR`): a participant is on the named net, but neither the netlist pin name nor the pin number equals the `pins` token. `expected.text` is the contract token. `observed.text` is the pin name, or the pin number when the pin name is empty
- `interface_pin_conflict` (`ERROR`): the observed pin name or pin number is bound to a different signal in the same `connected` contract. `expected.text` is the other signal. `observed.text` is that pin name or pin number
- `termination_count_low` (`ERROR`): a `terminated` contract found fewer two-pin parts of the required resistance between the named nets than `count`. `expected.text` is the required count. `observed.text` is the actual count
- `termination_count_high` (`ERROR`): a `terminated` contract found more of those parts than `count`. `expected.text` is the required count. `observed.text` is the actual count
- `spi_cs_shared` (`ERROR`): two `connected.chip_selects` entries name the same net, or two slave refs each have a pin on the same chip-select net. `expected.text` is that net. `observed.text` is the slave refs that share it
- `power_budget_exceeded` (`ERROR`): declared consumer current is greater than `power_budget.source.max_current_a`. `expected.max` is that max current in amps. `observed.max` is the load in amps. `unit` is `A`
- `power_margin_low` (`ERROR`): declared load is within the source limit and the remaining margin is below `minimum_margin_pct`. `expected.min` is the minimum margin percent. `observed.min` is the actual margin percent. `unit` is `percent`. A margin exactly equal to the minimum passes. A margin just below it fails

Voltage-based findings include inference provenance when available: net name, source, confidence score, confidence level, and reason.

Contract source precedence is deterministic: explicit `.architon/meta.yaml`, then schematic/BOM contract fields, then the curated built-in contract source, then explicit custom contracts from `.architon/contracts.yaml` or `--contracts`, then inferred net names. Built-ins are intentionally small and local; there is no network lookup, datasheet scraping, or generic parts database.

Built-in pin functions are citations copied from a datasheet pin table. They are not inferred currents and not a generated schematic. A function without a title, revision, and table or section is ignored. `pin_function_mismatch` and `pin_bus_short` run only when the netlist pin name or pin number matches a cited dedicated SDA or SCL pin. They do not replace `supply_abs_max` or `gpio_abs_max`. `rv scan --format json` adds `coverage`. `proved` counts those bus checks that passed. `refused` lists existing `ERROR` and `WARN` findings. `not_checked` lists unmatched parts, matched pins with no cited function, and a `gpio_candidate` on an I2C net name. Coverage does not change the exit code. An unchecked pin is not a violation. `rv connections propose <path>` writes `.architon/connections.proposal.yaml` from those pin functions and the netlist. A decided entry is not a scan result. A `needs_choice` entry is not accepted. The command does not evaluate the proposal. `rv connections apply <path>` adds net labels only for entries whose status is `accepted`. A `needs_choice` entry is skipped. An accepted entry that lists `candidates` may name only a pin copied from that list. Any other pin writes nothing. It does not scan. See [docs/contracts.md](contracts.md).

Custom contracts are deterministic explicit YAML. `rv contracts draft <path>` writes a reviewable draft from net names. That draft is not a verification result. `rv contracts validate` validates schema only; use `rv scan --contracts <path>` to enforce a contract file against a design.

`connected` checks that every listed component has a pin on every listed net. Roles are `master` or `slave` labels. Optional `participants[].pins` requires the pin name or pin number that lands on each named net. Optional `chip_selects` requires each slave on its own SPI chip-select net and reports `spi_cs_shared` when those nets are not exclusive. The rule does not infer a peripheral from net names, does not consult an MCU pin database, and does not check clock polarity, SPI mode, or pull-ups. Findings include `expected` and `observed`.

`terminated` counts two-pin parts whose value or `resistance` / `resistance_ohms` field equals `resistance_ohms`, with one pin on each of the two named nets. It does not repeat the `connected` shared-net check, does not look up terminator parts in a component database, and does not calculate impedance. A missing net is `interface_net_missing`. `rv scan --format json` keeps `expected` and `observed` for these findings.

`power_budget` adds the declared `consumers[].current_a` values and compares that load with `source.max_current_a`. Remaining margin percent is `(max_current_a - load) / max_current_a * 100`. Currents come only from the contract. The rule does not read datasheets, built-in parts, or part current fields, and it does not check which net the parts use. A consumer on the wrong rail remains a `connected` contract on that power net. A missing source or consumer ref is `interface_component_missing`. `power_budget` does not replace `current_budget`, `supply_abs_max`, or `voltage_compatible`. `rv scan --format json` keeps numeric `expected` and `observed` for `power_budget_exceeded` and `power_margin_low`.

`pullup_ohms` is resistance-only in v0.4.0. I2C capacitance and rise-time validation require physical bus data and are future work. When the effective resistance is outside the contract range, `expected` carries `min` and `max` in ohms and `observed` carries the effective resistance. A missing pull-up has no separate measured value, so those fields stay unset.

Supply, GPIO, recommended-range, motor-supply, regulator-current, logic-level, and current-budget findings set `expected` and `observed` from the limit and the measured value the rule already computed. `current_budget` uses `unit` `percent`. Voltage findings use `unit` `V`. Regulator current uses `unit` `A`. A finding whose rule has no separate value, such as a consumer with no voltage limits or an I2C net that mixes SDA and SCL, leaves both fields unset. The values are not parsed out of `message`.

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
