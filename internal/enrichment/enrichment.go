package enrichment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/badimirzai/architon-cli/internal/contracts"
	"github.com/badimirzai/architon-cli/internal/ir"
	"github.com/badimirzai/architon-cli/internal/meta"
)

// ContractSource is one independent source of contract data.
// Examples today are meta.yaml and inferred net voltages; future sources can be
// a parts database, datasheet lookup, manual UI input, or native Architon data.
type ContractSource interface {
	Name() string
	Enrich(design *ir.DesignIR) (*contracts.ContractIR, error)
}

// ContractEnricher combines multiple contract sources into one ContractIR.
// The rule engine consumes only this merged ContractIR plus DesignIR.
type ContractEnricher struct {
	Sources []ContractSource
}

func (e ContractEnricher) Enrich(design *ir.DesignIR) (*contracts.ContractIR, error) {
	out := contracts.NewContractIR()
	for _, source := range e.Sources {
		if source == nil {
			continue
		}
		partial, err := source.Enrich(design)
		if err != nil {
			return nil, fmt.Errorf("%s contract enrichment: %w", source.Name(), err)
		}
		out.Merge(partial)
	}

	// Missing contract data is reported after all sources have contributed, so a
	// later source can fill gaps left by an earlier one before warnings are made.
	out.MissingContractData = append(out.MissingContractData, missingContractData(design, out)...)
	sortMissing(out.MissingContractData)
	return out, nil
}

// NetVoltage is normalized voltage evidence for a net before it is attached as
// a contract. The source records where that evidence came from.
type NetVoltage struct {
	Net     string
	Voltage float64
	Source  string
}

// NetVoltageSource turns inferred or propagated rail voltages into NetContract
// entries. It does not infer pin roles; it only says "this net is nominally X V".
type NetVoltageSource struct {
	name     string
	voltages []NetVoltage
}

func NewNetVoltageSource(name string, voltages []NetVoltage) NetVoltageSource {
	if strings.TrimSpace(name) == "" {
		name = "net-voltage"
	}
	return NetVoltageSource{name: name, voltages: append([]NetVoltage(nil), voltages...)}
}

func (s NetVoltageSource) Name() string {
	return s.name
}

func (s NetVoltageSource) Enrich(_ *ir.DesignIR) (*contracts.ContractIR, error) {
	out := contracts.NewContractIR()

	// Copy and sort input evidence so reports and tests stay deterministic even
	// when map iteration upstream produced the voltage list.
	voltages := append([]NetVoltage(nil), s.voltages...)
	sort.Slice(voltages, func(i, j int) bool {
		if voltages[i].Net != voltages[j].Net {
			return voltages[i].Net < voltages[j].Net
		}
		return voltages[i].Source < voltages[j].Source
	})
	for _, voltage := range voltages {
		netName := strings.TrimSpace(voltage.Net)
		if netName == "" {
			continue
		}
		v := voltage.Voltage
		source := strings.TrimSpace(voltage.Source)
		if source == "" {
			source = s.name
		}
		out.PutNet(netName, contracts.NetContract{
			Net:            netName,
			VoltageNominal: &v,
			Source:         source,
		})
	}
	return out, nil
}

// MetaYAMLSource adapts the current meta.yaml schema into ContractIR.
// meta.yaml is intentionally only one source of contracts, not a rule input.
type MetaYAMLSource struct {
	meta *meta.Meta
}

func NewMetaYAMLSource(m *meta.Meta) MetaYAMLSource {
	return MetaYAMLSource{meta: m}
}

func (s MetaYAMLSource) Name() string {
	return "meta.yaml"
}

func (s MetaYAMLSource) Enrich(design *ir.DesignIR) (*contracts.ContractIR, error) {
	out := contracts.NewContractIR()
	if s.meta == nil {
		return out, nil
	}

	// Sources describe known rail voltages directly on nets.
	for _, source := range s.meta.Sources {
		netName := strings.TrimSpace(source.Net)
		if netName == "" || source.Voltage == 0 {
			continue
		}
		voltage := source.Voltage
		out.PutNet(netName, contracts.NetContract{
			Net:            netName,
			VoltageNominal: &voltage,
			Source:         s.Name(),
		})
	}

	// Regulators contribute a power-output contract on the output pin. If the
	// output pin is connected in DesignIR, the output net also gets voltage data.
	for _, regulator := range s.meta.Regulators {
		ref := strings.TrimSpace(regulator.Ref)
		outPin := strings.TrimSpace(regulator.OutPin)
		if ref == "" || outPin == "" {
			continue
		}
		outVoltage := regulator.OutVoltage
		out.PutPin(ref, outPin, contracts.PinContract{
			Role:           contracts.RoleRegulatorOut,
			VoltageNominal: &outVoltage,
			VoltageMax:     &outVoltage,
			Direction:      contracts.DirectionOutput,
			Source:         s.Name(),
		})
		if design == nil {
			continue
		}
		if outNet, ok := netForRefPin(design, ref, outPin); ok {
			out.PutNet(outNet, contracts.NetContract{
				Net:            outNet,
				VoltageNominal: &outVoltage,
				Source:         "regulator:" + ref,
			})
		}
	}

	// Legacy component max_voltage metadata becomes a power-input voltage limit
	// for each connected pin on that component.
	for _, component := range s.meta.Components {
		ref := strings.TrimSpace(component.Ref)
		if ref == "" || component.MaxVoltage == 0 {
			continue
		}
		maxVoltage := component.MaxVoltage
		componentContract := out.EnsureComponent(ref)
		componentContract.VoltageMax = &maxVoltage
		componentContract.Source = s.Name()
		out.PutComponent(componentContract)

		for _, conn := range connectionsForRef(design, ref) {
			out.PutPin(ref, conn.Pin, contracts.PinContract{
				Role:       contracts.RolePowerIn,
				VoltageMax: &maxVoltage,
				Direction:  contracts.DirectionInput,
				Source:     s.Name(),
			})
		}
	}

	return out, nil
}

type pinConnection struct {
	Net string
	Pin string
}

// connectionsForRef returns all DesignIR pin connections for a component ref.
// It is local to enrichment so rules do not need topology or file parsing code.
func connectionsForRef(design *ir.DesignIR, ref string) []pinConnection {
	if design == nil {
		return nil
	}
	ref = strings.TrimSpace(ref)
	out := make([]pinConnection, 0)
	for _, net := range design.Nets {
		for _, pin := range net.Pins {
			if pin.Ref == ref {
				out = append(out, pinConnection{Net: net.Name, Pin: pin.Pin})
			}
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

// netForRefPin maps a component pin back to its connected net when DesignIR has
// connectivity. It is used to attach regulator output voltage to the net.
func netForRefPin(design *ir.DesignIR, ref string, pin string) (string, bool) {
	ref = strings.TrimSpace(ref)
	pin = strings.TrimSpace(pin)
	if design == nil || ref == "" || pin == "" {
		return "", false
	}
	for _, net := range design.Nets {
		for _, pinRef := range net.Pins {
			if pinRef.Ref == ref && pinRef.Pin == pin {
				return net.Name, true
			}
		}
	}
	return "", false
}

// missingContractData records places where Architon knows a net voltage but has
// no contract for a connected pin. These are warnings, not violations, because
// missing data should not create false confidence.
func missingContractData(design *ir.DesignIR, contractIR *contracts.ContractIR) []contracts.MissingContractData {
	if design == nil || contractIR == nil {
		return nil
	}
	missing := make([]contracts.MissingContractData, 0)
	seen := map[string]struct{}{}
	add := func(item contracts.MissingContractData) {
		key := item.Kind + "\x00" + item.Ref + "\x00" + item.Pin + "\x00" + item.Net
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		missing = append(missing, item)
	}
	for _, net := range design.Nets {
		netContract, hasNetContract := contractIR.Net(net.Name)
		if !hasNetContract || netContract.VoltageNominal == nil {
			continue
		}
		for _, pinRef := range net.Pins {
			if _, ok := contractIR.Pin(pinRef.Ref, pinRef.Pin); ok {
				continue
			}
			add(contracts.MissingContractData{
				Kind: "pin_contract",
				Ref:  pinRef.Ref,
				Pin:  pinRef.Pin,
				Net:  net.Name,
				Message: fmt.Sprintf(
					"Missing contract data for %s pin %s on powered net %s",
					pinRef.Ref,
					pinRef.Pin,
					net.Name,
				),
			})
		}
	}
	return missing
}

// sortMissing keeps warning output stable across runs.
func sortMissing(items []contracts.MissingContractData) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Net != items[j].Net {
			return items[i].Net < items[j].Net
		}
		if items[i].Ref != items[j].Ref {
			return items[i].Ref < items[j].Ref
		}
		if items[i].Pin != items[j].Pin {
			return items[i].Pin < items[j].Pin
		}
		return items[i].Kind < items[j].Kind
	})
}
