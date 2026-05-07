# Importers

`rv scan` imports hardware design data into a normalized deterministic DesignIR report.

## Supported scan inputs

`rv scan .` supports three input modes:

- BOM CSV input such as `bom.csv`
- KiCad `.net` S-expression netlist input such as `exports/project.net`
- Project directory input such as `.`

Architon automatically detects:

- KiCad `.net` netlists
- KiCad BOM CSV files
- A single root KiCad schematic (`*.kicad_sch`) when no `.net` exists, exported with KiCad CLI

## Directory discovery

When you scan a directory, Architon looks for BOM candidates using this order:

1. `bom/bom.csv`
2. `bom.csv`
3. `exports/bom.csv`
4. Lexical `*bom*.csv` matches in `bom/`, `exports/`, and the project root

Netlist candidates are selected in this order:

1. Lexical `exports/*.net`
2. Lexical `*.net` in the project root

If no netlist is found, Architon looks for exactly one root `*.kicad_sch` and exports it with:

```bash
kicad-cli sch export netlist --format kicadsexpr --output .architon/generated.net <schematic>
```

`rv scan .` can generate KiCad netlists automatically when `kicad-cli` is on `PATH` or installed in a common KiCad location on macOS, Linux, or Windows. If KiCad is installed somewhere custom, pass `--kicad-cli /full/path/to/kicad-cli`.

Disable KiCad CLI netlist generation with:

```bash
rv scan . --no-kicad-cli
```

## BOM and netlist merge behavior

If both a BOM and a netlist are found, Architon merges them deterministically into one DesignIR:

- BOM remains the base source of parts and raw `fields`
- Missing BOM `value` and `footprint` fields are filled from matching netlist parts
- Net connectivity is taken from the netlist and exported as `design_ir.nets`

## Explicit overrides

You can override discovery explicitly:

```bash
rv scan . --bom bom/bom.csv --netlist exports/project.net
rv scan exports/project.net --meta .architon/meta.yaml
rv scan . --kicad-cli /full/path/to/kicad-cli
```

Architon produces a normalized deterministic DesignIR report for all supported scan inputs.
