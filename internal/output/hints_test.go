package output

import "testing"

func TestFixHint_KnownRule(t *testing.T) {
	t.Parallel()

	hint, ok := FixHint("DRV_SUPPLY_RANGE")
	if !ok {
		t.Fatal("expected hint for DRV_SUPPLY_RANGE")
	}
	if hint == "" {
		t.Fatal("expected non-empty hint")
	}
}

func TestFixHint_UnknownRule(t *testing.T) {
	t.Parallel()

	hint, ok := FixHint("UNKNOWN_RULE")
	if ok {
		t.Fatal("expected no hint for unknown rule")
	}
	if hint != "" {
		t.Fatalf("expected empty hint, got %q", hint)
	}
}
