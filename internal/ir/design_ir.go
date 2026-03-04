package ir

const SchemaVersion = "0"

// DesignIR is a stable internal representation of an imported hardware design.
type DesignIR struct {
	Version       string     `json:"version"`
	Source        string     `json:"source"`
	Parts         []Part     `json:"parts"`
	Metadata      IRMetadata `json:"metadata"`
	ParseErrors   []string   `json:"-"`
	ParseWarnings []string   `json:"-"`
	Nets          []Net      `json:"nets,omitempty"`
}

// Part is an input-agnostic component entry.
type Part struct {
	Ref          string            `json:"ref"`
	Value        string            `json:"value"`
	Footprint    string            `json:"footprint"`
	MPN          string            `json:"mpn,omitempty"`
	Manufacturer string            `json:"manufacturer,omitempty"`
	Fields       map[string]string `json:"fields,omitempty"`
}

// IRMetadata captures deterministic metadata about the imported source.
type IRMetadata struct {
	InputFile string `json:"input_file"`
	ParsedAt  string `json:"parsed_at"`
	Delimiter string `json:"-"`
}

type Net struct {
	Name string   `json:"name"`
	Pins []PinRef `json:"pins"`
}

type PinRef struct {
	Ref string `json:"ref"`
	Pin string `json:"pin"`
}
