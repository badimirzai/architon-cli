# Supported Configurations

Architon CLI currently focuses on deterministic verification of mobile robot electrical architecture and BOM integrity.

## Supported

Electrical architecture validation with `rv check`:

- DC motors, single motor per driver channel
- H-bridge motor drivers, including TB6612FNG, L298 class, and compatible drivers
- Battery supply and driver supply compatibility checks
- Logic rail voltage compatibility between MCU and drivers
- Driver continuous and peak current margin checks
- Power budget validation where current limits are specified
- YAML-based architecture specification
- Deterministic exit codes and CI integration

BOM ingestion and normalization with `rv scan`:

- KiCad BOM CSV import
- KiCad `.net` S-expression netlist import
- Deterministic project-folder scan with `rv scan .` and BOM + netlist auto-detection
- Deterministic BOM + netlist merge into one DesignIR
- Metadata-backed voltage propagation and overvoltage rule findings for netlist scans
- Deterministic rail voltage inference, confidence reporting, and rail coverage metrics
- Automatic delimiter detection for comma, semicolon, and tab
- Deterministic DesignIR JSON generation
- Parse error reporting with remediation guidance
- Stable versioned report format with `report_version` and `design_ir.version`

## Not supported yet

- BLDC and ESC validation
- Stepper motor driver validation
- Multi-rail power tree modeling
- Thermal and derating models
- Detailed signal integrity validation
- ROS URDF or firmware-level integration

Architon CLI is a deterministic architecture verifier, not a circuit simulator.
