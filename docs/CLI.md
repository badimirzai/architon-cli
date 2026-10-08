
## CLI usage

`rv check` validates system architecture from a YAML specification.
`rv scan` imports KiCad BOM/netlist data and generates a normalized DesignIR report.
`rv graph` emits stable GraphIR JSON for Studio and other renderers.
`rv export` writes the Studio report and GraphIR under `.architon/studio/`.
`rv report` generates a static offline HTML report for CI artifacts and sharing.
`rv contracts draft` writes a reviewable contracts draft from a netlist. That draft is not a verification result.
`rv connections propose` writes a reviewable connection proposal from built-in pin functions and the netlist. That proposal is not a scan result.

Core commands:

```text
rv check <file.yaml>          Run deterministic analysis
rv scan <path>                Import BOM CSV, KiCad .net, or project directory and emit DesignIR report JSON
rv mcp                        Serve one MCP tool, verify, over stdio. See docs/mcp.md
rv graph <path>               Emit stable architecture GraphIR JSON
rv export <path>              Write Studio report and GraphIR under .architon/studio/
rv report <path>              Generate a static offline HTML report
rv contracts draft <path>     Write .architon/contracts.draft.yaml from a netlist
rv connections propose <path> Write .architon/connections.proposal.yaml from pin functions
rv contracts validate <path>  Validate a custom contracts.yaml schema only
rv parts list                 List built-in deterministic contract parts
rv parts show <mpn>           Show one built-in contract part, including cited pin functions
rv init                       Create .architon metadata or write a starter robot spec
rv version                    Show installed version
rv check --output json        Emit JSON findings to stdout
rv scan . --format json       Emit stable scan JSON to stdout
rv graph . --format json      Emit stable GraphIR JSON to stdout
rv graph . --format json --out graph.json
                              Emit GraphIR JSON to stdout and graph.json
rv report . --format html --out architon-report.html
                              Write an offline HTML report
rv scan . --format markdown   Emit PR-comment-ready Markdown
rv scan . --format github     Emit GitHub Actions annotations
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
rv graph exports/project.net --meta .architon/meta.yaml --format json
rv graph . --contracts examples/contracts/i2c_policy.yaml --format json --out graph.json
rv export .
rv report . --format html --out architon-report.html
rv scan . --contracts i2c_pullup_policy.yaml --verbose
rv scan . --format github
rv contracts draft .
rv connections propose .
rv parts list
rv parts show ESP32-WROOM-32
```

## Container image

`ghcr.io/badimirzai/architon` is `rv` and KiCad's `kicad-cli` packaged as a container image. It runs `rv export /project` on a project folder mounted at `/project`. You need Docker, not Go or KiCad. Architon Studio uses the same image.

Run it from the KiCad project folder, the one that contains the root `.kicad_sch` or the `.net` file:

```bash
docker run --rm --pull always --network none -v "$PWD":/project ghcr.io/badimirzai/architon
```

- `-v "$PWD":/project` shares the current folder with the container as `/project`. This is the only folder the container can see.
- `--network none` gives the container no network. This is the supported way to run it, and `rv` never needs the network.
- `--pull always` checks the registry for a newer image before the run. Without it, Docker reuses the copy it downloaded last time. The download happens before the container starts, so it works together with `--network none`.
- With no tag, Docker uses `latest`, which is the most recent release.
- On Linux, add `--user "$(id -u):$(id -g)"` after `--network none`. The image runs as non-root uid `1000`, and a mounted folder keeps its host owner, so a different uid could not write `.architon/studio`. Docker Desktop on macOS and Windows does not need it.

The run writes `.architon/studio/report.json` and `.architon/studio/graph.json`, the two files Studio imports. The exit code is the `rv export` exit code: `0` clean, `1` warnings, `2` violations, `3` tool failure. Exit `2` still writes both files. Exit `3` writes nothing new, for example when the folder has no schematic, netlist, or BOM.

A project that already has a `.net` file needs nothing else. A project with only one root `.kicad_sch` gets `.architon/generated.net` from the `kicad-cli` inside the image, which is the latest KiCad 9.0 release from Debian backports. A schematic saved by a newer major KiCad version may not open. Check the bundled version with:

```bash
docker run --rm --network none ghcr.io/badimirzai/architon kicad-cli version
```

The output matches `rv export` on the host for the same project and the same KiCad version, with two exceptions. Absolute paths name the project `/project`, and the `imported` and `parsed_at` timestamps record when the run happened.

### Versions

Each release tag publishes two tags: the version, such as `v0.15.0`, and `latest`. Use `latest` to stay current. Pin a version when a run must be repeatable, for example in CI:

```bash
docker run --rm --network none -v "$PWD":/project ghcr.io/badimirzai/architon:v0.15.0
```

`rv version` inside the image prints its release, and `graph.json` records it as `rv_version`:

```bash
docker run --rm --network none ghcr.io/badimirzai/architon rv version
```

### Other commands

The default command is `rv export /project`. To run a different `rv` command, put it after the image name:

```bash
docker run --rm --network none -v "$PWD":/project ghcr.io/badimirzai/architon rv scan /project --format json
```

For GitHub Actions, see [docs/ci.md](ci.md#architon-studio).

### Build and test the image locally

```bash
docker build --build-arg VERSION=v0.15.0 -t architon:local .
ARCHITON_IMAGE=architon:local EXPECT_VERSION=v0.15.0 bash scripts/docker-smoke.sh
```

Then run `architon:local` in a KiCad project folder in place of `ghcr.io/badimirzai/architon`.

```bash
docker build --build-arg VERSION=v0.15.0 -t architon:v0.15.0 .
ARCHITON_IMAGE=architon:v0.15.0 EXPECT_VERSION=v0.15.0 bash scripts/docker-smoke.sh
```

`scripts/docker-smoke.sh` runs the image on `examples/agent-loop/broken` with `--network none`. It checks for exit `2`, both Studio files, and the same files as host `rv export`. It prints `SKIP` and exits `0` when Docker is unavailable.

Architon normalizes imported hardware designs into DesignIR, applies ContractIR requirements, and emits deterministic findings. See [docs/architecture.md](docs/architecture.md) for details.

Contracts come from built-in component data, project metadata, schematic/BOM fields, and explicit user YAML policies. Custom contracts can enforce rules such as I2C pull-up resistance, duplicate I2C addresses, voltage compatibility, current budgets, and interface connectivity (`require.connected`). A participant's optional `pins` map names the pin name or pin number that must land on each connected net. A different pin on that net is `interface_pin_mismatch`. The same pin bound to two signals in the contract is `interface_pin_conflict`. See [docs/contracts.md](docs/contracts.md).

Use `--verbose` or `--rails` to inspect rail inference, confidence, and voltage coverage. See [docs/rail-inference.md](docs/rail-inference.md).

Architon writes deterministic JSON reports for CI and tooling. Default scan output is `architon-report.json`; default HTML report output is `architon-report.html`. See [docs/report-format.md](docs/report-format.md), [docs/html-report.md](docs/html-report.md), [docs/graph-ir.md](docs/graph-ir.md), and [docs/ci.md](docs/ci.md).

YAML architecture specs and part lookup behavior are documented in [docs/spec.md](docs/spec.md).
