package contracts

import (
	"testing"

	"github.com/badimirzai/architon-cli/internal/ir"
)

func TestBuiltinPartsSourceAmbiguousMatchReturnsNoContract(t *testing.T) {
	source := BuiltinPartsSource{contracts: []SystemContract{
		{MPN: "PART-A", Aliases: []string{"DUPLICATE"}},
		{MPN: "PART-B", Aliases: []string{"DUPLICATE"}},
	}}
	design := &ir.DesignIR{
		Parts: []ir.Part{{Ref: "U1", MPN: "DUPLICATE"}},
	}

	contractIR, err := source.Enrich(design)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(contractIR.AppliedRequirements) != 0 {
		t.Fatalf("expected no applied contract for ambiguous match, got %+v", contractIR.AppliedRequirements)
	}
	if _, ok := contractIR.Components["U1"]; ok {
		t.Fatalf("expected no component contract for ambiguous match, got %+v", contractIR.Components["U1"])
	}
	if len(contractIR.MissingContractData) != 1 || contractIR.MissingContractData[0].Kind != "ambiguous_part_contract" {
		t.Fatalf("expected ambiguous missing-data entry, got %+v", contractIR.MissingContractData)
	}
}
