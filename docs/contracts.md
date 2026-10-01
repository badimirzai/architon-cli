# Contracts

Architon supports built-in component contracts and user-defined system contracts. Both are deterministic and are evaluated against DesignIR plus ContractIR, not against raw KiCad files.

## Built-in contracts

Built-in contracts represent known electrical characteristics of components.

Examples:

- Voltage limits
- GPIO tolerances
- Regulator current limits
- Driver supply ranges

These are automatically applied when components are recognized.

The built-in contract source is intentionally small and deterministic. It is not a generic parts database or datasheet lookup. v0.3.1 includes curated contracts for `ESP32-WROOM-32`, `STM32F103C8T6`, `RP2040`, `MPU-6050`, `BNO055`, `AMS1117-3.3`, `AP2114H-3.3`, `DRV8833`, `TB6612FNG`, `L298N`, `PCA9306`, and `TXS0108E`.

Inspect built-in contract parts with:

```bash
rv parts list
rv parts show ESP32-WROOM-32
```

## User contracts

User contracts represent system-level design intent.

Examples:

- I2C pull-up policy
- Allowed voltage domains
- Current budget constraints
- Bus topology requirements
- Organization-specific hardware standards

User contracts allow deterministic enforcement of architecture policies across projects.

## Contract rules for imported designs

KiCad netlists provide connectivity, but they do not reliably provide electrical intent such as source voltages, regulator outputs, or component maximum voltage ratings. Architon imports KiCad into the same DesignIR used by other adapters, then enriches contracts from `.architon/meta.yaml` or `--meta`.

For a direct netlist scan, pass the metadata file explicitly:

```bash
rv scan exports/project.net --meta .architon/meta.yaml
```

For a project directory scan, Architon auto-discovers `.architon/meta.yaml`:

```bash
rv scan .
```

Architon deterministically infers obvious rail voltages from net names, then enriches contracts with metadata sources, explicit schematic/BOM fields, curated built-in contracts, and regulator outputs from `meta.yaml`.

## Contract source precedence

Contract source precedence is:

1. explicit `.architon/meta.yaml`
2. schematic/BOM fields
3. built-in contract source
4. explicit custom contracts from `.architon/contracts.yaml` or `--contracts`
5. inferred net names

## Custom contracts

Custom contracts are explicit YAML policies. They can enforce project or organization rules such as I2C pull-up resistance, duplicate I2C addresses, voltage compatibility, current-budget utilization, explicit power budgets, which physical pin lands on each named interface net, CAN terminator count, and SPI chip-select exclusivity.

Custom contracts are deterministic. AI may generate contracts in future Studio workflows, but `rv` only validates and enforces explicit YAML.

## Interface contracts

An interface contract states that named components must share named nets. It checks structure only. It does not look up MCU peripherals such as SPI2, and it does not check voltage, clock polarity, or pull-ups.

`scope.bus_id` is the interface name. `scope.bus_type` is a label such as `spi`, `i2c`, `uart`, or `can`. `require.connected.nets` lists the nets that define the connection. Each participant has a `ref` and a role of `master` or `slave`. The role is recorded on the contract. The check uses the ref.

A participant passes when that component exists and has at least one pin on every named net. Net names match with or without a leading `/`.

A participant may set `pins`. Each key is a net name from `connected.nets`, and each value is the pin name or pin number that must land on that net. A key that is not in `connected.nets` is a schema error. Nets left out of `pins` are still checked for connectivity. If `pins` is omitted, the participant is checked for connectivity only.

When `pins` is set, Architon finds that component's pin on the net and compares the contract token to the netlist pin name and the pin number. It does not look up an MCU pin database and it does not infer alternate pin functions.

```yaml
contracts:
  - id: imu_spi
    description: U1 and U2 must share the IMU SPI nets.
    scope:
      bus_type: spi
      bus_id: imu_spi
    require:
      connected:
        nets: [SPI_SCK, SPI_MOSI, SPI_MISO, IMU_CS]
        participants:
          - ref: U1
            role: master
            pins:
              SPI_SCK: PB13
              SPI_MOSI: PB15
              SPI_MISO: PB14
          - ref: U2
            role: slave
    severity: error
```

See [examples/contracts/spi_interface.yaml](../examples/contracts/spi_interface.yaml).

Failures use these rule IDs:

- `interface_component_missing`: a participant ref, or a `power_budget` source or consumer ref, is not in the design
- `interface_net_missing`: a named net is not in the design
- `interface_not_connected`: a participant has no pin on a named net
- `interface_pin_mismatch`: the component is on the net, but neither the pin name nor the pin number equals the contract token. `expected.text` is the contract token. `observed.text` is the pin name, or the pin number when the pin name is empty
- `interface_pin_conflict`: the observed pin name or pin number is bound to a different signal in the same contract. `expected.text` is that other signal. `observed.text` is the pin name or pin number. The check uses only this contract

Each failure includes `expected` and `observed`. Human-readable output prints the finding message. `rv scan --format json` includes the same objects. A design with no `connected` or `terminated` requirement is unchanged.

## CAN termination

`require.terminated` counts terminators between two nets. `connected` still checks whether participants share nets. Termination does not repeat that check. It reads the part value or a `resistance` / `resistance_ohms` field. It does not look up a component database and it does not calculate impedance.

```yaml
contracts:
  - id: can_bus
    scope:
      bus_type: can
      bus_id: vehicle_can
    require:
      terminated:
        nets: [CANH, CANL]
        resistance_ohms: 120
        count: 2
    severity: error
```

See [examples/contracts/can_termination.yaml](../examples/contracts/can_termination.yaml).

A terminator is a two-pin part with one pin on each named net. The value or resistance field is parsed as ohms. `120`, `120R`, and `120 ohm` match `resistance_ohms: 120`. A part with any other pin, or a part on only one of the nets, is not counted.

`count` is how many of those parts the contract requires.

- `termination_count_low`: fewer parts match. `expected.text` is the required count. `observed.text` is the actual count
- `termination_count_high`: more parts match. The evidence fields are the same
- `interface_net_missing`: a named net is not in the design. This is the same rule used by `connected`. When both requirements name that net, the missing net is reported once

The count is reported only when both nets exist. A bus can fail the count while every required component is still on the nets.

## SPI chip-selects

`connected.chip_selects` names the chip-select net for each slave. The shared bus stays in `connected.nets`. Chip-select nets are separate, so this check does not repeat the shared-net test. `scope.bus_type` must be `spi`. Each `ref` must be a slave in `participants`.

```yaml
contracts:
  - id: imu_spi
    scope:
      bus_type: spi
      bus_id: imu_spi
    require:
      connected:
        nets: [SPI_SCK, SPI_MOSI, SPI_MISO]
        chip_selects:
          - ref: U2
            net: IMU_CS
          - ref: U3
            net: MAG_CS
        participants:
          - { ref: U1, role: master }
          - { ref: U2, role: slave }
          - { ref: U3, role: slave }
    severity: error
```

See [examples/contracts/spi_chip_select.yaml](../examples/contracts/spi_chip_select.yaml).

- `interface_not_connected`: the chip-select net exists and that slave has no pin on it. `expected.text` is `<ref> connected to <net>`. `observed.text` is `<ref> has no pin on <net>`
- `interface_net_missing`: the chip-select net is not in the design
- `spi_cs_shared`: two chip-select entries name the same net, or two slave refs each have a pin on the same chip-select net. `expected.text` is that net. `observed.text` is the slave refs that share it, in ref order. The master may sit on the net

Two entries may name the same net in YAML. That is `spi_cs_shared` when the design is checked, including when every slave is still on the shared SPI nets. Clock polarity, SPI mode, and pull-ups are left unchecked.

## Power budget

`require.power_budget` compares a declared source current with the sum of declared consumer currents. Every amp value comes from the contract. The check does not read datasheets, built-in parts, schematic fields, or part current fields. It does not replace `current_budget`, `supply_abs_max`, or `voltage_compatible`.

It does not check which net a part uses. A consumer on the wrong rail is a `require.connected` contract on that power net.

```yaml
contracts:
  - id: rail_3v3
    scope:
      bus_type: power
      bus_id: "3V3"
      rail: "+3V3"
    require:
      power_budget:
        source:
          ref: U2
          max_current_a: 1.0
        consumers:
          - { ref: U3, current_a: 0.3 }
          - { ref: U4, current_a: 0.4 }
        minimum_margin_pct: 20
    severity: error
```

See [examples/contracts/power_budget.yaml](../examples/contracts/power_budget.yaml).

`scope.bus_type: power` is a label, as are `scope.bus_id` and `scope.rail`. Load is the sum of `consumers[].current_a`. Remaining margin percent is `(max_current_a - load) / max_current_a * 100`.

- `power_budget_exceeded`: load is greater than `source.max_current_a`. `expected.max` is that max current. `observed.max` is the load. `unit` is `A`
- `power_margin_low`: load is within the source limit and the remaining margin is below `minimum_margin_pct`. `expected.min` is the minimum margin percent. `observed.min` is the actual margin percent. `unit` is `percent`. A margin exactly equal to `minimum_margin_pct` passes. A margin just below it fails
- `interface_component_missing`: the source ref or a consumer ref is not in the design. This is the same rule used by `connected`. `expected.text` is `component <ref>`. `observed.text` is `missing`. The current comparison is skipped when a named ref is missing

Human-readable output prints the finding message. `rv scan --format json` includes `expected` and `observed`. A design with no `power_budget` requirement is unchanged.

Other custom requirements, such as `pullup_ohms` and `no_i2c_address_conflict`, still require `scope.bus_type: i2c`.

Validate contract schema only:

```bash
rv contracts validate <path>
```

Schema validation does not verify a design. Use `rv scan --contracts <path>` to enforce contracts against a project.

`pullup_ohms` is resistance-only in v0.4.0. Capacitance/rise-time validation is future work.

## Minimal voltage-rule metadata

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

## Example overvoltage output

```text
ARCHITON SCAN
Target: .
Result: FAIL — scan violations detected

Parts: 3
Nets: 3
Rules: 1
Violations: 1

User contracts loaded: 0
Built-in contracts loaded: <n>
Active user requirements: 0
Part contract coverage: 66.67%
Parts matched: 0/3

Rule findings:
- ERROR RULE_SUPPLY_CONTRACT: Net /+5V provides 5.00V but U1 pin 1 allows max 3.30V
Generated Netlist: .architon/generated.net
Wrote architon-report.json
exit code: 2
```

`Errors` are parse/import errors. Rule failures are reported as `Violations`.
