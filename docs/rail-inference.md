# Rail Inference

Rail voltage inference is deterministic and transparent. Each inference includes source, confidence score, reason, evidence, and warnings.

Confidence score indicates the reliability of an inference. Coverage indicates how much of the design can be safely checked by voltage rules. High coverage does not guarantee correctness. Low coverage means some checks may be skipped.

## Inspect rail inference

Use `--verbose` to include compact rail inference counts in terminal output:

```bash
rv scan example.net --verbose
```

Verbose output includes:

```text
Inferred voltages: 2 Unknown voltage nets: 1 Rail coverage: MEDIUM 67%
Inferred rails: 2
Voltage coverage: 2/3 nets with inferred voltage
Metadata: inferred
```

Use `--rails` to print deterministic rail inference details, including voltage, confidence level, confidence score, source, and rail coverage:

```bash
rv scan example.net --rails
```

Example detail:

```text
Rail inference:

- /+5V: 5.00V  HIGH   0.95  net_name
- VIN:  UNKNOWN LOW    0.35  HEURISTIC

Rail coverage:

- Total nets: 3
- Usable for rules: 2/3
- Coverage: MEDIUM 67%
```

`--explain-rails` remains supported as a legacy alias.

## Coverage fields

Netlist-backed scan reports may include:

- `derived.net_voltages`
- `derived.inferred_net_voltages`
- `derived.unknown_voltage_nets`
- `derived.rail_inferences`
- `derived.rail_coverage`
- optional `findings[].inference` provenance for voltage-based findings

Use the JSON report for full rail inference provenance in CI and review workflows.
