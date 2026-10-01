# Report Format

Architon produces deterministic structured reports for automation.

## Structured report output

Default scan output path:

```bash
architon-report.json
```

Custom output path:

```bash
rv check robot.yaml --output json --out-file report.json
rv scan bom.csv --out report.json
```

The report includes:

- Summary counts
- Findings for `rv check`, and canonical scan `findings` for `rv scan`
- Normalized DesignIR for scans, including nets when imported
- Rail inference, rail coverage, and voltage-finding provenance for netlist-backed scans
- Contract loading and coverage summary fields: `user_contracts_loaded`, `built_in_contracts_loaded`, `requirements_enabled`, `parts_matched`, `part_contract_coverage_percentage`, `unknown_power_critical_refs`, and `enabled_contract_rules`

Exit codes indicate pass/fail. The JSON report provides detailed structured results for CI integration and tooling.

For netlist-backed scans, `summary.nets` and `design_ir.nets` are populated. For BOM-only scans, these fields remain omitted to keep the JSON stable.

## Example: `report.json` from `rv scan bom.csv`

```json
{
  "report_version": "1",
  "summary": {
    "source": "kicad_bom_csv",
    "input_file": "bom.csv",
    "parts": 2,
    "rules": 0,
    "has_failures": false,
    "delimiter": ",",
    "parse_errors_count": 0,
    "parse_warnings_count": 0,
    "parse_errors": [],
    "parse_warnings": [],
    "user_contracts_loaded": 0,
    "built_in_contracts_loaded": 12,
    "requirements_enabled": 0,
    "parts_matched": 0,
    "part_contract_coverage_percentage": 0,
    "contracts_applied": 0,
    "contract_coverage_percentage": 0,
    "enabled_contract_rules": [
      "supply_abs_max",
      "supply_recommended_range",
      "gpio_abs_max",
      "motor_driver_vm_range",
      "regulator_output_current"
    ]
  },
  "design_ir": {
    "version": "0",
    "source": "kicad_bom_csv",
    "parts": [],
    "metadata": {
      "input_file": "bom.csv",
      "parsed_at": "2026-02-26T00:00:00Z"
    }
  },
  "rules": []
}
```

On parse failures, the report still includes `report_version`, `design_ir.version`, `delimiter`, and deterministic guidance in `summary.next_steps`.

## Example: `report.json` from `rv check robot.yaml`

```json
{
  "spec_file": "robot.yaml",
  "summary": {
    "errors": 1,
    "warnings": 2,
    "infos": 1,
    "exit_code": 2
  },
  "findings": [
    {
      "id": "DRV_SUPPLY_RANGE",
      "severity": "ERROR",
      "message": "battery voltage exceeds driver supply range",
      "path": "power.battery.voltage_v",
      "location": {
        "line": 5,
        "column": 5
      },
      "meta": {}
    }
  ]
}
```

## Schema versioning

`rv scan` reports include `report_version` and `design_ir.version`. `report_version` is currently `"1"` and `design_ir.version` is currently `"0"`.

`summary.delimiter` is set for BOM scans and uses one of `","`, `";"`, or `"\t"`.

`summary.nets` is set when netlist data is present.

`summary.next_steps` appears only when parse failures are present.

`summary.user_contracts_loaded`, `summary.built_in_contracts_loaded`, `summary.requirements_enabled`, `summary.parts_matched`, `summary.part_contract_coverage_percentage`, `summary.unknown_power_critical_refs`, and `summary.enabled_contract_rules` describe deterministic contract loading and coverage.

`summary.contracts_applied` and `summary.contract_coverage_percentage` remain as deprecated compatibility aliases for one release.

Netlist-backed scan reports may include `derived.net_voltages`, `derived.inferred_net_voltages`, `derived.unknown_voltage_nets`, `derived.rail_inferences`, `derived.rail_coverage`, and optional `findings[].inference` provenance.

Contract findings may include `rule_id`, `severity`, `message`, `component_ref`, `net`, `pin`, `bus_id`, `bus_type`, `bus_nets`, `source`, `provenance`, `why_this_matters`, `fix`, `expected`, and `observed`.

`expected` and `observed` are optional. They are omitted when a rule does not set them. Interface findings set `text`. A numeric comparison may instead set `min`, `max`, and `unit`. Rules that already have a numeric limit and a measured value set those fields from the values in code. A rule with no separate value leaves both unset.

`rv scan --format json` sets `design_fixable` on every finding. It is true when editing the schematic or the contract can clear the finding. It is false for parse and tool failures. A parse failure is reported as `rule_id` `parse_error`.

`design_ir.metadata.parsed_at` is the time of that run and changes every run. Compare `findings`, not the whole report file.

`rv scan --format json` includes the same `expected` and `observed` objects. Interface rule IDs are `interface_component_missing`, `interface_net_missing`, `interface_not_connected`, `interface_pin_mismatch`, and `interface_pin_conflict`. Topology rule IDs are `termination_count_low`, `termination_count_high`, and `spi_cs_shared`. Power-budget rule IDs are `power_budget_exceeded` and `power_margin_low`.

`interface_pin_mismatch` sets `expected.text` to the pin token from `require.connected.participants[].pins` and `observed.text` to the netlist pin name, or the pin number when that name is empty. `interface_pin_conflict` sets `expected.text` to the other signal in the same contract and `observed.text` to the pin name or pin number bound to both signals. `termination_count_low` and `termination_count_high` set `expected.text` to the required terminator count and `observed.text` to the actual count. `spi_cs_shared` sets `expected.text` to the chip-select net and `observed.text` to the slave refs that share it. `power_budget_exceeded` sets `expected.max` to the source max current and `observed.max` to the load, with `unit` `A`. `power_margin_low` sets `expected.min` to the minimum margin percent and `observed.min` to the actual margin percent, with `unit` `percent`. Human-readable output prints `message`. It does not rebuild the sentence from `expected` and `observed`.

`rules` is a deprecated alias of `findings`.

Human-readable output is colorized in TTY environments. Disable with `--no-color` or `NO_COLOR=1`.
