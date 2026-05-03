package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"

	partlib "github.com/badimirzai/architon-cli/internal/parts"
	"github.com/spf13/cobra"
)

var partsCmd = newPartsCmd()

func init() {
	rootCmd.AddCommand(partsCmd)
}

func newPartsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "parts",
		Short: "Inspect built-in deterministic power contracts",
	}
	cmd.AddCommand(newPartsListCmd())
	cmd.AddCommand(newPartsShowCmd())
	return cmd
}

func newPartsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in power contracts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := partlib.BuiltInPowerContracts()
			fmt.Fprintf(cmd.OutOrStdout(), "Built-in power contracts: %d\n", len(lib))
			for _, part := range lib {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s [%s]", part.MPN, part.Category)
				if part.Manufacturer != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " %s", part.Manufacturer)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
}

func newPartsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <mpn>",
		Short: "Show one built-in power contract",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			lib := partlib.BuiltInPowerContracts()
			match := partlib.MatchPart(args[0], args[0], nil, lib)
			if !match.Matched {
				return userError(fmt.Errorf("part %q not found in built-in contracts", args[0]))
			}
			printPartContract(cmd.OutOrStdout(), match.Part)
			return nil
		},
	}
}

func printPartContract(w io.Writer, part partlib.PartContract) {
	fmt.Fprintf(w, "%s\n", part.MPN)
	fmt.Fprintf(w, "Category: %s\n", part.Category)
	if part.Manufacturer != "" {
		fmt.Fprintf(w, "Manufacturer: %s\n", part.Manufacturer)
	}
	if part.Package != "" {
		fmt.Fprintf(w, "Package: %s\n", part.Package)
	}
	if part.DatasheetURL != "" {
		fmt.Fprintf(w, "Datasheet: %s\n", part.DatasheetURL)
	}
	fmt.Fprintf(w, "Matched aliases: %s\n", partsFormatList(part.Aliases))

	if len(part.PowerContract.Supplies) > 0 {
		fmt.Fprintln(w, "Supplies:")
		for _, supply := range part.PowerContract.Supplies {
			fmt.Fprintf(w, "- %s pins=%s", supply.Name, partsFormatList(supply.PinAliases))
			if supply.NominalV != nil {
				fmt.Fprintf(w, " nominal=%.2fV", *supply.NominalV)
			}
			if supply.RecommendedMinV != nil || supply.RecommendedMaxV != nil {
				fmt.Fprintf(w, " recommended=%s..%sV", partsFloatOrStar(supply.RecommendedMinV), partsFloatOrStar(supply.RecommendedMaxV))
			}
			if supply.AbsMaxV != nil {
				fmt.Fprintf(w, " abs_max=%.2fV", *supply.AbsMaxV)
			}
			if supply.TypicalCurrentA != nil {
				fmt.Fprintf(w, " typical=%.3fA", *supply.TypicalCurrentA)
			}
			fmt.Fprintln(w)
		}
	}

	if len(part.PowerContract.Grounds) > 0 {
		fmt.Fprintln(w, "Grounds:")
		for _, ground := range part.PowerContract.Grounds {
			fmt.Fprintf(w, "- %s pins=%s\n", ground.Name, partsFormatList(ground.PinAliases))
		}
	}

	printPartsLogic(w, part.PowerContract.Logic)
	printPartsPowerOutputs(w, part.PowerContract.PowerOutputs)
	printPartsMotorDriver(w, part.PowerContract.MotorDriver)
	printPartsProtection(w, part.PowerContract.Protection)

	conf := part.PowerContract.Confidence
	if conf.Source != "" || conf.Level != "" {
		fmt.Fprintf(w, "Confidence: source=%s level=%s", conf.Source, conf.Level)
		if conf.Notes != "" {
			fmt.Fprintf(w, " notes=%s", conf.Notes)
		}
		fmt.Fprintln(w)
	}
}

func printPartsLogic(w io.Writer, logic partlib.LogicContract) {
	if logic.IOAbsMaxV == nil && logic.VIHMinV == nil && logic.VILMaxV == nil && logic.FiveVTolerant == nil {
		return
	}
	fmt.Fprintln(w, "Logic:")
	if logic.DefaultIODomain != "" {
		fmt.Fprintf(w, "- default_io_domain: %s\n", logic.DefaultIODomain)
	}
	if logic.IOAbsMaxV != nil {
		fmt.Fprintf(w, "- io_abs_max_v: %.2f\n", *logic.IOAbsMaxV)
	}
	if logic.IORecommendedMinV != nil || logic.IORecommendedMaxV != nil {
		fmt.Fprintf(w, "- io_recommended: %s..%sV\n", partsFloatOrStar(logic.IORecommendedMinV), partsFloatOrStar(logic.IORecommendedMaxV))
	}
	if logic.FiveVTolerant != nil {
		fmt.Fprintf(w, "- five_v_tolerant: %t\n", *logic.FiveVTolerant)
	}
	if logic.VIHMinV != nil {
		fmt.Fprintf(w, "- vih_min_v: %.2f\n", *logic.VIHMinV)
	}
	if logic.VILMaxV != nil {
		fmt.Fprintf(w, "- vil_max_v: %.2f\n", *logic.VILMaxV)
	}
	if logic.VOHMinV != nil {
		fmt.Fprintf(w, "- voh_min_v: %.2f\n", *logic.VOHMinV)
	}
	if logic.VOLMaxV != nil {
		fmt.Fprintf(w, "- vol_max_v: %.2f\n", *logic.VOLMaxV)
	}
}

func printPartsPowerOutputs(w io.Writer, outputs []partlib.PowerOutputContract) {
	if len(outputs) == 0 {
		return
	}
	fmt.Fprintln(w, "Power outputs:")
	for _, output := range outputs {
		fmt.Fprintf(w, "- %s pins=%s", output.Name, partsFormatList(output.PinAliases))
		if output.OutputNominalV != nil {
			fmt.Fprintf(w, " nominal=%.2fV", *output.OutputNominalV)
		}
		if output.MaxOutputCurrentA != nil {
			fmt.Fprintf(w, " max_current=%.3fA", *output.MaxOutputCurrentA)
		}
		if output.DropoutV != nil {
			fmt.Fprintf(w, " dropout=%.2fV", *output.DropoutV)
		}
		if output.RequiresInputSupply != "" {
			fmt.Fprintf(w, " requires=%s", output.RequiresInputSupply)
		}
		fmt.Fprintln(w)
	}
}

func printPartsMotorDriver(w io.Writer, motor partlib.MotorDriverContract) {
	if motor.AbsVMMaxV == nil && motor.ContinuousOutputCurrentA == nil && motor.PeakOutputCurrentA == nil {
		return
	}
	fmt.Fprintln(w, "Motor driver:")
	if motor.VMSupplyName != "" {
		fmt.Fprintf(w, "- vm_supply_name: %s\n", motor.VMSupplyName)
	}
	if motor.RecommendedVMMinV != nil || motor.RecommendedVMMaxV != nil {
		fmt.Fprintf(w, "- recommended_vm: %s..%sV\n", partsFloatOrStar(motor.RecommendedVMMinV), partsFloatOrStar(motor.RecommendedVMMaxV))
	}
	if motor.AbsVMMaxV != nil {
		fmt.Fprintf(w, "- abs_vm_max_v: %.2f\n", *motor.AbsVMMaxV)
	}
	if motor.ContinuousOutputCurrentA != nil {
		fmt.Fprintf(w, "- continuous_output_current_a: %.3f\n", *motor.ContinuousOutputCurrentA)
	}
	if motor.PeakOutputCurrentA != nil {
		fmt.Fprintf(w, "- peak_output_current_a: %.3f\n", *motor.PeakOutputCurrentA)
	}
	if len(motor.MotorOutputPins) > 0 {
		fmt.Fprintf(w, "- motor_output_pins: %s\n", partsFormatList(motor.MotorOutputPins))
	}
}

func printPartsProtection(w io.Writer, protection partlib.ProtectionContract) {
	if protection.ReversePolarityProtected == nil && protection.OvercurrentProtected == nil && protection.ThermalShutdown == nil && protection.ClampVoltageV == nil && protection.MaxClampCurrentMA == nil {
		return
	}
	fmt.Fprintln(w, "Protection:")
	if protection.ReversePolarityProtected != nil {
		fmt.Fprintf(w, "- reverse_polarity_protected: %t\n", *protection.ReversePolarityProtected)
	}
	if protection.OvercurrentProtected != nil {
		fmt.Fprintf(w, "- overcurrent_protected: %t\n", *protection.OvercurrentProtected)
	}
	if protection.ThermalShutdown != nil {
		fmt.Fprintf(w, "- thermal_shutdown: %t\n", *protection.ThermalShutdown)
	}
	if protection.ClampVoltageV != nil {
		fmt.Fprintf(w, "- clamp_voltage_v: %.2f\n", *protection.ClampVoltageV)
	}
	if protection.MaxClampCurrentMA != nil {
		fmt.Fprintf(w, "- max_clamp_current_ma: %.1f\n", *protection.MaxClampCurrentMA)
	}
}

func partsFormatList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return "none"
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func partsFloatOrStar(value *float64) string {
	if value == nil {
		return "*"
	}
	return fmt.Sprintf("%.2f", *value)
}
