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
rv scan <netlist.net> --meta .architon/meta.yaml
                                      Enable metadata-backed voltage rules
rv scan <netlist.net> --rails          Show rail inference details
rv scan .                              Auto-detect BOM/netlist, or export netlist from root schematic
rv scan . --bom bom/bom.csv --netlist exports/project.net
                                      Override detected project files
rv scan . --kicad-cli /full/path/to/kicad-cli
                                      Use an explicit KiCad CLI binary
rv doctor                             Check rv and KiCad CLI setup
rv init                                Create .architon/meta.yaml and README.md
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
--style report|classic    force human-readable style
--warn-as-error           treat warning-only results as violations
--pretty                  pretty print JSON to stdout (requires --output json)
--out-file <path>         write compact JSON to file (requires --output json)
--parts-dir <dir>         add parts directory (repeatable)
--no-color                disable colored output
--debug                   enable debug mode (or use RV_DEBUG=1)
```

## Scan flags (scan command)

```text
--map <file.yaml>         explicit BOM header mapping file
--bom <file>              override BOM file path for project directory scans
--netlist <file>          override netlist file path for project directory scans
--meta <file.yaml>        metadata for sources, regulators, and component limits
--rails                   print rail voltage inference and confidence details
--explain-rails           legacy alias for --rails
--no-kicad-cli            disable automatic schematic netlist generation
--kicad-cli <path>        override KiCad CLI binary name/path
--out <report.json>       write scan report to a specific path
```

Exit behavior is documented in `README.md`.

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
rv scan . --kicad-cli /full/path/to/kicad-cli
rv scan . --bom bom/bom.csv --netlist exports/project.net
rv scan bom.csv --map examples/mapping.yaml
rv scan bom.csv --out my-report.json
rv scan exports/project.net --meta .architon/meta.yaml --rails
rv init
rv init --template 4wd-problem
rv check robot.yaml
rv init --template 4wd-clean --out robot.yaml --force
rv check robot.yaml
```

## Scan report summary

`rv scan` report includes:

- `summary.delimiter` for KiCad BOM imports: `,`, `;`, or `\t`
- `summary.nets` when KiCad netlist data is present
- `summary.next_steps` only when `parse_errors_count > 0`
- `derived.rail_inferences` and `derived.rail_coverage` for netlist-backed voltage inference
- `rules[].inference` provenance for voltage-based findings when available

Directory scan detection order:

- BOM: `bom/bom.csv`, `bom.csv`, `exports/bom.csv`, then lexical `*bom*.csv`
- Netlist: lexical `exports/*.net`, then lexical `*.net` in project root
- Schematic fallback: if no `.net` exists, one root `*.kicad_sch` can be exported through KiCad CLI

Successful `rv scan` terminal output includes:

- `ARCHITON SCAN`
- `Target`, `Result`, `Parts`, `Nets`, `Errors`, `Warnings`, `Rules`, `Violations`
- compact rail coverage: `Inferred voltages: N Unknown voltage nets: N Rail coverage: LEVEL PCT%`
- `Inferred rails`, `Voltage coverage`, and `Metadata`
- `Detected BOM`, `Detected Netlist`, and `Generated Netlist` when directory scan auto-detects or exports files
- rail inference table when `--rails` is passed; `--explain-rails` remains supported as a legacy alias

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
