package ir

const SchemaVersion = "0"

// DesignIR is the stable, EDA-neutral representation of an imported hardware
// design. Importers may know about KiCad, Altium, or native project files; code
// after import should only consume this normalized schema.
type DesignIR struct {
	Version       string     `json:"version"`
	Source        string     `json:"source"`
	SourceInfo    SourceInfo `json:"source_info,omitempty"`
	Parts         []Part     `json:"parts"`
	Pins          []Pin      `json:"pins,omitempty"`
	Nets          []Net      `json:"nets,omitempty"`
	Metadata      IRMetadata `json:"metadata"`
	ParseErrors   []string   `json:"-"`
	ParseWarnings []string   `json:"-"`
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

// Pin is an input-agnostic component pin entry. Connectivity is represented on
// Net.Pins so designs can be built from either a flat connection list or a
// net-centric EDA export.
type Pin struct {
	Ref  string `json:"ref"`
	Pin  string `json:"pin"`
	Name string `json:"name,omitempty"`
}

// SourceInfo captures normalized importer/source metadata. It is intentionally
// descriptive only; rules must not branch on importer names or file formats.
type SourceInfo struct {
	Importer string `json:"importer,omitempty"`
	Format   string `json:"format,omitempty"`
	Input    string `json:"input,omitempty"`
	Imported string `json:"imported,omitempty"`
}

// IRMetadata captures deterministic metadata about the imported source.
type IRMetadata struct {
	InputFile string `json:"input_file"`
	ParsedAt  string `json:"parsed_at"`
	Delimiter string `json:"-"`
}

// Net is a normalized electrical connection with all attached component pins.
type Net struct {
	Name string   `json:"name"`
	Pins []PinRef `json:"pins"`
}

// PinRef identifies one component pin connected to a net.
type PinRef struct {
	Ref  string `json:"ref"`
	Pin  string `json:"pin"`
	Name string `json:"name,omitempty"`
}
