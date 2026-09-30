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

Custom contracts are explicit YAML policies. They can enforce project or organization rules such as I2C pull-up resistance, duplicate I2C addresses, voltage compatibility, current budgets, and which physical pin lands on each named interface net.

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

- `interface_component_missing`: a participant ref is not in the design
- `interface_net_missing`: a named net is not in the design
- `interface_not_connected`: a participant has no pin on a named net
- `interface_pin_mismatch`: the component is on the net, but neither the pin name nor the pin number equals the contract token. `expected.text` is the contract token. `observed.text` is the pin name, or the pin number when the pin name is empty
- `interface_pin_conflict`: the observed pin name or pin number is bound to a different signal in the same contract. `expected.text` is that other signal. `observed.text` is the pin name or pin number. The check uses only this contract

Each failure includes `expected` and `observed`. Human-readable output prints the finding message. A design with no `connected` contract is unchanged.

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
