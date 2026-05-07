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

Contract findings may include `rule_id`, `severity`, `message`, `component_ref`, `net`, `pin`, `bus_id`, `bus_type`, `bus_nets`, `source`, `provenance`, and `fix`.

`rules` is a deprecated alias of `findings`.

Human-readable output is colorized in TTY environments. Disable with `--no-color` or `NO_COLOR=1`.
