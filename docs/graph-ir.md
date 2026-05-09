# GraphIR

GraphIR is the stable machine-readable architecture graph emitted by:

```bash
rv graph <path> --format json
rv graph <path> --format json --out graph.json
rv graph <path> --contracts .architon/contracts.yaml --format json
```

`rv graph` uses the same project detection, import, metadata, contract loading,
voltage propagation, and deterministic rule evaluation pipeline as `rv scan`.
It does not require Studio and does not call AI.

GraphIR is for Studio and other graph renderers. It is not a replacement for
the scan report: the scan report remains the primary CI/reporting artifact,
while GraphIR embeds the same findings so renderers can highlight affected
graph elements directly.

## Schema

Top-level object:

```json
{
  "graph_version": "1",
  "rv_version": "v0.6.0-dev",
  "input_path": "project-or-file-path",
  "summary": {
    "violations": 0,
    "warnings": 0,
    "infos": 0,
    "findings": 0,
    "has_failures": false,
    "user_contracts_loaded": 0,
    "built_in_contracts_loaded": 0,
    "active_user_requirements": 0
  },
  "nodes": [],
  "edges": [],
  "rails": [],
  "interfaces": [],
  "findings": [],
  "findings_index": {}
}
```

`summary` counts embedded scan findings by severity. `ERROR` increments
`violations`, `WARN` increments `warnings`, and `INFO` increments `infos`.
The contract counts and `has_failures` are copied from the same canonical scan
result used to build the graph.

### Nodes

Nodes are physical components from the imported design.

```json
{
  "id": "U1",
  "ref": "U1",
  "label": "U1 ESP32-WROOM-32",
  "type": "mcu",
  "contract_coverage": "full",
  "violations": 0,
  "warnings": 0,
  "nets": ["/I2C_SDA", "+3V3"],
  "metadata": {
    "value": "ESP32-WROOM-32",
    "footprint": "RF_Module:ESP32-WROOM-32",
    "mpn": "ESP32-WROOM-32",
    "manufacturer": "Espressif"
  }
}
```

Node `type` is one of:

```text
mcu, sensor, motor_driver, regulator, connector, passive, power_source, unknown
```

`contract_coverage` is one of:

```text
full, partial, missing, unknown
```

### Edges

Edges are electrical relationships between components on shared nets. In
GraphIR v1 these are pairwise component-to-component edges, sorted by stable
edge ID. Renderers may group them by `interface`, `type`, `domain`, or `net` to
avoid visual clutter.

```json
{
  "id": "edge_U1_U2_I2C_SDA",
  "source": "U1",
  "target": "U2",
  "net": "/I2C_SDA",
  "type": "i2c",
  "domain": "data",
  "interface": "I2C_1",
  "voltage_v": 3.3,
  "violations": 0,
  "warnings": 0,
  "finding_ids": []
}
```

Edge `type` is one of:

```text
power, i2c, spi, uart, gpio, analog, motor, unknown
```

Edge `domain` is one of:

```text
power, data, control, unknown
```

`voltage_v` is omitted when no deterministic voltage is known for the net.
`interface` is always present. For power edges it is the rail/net name; for
unknown edges it is `"unknown"`.

### Rails

Rails are power domains inferred from net names, pin roles, metadata, and
voltage propagation.

```json
{
  "name": "/+3V3",
  "voltage_v": 3.3,
  "source_ref": "U2",
  "consumers": ["U1", "U3"],
  "violations": 0,
  "warnings": 0,
  "finding_ids": []
}
```

`voltage_v` is emitted for explicit, propagated, or inferred rails. `source_ref`
is the best deterministic source component when one is known, otherwise an empty
string.

### Interfaces

Interfaces group bus-like electrical relationships such as I2C, SPI, and UART.
They are intended for Studio-level rendering when pairwise edges would be too
dense.

```json
{
  "id": "I2C_1",
  "type": "i2c",
  "nets": ["/I2C_SCL", "/I2C_SDA"],
  "participants": ["U1", "U2"],
  "pullups": [
    {
      "ref": "R1",
      "net": "/I2C_SDA",
      "resistance_ohms": 4700,
      "to_net": "/+3V3"
    }
  ],
  "violations": 0,
  "warnings": 0,
  "finding_ids": []
}
```

`pullups` is deterministic topology metadata. GraphIR does not invent pull-up
findings; it only embeds and links findings produced by the scan pipeline.

### Findings

Findings are the same deterministic scan findings produced by `rv scan`, with
stable IDs assigned for graph linking.

```json
{
  "id": "pullup_ohms_01",
  "rule_id": "pullup_ohms",
  "contract_id": "i2c_pullups",
  "contract_source": "user_yaml",
  "severity": "ERROR",
  "message": "Net /I2C_SDA has no pull-up resistor in scope",
  "component_ref": "",
  "net": "/I2C_SDA",
  "pin": "",
  "requirement": "pullup_ohms",
  "fix": "Add a pull-up resistor from /I2C_SDA to the I2C rail between 2200 and 10000 ohms.",
  "provenance": "source=user_yaml; source_id=i2c_pullups"
}
```

Severity is one of `ERROR`, `WARN`, or `INFO`.

### Findings Index

`findings_index` maps each finding ID to the graph elements affected by that
finding.

```json
{
  "pullup_ohms_01": {
    "node_ids": ["R1"],
    "edge_ids": ["edge_R1_U1_I2C_SDA", "edge_R1_U2_I2C_SDA"],
    "rail_names": [],
    "interface_ids": ["I2C_1"]
  }
}
```

Rollup rules:

- If a finding has `component_ref`, it attaches to the matching node.
- If a finding has `net`, it attaches to all graph edges for that net.
- If a finding net is a rail, it attaches to the matching rail.
- If a finding mentions multiple nets, each matching edge or rail can be linked.
- Bus-level findings attach to matching interface objects and relevant bus edges.
- `ERROR` increments `violations`; `WARN` increments `warnings`; `INFO` only
  adds finding links.

Finding IDs are stable within one graph. When multiple report findings share the
same rule ID, GraphIR appends deterministic numeric suffixes such as
`pullup_ohms_01`.

## Example

```json
{
  "graph_version": "1",
  "rv_version": "v0.6.0-dev",
  "input_path": "examples/custom-contracts/exports/custom_contracts.net",
  "summary": {
    "violations": 1,
    "warnings": 0,
    "infos": 0,
    "findings": 1
  },
  "nodes": [
    {
      "id": "U1",
      "ref": "U1",
      "label": "U1 MCU_A",
      "type": "mcu",
      "contract_coverage": "partial",
      "violations": 0,
      "warnings": 0,
      "nets": ["/+3V3", "/I2C_SDA", "GND"],
      "metadata": {
        "value": "MCU_A",
        "footprint": "",
        "mpn": "MCU_A",
        "manufacturer": ""
      }
    }
  ],
  "edges": [
    {
      "id": "edge_U1_U2_I2C_SDA",
      "source": "U1",
      "target": "U2",
      "net": "/I2C_SDA",
      "type": "i2c",
      "domain": "data",
      "interface": "I2C_MAIN",
      "voltage_v": 3.3,
      "violations": 1,
      "warnings": 0,
      "finding_ids": ["pullup_ohms_01"]
    }
  ],
  "rails": [
    {
      "name": "/+3V3",
      "voltage_v": 3.3,
      "source_ref": "",
      "consumers": ["U1", "U2"],
      "violations": 0,
      "warnings": 0,
      "finding_ids": []
    }
  ],
  "interfaces": [
    {
      "id": "I2C_MAIN",
      "type": "i2c",
      "nets": ["/I2C_SCL", "/I2C_SDA"],
      "participants": ["U1", "U2"],
      "pullups": [],
      "violations": 1,
      "warnings": 0,
      "finding_ids": ["pullup_ohms_01"]
    }
  ],
  "findings": [
    {
      "id": "pullup_ohms_01",
      "rule_id": "pullup_ohms",
      "contract_id": "i2c_pullups",
      "contract_source": "user_yaml",
      "severity": "ERROR",
      "message": "Net /I2C_SDA has no pull-up resistor in scope",
      "component_ref": "",
      "net": "/I2C_SDA",
      "pin": "",
      "requirement": "pullup_ohms",
      "fix": "Add a pull-up resistor from /I2C_SDA to the I2C rail between 2200 and 10000 ohms.",
      "provenance": "source=user_yaml; source_id=i2c_pullups"
    }
  ],
  "findings_index": {
    "pullup_ohms_01": {
      "node_ids": [],
      "edge_ids": ["edge_U1_U2_I2C_SDA"],
      "rail_names": [],
      "interface_ids": ["I2C_MAIN"]
    }
  }
}
```

## Studio Consumption

Studio should treat `nodes`, `edges`, `rails`, and `interfaces` as renderable
graph data:

- Use `node.id` as the graph node key and `node.ref` as the schematic reference.
- Use `edge.source` and `edge.target` to connect physical components.
- Use `edge.net` as the electrical net represented by an edge.
- Group or style edges by `edge.interface`, `edge.type`, and `edge.domain`.
- Prefer `interfaces` for bus rendering when pairwise edges would be visually
  dense.
- Render rails from `rails`; rails may also appear as power edges when connected
  components share a rail net.
- Use `violations` and `warnings` on nodes, edges, rails, and interfaces for
  badges.
- Use `findings` for finding detail panels.
- Use `finding_ids` and `findings_index` to highlight all graph elements
  affected by a selected finding.

Studio should not parse human scan output and should not infer additional
electrical meaning from labels. The stable fields are the typed fields above.

## Stability

GraphIR `graph_version` is currently `"1"`. For a given CLI build, input files,
metadata, contracts, and command flags, output is deterministic across repeated
runs.

GraphIR v1 preserves stable ordering:

- nodes sort by component reference
- edges sort by edge ID
- rails sort by rail name
- interfaces sort by interface ID
- findings preserve canonical scan finding order

Additive fields may appear in future `graph_version` `"1"` outputs. Existing
fields, enum values, and link semantics will not be removed or redefined without
incrementing `graph_version`.
