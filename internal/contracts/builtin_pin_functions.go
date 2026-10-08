package contracts

// builtInPinFunctions returns cited pin functions for one built-in MPN.
// A part with no entry has no pin functions. Voltage contracts are unchanged.
func builtInPinFunctions(mpn string) []PinFunction {
	return clonePinFunctions(builtInPinFunctionCatalog()[mpn])
}

// builtInPinFunctionCatalog holds pin functions copied from datasheets read for this catalog.
// An empty entry means the function was omitted because it could not be cited.
func builtInPinFunctionCatalog() map[string][]PinFunction {
	esp32Pins := Citation{
		Datasheet: "ESP32-WROOM-32 Datasheet",
		Revision:  "3.8",
		Table:     "2 Pin Definitions",
	}
	esp32I2C := esp32Pins
	esp32I2C.Section = "4.2.4 I2C Interface"

	rpPower := Citation{
		Datasheet: "RP2040 Datasheet",
		Revision:  "3184e62-clean",
		Table:     "621 Power supply pins",
		Section:   "5.5.2.2",
	}
	rpGPIO := Citation{
		Datasheet: "RP2040 Datasheet",
		Revision:  "3184e62-clean",
		Table:     "2 GPIO Bank 0 Functions; 615 GPIO pins",
		Section:   "1.4.3",
	}
	ap2114 := Citation{
		Datasheet: "AP2114 1A Low Noise CMOS LDO Regulator with Enable",
		Revision:  "2.2",
		Table:     "Pin Descriptions",
	}
	bno055 := Citation{
		Datasheet: "BNO055 Intelligent 9-axis absolute orientation sensor",
		Revision:  "1.8",
		Table:     "5-1 Pin description",
		Section:   "5.1 Pin-out",
	}
	mpu6050 := Citation{
		Datasheet: "MPU-6000 and MPU-6050 Product Specification",
		Revision:  "3.4",
		Section:   "7.1 Pin Out and Signal Description",
	}
	stm32 := Citation{
		Datasheet: "STM32F103x8 STM32F103xB Datasheet",
		Revision:  "17",
		Table:     "5 Medium-density STM32F103xx pin definitions, LQFP48 column",
		Section:   "Pinouts and pin description",
	}

	return map[string][]PinFunction{
		"ESP32-WROOM-32": append([]PinFunction{
			{Name: "GND", Number: "1", Kind: PinFunctionGround, Citation: esp32Pins},
			{Name: "3V3", Number: "2", Kind: PinFunctionPower, Citation: esp32Pins},
			{Name: "GND", Number: "15", Kind: PinFunctionGround, Citation: esp32Pins},
			{Name: "GND", Number: "38", Kind: PinFunctionGround, Citation: esp32Pins},
		}, esp32GPIOCandidates(esp32I2C)...),
		"RP2040": append(rp2040PowerPins(rpPower), rp2040GPIOCandidates(rpGPIO)...),
		"AP2114H-3.3": {
			{Name: "VIN", Kind: PinFunctionPower, Signal: "VIN", Citation: ap2114},
			{Name: "VOUT", Kind: PinFunctionPower, Signal: "VOUT", Citation: ap2114},
			{Name: "GND", Kind: PinFunctionGround, Citation: ap2114},
		},
		"BNO055": {
			{Name: "VDD", Number: "3", Kind: PinFunctionPower, Citation: bno055},
			{Name: "VDDIO", Number: "28", Kind: PinFunctionPower, Citation: bno055},
			{Name: "GND", Number: "2", Kind: PinFunctionGround, Citation: bno055},
			{Name: "GNDIO", Number: "25", Kind: PinFunctionGround, Citation: bno055},
			{Name: "SDA", Number: "20", Kind: PinFunctionBus, Signal: "SDA", Citation: bno055},
			{Name: "COM0", Number: "20", Kind: PinFunctionBus, Signal: "SDA", Citation: bno055},
			{Name: "SCL", Number: "19", Kind: PinFunctionBus, Signal: "SCL", Citation: bno055},
			{Name: "COM1", Number: "19", Kind: PinFunctionBus, Signal: "SCL", Citation: bno055},
		},
		"MPU-6050": {
			{Name: "VDD", Number: "13", Kind: PinFunctionPower, Citation: mpu6050},
			{Name: "VLOGIC", Number: "8", Kind: PinFunctionPower, Citation: mpu6050},
			{Name: "GND", Number: "18", Kind: PinFunctionGround, Citation: mpu6050},
			{Name: "SDA", Number: "24", Kind: PinFunctionBus, Signal: "SDA", Citation: mpu6050},
			{Name: "SCL", Number: "23", Kind: PinFunctionBus, Signal: "SCL", Citation: mpu6050},
		},
		"STM32F103C8T6": append(stm32PowerPins(stm32), stm32I2CCandidates(stm32)...),
	}
}

func stm32PowerPins(citation Citation) []PinFunction {
	type named struct {
		name   string
		number string
		kind   PinFunctionKind
	}
	pins := []named{
		{"VBAT", "1", PinFunctionPower},
		{"VSSA", "8", PinFunctionGround},
		{"VDDA", "9", PinFunctionPower},
		{"VSS_1", "23", PinFunctionGround},
		{"VDD_1", "24", PinFunctionPower},
		{"VSS_2", "35", PinFunctionGround},
		{"VDD_2", "36", PinFunctionPower},
		{"VSS_3", "47", PinFunctionGround},
		{"VDD_3", "48", PinFunctionPower},
	}
	out := make([]PinFunction, 0, len(pins))
	for _, pin := range pins {
		out = append(out, PinFunction{
			Name:     pin.name,
			Number:   pin.number,
			Kind:     pin.kind,
			Citation: citation,
		})
	}
	return out
}

// stm32I2CCandidates lists LQFP48 pins whose Table 5 alternate function is I2C.
// The signal is unset because those pins are pinmux, not a dedicated SDA or SCL pin.
func stm32I2CCandidates(citation Citation) []PinFunction {
	pins := []struct {
		name   string
		number string
	}{
		{"PB5", "41"},
		{"PB6", "42"},
		{"PB7", "43"},
		{"PB8", "45"},
		{"PB9", "46"},
		{"PB10", "21"},
		{"PB11", "22"},
		{"PB12", "25"},
	}
	out := make([]PinFunction, 0, len(pins))
	for _, pin := range pins {
		out = append(out, PinFunction{
			Name:     pin.name,
			Number:   pin.number,
			Kind:     PinFunctionGPIOCandidate,
			Citation: citation,
		})
	}
	return out
}

// esp32GPIOCandidates lists module pins Table 2 marks as I/O, excluding the
// SPI-flash pins the same table says are not for other uses. Input-only pins
// are not included. None of these is a dedicated SDA or SCL pin.
func esp32GPIOCandidates(citation Citation) []PinFunction {
	pins := []struct {
		name   string
		gpio   string
		number string
	}{
		{"IO32", "GPIO32", "8"},
		{"IO33", "GPIO33", "9"},
		{"IO25", "GPIO25", "10"},
		{"IO26", "GPIO26", "11"},
		{"IO27", "GPIO27", "12"},
		{"IO14", "GPIO14", "13"},
		{"IO12", "GPIO12", "14"},
		{"IO13", "GPIO13", "16"},
		{"IO15", "GPIO15", "23"},
		{"IO2", "GPIO2", "24"},
		{"IO0", "GPIO0", "25"},
		{"IO4", "GPIO4", "26"},
		{"IO16", "GPIO16", "27"},
		{"IO17", "GPIO17", "28"},
		{"IO5", "GPIO5", "29"},
		{"IO18", "GPIO18", "30"},
		{"IO19", "GPIO19", "31"},
		{"IO21", "GPIO21", "33"},
		{"RXD0", "GPIO3", "34"},
		{"TXD0", "GPIO1", "35"},
		{"IO22", "GPIO22", "36"},
		{"IO23", "GPIO23", "37"},
	}
	out := make([]PinFunction, 0, len(pins)*2)
	for _, pin := range pins {
		out = append(out,
			PinFunction{Name: pin.name, Number: pin.number, Kind: PinFunctionGPIOCandidate, Citation: citation},
			PinFunction{Name: pin.gpio, Number: pin.number, Kind: PinFunctionGPIOCandidate, Citation: citation},
		)
	}
	return out
}

func rp2040PowerPins(citation Citation) []PinFunction {
	type group struct {
		name   string
		kind   PinFunctionKind
		signal string
		nums   []string
	}
	groups := []group{
		{name: "IOVDD", kind: PinFunctionPower, nums: []string{"1", "10", "22", "33", "42", "49"}},
		{name: "DVDD", kind: PinFunctionPower, nums: []string{"23", "50"}},
		{name: "VREG_VIN", kind: PinFunctionPower, signal: "VIN", nums: []string{"44"}},
		{name: "VREG_VOUT", kind: PinFunctionPower, signal: "VOUT", nums: []string{"45"}},
		{name: "USB_VDD", kind: PinFunctionPower, nums: []string{"48"}},
		{name: "ADC_AVDD", kind: PinFunctionPower, nums: []string{"43"}},
		{name: "GND", kind: PinFunctionGround, nums: []string{"57"}},
	}
	var out []PinFunction
	for _, group := range groups {
		for _, number := range group.nums {
			out = append(out, PinFunction{
				Name:     group.name,
				Number:   number,
				Kind:     group.kind,
				Signal:   group.signal,
				Citation: citation,
			})
		}
	}
	return out
}

// rp2040GPIOCandidates lists GPIO0-GPIO29. Table 2 gives each of them an I2C
// function through the GPIO mux. The signal is left unset so none is the SDA pin.
func rp2040GPIOCandidates(citation Citation) []PinFunction {
	numbers := map[int]string{
		0: "2", 1: "3", 2: "4", 3: "5", 4: "6", 5: "7", 6: "8", 7: "9",
		8: "11", 9: "12", 10: "13", 11: "14", 12: "15", 13: "16", 14: "17", 15: "18",
		16: "27", 17: "28", 18: "29", 19: "30", 20: "31", 21: "32", 22: "34", 23: "35",
		24: "36", 25: "37", 26: "38", 27: "39", 28: "40", 29: "41",
	}
	out := make([]PinFunction, 0, 30)
	for gpio := 0; gpio <= 29; gpio++ {
		out = append(out, PinFunction{
			Name:     "GPIO" + itoa(gpio),
			Number:   numbers[gpio],
			Kind:     PinFunctionGPIOCandidate,
			Citation: citation,
		})
	}
	return out
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
