package parts

import (
	"sort"
	"strings"
	"unicode"
)

const (
	CategoryMCU          = "mcu"
	CategorySensor       = "sensor"
	CategoryRegulator    = "regulator"
	CategoryMotorDriver  = "motor_driver"
	CategoryLevelShifter = "level_shifter"
	CategoryProtectionIC = "protection_ic"
	CategoryConnector    = "connector"
	CategoryPowerSwitch  = "power_switch"
	CategoryGenericIC    = "generic_ic"

	ContractSourceDatasheet  = "datasheet"
	ContractSourceVendorPage = "vendor_page"
	ContractSourceCurated    = "curated"
	ContractSourceInferred   = "inferred"

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// PartContract is the richer v0.3.1 built-in component power-contract schema.
type PartContract struct {
	MPN           string        `yaml:"mpn" json:"mpn"`
	Aliases       []string      `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Category      string        `yaml:"category" json:"category"`
	Package       string        `yaml:"package,omitempty" json:"package,omitempty"`
	Manufacturer  string        `yaml:"manufacturer,omitempty" json:"manufacturer,omitempty"`
	DatasheetURL  string        `yaml:"datasheet_url,omitempty" json:"datasheet_url,omitempty"`
	PowerContract PowerContract `yaml:"power_contract" json:"power_contract"`
}

type PowerContract struct {
	Supplies     []SupplyContract      `yaml:"supplies,omitempty" json:"supplies,omitempty"`
	Grounds      []GroundContract      `yaml:"grounds,omitempty" json:"grounds,omitempty"`
	Logic        LogicContract         `yaml:"logic,omitempty" json:"logic,omitempty"`
	PowerOutputs []PowerOutputContract `yaml:"power_outputs,omitempty" json:"power_outputs,omitempty"`
	MotorDriver  MotorDriverContract   `yaml:"motor_driver,omitempty" json:"motor_driver,omitempty"`
	Protection   ProtectionContract    `yaml:"protection,omitempty" json:"protection,omitempty"`
	Thermal      ThermalContract       `yaml:"thermal,omitempty" json:"thermal,omitempty"`
	Confidence   ConfidenceContract    `yaml:"confidence,omitempty" json:"confidence,omitempty"`
}

type SupplyContract struct {
	Name            string   `yaml:"name" json:"name"`
	PinAliases      []string `yaml:"pin_aliases,omitempty" json:"pin_aliases,omitempty"`
	Required        bool     `yaml:"required" json:"required"`
	NominalV        *float64 `yaml:"nominal_v,omitempty" json:"nominal_v,omitempty"`
	RecommendedMinV *float64 `yaml:"recommended_min_v,omitempty" json:"recommended_min_v,omitempty"`
	RecommendedMaxV *float64 `yaml:"recommended_max_v,omitempty" json:"recommended_max_v,omitempty"`
	AbsMinV         *float64 `yaml:"abs_min_v,omitempty" json:"abs_min_v,omitempty"`
	AbsMaxV         *float64 `yaml:"abs_max_v,omitempty" json:"abs_max_v,omitempty"`
	MaxCurrentA     *float64 `yaml:"max_current_a,omitempty" json:"max_current_a,omitempty"`
	TypicalCurrentA *float64 `yaml:"typical_current_a,omitempty" json:"typical_current_a,omitempty"`
	InrushCurrentA  *float64 `yaml:"inrush_current_a,omitempty" json:"inrush_current_a,omitempty"`
	Notes           string   `yaml:"notes,omitempty" json:"notes,omitempty"`
}

type GroundContract struct {
	Name       string   `yaml:"name" json:"name"`
	PinAliases []string `yaml:"pin_aliases,omitempty" json:"pin_aliases,omitempty"`
}

type LogicContract struct {
	DefaultIODomain       string   `yaml:"default_io_domain,omitempty" json:"default_io_domain,omitempty"`
	IOAbsMinV             *float64 `yaml:"io_abs_min_v,omitempty" json:"io_abs_min_v,omitempty"`
	IOAbsMaxV             *float64 `yaml:"io_abs_max_v,omitempty" json:"io_abs_max_v,omitempty"`
	IORecommendedMinV     *float64 `yaml:"io_recommended_min_v,omitempty" json:"io_recommended_min_v,omitempty"`
	IORecommendedMaxV     *float64 `yaml:"io_recommended_max_v,omitempty" json:"io_recommended_max_v,omitempty"`
	FiveVTolerant         *bool    `yaml:"five_v_tolerant,omitempty" json:"five_v_tolerant,omitempty"`
	FiveVTolerantPins     []string `yaml:"five_v_tolerant_pins,omitempty" json:"five_v_tolerant_pins,omitempty"`
	NonFiveVTolerantPins  []string `yaml:"non_five_v_tolerant_pins,omitempty" json:"non_five_v_tolerant_pins,omitempty"`
	VIHMinV               *float64 `yaml:"vih_min_v,omitempty" json:"vih_min_v,omitempty"`
	VILMaxV               *float64 `yaml:"vil_max_v,omitempty" json:"vil_max_v,omitempty"`
	VOHMinV               *float64 `yaml:"voh_min_v,omitempty" json:"voh_min_v,omitempty"`
	VOLMaxV               *float64 `yaml:"vol_max_v,omitempty" json:"vol_max_v,omitempty"`
	MaxInjectionCurrentMA *float64 `yaml:"max_injection_current_ma,omitempty" json:"max_injection_current_ma,omitempty"`
}

type PowerOutputContract struct {
	Name                string   `yaml:"name" json:"name"`
	PinAliases          []string `yaml:"pin_aliases,omitempty" json:"pin_aliases,omitempty"`
	OutputNominalV      *float64 `yaml:"output_nominal_v,omitempty" json:"output_nominal_v,omitempty"`
	OutputMinV          *float64 `yaml:"output_min_v,omitempty" json:"output_min_v,omitempty"`
	OutputMaxV          *float64 `yaml:"output_max_v,omitempty" json:"output_max_v,omitempty"`
	MaxOutputCurrentA   *float64 `yaml:"max_output_current_a,omitempty" json:"max_output_current_a,omitempty"`
	DropoutV            *float64 `yaml:"dropout_v,omitempty" json:"dropout_v,omitempty"`
	RequiresInputSupply string   `yaml:"requires_input_supply,omitempty" json:"requires_input_supply,omitempty"`
}

type MotorDriverContract struct {
	VMSupplyName             string   `yaml:"vm_supply_name,omitempty" json:"vm_supply_name,omitempty"`
	LogicSupplyName          string   `yaml:"logic_supply_name,omitempty" json:"logic_supply_name,omitempty"`
	MotorOutputPins          []string `yaml:"motor_output_pins,omitempty" json:"motor_output_pins,omitempty"`
	RecommendedVMMinV        *float64 `yaml:"recommended_vm_min_v,omitempty" json:"recommended_vm_min_v,omitempty"`
	RecommendedVMMaxV        *float64 `yaml:"recommended_vm_max_v,omitempty" json:"recommended_vm_max_v,omitempty"`
	AbsVMMaxV                *float64 `yaml:"abs_vm_max_v,omitempty" json:"abs_vm_max_v,omitempty"`
	ContinuousOutputCurrentA *float64 `yaml:"continuous_output_current_a,omitempty" json:"continuous_output_current_a,omitempty"`
	PeakOutputCurrentA       *float64 `yaml:"peak_output_current_a,omitempty" json:"peak_output_current_a,omitempty"`
	CurrentLimitA            *float64 `yaml:"current_limit_a,omitempty" json:"current_limit_a,omitempty"`
	HasCurrentRegulation     *bool    `yaml:"has_current_regulation,omitempty" json:"has_current_regulation,omitempty"`
	HasThermalShutdown       *bool    `yaml:"has_thermal_shutdown,omitempty" json:"has_thermal_shutdown,omitempty"`
	HasUVLO                  *bool    `yaml:"has_uvlo,omitempty" json:"has_uvlo,omitempty"`
}

type ProtectionContract struct {
	ReversePolarityProtected *bool    `yaml:"reverse_polarity_protected,omitempty" json:"reverse_polarity_protected,omitempty"`
	OvercurrentProtected     *bool    `yaml:"overcurrent_protected,omitempty" json:"overcurrent_protected,omitempty"`
	ThermalShutdown          *bool    `yaml:"thermal_shutdown,omitempty" json:"thermal_shutdown,omitempty"`
	UVLOThresholdV           *float64 `yaml:"uvlo_threshold_v,omitempty" json:"uvlo_threshold_v,omitempty"`
	OVPThresholdV            *float64 `yaml:"ovp_threshold_v,omitempty" json:"ovp_threshold_v,omitempty"`
	ClampVoltageV            *float64 `yaml:"clamp_voltage_v,omitempty" json:"clamp_voltage_v,omitempty"`
	MaxClampCurrentMA        *float64 `yaml:"max_clamp_current_ma,omitempty" json:"max_clamp_current_ma,omitempty"`
}

type ThermalContract struct {
	MaxJunctionTempC       *float64 `yaml:"max_junction_temp_c,omitempty" json:"max_junction_temp_c,omitempty"`
	RecommendedAmbientMaxC *float64 `yaml:"recommended_ambient_max_c,omitempty" json:"recommended_ambient_max_c,omitempty"`
	ThetaJACPerW           *float64 `yaml:"theta_ja_c_per_w,omitempty" json:"theta_ja_c_per_w,omitempty"`
	PowerDissipationW      *float64 `yaml:"power_dissipation_w,omitempty" json:"power_dissipation_w,omitempty"`
}

type ConfidenceContract struct {
	Source string `yaml:"source" json:"source"`
	Level  string `yaml:"level" json:"level"`
	Notes  string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

type MatchKind string

const (
	MatchNone            MatchKind = "none"
	MatchExactMPN        MatchKind = "exact_mpn"
	MatchExactAlias      MatchKind = "exact_alias"
	MatchNormalizedValue MatchKind = "normalized_value"
	MatchFuzzyPrefix     MatchKind = "fuzzy_prefix"
	MatchAmbiguous       MatchKind = "ambiguous"
)

type MatchResult struct {
	Matched    bool
	Kind       MatchKind
	Part       PartContract
	Candidates []PartContract
	Query      string
	Reason     string
}

// MatchPart resolves a BOM/schematic part to one deterministic built-in power
// contract. Ambiguous fuzzy matches intentionally return no match.
func MatchPart(value string, mpn string, fields map[string]string, lib []PartContract) MatchResult {
	if len(lib) == 0 {
		return MatchResult{Kind: MatchNone, Reason: "empty library"}
	}
	if exact := matchExactMPN(mpnCandidates(mpn, fields), lib); exact.Matched {
		return exact
	}
	if exact := matchExactAlias(valueCandidates(value, fields), lib); exact.Matched {
		return exact
	}
	if exact := matchNormalizedValue(valueCandidates(value, fields), lib); exact.Matched || exact.Kind == MatchAmbiguous {
		return exact
	}
	return matchFuzzyPrefix(valueCandidates(value, fields), lib)
}

func BuiltInPowerContracts() []PartContract {
	out := []PartContract{
		mcuSTM32F103C8T6(),
		mcuSTM32F407VGT6(),
		mcuESP32WROOM32(),
		mcuESP32S3(),
		mcuRP2040(),
		sensorMPU6050(),
		sensorBMP280(),
		sensorBNO055(),
		regAMS111733(),
		regAP2114H33(),
		regMCP17003302(),
		driverDRV8833(),
		driverDRV8871(),
		driverTB6612FNG(),
		driverL298N(),
		shifterPCA9306(),
		shifterTXS0108E(),
		shifterBSS138Module(),
	}
	sort.Slice(out, func(i, j int) bool {
		return normalizedPartKey(out[i].MPN) < normalizedPartKey(out[j].MPN)
	})
	return out
}

func normalizedPartKey(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func mpnCandidates(mpn string, fields map[string]string) []string {
	out := uniqueNonEmpty([]string{mpn})
	for key, value := range fields {
		normKey := normalizedPartKey(key)
		switch normKey {
		case "MPN", "MANUFACTURERPARTNUMBER", "PARTNUMBER", "MFRPARTNUMBER", "MFRPN":
			out = append(out, value)
		}
	}
	return uniqueNonEmpty(out)
}

func valueCandidates(value string, fields map[string]string) []string {
	out := uniqueNonEmpty([]string{value})
	for key, fieldValue := range fields {
		normKey := normalizedPartKey(key)
		switch normKey {
		case "VALUE", "COMPONENT", "COMMENT", "DESIGNATION", "DESCRIPTION", "MPN", "MANUFACTURERPARTNUMBER", "PARTNUMBER":
			out = append(out, fieldValue)
		}
	}
	return uniqueNonEmpty(out)
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizedPartKey(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func matchExactMPN(queries []string, lib []PartContract) MatchResult {
	for _, query := range queries {
		nq := normalizedPartKey(query)
		if nq == "" {
			continue
		}
		matches := make([]PartContract, 0, 1)
		for _, part := range lib {
			if normalizedPartKey(part.MPN) == nq {
				matches = append(matches, part)
			}
		}
		if len(matches) == 1 {
			return MatchResult{Matched: true, Kind: MatchExactMPN, Part: matches[0], Query: query, Reason: "exact MPN"}
		}
		if len(matches) > 1 {
			return ambiguousMatch(query, matches)
		}
	}
	return MatchResult{Kind: MatchNone}
}

func matchExactAlias(queries []string, lib []PartContract) MatchResult {
	for _, query := range queries {
		nq := normalizedPartKey(query)
		if nq == "" {
			continue
		}
		matches := make([]PartContract, 0, 1)
		for _, part := range lib {
			for _, alias := range part.Aliases {
				if normalizedPartKey(alias) == nq {
					matches = append(matches, part)
				}
			}
		}
		if len(matches) == 1 {
			return MatchResult{Matched: true, Kind: MatchExactAlias, Part: matches[0], Query: query, Reason: "exact alias"}
		}
		if len(matches) > 1 {
			return ambiguousMatch(query, matches)
		}
	}
	return MatchResult{Kind: MatchNone}
}

func matchNormalizedValue(queries []string, lib []PartContract) MatchResult {
	for _, query := range queries {
		nq := normalizedPartKey(query)
		if nq == "" {
			continue
		}
		matches := make([]PartContract, 0, 1)
		for _, part := range lib {
			if normalizedPartKey(part.MPN) == nq {
				matches = append(matches, part)
				continue
			}
			for _, alias := range part.Aliases {
				if normalizedPartKey(alias) == nq {
					matches = append(matches, part)
					break
				}
			}
		}
		if len(matches) == 1 {
			return MatchResult{Matched: true, Kind: MatchNormalizedValue, Part: matches[0], Query: query, Reason: "normalized value"}
		}
		if len(matches) > 1 {
			return ambiguousMatch(query, matches)
		}
	}
	return MatchResult{Kind: MatchNone}
}

func matchFuzzyPrefix(queries []string, lib []PartContract) MatchResult {
	for _, query := range queries {
		nq := normalizedPartKey(query)
		if len(nq) < 4 {
			continue
		}
		matches := make([]PartContract, 0, 2)
		seen := map[string]struct{}{}
		for _, part := range lib {
			keys := append([]string{part.MPN}, part.Aliases...)
			for _, key := range keys {
				nk := normalizedPartKey(key)
				if len(nk) < 4 {
					continue
				}
				if strings.HasPrefix(nk, nq) || strings.HasPrefix(nq, nk) {
					if _, ok := seen[part.MPN]; !ok {
						seen[part.MPN] = struct{}{}
						matches = append(matches, part)
					}
					break
				}
			}
		}
		if len(matches) == 1 {
			return MatchResult{Matched: true, Kind: MatchFuzzyPrefix, Part: matches[0], Query: query, Reason: "fuzzy prefix"}
		}
		if len(matches) > 1 {
			return ambiguousMatch(query, matches)
		}
	}
	return MatchResult{Kind: MatchNone, Reason: "no deterministic match"}
}

func ambiguousMatch(query string, matches []PartContract) MatchResult {
	sort.Slice(matches, func(i, j int) bool {
		return normalizedPartKey(matches[i].MPN) < normalizedPartKey(matches[j].MPN)
	})
	return MatchResult{
		Matched:    false,
		Kind:       MatchAmbiguous,
		Candidates: matches,
		Query:      query,
		Reason:     "ambiguous match",
	}
}

func f(v float64) *float64 { return &v }
func b(v bool) *bool       { return &v }

func confidence(level string) ConfidenceContract {
	return ConfidenceContract{Source: ContractSourceDatasheet, Level: level}
}

func mcuSupplies(nominal float64, recMin float64, recMax float64, absMax float64, typicalCurrent float64, aliases []string) []SupplyContract {
	return []SupplyContract{{
		Name:            "VDD",
		PinAliases:      aliases,
		Required:        true,
		NominalV:        f(nominal),
		RecommendedMinV: f(recMin),
		RecommendedMaxV: f(recMax),
		AbsMinV:         f(-0.3),
		AbsMaxV:         f(absMax),
		TypicalCurrentA: f(typicalCurrent),
	}}
}

func standardGrounds(aliases []string) []GroundContract {
	return []GroundContract{{Name: "GND", PinAliases: aliases}}
}

func logic33(absMax float64, fiveVTolerant bool) LogicContract {
	return LogicContract{
		DefaultIODomain:   "VDD",
		IOAbsMinV:         f(-0.3),
		IOAbsMaxV:         f(absMax),
		IORecommendedMinV: f(0),
		IORecommendedMaxV: f(3.3),
		FiveVTolerant:     b(fiveVTolerant),
		VIHMinV:           f(2.0),
		VILMaxV:           f(0.8),
		VOHMinV:           f(2.4),
		VOLMaxV:           f(0.4),
	}
}

func mcuSTM32F103C8T6() PartContract {
	return PartContract{
		MPN:          "STM32F103C8T6",
		Aliases:      []string{"STM32F103C8", "STM32F103C8T6TR", "STM32F103"},
		Category:     CategoryMCU,
		Package:      "LQFP-48",
		Manufacturer: "STMicroelectronics",
		DatasheetURL: "https://www.st.com/resource/en/datasheet/stm32f103c8.pdf",
		PowerContract: PowerContract{
			Supplies:   mcuSupplies(3.3, 2.0, 3.6, 4.0, 0.024, []string{"VDD", "VDDA", "VBAT"}),
			Grounds:    standardGrounds([]string{"VSS", "VSSA", "GND"}),
			Logic:      logic33(4.0, false),
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func mcuSTM32F407VGT6() PartContract {
	return PartContract{
		MPN:          "STM32F407VGT6",
		Aliases:      []string{"STM32F407VG", "STM32F407", "STM32F407VGT6TR"},
		Category:     CategoryMCU,
		Package:      "LQFP-100",
		Manufacturer: "STMicroelectronics",
		DatasheetURL: "https://www.st.com/resource/en/datasheet/stm32f407vg.pdf",
		PowerContract: PowerContract{
			Supplies:   mcuSupplies(3.3, 1.8, 3.6, 4.0, 0.08, []string{"VDD", "VDDA", "VBAT"}),
			Grounds:    standardGrounds([]string{"VSS", "VSSA", "GND"}),
			Logic:      logic33(4.0, false),
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func mcuESP32WROOM32() PartContract {
	return PartContract{
		MPN:          "ESP32-WROOM-32",
		Aliases:      []string{"ESP32-WROOM-32D", "ESP32-WROOM-32E", "ESP32 WROOM 32"},
		Category:     CategoryMCU,
		Manufacturer: "Espressif",
		DatasheetURL: "https://www.espressif.com/sites/default/files/documentation/esp32-wroom-32_datasheet_en.pdf",
		PowerContract: PowerContract{
			Supplies:   mcuSupplies(3.3, 3.0, 3.6, 3.6, 0.08, []string{"3V3", "VDD", "VDDA", "VDD3P3", "VDD_SDIO"}),
			Grounds:    standardGrounds([]string{"GND"}),
			Logic:      logic33(3.6, false),
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func mcuESP32S3() PartContract {
	return PartContract{
		MPN:          "ESP32-S3",
		Aliases:      []string{"ESP32-S3-WROOM-1", "ESP32-S3-MINI-1", "ESP32S3"},
		Category:     CategoryMCU,
		Manufacturer: "Espressif",
		DatasheetURL: "https://www.espressif.com/sites/default/files/documentation/esp32-s3_datasheet_en.pdf",
		PowerContract: PowerContract{
			Supplies:   mcuSupplies(3.3, 3.0, 3.6, 3.6, 0.08, []string{"3V3", "VDD", "VDDA", "VDD3P3", "VDDA3P3"}),
			Grounds:    standardGrounds([]string{"GND"}),
			Logic:      logic33(3.6, false),
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func mcuRP2040() PartContract {
	return PartContract{
		MPN:          "RP2040",
		Aliases:      []string{"Raspberry Pi RP2040"},
		Category:     CategoryMCU,
		Package:      "QFN-56",
		Manufacturer: "Raspberry Pi",
		DatasheetURL: "https://datasheets.raspberrypi.com/rp2040/rp2040-datasheet.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "IOVDD", PinAliases: []string{"IOVDD", "VDDIO"}, Required: true, NominalV: f(3.3), RecommendedMinV: f(1.62), RecommendedMaxV: f(3.63), AbsMinV: f(-0.3), AbsMaxV: f(3.63), TypicalCurrentA: f(0.02)},
				{Name: "DVDD", PinAliases: []string{"DVDD", "VREG_VOUT"}, Required: true, NominalV: f(1.1), RecommendedMinV: f(1.0), RecommendedMaxV: f(1.2), AbsMinV: f(-0.3), AbsMaxV: f(1.32), TypicalCurrentA: f(0.02)},
				{Name: "USB_VDD", PinAliases: []string{"USB_VDD"}, Required: false, NominalV: f(3.3), RecommendedMinV: f(3.0), RecommendedMaxV: f(3.6), AbsMinV: f(-0.3), AbsMaxV: f(3.63)},
			},
			Grounds:    standardGrounds([]string{"GND"}),
			Logic:      logic33(3.63, false),
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func sensorMPU6050() PartContract {
	return PartContract{
		MPN:          "MPU-6050",
		Aliases:      []string{"MPU6050", "MPU-6000"},
		Category:     CategorySensor,
		Manufacturer: "TDK InvenSense",
		DatasheetURL: "https://invensense.tdk.com/wp-content/uploads/2015/02/MPU-6000-Datasheet1.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "VDD", PinAliases: []string{"VDD"}, Required: true, NominalV: f(3.3), RecommendedMinV: f(2.375), RecommendedMaxV: f(3.46), AbsMinV: f(-0.5), AbsMaxV: f(6.0), TypicalCurrentA: f(0.0039)},
				{Name: "VLOGIC", PinAliases: []string{"VLOGIC", "VDDIO"}, Required: false, NominalV: f(3.3), RecommendedMinV: f(1.71), RecommendedMaxV: f(3.46), AbsMinV: f(-0.5), AbsMaxV: f(6.0)},
			},
			Grounds: standardGrounds([]string{"GND"}),
			Logic: LogicContract{
				DefaultIODomain:   "VLOGIC",
				IOAbsMinV:         f(-0.5),
				IOAbsMaxV:         f(3.6),
				IORecommendedMinV: f(1.71),
				IORecommendedMaxV: f(3.46),
				FiveVTolerant:     b(false),
				VIHMinV:           f(0.7 * 3.3),
				VILMaxV:           f(0.3 * 3.3),
			},
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func sensorBMP280() PartContract {
	return PartContract{
		MPN:          "BMP280",
		Aliases:      []string{"BME280"},
		Category:     CategorySensor,
		Manufacturer: "Bosch Sensortec",
		DatasheetURL: "https://www.bosch-sensortec.com/media/boschsensortec/downloads/datasheets/bst-bmp280-ds001.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "VDD", PinAliases: []string{"VDD"}, Required: true, NominalV: f(3.3), RecommendedMinV: f(1.71), RecommendedMaxV: f(3.6), AbsMinV: f(-0.3), AbsMaxV: f(4.25), TypicalCurrentA: f(0.0000036)},
				{Name: "VDDIO", PinAliases: []string{"VDDIO", "VIO"}, Required: true, NominalV: f(3.3), RecommendedMinV: f(1.2), RecommendedMaxV: f(3.6), AbsMinV: f(-0.3), AbsMaxV: f(4.25)},
			},
			Grounds: standardGrounds([]string{"GND"}),
			Logic:   logic33(3.6, false),
			Confidence: ConfidenceContract{
				Source: ContractSourceDatasheet,
				Level:  ConfidenceHigh,
				Notes:  "BME280 alias shares the same common module voltage constraints for architecture checks.",
			},
		},
	}
}

func sensorBNO055() PartContract {
	return PartContract{
		MPN:          "BNO055",
		Aliases:      []string{"BNO055 Shuttle", "BNO055 Module"},
		Category:     CategorySensor,
		Manufacturer: "Bosch Sensortec",
		DatasheetURL: "https://www.bosch-sensortec.com/media/boschsensortec/downloads/datasheets/bst-bno055-ds000.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "VDD", PinAliases: []string{"VDD"}, Required: true, NominalV: f(3.3), RecommendedMinV: f(2.4), RecommendedMaxV: f(3.6), AbsMinV: f(-0.3), AbsMaxV: f(4.25), TypicalCurrentA: f(0.0123)},
				{Name: "VDDIO", PinAliases: []string{"VDDIO", "VIO"}, Required: true, NominalV: f(3.3), RecommendedMinV: f(1.7), RecommendedMaxV: f(3.6), AbsMinV: f(-0.3), AbsMaxV: f(4.25)},
			},
			Grounds:    standardGrounds([]string{"GND"}),
			Logic:      logic33(3.6, false),
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func regAMS111733() PartContract {
	return PartContract{
		MPN:          "AMS1117-3.3",
		Aliases:      []string{"AMS1117-3V3", "AMS1117 3.3", "AMS1117"},
		Category:     CategoryRegulator,
		Manufacturer: "Advanced Monolithic Systems",
		DatasheetURL: "http://www.advanced-monolithic.com/pdf/ds1117.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{{Name: "VIN", PinAliases: []string{"VIN", "IN", "3"}, Required: true, RecommendedMinV: f(4.5), RecommendedMaxV: f(12.0), AbsMinV: f(-0.3), AbsMaxV: f(15.0)}},
			Grounds:  standardGrounds([]string{"GND", "1", "ADJ"}),
			PowerOutputs: []PowerOutputContract{{
				Name: "VOUT", PinAliases: []string{"VOUT", "OUT", "2", "TAB"}, OutputNominalV: f(3.3), OutputMinV: f(3.168), OutputMaxV: f(3.432), MaxOutputCurrentA: f(1.0), DropoutV: f(1.1), RequiresInputSupply: "VIN",
			}},
			Protection: ProtectionContract{ThermalShutdown: b(true), OvercurrentProtected: b(true)},
			Confidence: confidence(ConfidenceMedium),
		},
	}
}

func regAP2114H33() PartContract {
	return PartContract{
		MPN:          "AP2114H-3.3",
		Aliases:      []string{"AP2114H-3.3TRG1", "AP2114-3.3", "AP2114K-3.3"},
		Category:     CategoryRegulator,
		Manufacturer: "Diodes Incorporated",
		DatasheetURL: "https://www.diodes.com/assets/Datasheets/AP2114.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{{Name: "VIN", PinAliases: []string{"VIN", "IN", "1"}, Required: true, RecommendedMinV: f(2.5), RecommendedMaxV: f(6.0), AbsMinV: f(-0.3), AbsMaxV: f(6.5)}},
			Grounds:  standardGrounds([]string{"GND", "2"}),
			PowerOutputs: []PowerOutputContract{{
				Name: "VOUT", PinAliases: []string{"VOUT", "OUT", "5"}, OutputNominalV: f(3.3), OutputMinV: f(3.201), OutputMaxV: f(3.399), MaxOutputCurrentA: f(1.0), DropoutV: f(0.25), RequiresInputSupply: "VIN",
			}},
			Protection: ProtectionContract{ThermalShutdown: b(true), OvercurrentProtected: b(true)},
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func regMCP17003302() PartContract {
	return PartContract{
		MPN:          "MCP1700-3302",
		Aliases:      []string{"MCP1700T-3302E", "MCP1700-3302E", "MCP1700 3.3"},
		Category:     CategoryRegulator,
		Manufacturer: "Microchip",
		DatasheetURL: "https://ww1.microchip.com/downloads/aemDocuments/documents/APID/ProductDocuments/DataSheets/MCP1700-Data-Sheet-20001826.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{{Name: "VIN", PinAliases: []string{"VIN", "IN", "3"}, Required: true, RecommendedMinV: f(3.5), RecommendedMaxV: f(6.0), AbsMinV: f(-0.3), AbsMaxV: f(6.5)}},
			Grounds:  standardGrounds([]string{"GND", "1"}),
			PowerOutputs: []PowerOutputContract{{
				Name: "VOUT", PinAliases: []string{"VOUT", "OUT", "2"}, OutputNominalV: f(3.3), OutputMinV: f(3.234), OutputMaxV: f(3.366), MaxOutputCurrentA: f(0.25), DropoutV: f(0.178), RequiresInputSupply: "VIN",
			}},
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func motorDriverCommon(mpn string, aliases []string, manufacturer string, url string, vmMin float64, vmMax float64, vmAbs float64, logicMin float64, logicMax float64, continuous float64, peak float64, vmAliases []string, logicAliases []string, outAliases []string) PartContract {
	return PartContract{
		MPN:          mpn,
		Aliases:      aliases,
		Category:     CategoryMotorDriver,
		Manufacturer: manufacturer,
		DatasheetURL: url,
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "VM", PinAliases: vmAliases, Required: true, RecommendedMinV: f(vmMin), RecommendedMaxV: f(vmMax), AbsMinV: f(-0.3), AbsMaxV: f(vmAbs)},
				{Name: "VCC", PinAliases: logicAliases, Required: true, RecommendedMinV: f(logicMin), RecommendedMaxV: f(logicMax), AbsMinV: f(-0.3), AbsMaxV: f(logicMax + 0.5), TypicalCurrentA: f(0.005)},
			},
			Grounds: standardGrounds([]string{"GND", "PGND", "AGND"}),
			Logic:   logic33(logicMax+0.5, true),
			MotorDriver: MotorDriverContract{
				VMSupplyName:             "VM",
				LogicSupplyName:          "VCC",
				MotorOutputPins:          outAliases,
				RecommendedVMMinV:        f(vmMin),
				RecommendedVMMaxV:        f(vmMax),
				AbsVMMaxV:                f(vmAbs),
				ContinuousOutputCurrentA: f(continuous),
				PeakOutputCurrentA:       f(peak),
				HasThermalShutdown:       b(true),
			},
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func driverDRV8833() PartContract {
	return motorDriverCommon("DRV8833", []string{"DRV8833PWP", "DRV8833PWPR"}, "Texas Instruments", "https://www.ti.com/lit/ds/symlink/drv8833.pdf", 2.7, 10.8, 11.8, 2.7, 5.5, 1.5, 2.0, []string{"VM", "VMOTOR", "VMOT"}, []string{"VCC", "VCP", "nSLEEP"}, []string{"AOUT1", "AOUT2", "BOUT1", "BOUT2", "OUT1", "OUT2", "OUT3", "OUT4"})
}

func driverDRV8871() PartContract {
	return motorDriverCommon("DRV8871", []string{"DRV8871DDAR", "DRV8871DDA"}, "Texas Instruments", "https://www.ti.com/lit/ds/symlink/drv8871.pdf", 6.5, 45.0, 50.0, 3.0, 5.5, 3.6, 3.6, []string{"VM", "VMOTOR", "VMOT"}, []string{"VCC", "IN1", "IN2"}, []string{"OUT1", "OUT2"})
}

func driverTB6612FNG() PartContract {
	return motorDriverCommon("TB6612FNG", []string{"TB6612FNGC8", "TB6612"}, "Toshiba", "https://toshiba.semicon-storage.com/info/docget.jsp?did=10660&prodName=TB6612FNG", 2.5, 13.5, 15.0, 2.7, 5.5, 1.2, 3.2, []string{"VM", "VMOT", "VMOTOR"}, []string{"VCC", "VCC_IO"}, []string{"AO1", "AO2", "BO1", "BO2", "AOUT1", "AOUT2", "BOUT1", "BOUT2"})
}

func driverL298N() PartContract {
	part := motorDriverCommon("L298N", []string{"L298", "L298HN", "L298 Module", "L298N Module"}, "STMicroelectronics", "https://www.st.com/resource/en/datasheet/l298.pdf", 5.0, 46.0, 50.0, 4.5, 7.0, 1.0, 2.0, []string{"VS", "VSS_MOTOR", "VM", "12V", "VIN"}, []string{"VSS", "VCC", "5V"}, []string{"OUT1", "OUT2", "OUT3", "OUT4"})
	part.PowerContract.Confidence.Level = ConfidenceMedium
	return part
}

func shifterPCA9306() PartContract {
	return PartContract{
		MPN:          "PCA9306",
		Aliases:      []string{"PCA9306DC", "PCA9306DCTR"},
		Category:     CategoryLevelShifter,
		Manufacturer: "Texas Instruments",
		DatasheetURL: "https://www.ti.com/lit/ds/symlink/pca9306.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "VREF1", PinAliases: []string{"VREF1", "VCCA", "LV"}, Required: true, RecommendedMinV: f(1.2), RecommendedMaxV: f(3.3), AbsMinV: f(-0.5), AbsMaxV: f(6.0)},
				{Name: "VREF2", PinAliases: []string{"VREF2", "VCCB", "HV"}, Required: true, RecommendedMinV: f(1.8), RecommendedMaxV: f(5.5), AbsMinV: f(-0.5), AbsMaxV: f(6.0)},
			},
			Grounds: standardGrounds([]string{"GND"}),
			Logic: LogicContract{
				DefaultIODomain:   "VREF1/VREF2",
				IOAbsMinV:         f(-0.5),
				IOAbsMaxV:         f(6.0),
				IORecommendedMinV: f(0),
				IORecommendedMaxV: f(5.5),
				FiveVTolerant:     b(true),
			},
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func shifterTXS0108E() PartContract {
	return PartContract{
		MPN:          "TXS0108E",
		Aliases:      []string{"TXS0108EPWR", "TXS0108ERGYR", "TXS0108"},
		Category:     CategoryLevelShifter,
		Manufacturer: "Texas Instruments",
		DatasheetURL: "https://www.ti.com/lit/ds/symlink/txs0108e.pdf",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "VCCA", PinAliases: []string{"VCCA", "A_VCC"}, Required: true, RecommendedMinV: f(1.2), RecommendedMaxV: f(3.6), AbsMinV: f(-0.5), AbsMaxV: f(4.6)},
				{Name: "VCCB", PinAliases: []string{"VCCB", "B_VCC"}, Required: true, RecommendedMinV: f(1.65), RecommendedMaxV: f(5.5), AbsMinV: f(-0.5), AbsMaxV: f(6.5)},
			},
			Grounds: standardGrounds([]string{"GND"}),
			Logic: LogicContract{
				DefaultIODomain:   "VCCA/VCCB",
				IOAbsMinV:         f(-0.5),
				IOAbsMaxV:         f(6.5),
				IORecommendedMinV: f(0),
				IORecommendedMaxV: f(5.5),
				FiveVTolerant:     b(true),
			},
			Confidence: confidence(ConfidenceHigh),
		},
	}
}

func shifterBSS138Module() PartContract {
	return PartContract{
		MPN:          "BSS138 logic-level shifter module",
		Aliases:      []string{"BSS138 Level Shifter", "BSS138 4-channel level shifter", "BSS138 module"},
		Category:     CategoryLevelShifter,
		Manufacturer: "generic",
		PowerContract: PowerContract{
			Supplies: []SupplyContract{
				{Name: "LV", PinAliases: []string{"LV", "VCCA", "3V3"}, Required: true, RecommendedMinV: f(1.8), RecommendedMaxV: f(3.3), AbsMinV: f(-0.3), AbsMaxV: f(4.0)},
				{Name: "HV", PinAliases: []string{"HV", "VCCB", "5V"}, Required: true, RecommendedMinV: f(3.3), RecommendedMaxV: f(5.0), AbsMinV: f(-0.3), AbsMaxV: f(5.5)},
			},
			Grounds: standardGrounds([]string{"GND"}),
			Logic: LogicContract{
				DefaultIODomain:   "LV/HV",
				IOAbsMinV:         f(-0.3),
				IOAbsMaxV:         f(5.5),
				IORecommendedMinV: f(0),
				IORecommendedMaxV: f(5.0),
				FiveVTolerant:     b(true),
			},
			Confidence: ConfidenceContract{Source: ContractSourceCurated, Level: ConfidenceMedium, Notes: "Generic MOSFET level-shifter module profile."},
		},
	}
}
