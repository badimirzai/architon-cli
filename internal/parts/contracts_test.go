package parts

import "testing"

func TestMatchPart_ExactMPN(t *testing.T) {
	match := MatchPart("module", "ESP32-WROOM-32", nil, BuiltInPowerContracts())
	if !match.Matched {
		t.Fatalf("expected exact MPN match, got %+v", match)
	}
	if match.Kind != MatchExactMPN {
		t.Fatalf("expected exact MPN kind, got %s", match.Kind)
	}
	if match.Part.MPN != "ESP32-WROOM-32" {
		t.Fatalf("expected ESP32-WROOM-32, got %s", match.Part.MPN)
	}
}

func TestMatchPart_Alias(t *testing.T) {
	match := MatchPart("ESP32S3", "", nil, BuiltInPowerContracts())
	if !match.Matched {
		t.Fatalf("expected alias match, got %+v", match)
	}
	if match.Kind != MatchExactAlias {
		t.Fatalf("expected exact alias kind, got %s", match.Kind)
	}
	if match.Part.MPN != "ESP32-S3" {
		t.Fatalf("expected ESP32-S3, got %s", match.Part.MPN)
	}
}

func TestMatchPart_AmbiguousFuzzyReturnsNoMatch(t *testing.T) {
	match := MatchPart("ESP32", "", nil, BuiltInPowerContracts())
	if match.Matched {
		t.Fatalf("expected ambiguous match to be rejected, got %+v", match)
	}
	if match.Kind != MatchAmbiguous {
		t.Fatalf("expected ambiguous kind, got %s", match.Kind)
	}
	if len(match.Candidates) != 2 {
		t.Fatalf("expected two candidates, got %+v", match.Candidates)
	}
}
