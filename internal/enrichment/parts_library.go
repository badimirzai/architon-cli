package enrichment

import (
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
	partlib "github.com/badimirzai/architon-cli/internal/parts"
)

const partsLibrarySourceName = "parts-library"

// PartsLibrarySource adapts the deterministic built-in power-contract library
// into rule-facing ContractIR.
type PartsLibrarySource struct {
	library []partlib.PartContract
}

func NewPartsLibrarySource(library []partlib.PartContract) PartsLibrarySource {
	if len(library) == 0 {
		library = partlib.BuiltInPowerContracts()
	}
	return PartsLibrarySource{library: append([]partlib.PartContract(nil), library...)}
}

func (s PartsLibrarySource) Name() string {
	return partsLibrarySourceName
}

func (s PartsLibrarySource) Enrich(design *ir.DesignIR) (*contracts.ContractIR, error) {
	out := contracts.NewContractIR()
	if design == nil {
		return out, nil
	}

	parts := append([]ir.Part(nil), design.Parts...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].Ref < parts[j].Ref })
	for _, part := range parts {
		match := partlib.MatchPart(part.Value, part.MPN, part.Fields, s.library)
		out.PartMatches = append(out.PartMatches, contracts.PartMatchSummary{
			Ref:           strings.TrimSpace(part.Ref),
			MPN:           strings.TrimSpace(part.MPN),
			Matched:       match.Matched,
			MatchedMPN:    match.Part.MPN,
			Category:      match.Part.Category,
			Source:        partsLibrarySourceName,
			Reason:        string(match.Kind),
			PowerCritical: isPowerCriticalRef(part.Ref),
		})
		if !match.Matched {
			continue
		}
		applyPartContract(out, design, part, match.Part)
	}
	sort.Slice(out.PartMatches, func(i, j int) bool { return out.PartMatches[i].Ref < out.PartMatches[j].Ref })
	return out, nil
}

func applyPartContract(out *contracts.ContractIR, design *ir.DesignIR, part ir.Part, contract partlib.PartContract) {
	ref := strings.TrimSpace(part.Ref)
	if ref == "" {
		return
	}
	connections := connectionsForRefWithNames(design, ref)
	component := out.EnsureComponent(ref)
	component.MPN = contract.MPN
	component.Category = contract.Category
	component.Source = partsLibrarySourceName
	component.Logic = convertLogicContract(contract.PowerContract.Logic)
	component.MotorDriver = convertMotorDriverContract(contract.PowerContract.MotorDriver)
	component.Protection = convertProtectionContract(contract.PowerContract.Protection)
	component.Thermal = convertThermalContract(contract.PowerContract.Thermal)
	out.PutComponent(component)

	claimedPins := map[string]struct{}{}
	for _, supply := range contract.PowerContract.Supplies {
		for _, conn := range matchingConnections(connections, supply.PinAliases) {
			claimedPins[conn.Pin] = struct{}{}
			isVM := strings.EqualFold(strings.TrimSpace(supply.Name), strings.TrimSpace(contract.PowerContract.MotorDriver.VMSupplyName))
			out.PutPin(ref, conn.Pin, contracts.PinContract{
				Name:                  conn.Name,
				Role:                  contracts.RolePowerIn,
				SupplyName:            supply.Name,
				VoltageNominal:        cloneFloat(supply.NominalV),
				VoltageMin:            cloneFloat(supply.RecommendedMinV),
				VoltageMax:            cloneFloat(supply.AbsMaxV),
				RecommendedVoltageMin: cloneFloat(supply.RecommendedMinV),
				RecommendedVoltageMax: cloneFloat(supply.RecommendedMaxV),
				AbsVoltageMin:         cloneFloat(supply.AbsMinV),
				AbsVoltageMax:         cloneFloat(supply.AbsMaxV),
				CurrentMax:            cloneFloat(supply.MaxCurrentA),
				TypicalCurrent:        cloneFloat(supply.TypicalCurrentA),
				InrushCurrent:         cloneFloat(supply.InrushCurrentA),
				MotorSupply:           isVM,
				Direction:             contracts.DirectionInput,
				Source:                partsLibrarySourceName,
			})
		}
	}

	for _, ground := range contract.PowerContract.Grounds {
		for _, conn := range matchingConnections(connections, ground.PinAliases) {
			claimedPins[conn.Pin] = struct{}{}
			out.PutPin(ref, conn.Pin, contracts.PinContract{
				Name:      conn.Name,
				Role:      contracts.RoleGround,
				Direction: contracts.DirectionPassive,
				Source:    partsLibrarySourceName,
			})
		}
	}

	for _, output := range contract.PowerContract.PowerOutputs {
		for _, conn := range matchingConnections(connections, output.PinAliases) {
			claimedPins[conn.Pin] = struct{}{}
			out.PutPin(ref, conn.Pin, contracts.PinContract{
				Name:                  conn.Name,
				Role:                  contracts.RoleRegulatorOut,
				SupplyName:            output.Name,
				VoltageNominal:        cloneFloat(output.OutputNominalV),
				VoltageMin:            cloneFloat(output.OutputMinV),
				VoltageMax:            cloneFloat(output.OutputMaxV),
				RecommendedVoltageMin: cloneFloat(output.OutputMinV),
				RecommendedVoltageMax: cloneFloat(output.OutputMaxV),
				OutputCurrentMax:      cloneFloat(output.MaxOutputCurrentA),
				CurrentMax:            cloneFloat(output.MaxOutputCurrentA),
				DropoutVoltage:        cloneFloat(output.DropoutV),
				RequiresInputSupply:   output.RequiresInputSupply,
				Direction:             contracts.DirectionOutput,
				Source:                partsLibrarySourceName,
			})
			if output.OutputNominalV != nil {
				if netName, ok := netForRefPin(design, ref, conn.Pin); ok {
					out.PutNet(netName, contracts.NetContract{
						Net:            netName,
						VoltageNominal: cloneFloat(output.OutputNominalV),
						Source:         partsLibrarySourceName,
					})
				}
			}
		}
	}

	for _, conn := range matchingConnections(connections, contract.PowerContract.MotorDriver.MotorOutputPins) {
		claimedPins[conn.Pin] = struct{}{}
		out.PutPin(ref, conn.Pin, contracts.PinContract{
			Name:        conn.Name,
			Role:        contracts.RoleMotorOut,
			MotorOutput: true,
			Direction:   contracts.DirectionOutput,
			Source:      partsLibrarySourceName,
		})
	}

	if hasLogicContract(contract.PowerContract.Logic) {
		for _, conn := range connections {
			if _, claimed := claimedPins[conn.Pin]; claimed {
				continue
			}
			if isGroundNetName(conn.Net) || isLikelyPowerNetName(conn.Net) {
				continue
			}
			logic := contract.PowerContract.Logic
			role := roleForSignalPin(conn)
			openDrain := (*bool)(nil)
			if role == contracts.RoleI2CSDA || role == contracts.RoleI2CSCL {
				openDrain = contracts.Bool(true)
			}
			out.PutPin(ref, conn.Pin, contracts.PinContract{
				Name:                  conn.Name,
				Role:                  role,
				VoltageMax:            cloneFloat(logic.IOAbsMaxV),
				RecommendedVoltageMin: cloneFloat(logic.IORecommendedMinV),
				RecommendedVoltageMax: cloneFloat(logic.IORecommendedMaxV),
				AbsVoltageMin:         cloneFloat(logic.IOAbsMinV),
				AbsVoltageMax:         cloneFloat(logic.IOAbsMaxV),
				VIHMin:                cloneFloat(logic.VIHMinV),
				VILMax:                cloneFloat(logic.VILMaxV),
				VOHMin:                cloneFloat(logic.VOHMinV),
				VOLMax:                cloneFloat(logic.VOLMaxV),
				ClampCurrentMaxMA:     cloneFloat(logic.MaxInjectionCurrentMA),
				Direction:             contracts.DirectionBidirectional,
				OpenDrain:             openDrain,
				Source:                partsLibrarySourceName,
			})
		}
	}
}

type namedPinConnection struct {
	Net  string
	Pin  string
	Name string
}

func connectionsForRefWithNames(design *ir.DesignIR, ref string) []namedPinConnection {
	if design == nil {
		return nil
	}
	names := map[string]string{}
	for _, pin := range design.Pins {
		if pin.Ref == ref && strings.TrimSpace(pin.Pin) != "" && strings.TrimSpace(pin.Name) != "" {
			names[pin.Pin] = pin.Name
		}
	}
	out := make([]namedPinConnection, 0)
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if pin.Ref != ref {
				continue
			}
			out = append(out, namedPinConnection{
				Net:  net.Name,
				Pin:  strings.TrimSpace(pin.Pin),
				Name: names[pin.Pin],
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Net != out[j].Net {
			return out[i].Net < out[j].Net
		}
		return out[i].Pin < out[j].Pin
	})
	return out
}

func matchingConnections(connections []namedPinConnection, aliases []string) []namedPinConnection {
	if len(aliases) == 0 {
		return nil
	}
	aliasSet := map[string]struct{}{}
	for _, alias := range aliases {
		normalized := normalizeElectricalName(alias)
		if normalized != "" {
			aliasSet[normalized] = struct{}{}
		}
	}
	out := make([]namedPinConnection, 0, 1)
	for _, conn := range connections {
		if _, ok := aliasSet[normalizeElectricalName(conn.Pin)]; ok {
			out = append(out, conn)
			continue
		}
		if _, ok := aliasSet[normalizeElectricalName(conn.Name)]; ok {
			out = append(out, conn)
		}
	}
	return out
}

func normalizeElectricalName(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func roleForSignalPin(conn namedPinConnection) contracts.PinRole {
	name := normalizeElectricalName(firstNonEmpty(conn.Name, conn.Pin))
	switch {
	case strings.Contains(name, "SDA"):
		return contracts.RoleI2CSDA
	case strings.Contains(name, "SCL") || strings.Contains(name, "SCK"):
		return contracts.RoleI2CSCL
	case strings.Contains(name, "MISO") || strings.Contains(name, "MOSI") || strings.Contains(name, "SPI"):
		return contracts.RoleSPI
	case strings.Contains(name, "TX") || strings.Contains(name, "RX") || strings.Contains(name, "UART"):
		return contracts.RoleUART
	default:
		return contracts.RoleGPIO
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isPowerCriticalRef(ref string) bool {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	if ref == "" {
		return false
	}
	for _, prefix := range []string{"U", "IC", "REG", "VR", "M", "DRV"} {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	return false
}

func isGroundNetName(net string) bool {
	normalized := normalizeElectricalName(net)
	return normalized == "GND" || normalized == "AGND" || normalized == "DGND" || normalized == "PGND"
}

func isLikelyPowerNetName(net string) bool {
	normalized := normalizeElectricalName(net)
	if normalized == "" {
		return false
	}
	if strings.Contains(normalized, "VCC") || strings.Contains(normalized, "VDD") || strings.Contains(normalized, "VBUS") || strings.Contains(normalized, "VBAT") || strings.Contains(normalized, "VIN") {
		return true
	}
	return strings.HasPrefix(normalized, "PWR") || strings.HasPrefix(normalized, "VMOT") || strings.HasPrefix(normalized, "VMOTOR")
}

func hasLogicContract(logic partlib.LogicContract) bool {
	return logic.IOAbsMaxV != nil || logic.VIHMinV != nil || logic.VILMaxV != nil || logic.VOHMinV != nil || logic.VOLMaxV != nil
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneBool(v *bool) *bool {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func convertLogicContract(in partlib.LogicContract) *contracts.LogicContract {
	if !hasLogicContract(in) && in.FiveVTolerant == nil {
		return nil
	}
	return &contracts.LogicContract{
		DefaultIODomain:       in.DefaultIODomain,
		IOAbsMinV:             cloneFloat(in.IOAbsMinV),
		IOAbsMaxV:             cloneFloat(in.IOAbsMaxV),
		IORecommendedMinV:     cloneFloat(in.IORecommendedMinV),
		IORecommendedMaxV:     cloneFloat(in.IORecommendedMaxV),
		FiveVTolerant:         cloneBool(in.FiveVTolerant),
		FiveVTolerantPins:     append([]string(nil), in.FiveVTolerantPins...),
		NonFiveVTolerantPins:  append([]string(nil), in.NonFiveVTolerantPins...),
		VIHMinV:               cloneFloat(in.VIHMinV),
		VILMaxV:               cloneFloat(in.VILMaxV),
		VOHMinV:               cloneFloat(in.VOHMinV),
		VOLMaxV:               cloneFloat(in.VOLMaxV),
		MaxInjectionCurrentMA: cloneFloat(in.MaxInjectionCurrentMA),
	}
}

func convertMotorDriverContract(in partlib.MotorDriverContract) *contracts.MotorDriverContract {
	if in.AbsVMMaxV == nil && in.RecommendedVMMinV == nil && in.RecommendedVMMaxV == nil && in.ContinuousOutputCurrentA == nil && in.PeakOutputCurrentA == nil {
		return nil
	}
	return &contracts.MotorDriverContract{
		VMSupplyName:             in.VMSupplyName,
		LogicSupplyName:          in.LogicSupplyName,
		MotorOutputPins:          append([]string(nil), in.MotorOutputPins...),
		RecommendedVMMinV:        cloneFloat(in.RecommendedVMMinV),
		RecommendedVMMaxV:        cloneFloat(in.RecommendedVMMaxV),
		AbsVMMaxV:                cloneFloat(in.AbsVMMaxV),
		ContinuousOutputCurrentA: cloneFloat(in.ContinuousOutputCurrentA),
		PeakOutputCurrentA:       cloneFloat(in.PeakOutputCurrentA),
		CurrentLimitA:            cloneFloat(in.CurrentLimitA),
		HasCurrentRegulation:     cloneBool(in.HasCurrentRegulation),
		HasThermalShutdown:       cloneBool(in.HasThermalShutdown),
		HasUVLO:                  cloneBool(in.HasUVLO),
	}
}

func convertProtectionContract(in partlib.ProtectionContract) *contracts.ProtectionContract {
	if in.ReversePolarityProtected == nil && in.OvercurrentProtected == nil && in.ThermalShutdown == nil && in.UVLOThresholdV == nil && in.OVPThresholdV == nil && in.ClampVoltageV == nil && in.MaxClampCurrentMA == nil {
		return nil
	}
	return &contracts.ProtectionContract{
		ReversePolarityProtected: cloneBool(in.ReversePolarityProtected),
		OvercurrentProtected:     cloneBool(in.OvercurrentProtected),
		ThermalShutdown:          cloneBool(in.ThermalShutdown),
		UVLOThresholdV:           cloneFloat(in.UVLOThresholdV),
		OVPThresholdV:            cloneFloat(in.OVPThresholdV),
		ClampVoltageV:            cloneFloat(in.ClampVoltageV),
		MaxClampCurrentMA:        cloneFloat(in.MaxClampCurrentMA),
	}
}

func convertThermalContract(in partlib.ThermalContract) *contracts.ThermalContract {
	if in.MaxJunctionTempC == nil && in.RecommendedAmbientMaxC == nil && in.ThetaJACPerW == nil && in.PowerDissipationW == nil {
		return nil
	}
	return &contracts.ThermalContract{
		MaxJunctionTempC:       cloneFloat(in.MaxJunctionTempC),
		RecommendedAmbientMaxC: cloneFloat(in.RecommendedAmbientMaxC),
		ThetaJACPerW:           cloneFloat(in.ThetaJACPerW),
		PowerDissipationW:      cloneFloat(in.PowerDissipationW),
	}
}
