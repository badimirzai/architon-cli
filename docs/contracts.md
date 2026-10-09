# Contracts

Architon supports built-in component contracts and user-defined system contracts. Both are deterministic and are evaluated against DesignIR plus ContractIR, not against raw KiCad files.

A reproducible `rv scan --format json` loop is in [Scan loop](#scan-loop).

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

## Pin functions

A built-in pin function is a citation copied from a datasheet pin table. It is not an inferred current, and it is not a generated schematic. `rv scan` does not fetch a datasheet, does not call a model, and does not invent a pin number. A function without a datasheet title, revision, and table or section is ignored.

Each function has a name, an optional number, a kind, and an optional signal. The kind is `power`, `ground`, `bus`, or `gpio_candidate`. Dedicated SDA and SCL pins are `bus` with signal `SDA` or `SCL`. VIN and VOUT are `power`. `gpio_candidate` means the datasheet allows that GPIO to carry I2C through pinmux. It is not a dedicated SDA or SCL pin.

`MPU-6050` and `BNO055` record dedicated power, ground, SDA, and SCL pins. `AP2114H-3.3` records dedicated VIN, VOUT, and GND, and it has no I2C function. `AMS1117-3.3` stays on its voltage contract: the datasheet that was read names those pins and does not print a revision, so no pin function was added. `ESP32-WROOM-32`, `STM32F103C8T6`, and `RP2040` record dedicated power and ground, plus GPIO pins the datasheet allows for I2C as `gpio_candidate`. `DRV8833`, `TB6612FNG`, `L298N`, `PCA9306`, and `TXS0108E` keep their voltage contracts only. This release does not add bus pins to them.

`rv parts show <mpn>` prints each function and its citation.

`rv connections propose` writes a reviewable join proposal from these pin functions and the netlist. That file is not a scan result. `rv connections apply` adds net labels for entries marked `accepted`. See [Connection proposals](#connection-proposals).

These checks run only when a cited function exists and the netlist pin name or pin number matches that function. They do not replace `supply_abs_max` or `gpio_abs_max`.

- `pin_function_mismatch` (`ERROR`): a dedicated SDA pin is on a net named `SCL` or `I2C_SCL`, or a dedicated SCL pin is on a net named `SDA` or `I2C_SDA`. A leading `/` is ignored. `expected.text` is the function signal. `observed.text` is the net as stored. `citations` is the datasheet citation.
- `pin_bus_short` (`ERROR`): dedicated SDA and SCL of the same part land on one net. `expected.text` is `SDA and SCL on different nets`. `observed.text` is that net. `citations` lists the datasheet citations for those pins.

A `gpio_candidate` does not produce these findings. A missing citation does not produce a finding.

`rv scan --format json` and `architon-report.json` include `coverage`. It does not change the exit code. An unchecked pin is not a violation.

- `proved.count` is the number of dedicated bus-pin checks that passed. `proved.rule_ids` lists `pin_function_mismatch` and `pin_bus_short` only when that rule passed at least once. A rule that only failed is not listed there.
- `refused` is the existing `ERROR` and `WARN` findings. Each item has `rule_id`, `severity`, and the finding's `component_ref`, `net`, `pin`, `message`, `expected`, `observed`, and `citations` when those are set.
- `not_checked` lists rows with `ref` and `reason`. `unmatched_part` is a part that did not match a built-in contract. `no_pin_function` is a pin on a matched part with no cited function; `pin` and `net` are set. `gpio_candidate` is a candidate pin on a net named `SDA`, `SCL`, `I2C_SDA`, or `I2C_SCL`.

A pass in `proved` is a check that ran. A `gpio_candidate` on `I2C_SDA` stays in `not_checked`.

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

Custom contracts are deterministic. `rv contracts draft` writes a reviewable YAML draft from net names. That draft is not a verification result. AI may generate contracts in future Studio workflows, but `rv` validates and enforces explicit YAML.

## Draft from a netlist

`rv contracts draft <path>` reads the same project directory or netlist path as `rv scan` and writes `.architon/contracts.draft.yaml`.

The draft is not a verification result. The command does not scan the design, does not evaluate the draft, and does not load a second rule set. It does not fill pin tokens from pin names in the netlist, and it does not fill currents from datasheets or built-in parts.

The command never writes `.architon/contracts.yaml`. If `.architon/contracts.draft.yaml` already exists, the command exits 3 and leaves that file unchanged. `--force` overwrites the draft file only.

A comment at the top of the draft says the user fills pin tokens and power_budget currents. The draft does not emit `power_budget`.

Recognized net names match with or without a leading `/`:

- `SPI_SCK`, `SPI_MOSI`, and `SPI_MISO` become one `require.connected` contract when all three nets exist. A participant is a ref with a pin on all three. The ref with the most pins on other nets is `master` and is listed first. The other refs are `slave`. The `pins` map is omitted. Among nets other than those three, a slave with a pin on exactly one net whose name ends in `_CS` or `CS` gets a `chip_selects` entry. A slave with two such nets is left out of `chip_selects`. Two slaves on the same chip-select net are both listed.
- `CANH` and `CANL` become one contract when both nets exist. `require.connected` lists every ref with a pin on either net. `require.terminated` names those nets with `resistance_ohms: 120` and `count: 2`. The count does not come from resistor values in the netlist.
- `SDA` and `SCL` become one I2C contract with id `i2c`. `I2C_SDA` and `I2C_SCL` become one I2C contract with id `i2c_bus`. Each sets `require.common_ground: true` and `require.pullup_ohms` min 2200 max 10000, and records the net names under `scope.nets`.

If none of these nets exist, the command exits 0 and writes an empty contracts list with the same comment. It does not invent a contract for another net.

Each contract has `scope.bus_type`, `scope.bus_id`, and `severity: error`. Contract ids are stable across runs on the same netlist. The command prints the draft path and those ids. It does not print a design pass or fail.

```bash
rv contracts draft .
rv contracts validate .architon/contracts.draft.yaml
```

`rv contracts validate` checks the draft schema only. That check is not a verification result. Use `rv scan --contracts .architon/contracts.draft.yaml` after reviewing the draft, including any pin tokens or power_budget currents you add.

## Connection proposals

`rv connections propose <path>` reads the same project directory or netlist path as `rv scan` and writes `.architon/connections.proposal.yaml`.

The proposal is not a scan result. The command does not verify the design, does not call a model, does not read a PDF, and does not evaluate the proposal with the rule engine. It does not edit a KiCad schematic, netlist, or other KiCad file. It never writes `.architon/contracts.yaml`.

If `.architon/connections.proposal.yaml` already exists, the command exits 3 and leaves that file unchanged. `--force` overwrites the proposal file only.

A comment at the top says a decided entry is not a scan result, and a needs_choice entry is not accepted.

The command matches parts to built-in pin functions, then reads nets from the imported design. A `gpio_candidate` pin cannot decide an entry. An unmatched part is left out. A function without a datasheet title, revision, and table or section is ignored.

An entry is `decided` only when all of these are true:

- Two different matched parts each have a dedicated pin function for the same signal. The signal is `SDA` or `SCL`.
- One of those pins is already on a net.
- The other pin is unconnected, or already on that same net.

A decided entry names both refs, both pin names, the signal, the net, and both citations. Its status is `decided`. The pin name is the datasheet function name, so a BNO055 pad recorded as both `SDA` and `COM0` is named `SDA`.

Net identity ignores leading `/` characters. `/I2C_SDA` and `I2C_SDA` are the same net. The displayed name prefers the form without a slash when both forms exist. A KiCad net whose name, ignoring leading slashes, starts with `unconnected` is unconnected. A pin that appears on no net is unconnected.

If the other side is a matched part with only `gpio_candidate` pins for that signal, the command emits `needs_choice`. `ESP32-WROOM-32`, `STM32F103C8T6`, and `RP2040` are in this group: their power and ground pins stay dedicated, and their I2C-capable GPIOs stay candidates. The entry lists every candidate pin. Pins that share a number are one candidate with every datasheet name, such as `IO21` and `GPIO21`. Catalog order is kept. That order is not a ranking. The entry does not pick a candidate, and it does not mark any GPIO accepted.

The choice names the net to join when the dedicated pins for that signal already agree on one net. A `gpio_candidate` that already sits on a net does not choose that net. When no dedicated pin is on a net, or dedicated pins disagree, the choice omits `net`.

If a dedicated pin is already on a different net than the proposed signal net, the command emits `conflict` and does not propose a join. The conflict entry has no top-level `net`. Each part names the net its dedicated pin is already on.

If nothing matches, the command exits 0 and writes an empty connections list with the same comment.

Ids are stable across runs on the same connectivity. A pair id is the lowercase signal plus the two refs in lexicographic order, such as `sda-U2-U3`. A choice id is the signal plus the MCU ref, such as `sda-U1`.

The command prints the proposal path, then `decided:`, `needs_choice:`, and `conflict:` counts. It does not print a design pass or fail.

```bash
rv connections propose .
```

```yaml
# A decided entry is not a scan result, and a needs_choice entry is not accepted.
connections:
  - id: "sda-U2-U3"
    status: decided
    signal: SDA
    net: I2C_SDA
    parts:
      - ref: U2
        mpn: "MPU-6050"
        pin: SDA
        citation:
          datasheet: "MPU-6000 and MPU-6050 Product Specification"
          revision: "3.4"
          section: "7.1 Pin Out and Signal Description"
      - ref: U3
        mpn: BNO055
        pin: SDA
        citation:
          datasheet: "BNO055 Intelligent 9-axis absolute orientation sensor"
          revision: "1.8"
          table: "5-1 Pin description"
          section: "5.1 Pin-out"
  - id: "sda-U1"
    status: needs_choice
    signal: SDA
    net: I2C_SDA
    ref: U1
    mpn: "ESP32-WROOM-32"
    candidates:
      - pins: [IO32, GPIO32]
        number: "8"
        citation:
          datasheet: "ESP32-WROOM-32 Datasheet"
          revision: "3.8"
          table: "2 Pin Definitions"
          section: "4.2.4 I2C Interface"
```

The real ESP32 choice continues with every cited `gpio_candidate`. When the dedicated SCL pins are unconnected, the SCL choice is a second `needs_choice` entry and it has no `net`.

### Applying an accepted entry

`rv connections apply <path>` reads `.architon/connections.proposal.yaml` and the project schematics. It adds a KiCad net label for each unconnected pin named by an entry whose status is `accepted`.

The command does not scan, does not call a model, and does not draw wires. It does not edit a `.kicad_pcb`. It does not move a symbol, and it does not add a no-connect or a new symbol.

`decided` is not `accepted`. A person or a later tool changes that status. `needs_choice` and `conflict` are never applied. A `needs_choice` entry in the file is skipped.

To accept a `needs_choice` entry, set its status to `accepted` and set `pin` to one name or number copied from that entry's `candidates`. Leave the `candidates` list on the entry. Apply adds one net label for that pin. A pin that is not in the list exits 3 and writes nothing.

An accepted pair names two pins. A candidate choice names one. Each of those pins must already be on a schematic. The pin is matched by reference and by pin name or pin number. A missing pin, a pin already on a different net, or a pin with a no-connect exits 3 and writes nothing. A pin already on the proposal net is left as it is. An unconnected pin gets one local label at that pin's connection point. The label uses the form saved by KiCad 9 (`version 20250114`): `(label "NAME" (at x y 0) ...)`.

Every accepted entry is applied, or none are. On failure the schematic bytes are restored.

The command prints the schematic paths that changed and the entry ids applied. It does not print a pass or fail. Run `rv scan` or the verify tool to see the verdict.

```bash
rv connections apply .
```

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

Schema validation does not verify a design. A file written by `rv contracts draft` is checked the same way, and that check is not a verification result. Use `rv scan --contracts <path>` to enforce contracts against a project.

`pullup_ohms` is resistance-only in v0.4.0. Capacitance/rise-time validation is future work.

## Scan loop

`examples/agent-loop/` is one small project with two copies. Both use the same contracts: SPI connectivity and pin tokens, SPI chip-selects, CAN connectivity, CAN termination, and a power budget. `fixed/` passes. `broken/` is a valid netlist that fails for two reasons already implemented by those contracts:

- `interface_pin_mismatch` on `U1` pin `4` of `SPI_MOSI`. `expected.text` is `PB15`. `observed.text` is `PA7`.
- `termination_count_low` on the CAN bus. `expected.text` is `2`. `observed.text` is `1`.

The power budget in both copies stays inside the declared current and margin.

The loop is:

1. Edit the design or the contract.
2. Run `rv scan . --format json`.
3. Read `rule_id`, `component_ref`, `net`, `pin`, `expected`, and `observed`.
4. Edit again.
5. Run `rv scan . --format json`.
6. The process exits 0.

An agent can run the same scan through the `verify` tool. See [mcp.md](mcp.md).

`examples/agent-loop/broken` is the project at step 2. That scan exits 2 and returns the two findings above. `design_fixable` is true when editing the schematic or the contract can clear the finding. It is false for parse and tool failures. A parse failure is a finding with `rule_id` `parse_error` and exits 3. A tool failure, such as invalid contract YAML or no importable input, exits 3 before scan JSON findings are written.

Step 4 on this fixture sets `U1`'s `SPI_MOSI` pin function to `PB15` and adds a second 120 ohm resistor between `CANH` and `CANL`. `examples/agent-loop/fixed/` is that design. Scanning it exits 0.

`rv scan` also writes `architon-report.json`. `design_ir.metadata.parsed_at` changes every run, so an agent must compare findings, not the whole file. Compare the `findings` from `rv scan . --format json`.

Rules that already compute a numeric limit and a measured value set `expected` and `observed` from those values. Examples are supply and GPIO voltage, recommended range, motor supply, regulator current, logic level, pull-up resistance, and current-budget utilization. A finding whose rule has no separate value, such as a missing common ground or two devices that share one I2C address, leaves both fields unset.

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
