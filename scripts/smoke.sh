#!/usr/bin/env bash
set -euo pipefail

# Smoke test for architon-cli Makefile + rv CLI
# Run from repo root: bash scripts/smoke.sh

echo "==> 0) Basic info"
pwd
go version
echo

echo "==> 1) Clean + format + tidy + vet + unit tests"
make clean
make fmt
make tidy
make vet
make test
echo

echo "==> 2) Build + version (built binary)"
make build
./bin/rv version
echo

echo "==> 3) Make targets (run/check/validate/version)"
make version
make run ARGS="version"
make check
make validate
echo

echo "==> 4) KiCad BOM scan fixtures: success cases"
# Adjust paths if your fixtures live elsewhere
fixtures_ok=(
  "internal/importers/kicad/testdata/bom_kicad_default.csv"
  "internal/importers/kicad/testdata/bom_semicolon.csv"
  "internal/importers/kicad/testdata/bom_preamble_rows.csv"
  "internal/importers/kicad/testdata/bom_grouped_refs.csv"
)

for f in "${fixtures_ok[@]}"; do
  echo "-- rv scan $f"
  ./bin/rv scan "$f" --out /tmp/rv-report.json
  test -f /tmp/rv-report.json
done
echo

echo "==> 5) KiCad BOM scan fixtures: expected parse error exit code=2 and report written"
fixtures_bad=(
  "internal/importers/kicad/testdata/bom_missing_columns.csv"
  "internal/importers/kicad/testdata/bom_missing_required_header.csv"
  "internal/importers/kicad/testdata/bom_bad_row_missing_comma.csv"
)

for f in "${fixtures_bad[@]}"; do
  echo "-- rv scan $f (expect exit=2, report still written)"
  set +e
  ./bin/rv scan "$f" --out /tmp/rv-report.json
  code=$?
  set -e
  if [[ "$code" -ne 2 ]]; then
    echo "FAIL: expected exit code 2, got $code for $f"
    exit 1
  fi
  test -f /tmp/rv-report.json
done
echo

echo "==> 6) Install + PATH check (non-destructive)"
make install
# If rv is already in PATH, this should work. If not, it will fail and that's fine.
if command -v rv >/dev/null 2>&1; then
  rv version
else
  echo "NOTE: rv not found in PATH. Add:"
  echo "  export PATH=\"${GOBIN:-$(go env GOPATH)/bin}:\$PATH\""
fi
echo

echo "==> 7) Quick JSON sanity checks (optional, best-effort)"
# jq optional
if command -v jq >/dev/null 2>&1; then
  ./bin/rv scan internal/importers/kicad/testdata/bom_kicad_default.csv --out /tmp/rv-report.json
  jq -e '.report_version == "0"' /tmp/rv-report.json >/dev/null
  jq -e '.design_ir.version == "0"' /tmp/rv-report.json >/dev/null
  jq -e '.summary.source == "kicad_bom_csv"' /tmp/rv-report.json >/dev/null
  echo "jq checks OK"
else
  echo "jq not installed; skipping JSON assertions"
fi
echo

echo "✅ ALL SMOKE TESTS PASSED"