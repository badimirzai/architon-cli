# Architon CLI (rv) Cheatsheet

## Core commands

```text
rv check <file.yaml>                   Run analysis (human-readable output)
rv check <file.yaml> --output json     JSON output to stdout (compact)
rv check <file.yaml> --output json --pretty
                                      JSON output to stdout (pretty)
rv check <file.yaml> --output json --out-file report.json
                                      Write compact JSON to file, stdout says "Written to ..."
rv check <file.yaml> --output json --pretty --out-file report.json
                                      Pretty JSON to stdout + compact JSON to file
rv scan <path>                         Import KiCad BOM CSV, KiCad .net, or project directory
rv scan <bom.csv> --map mapping.yaml   Use explicit header mapping YAML
rv scan <bom.csv> --out report.json    Write scan report to a specific path
rv scan .                              Auto-detect BOM and/or netlist in current directory
rv scan . --bom bom/bom.csv --netlist exports/project.net
                                      Override detected project files
rv init --list                        List available templates
rv init --template <name>             Write a template to robot.yaml
rv init --template <name> --out path  Write a template to a specific path
rv init --template <name> --force     Overwrite existing output file
rv version                             Show installed version
rv --help                              Show all commands and flags
rv check --help                        Show check command options
rv scan --help                         Show scan command options
```

## Output flags (check command)

```text
--output json             print machine readable JSON to stdout
--pretty                  pretty print JSON to stdout (requires --output json)
--out-file <path>         write compact JSON to file (requires --output json)
--no-color                disable colored output
--debug                   enable debug mode (or use RV_DEBUG=1)
```

## Scan flags (scan command)

```text
--map <file.yaml>         explicit BOM header mapping file
--bom <file>              override BOM file path for project directory scans
--netlist <file>          override netlist file path for project directory scans
--out <report.json>       write scan report to a specific path
```

## Exit codes (`rv scan`)

```text
0  success
1  rule violations
2  parse errors
3  tool failure / internal error
```

## Examples

```bash
rv check examples/minimal_voltage_mismatch.yaml
rv check examples/minimal_voltage_mismatch.yaml --output json
rv check examples/minimal_voltage_mismatch.yaml --output json --pretty
rv check examples/minimal_voltage_mismatch.yaml --output json --out-file result.json
rv check examples/minimal_voltage_mismatch.yaml --output json --pretty --out-file result.json
NO_COLOR=1 rv check examples/minimal_voltage_mismatch.yaml
rv scan bom.csv
rv scan exports/example.net
rv scan .
rv scan . --bom bom/bom.csv --netlist exports/project.net
rv scan bom.csv --map examples/mapping.yaml
rv scan bom.csv --out my-report.json
rv init --template 4wd-problem
rv check robot.yaml
rv init --template 4wd-clean --out robot.yaml --force
rv check robot.yaml
```

## Scan report summary

`rv scan` report `summary` includes:

- `delimiter` for KiCad BOM imports: `,`, `;`, or `\t`
- `nets` when KiCad netlist data is present
- `next_steps` only when `parse_errors_count > 0`

Directory scan detection order:

- BOM: `bom/bom.csv`, `bom.csv`, `exports/bom.csv`, then lexical `*bom*.csv`
- Netlist: lexical `exports/*.net`, then lexical `*.net` in project root

Successful `rv scan` terminal output includes:

- `ARCHITON SCAN`
- `Target`, `Parts`, `Nets`, `Errors`, `Warnings`
- `Detected BOM` and `Detected Netlist` when directory scan auto-detects files

Example success snippet:

```json
{
  "report_version": "0",
  "summary": {
    "delimiter": ","
  },
  "design_ir": {
    "version": "0"
  }
}
```

Example failure snippet:

```json
{
  "report_version": "0",
  "summary": {
    "delimiter": "\\t",
    "parse_errors_count": 1,
    "next_steps": [
      "Re-export BOM (CSV) and check missing delimiters/quotes",
      "Run rv scan <bom.csv> --out report.json and inspect summary.parse_errors"
    ]
  },
  "design_ir": {
    "version": "0"
  }
}
```

## Parts libraries

```bash
# Use project-local parts in ./rv_parts (automatic)
rv check robot.yaml

# Add extra search directories (repeatable)
rv check robot.yaml --parts-dir ./vendor/parts --parts-dir /opt/robot-parts

# Use environment variable paths (split by OS list separator)
RV_PARTS_DIRS="./vendor/parts:/opt/robot-parts" rv check robot.yaml
```
