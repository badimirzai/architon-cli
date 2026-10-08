# MCP

`rv mcp` is a local [Model Context Protocol](https://modelcontextprotocol.io) server. A client starts it and talks to it over stdin and stdout. The server stays running until the client disconnects. Running `rv mcp` in a terminal by itself waits for that protocol and looks idle.

The server exposes one tool, `verify`. That tool runs the same scan as:

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

Call `verify` again on the same path after the design or the contract changes. Compare `scan.findings`. `rv scan` also writes `architon-report.json`, and `design_ir.metadata.parsed_at` changes every run, so that file is the wrong thing to diff. The MCP server writes its report to a temporary file and removes it. The findings the client sees are the stdout JSON.

The same check without a model is:

```bash
rv scan /absolute/path/to/project --format json
```

## What verify does not do

`verify` checks a design against contracts. It does not create a schematic, place parts, route a PCB, or write manufacturing files. A clean result means the declared contracts passed.

The tool does not edit the project. The person or the agent edits the schematic or `.architon/contracts.yaml`, then calls `verify` again.

These commands stay on the terminal. The MCP server does not expose them:

```text
rv init
rv contracts draft
rv contracts validate
rv connections propose
rv connections apply
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

Open this repo, run `make build`, then reload the window or open Customize and enable the `architon` server. It should list one tool, `verify`. Start a new Agent chat after the server is enabled.

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

Expected result: `exit_code` 0 and no findings.

The KiCad project does not have to be the Cursor workspace. Pass its absolute path as `project_path`.

## Claude Code

Install `rv` so the `claude` process can run it, then:

```bash
claude mcp add --transport stdio architon -- rv mcp
```

In Claude Code:

```text
Use the architon verify tool on /absolute/path/to/project.
Report exit_code and scan.findings. Do not edit files unless I ask.
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
7. Ask it to call `verify` with the absolute project path.

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

`rv init contracts` writes `.architon/contracts.yaml`. The starter contract checks I2C pull-ups. Replace it with the contracts for this board, then scan again. `rv contracts validate .architon/contracts.yaml` checks the YAML. `rv scan` checks the design. `rv connections propose .` writes `.architon/connections.proposal.yaml` for review. `rv connections apply .` adds net labels for entries marked `accepted`. `verify` does not read that file, and a decided entry is not a scan result.

`rv scan .` uses a root `*.net` when one exists. With no netlist and one root `*.kicad_sch`, it exports `.architon/generated.net` through `kicad-cli`. See [importers.md](importers.md).

From an agent, pass that project’s absolute path to `verify`. Edit the schematic in KiCad, or edit the contract file, then call `verify` again until `exit_code` is 0.

The loop and the finding fields are also described in [contracts.md](contracts.md#scan-loop).
