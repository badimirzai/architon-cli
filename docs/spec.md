# Hardware Spec (YAML)

This document defines the YAML format consumed by `rv check`.

Validation is deterministic and runs on resolved values after part expansion.

## Top-level structure

```yaml
name: "robot-name"

power:
  battery:
    chemistry: "Li-ion"
    voltage_v: 12.0
    max_current_a: 10.0
    capacity_ah: 2.0
    c_rating: 5.0
    max_discharge_a: 12.0
  logic_rail:
    voltage_v: 5.0
    max_current_a: 2.0

mcu:
  part: mcus/esp32-s3-devkitc-1
  name: "ESP32-S3"
  logic_voltage_v: 3.3
  max_gpio_current_ma: 20

motor_driver:
  part: drivers/tb6612fng
  name: "TB6612FNG"
  motor_supply_min_v: 2.5
  motor_supply_max_v: 13.5
  continuous_per_channel_a: 1.2
  peak_per_channel_a: 3.2
  channels: 2
  logic_voltage_min_v: 2.7
  logic_voltage_max_v: 5.5

motors:
  - part: motors/tt_6v_dc_gearmotor
    name: "TT motor"
    count: 2
    voltage_min_v: 3.0
    voltage_max_v: 6.0
    stall_current_a: 1.6
    nominal_current_a: 0.25

i2c_buses:
  - name: "i2c0"
    devices:
      - part: sensors/mpu6050
        name: "imu"
        address_hex: 0x68
```

## Field reference

### `name`

- Type: `string`
- Required: no
- Notes: project label only; not used by rule execution.

### `power.battery`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `chemistry` | string | No | Informational. |
| `voltage_v` | float | Optional for parser; needed for `DRV_SUPPLY_RANGE` checks | Nominal battery voltage. |
| `max_current_a` | float | No | Fallback source for battery max current if other sources missing. |
| `capacity_ah` | float | No | Used with `c_rating` to compute battery max (`Ah * C`). |
| `c_rating` | float | No | Used with `capacity_ah`. |
| `max_discharge_a` | float | No | Highest-priority explicit battery max current for discharge checks. |

Battery max-current precedence used by rules:

1. `power.battery.max_discharge_a`
2. `power.battery.capacity_ah * power.battery.c_rating`
3. `power.battery.max_current_a`

### `power.logic_rail`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `voltage_v` | float | Optional for parser; needed for logic-voltage checks | Compared against driver logic range. |
| `max_current_a` | float | No | If absent/zero, rule emits `RAIL_I_UNKNOWN`. |

### `mcu`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `part` | string | No | Part ID (for example `mcus/esp32-s3-devkitc-1`). |
| `name` | string | No | Optional human label. |
| `logic_voltage_v` | float | Yes after resolve | Must come from explicit field or part defaults. |
| `max_gpio_current_ma` | float | No | Informational in current rule set. |

### `motor_driver`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `part` | string | No | Part ID (for example `drivers/tb6612fng`). |
| `name` | string | No | Optional human label. |
| `motor_supply_min_v` | float | Yes after resolve | Required for supply-range checks. |
| `motor_supply_max_v` | float | Yes after resolve | Required for supply-range checks. |
| `continuous_per_channel_a` | float | No | Used for `DRV_CONT_LOW_MARGIN` warnings. |
| `peak_per_channel_a` | float | Yes after resolve | Used for stall checks and overload checks. |
| `channels` | int | Yes after resolve | Must be `> 0`. |
| `logic_voltage_min_v` | float | Yes after resolve | Driver logic window minimum. |
| `logic_voltage_max_v` | float | Yes after resolve | Driver logic window maximum. |

### `motors[]`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `part` | string | No | Part ID (for example `motors/tt_6v_dc_gearmotor`). |
| `name` | string | No | Optional human label. |
| `count` | int | Yes after resolve | Must be `> 0`. |
| `voltage_min_v` | float | No | Informational in current rule set. |
| `voltage_max_v` | float | No | Informational in current rule set. |
| `stall_current_a` | float | Yes after resolve | Required for current and battery overload checks. |
| `nominal_current_a` | float | No | Used for continuous headroom recommendation. |

### `i2c_buses[]`

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `name` | string | No | Used in conflict messages. |
| `devices` | list | No | Devices on this bus. |

`i2c_buses[].devices[]` fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `part` | string | No | Part ID (for example `sensors/mpu6050`). |
| `name` | string | No | Optional human label; used in conflict output. |
| `address_hex` | scalar | No | Accepts decimal (`104`) or hex (`0x68`). |

## Required fields summary (resolver stage)

`rv check` requires these values after part resolution:

- `mcu.logic_voltage_v`
- `motor_driver.channels`
- `motor_driver.motor_supply_min_v`
- `motor_driver.motor_supply_max_v`
- `motor_driver.logic_voltage_min_v`
- `motor_driver.logic_voltage_max_v`
- `motor_driver.peak_per_channel_a`
- `motors[].count`
- `motors[].stall_current_a`

Missing required values cause resolve failures (exit code `3`) before rule findings are emitted.

## Parts system

### Built-in parts

Built-in library path: `./parts`

Current built-in categories include:

- `parts/drivers/*`
- `parts/motors/*`
- `parts/mcus/*`
- `parts/sensors/*` (I2C sensor defaults)

### Project-local parts

Project-local overrides path: `./rv_parts`

Example layout:

```text
rv_parts/
  motors/
    custom_gear_motor.yaml
  drivers/
    custom_driver.yaml
```

Use in spec:

```yaml
motor_driver:
  part: drivers/custom_driver

motors:
  - part: motors/custom_gear_motor
    count: 2
```

### Part inheritance / override model

Behavior is merge-based, not class inheritance:

- `part:` loads a YAML part definition.
- Missing zero-valued fields in the spec are filled from part defaults.
- Explicit fields in the spec always override the part file.

This allows a stable baseline part with per-project overrides without mutating the shared part library.

## Example valid spec

```yaml
name: "amr-basic"

power:
  battery:
    chemistry: "Li-ion"
    voltage_v: 12.8
    max_current_a: 20
  logic_rail:
    voltage_v: 3.3
    max_current_a: 2

mcu:
  name: "ESP32-S3"
  logic_voltage_v: 3.3

motor_driver:
  name: "TB6612FNG-like"
  motor_supply_min_v: 2.5
  motor_supply_max_v: 13.5
  continuous_per_channel_a: 1.2
  peak_per_channel_a: 3.2
  channels: 2
  logic_voltage_min_v: 2.7
  logic_voltage_max_v: 5.5

motors:
  - name: "DC gearmotor"
    count: 2
    stall_current_a: 2.5
    nominal_current_a: 0.9
```

## Example invalid spec

The following spec is syntactically valid YAML but violates deterministic rules:

```yaml
name: "invalid-contract"

power:
  battery:
    voltage_v: 12
    capacity_ah: 2
    c_rating: 2
  logic_rail:
    voltage_v: 5.0

mcu:
  logic_voltage_v: 3.3

motor_driver:
  motor_supply_min_v: 18
  motor_supply_max_v: 24
  continuous_per_channel_a: 0.4
  peak_per_channel_a: 1.0
  channels: 1
  logic_voltage_min_v: 4.5
  logic_voltage_max_v: 5.5

motors:
  - count: 1
    stall_current_a: 2.0
    nominal_current_a: 1.0

i2c_buses:
  - name: "i2c0"
    devices:
      - name: "imu_left"
        address_hex: 0x68
      - name: "imu_right"
        address_hex: 104
```

Expected deterministic failures include:

- Driver supply range violation
- MCU/driver logic level mismatch
- Driver peak below motor stall current
- Battery peak current over allowed discharge
- I2C address conflict on the same bus
