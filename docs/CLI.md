
## CLI usage

`rv check` validates system architecture from a YAML specification.
`rv scan` imports KiCad BOM/netlist data and generates a normalized DesignIR report.

Core commands:

```text
rv check <file.yaml>          Run deterministic analysis
rv scan <path>                Import BOM CSV, KiCad .net, or project directory and emit DesignIR report JSON
rv contracts validate <path>  Validate a custom contracts.yaml schema only
rv parts list                 List built-in deterministic contract parts
rv parts show <mpn>           Show one built-in contract part
rv init                       Create .architon metadata or write a starter robot spec
rv version                    Show installed version
rv check --output json        Emit JSON findings to stdout
rv --help                     Show all commands and flags
rv check --help               Show check command options
```

Findings severity:

- `INFO` context or non-blocking notes
- `WARN` risk indications
- `ERROR` rule violations

Common examples:

```bash
rv init --list
rv init --template 4wd-problem
rv check robot.yaml
rv check specs/robot.yaml --output json --pretty
rv scan examples/bom/bom.csv
rv scan examples/bom/bom.csv --map examples/mapping.yaml
rv scan exports/project.net --meta .architon/meta.yaml --rails
rv scan . --contracts i2c_pullup_policy.yaml --verbose
rv parts list
rv parts show ESP32-WROOM-32
```

Architon normalizes imported hardware designs into DesignIR, applies ContractIR requirements, and emits deterministic findings. See [docs/architecture.md](docs/architecture.md) for details.

Contracts come from built-in component data, project metadata, schematic/BOM fields, and explicit user YAML policies. Custom contracts can enforce rules such as I2C pull-up resistance, duplicate I2C addresses, voltage compatibility, and current budgets. See [docs/contracts.md](docs/contracts.md).

Use `--verbose` or `--rails` to inspect rail inference, confidence, and voltage coverage. See [docs/rail-inference.md](docs/rail-inference.md).

Architon writes deterministic JSON reports for CI and tooling. Default scan output is `architon-report.json`. See [docs/report-format.md](docs/report-format.md).

YAML architecture specs and part lookup behavior are documented in [docs/spec.md](docs/spec.md).
