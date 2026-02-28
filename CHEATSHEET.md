# Robotics Verifier CLI Cheatsheet

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
rv scan <bom.csv>                      Import KiCad BOM CSV and write architon-report.json
rv scan <bom.csv> --map mapping.yaml   Use explicit header mapping YAML
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
```

## Exit codes

```text
0  clean run, no ERROR findings
2  rule violations (ERROR findings present)
3+ internal or unexpected errors
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
rv scan bom.csv --map examples/mapping.yaml
rv init --template 4wd-problem
rv check robot.yaml
rv init --template 4wd-clean --out robot.yaml --force
rv check robot.yaml
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
