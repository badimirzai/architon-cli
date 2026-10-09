# MCP

`rv mcp` is a local [Model Context Protocol](https://modelcontextprotocol.io) server. A client starts it and talks to it over stdin and stdout. The server stays running until the client disconnects. Running `rv mcp` in a terminal by itself waits for that protocol and looks idle.

The server exposes three tools: `verify`, `propose`, and `apply`. `verify` is the only pass. `propose` and `apply` do not return an exit code for the design.

`verify` runs the same scan as:

```bash
rv scan <project_path> --format json
rv scan <project_path> --format json --contracts <contracts_path>
```

## What verify does

`verify` reads a project that already exists on disk and returns the scan JSON plus the scan process result.

| Argument | Required | Meaning |
| --- | --- | --- |
| `project_path` | yes | Project directory, KiCad netlist, or BOM. Pass an absolute path. |
| `contracts_path` | no | Contracts file. When omitted, scan uses `<project>/.architon/contracts.yaml` if that file exists. |

The tool result is JSON:

| Field | Meaning |
| --- | --- |
| `exit_code` | Scan process result: `0` clean or info, `1` warnings, `2` violations, `3` the scan could not finish. |
| `scan` | The object `rv scan --format json` prints. Findings are in `scan.findings`. |
| `error` | Set only when scan exits before that JSON exists. |

Each finding includes `rule_id`, `severity`, `component_ref`, `net`, `pin`, `expected`, `observed`, and `design_fixable`. Exit code `2` is a successful tool call. The violations are the result. `design_fixable: true` means a schematic edit or a contract edit can clear that finding.

`scan.coverage.not_checked` is part of that same JSON. Each row has `ref`, and may have `pin`, `net`, and `reason`. Exit code `0` with those rows is a pass of the proved checks only. Say what was not checked. An unchecked pin is not a violation. It is also not a pass of that pin. `scan.coverage.proved` lists the checks that ran.

Call `verify` again on the same path after the design or the contract changes. Compare `scan.findings`. `rv scan` also writes `architon-report.json`, and `design_ir.metadata.parsed_at` changes every run, so that file is the wrong thing to diff. The MCP server writes its report to a temporary file and removes it. The findings the client sees are the stdout JSON.

The same check without a model is:

```bash
rv scan /absolute/path/to/project --format json
```

## What propose does

`propose` reads the same project directory or netlist as `rv connections propose` and returns that proposal as JSON. It does not scan, and it does not mark a GPIO accepted. A `needs_choice` entry lists every candidate pin. Catalog order is not a ranking.

| Argument | Required | Meaning |
| --- | --- | --- |
| `project_path` | yes | Project directory, KiCad netlist, or BOM. Pass an absolute path. |
| `write` | no | When `true`, write `.architon/connections.proposal.yaml`. When omitted or `false`, the tool writes nothing. |
| `force` | no | When `write` is `true`, overwrite an existing proposal file. This is the same rule as `rv connections propose --force`. |

The tool result is JSON:

| Field | Meaning |
| --- | --- |
| `decided` | Entries whose status is `decided`, with the same fields as the YAML: `id`, `status`, `signal`, `net`, `parts`, and each part's `citation`. A decided entry is not a scan result and is not accepted. |
| `needs_choice` | Entries whose status is `needs_choice`, including `ref`, `mpn`, `net` when the dedicated pins already agree, and `candidates`. Each candidate has `pins`, `number` when the datasheet prints one, and `citation`. |
| `conflict` | Entries whose status is `conflict`. A conflict does not propose a join. |
| `citations` | The datasheet citations used in those entries. |
| `path` | Set only when `write` is `true` and the file was written. |
| `error` | Set when the proposal could not be built, or when the file already exists and `force` is not `true`. The existing file is left unchanged. |

The written file is YAML with a `connections` list. The JSON groups those same entries by status. To accept one listed MCU pin, edit that YAML: set that entry's `status` to `accepted`, and set `pin` to one name or number copied from that entry's `candidates`. Leave `candidates` on the entry. `IO21` and `GPIO21` on the same candidate are one pin with two names. Copy either name, or copy `number`.

`propose` does not edit a schematic or `.architon/contracts.yaml`.

## What apply does

`apply` reads a proposal and adds net labels for entries whose status is `accepted`. It runs the same label apply as `rv connections apply`. It does not scan.

| Argument | Required | Meaning |
| --- | --- | --- |
| `proposal_path` | yes | Path to `.architon/connections.proposal.yaml`, or the project directory that contains it. Pass an absolute path. |

The tool result is JSON:

| Field | Meaning |
| --- | --- |
| `files_changed` | Schematic paths whose bytes changed. |
| `ids_applied` | Ids of the accepted entries that were applied. |
| `error` | Set when apply rejects the proposal. No schematic is written. |

`apply` does not return an exit code. `decided`, `needs_choice`, and `conflict` are skipped. A `needs_choice` entry is not treated as accepted.

A pin on an accepted entry that still has `candidates` must be one of those names or numbers. Any other pin is rejected. `files_changed` and `ids_applied` are empty, and the schematic bytes stay as they were. Every accepted entry is applied, or none are.

The label is one KiCad net label at an unconnected pin. Apply does not draw a wire, move a symbol, add a no-connect, or edit a `.kicad_pcb`.

## Order

1. Call `propose` first. Pass `write` true when the proposal file should be saved.
2. You may set a `needs_choice` entry to `accepted` only by copying a pin from that entry's `candidates`. A pin that is not in the list is rejected by `apply` with no file change.
3. Call `apply`, then call `verify` on the same project.
4. Report `verify`'s `exit_code` and each finding.
5. A clean `verify` with `not_checked` rows is a pass of the proved checks only. Say what was not checked.
6. Do not edit the schematic except through `apply`.

`verify` is the only pass. `exit_code` `0` is that pass. `propose` and `apply` cannot report one.

## What these tools do not do

`verify` checks a design against contracts. It does not create a schematic, place parts, route a PCB, or write manufacturing files. A clean result means the proved checks passed. Read `scan.coverage.not_checked` before calling that a pass of the whole board.

`propose` does not verify the design. `apply` adds net labels and does not verify the design.

These commands stay on the terminal. The MCP server does not expose them:

```text
rv init
rv contracts draft
rv contracts validate
rv parts list
rv parts show
rv graph
rv report
rv check
```

ChatGPT connectors need a remote MCP URL. This server speaks stdio on the local machine and does not open one.

## Build the binary the project config uses

From a clone of this repo:

```bash
make build
./bin/rv version
```

`make build` writes `./bin/rv`. On Windows the file is `./bin/rv.exe`. Cursor expands `${workspaceFolder}` to the directory that contains `.cursor/mcp.json`, so the committed config resolves to that binary on each machine.

```json
{
  "mcpServers": {
    "architon": {
      "type": "stdio",
      "command": "${workspaceFolder}/bin/rv",
      "args": ["mcp"]
    }
  }
}
```

On Windows, set `command` to `${workspaceFolder}/bin/rv.exe`.

To call `rv` from any directory, install it and use that command instead of a checkout path:

```bash
go install github.com/badimirzai/architon-cli/cmd/rv@latest
# or, from this repo:
make install
which rv
```

## Cursor

Open this repo, run `make build`, then reload the window or open Customize and enable the `architon` server. It should list `verify`, `propose`, and `apply`. Start a new Agent chat after the server is enabled.

The committed file is `.cursor/mcp.json`. You do not put your home directory in it.

For a global server that follows an installed `rv`, create `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "architon": {
      "type": "stdio",
      "command": "rv",
      "args": ["mcp"]
    }
  }
}
```

Use the absolute path from `which rv` when Cursor cannot find `rv` on its PATH.

Ask for the tool by name. This checks the closed-loop fixture without editing it:

```text
Use the architon verify tool on the absolute path of examples/agent-loop/broken.
Do not edit files. Report exit_code and, for each finding, rule_id, component_ref, net, pin, expected, observed, and design_fixable.
```

Expected result: `exit_code` 2, then two findings.

- `interface_pin_mismatch` on `U1` pin `4` of `SPI_MOSI`. `expected` is `PB15`. `observed` is `PA7`. `design_fixable` is true.
- `termination_count_low`. `expected` is `2`. `observed` is `1`. `component_ref`, `net`, and `pin` are empty because the finding is the terminator count, not one pin. `design_fixable` is true.

Then:

```text
Use the architon verify tool on the absolute path of examples/agent-loop/fixed.
Do not edit files. Report exit_code and the findings.
```

Expected result: `exit_code` 0 and no findings. If `scan.coverage.not_checked` has rows, say what was not checked. That result is a pass of the proved checks only.

The KiCad project does not have to be the Cursor workspace. Pass its absolute path as `project_path`.

To accept one listed MCU pin:

```text
Use architon propose on the absolute project path with write true.
For one needs_choice entry, set status to accepted and set pin to one name copied from that entry's candidates.
Leave every other entry unchanged. Call apply on the proposal path, then verify on the same project.
Report verify's exit_code and each finding. If not_checked has rows, say what was not checked.
Do not edit the schematic except through apply.
```

`propose` returns `needs_choice` for an MCU such as `ESP32-WROOM-32` and does not mark a GPIO accepted. `apply` rejects a pin that is not in that entry's `candidates` and writes nothing. The pass is `verify`'s `exit_code` `0`.

## Claude Code

Install `rv` so the `claude` process can run it, then:

```bash
claude mcp add --transport stdio architon -- rv mcp
```

In Claude Code:

```text
Use architon propose, then apply, then verify on /absolute/path/to/project.
Report verify's exit_code and each finding. Say what scan.coverage.not_checked lists.
Do not edit the schematic except through apply.
```

## Claude Desktop

Claude Desktop starts local servers from its own config. It does not read `.cursor/mcp.json`, and it does not expand `${workspaceFolder}`.

1. Install `rv` (`make install` or `go install github.com/badimirzai/architon-cli/cmd/rv@latest`).
2. In a terminal, run `which rv` and copy that path. Desktop apps often cannot see the PATH from your shell profile.
3. Open Claude Desktop → Settings → Developer → Edit Config.
4. The file is `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS and `%APPDATA%\Claude\claude_desktop_config.json` on Windows.
5. Add the server. Keep any servers already in the file. Replace the command with the path from `which rv`.

```json
{
  "mcpServers": {
    "architon": {
      "command": "/absolute/path/from/which/rv",
      "args": ["mcp"]
    }
  }
}
```

6. Quit Claude Desktop completely and open it again.
7. Ask it to call `propose`, then `apply`, then `verify` with the absolute project path. `verify` is the only pass.

## ChatGPT

ChatGPT custom connectors connect to a remote MCP URL. `rv mcp` does not serve one, so there is no connector URL to paste in.

Run the same check in a terminal:

```bash
rv scan /absolute/path/to/project --format json
```

Cursor and Claude can launch the local server.

## Check a real KiCad project

From the project directory:

```bash
rv init contracts
rv scan . --format json
```

`rv init contracts` writes `.architon/contracts.yaml`. The starter contract checks I2C pull-ups. Replace it with the contracts for this board, then scan again. `rv contracts validate .architon/contracts.yaml` checks the YAML. `rv scan` checks the design. `propose` returns the same reviewable joins as `rv connections propose .`. `apply` adds net labels for entries marked `accepted`. `verify` does not read that file, and a decided entry is not a scan result.

`rv scan .` uses a root `*.net` when one exists. With no netlist and one root `*.kicad_sch`, it exports `.architon/generated.net` through `kicad-cli`. See [importers.md](importers.md).

From an agent, call `propose` on that project's absolute path, accept one pin from the entry's `candidates`, call `apply`, then call `verify` on the same path. The only pass is `verify` `exit_code` `0`. Read `scan.coverage.not_checked` and say what was not checked. Do not edit the schematic except through `apply`.

The loop and the finding fields are also described in [contracts.md](contracts.md#scan-loop).
