package contracts

import "testing"

func TestMatchesPinAliasSafeNumberedMatches(t *testing.T) {
	tests := []struct {
		name    string
		pin     string
		aliases []string
	}{
		{name: "VM matches VM1", pin: "VM1", aliases: []string{"VM"}},
		{name: "VM matches VM2", pin: "VM2", aliases: []string{"VM"}},
		{name: "GND matches GND1", pin: "GND1", aliases: []string{"GND"}},
		{name: "PGND matches PGND2", pin: "PGND2", aliases: []string{"PGND"}},
		{name: "case insensitive", pin: " vm1 ", aliases: []string{"VM"}},
		{name: "separator normalized", pin: "SDA-SDI", aliases: []string{"SDA/SDI"}},
		{name: "OUT matches explicit VO alias", pin: "VO", aliases: []string{"OUT", "VO"}},
		{name: "IN matches explicit VI alias", pin: "VI", aliases: []string{"IN", "VI"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !matchesPinAlias(tt.pin, tt.aliases) {
				t.Fatalf("expected %q to match aliases %v", tt.pin, tt.aliases)
			}
		})
	}
}

func TestMatchesPinAliasUnsafeNonMatches(t *testing.T) {
	tests := []struct {
		name    string
		pin     string
		aliases []string
	}{
		{name: "VDD does not match VDDIO unless explicit", pin: "VDDIO", aliases: []string{"VDD"}},
		{name: "SCL does not match CLKIN", pin: "CLKIN", aliases: []string{"SCL"}},
		{name: "SDA does not match AUX_DA", pin: "AUX_DA", aliases: []string{"SDA"}},
		{name: "VM does not match VMON unless explicit", pin: "VMON", aliases: []string{"VM"}},
		{name: "OUT does not match VO without explicit alias", pin: "VO", aliases: []string{"OUT"}},
		{name: "IN does not match VI without explicit alias", pin: "VI", aliases: []string{"IN"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if matchesPinAlias(tt.pin, tt.aliases) {
				t.Fatalf("expected %q not to match aliases %v", tt.pin, tt.aliases)
			}
		})
	}
}

func TestMatchesPinAliasExplicitUnsafeNames(t *testing.T) {
	if !matchesPinAlias("VDDIO", []string{"VDD", "VDDIO"}) {
		t.Fatal("expected explicit VDDIO alias to match")
	}
	if !matchesPinAlias("VMON", []string{"VM", "VMON"}) {
		t.Fatal("expected explicit VMON alias to match")
	}
}
