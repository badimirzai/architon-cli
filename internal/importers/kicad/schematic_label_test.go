package kicad

import (
	"strings"
	"testing"
)

func TestWorldPinMatchesKiCad9Schematic(t *testing.T) {
	// valid_pull_ups.kicad_sch is (version 20250114) (generator "eeschema") (generator_version "9.0").
	// R2 pin 1 is library (0, 3.81) on a symbol at (120.65, 91.44). The wire ends at (120.65, 87.63).
	origin := point{x: mustNM(t, "120.65"), y: mustNM(t, "91.44")}
	got, err := worldPin(origin, 0, false, false, mustNM(t, "0"), mustNM(t, "3.81"))
	if err != nil {
		t.Fatal(err)
	}
	if formatNM(got.x) != "120.65" || formatNM(got.y) != "87.63" {
		t.Fatalf("resistor pin = %s %s", formatNM(got.x), formatNM(got.y))
	}

	// MPU-6050 SDA is library (-17.78, 7.62) on a symbol at (148.59, 113.03).
	// The SDA wire ends at (130.81, 105.41).
	origin = point{x: mustNM(t, "148.59"), y: mustNM(t, "113.03")}
	got, err = worldPin(origin, 0, false, false, mustNM(t, "-17.78"), mustNM(t, "7.62"))
	if err != nil {
		t.Fatal(err)
	}
	if formatNM(got.x) != "130.81" || formatNM(got.y) != "105.41" {
		t.Fatalf("SDA pin = %s %s", formatNM(got.x), formatNM(got.y))
	}

	// Rotation 90 moves the resistor's top pin to the left.
	origin = point{x: mustNM(t, "100"), y: mustNM(t, "100")}
	got, err = worldPin(origin, 90, false, false, mustNM(t, "0"), mustNM(t, "3.81"))
	if err != nil {
		t.Fatal(err)
	}
	if formatNM(got.x) != "96.19" || formatNM(got.y) != "100" {
		t.Fatalf("rotated pin = %s %s", formatNM(got.x), formatNM(got.y))
	}
}

func TestAddNetLabelsUsesKiCad9LabelAtPin(t *testing.T) {
	src := resistorLikeSchematic()
	files := map[string][]byte{"/tmp/board.kicad_sch": []byte(src)}
	changed, err := AddNetLabels(files, []NetLabelEntry{{
		ID:  "sda-U2-U3",
		Net: "I2C_SDA",
		Pins: [2]NetLabelPin{
			{Ref: "U2", Pin: "SDA"},
			{Ref: "U3", Pin: "1"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(changed["/tmp/board.kicad_sch"])
	if !strings.Contains(got, "\t(label \"I2C_SDA\"\n\t\t(at 120.65 87.63 0)\n\t\t(effects\n\t\t\t(font\n\t\t\t\t(size 1.27 1.27)\n\t\t\t)\n\t\t\t(justify left bottom)\n") {
		t.Fatalf("missing KiCad 9 label at the pin:\n%s", got)
	}
	if !strings.Contains(got, "(at 148.59 106.68 0)") {
		t.Fatalf("pin number label missing:\n%s", got)
	}
	if strings.Contains(got, "(wire") || strings.Contains(got, "(no_connect") {
		t.Fatalf("label placement drew a wire or no-connect:\n%s", got)
	}
	if strings.Count(got, "(symbol") != strings.Count(src, "(symbol") {
		t.Fatal("symbol count changed")
	}

	again, err := AddNetLabels(map[string][]byte{"/tmp/board.kicad_sch": []byte(got)}, []NetLabelEntry{{
		ID:  "sda-U2-U3",
		Net: "I2C_SDA",
		Pins: [2]NetLabelPin{
			{Ref: "U2", Pin: "SDA"},
			{Ref: "U3", Pin: "1"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second apply changed %d files", len(again))
	}
}

func TestAddNetLabelsLeavesPinAlreadyOnProposalNet(t *testing.T) {
	src := strings.Replace(resistorLikeSchematic(), "\t(embedded_fonts no)\n", indentKiCad(`>(wire
>>(pts
>>>(xy 120.65 87.63) (xy 100 87.63)
>>)
>>(stroke
>>>(width 0)
>>>(type default)
>>)
>>(uuid "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
>)
>(label "I2C_SDA"
>>(at 100 87.63 0)
>>(effects
>>>(font
>>>>(size 1.27 1.27)
>>>)
>>>(justify left bottom)
>>)
>>(uuid "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
>)
`)+"\t(embedded_fonts no)\n", 1)
	changed, err := AddNetLabels(map[string][]byte{"/tmp/board.kicad_sch": []byte(src)}, []NetLabelEntry{{
		ID:  "sda-U2-U3",
		Net: "/I2C_SDA",
		Pins: [2]NetLabelPin{
			{Ref: "U2", Pin: "SDA"},
			{Ref: "U3", Pin: "SDA"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(changed["/tmp/board.kicad_sch"])
	if strings.Count(got, "(label ") != 2 {
		t.Fatalf("labels = %d\n%s", strings.Count(got, "(label "), got)
	}
	if !strings.Contains(got, "(at 148.59 109.22 0)") {
		t.Fatalf("unconnected pin was not labeled:\n%s", got)
	}
	if strings.Count(got, "(wire") != 1 {
		t.Fatal("wire count changed")
	}
}

func TestAddNetLabelsDifferentNetAndPartialFailure(t *testing.T) {
	src := strings.Replace(resistorLikeSchematic(), "\t(embedded_fonts no)\n", indentKiCad(`>(wire
>>(pts
>>>(xy 120.65 87.63) (xy 100 87.63)
>>)
>>(stroke
>>>(width 0)
>>>(type default)
>>)
>>(uuid "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
>)
>(label "OTHER"
>>(at 100 87.63 0)
>>(effects
>>>(font
>>>>(size 1.27 1.27)
>>>)
>>>(justify left bottom)
>>)
>>(uuid "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
>)
`)+"\t(embedded_fonts no)\n", 1)
	files := map[string][]byte{"/tmp/board.kicad_sch": []byte(src)}
	changed, err := AddNetLabels(files, []NetLabelEntry{{
		ID:  "sda-U2-U3",
		Net: "I2C_SDA",
		Pins: [2]NetLabelPin{
			{Ref: "U2", Pin: "SDA"},
			{Ref: "U3", Pin: "SDA"},
		},
	}})
	if err == nil || !strings.Contains(err.Error(), "pin U2 SDA is already on net OTHER") {
		t.Fatalf("err = %v", err)
	}
	if changed != nil {
		t.Fatal("different net returned schematic bytes")
	}
	if string(files["/tmp/board.kicad_sch"]) != src {
		t.Fatal("input bytes changed")
	}

	changed, err = AddNetLabels(map[string][]byte{"/tmp/board.kicad_sch": []byte(resistorLikeSchematic())}, []NetLabelEntry{
		{
			ID:   "sda-U2-U3",
			Net:  "I2C_SDA",
			Pins: [2]NetLabelPin{{Ref: "U2", Pin: "SDA"}, {Ref: "U3", Pin: "SDA"}},
		},
		{
			ID:   "scl-U2-U9",
			Net:  "I2C_SCL",
			Pins: [2]NetLabelPin{{Ref: "U9", Pin: "SCL"}, {Ref: "U2", Pin: "SDA"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "pin U9 SCL not found") {
		t.Fatalf("err = %v", err)
	}
	if changed != nil {
		t.Fatal("partial apply returned bytes")
	}
}

func TestAddNetLabelsMatchesSlashPinName(t *testing.T) {
	src := resistorLikeSchematic()
	changed, err := AddNetLabels(map[string][]byte{"/tmp/board.kicad_sch": []byte(src)}, []NetLabelEntry{{
		ID:  "sda-U2-U3",
		Net: "I2C_SDA",
		Pins: [2]NetLabelPin{
			{Ref: "U2", Pin: "IO1"},
			{Ref: "U3", Pin: "SDA"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := string(changed["/tmp/board.kicad_sch"])
	if !strings.Contains(got, "(at 120.65 85.09 0)") {
		t.Fatalf("slash name did not land on TXD0/IO1:\n%s", got)
	}
}

func resistorLikeSchematic() string {
	return indentKiCad(`(kicad_sch
>(version 20250114)
>(generator "eeschema")
>(generator_version "9.0")
>(uuid "11111111-1111-4111-8111-111111111111")
>(paper "A4")
>(lib_symbols
>>(symbol "Sensor:Part"
>>>(symbol "Part_1_1"
>>>>(pin bidirectional line
>>>>>(at 0 3.81 270)
>>>>>(length 2.54)
>>>>>(name "SDA"
>>>>>>(effects
>>>>>>>(font
>>>>>>>(size 1.27 1.27)
>>>>>>)
>>>>>)
>>>>)
>>>>>(number "24"
>>>>>>(effects
>>>>>>>(font
>>>>>>>(size 1.27 1.27)
>>>>>>)
>>>>>)
>>>>)
>>>>)
>>>>(pin bidirectional line
>>>>>(at 0 6.35 270)
>>>>>(length 2.54)
>>>>>(name "TXD0/IO1"
>>>>>>(effects
>>>>>>>(font
>>>>>>>(size 1.27 1.27)
>>>>>>)
>>>>>)
>>>>)
>>>>>(number "1"
>>>>>>(effects
>>>>>>>(font
>>>>>>>(size 1.27 1.27)
>>>>>>)
>>>>>)
>>>>)
>>>>)
>>>)
>>)
>)
>(symbol
>>(lib_id "Sensor:Part")
>>(at 120.65 91.44 0)
>>(unit 1)
>>(exclude_from_sim no)
>>(in_bom yes)
>>(on_board yes)
>>(dnp no)
>>(uuid "22222222-2222-4222-8222-222222222222")
>>(property "Reference" "U2"
>>>(at 120.65 80 0)
>>>(effects
>>>>(font
>>>>>(size 1.27 1.27)
>>>>)
>>>)
>>)
>>(pin "24"
>>>(uuid "33333333-3333-4333-8333-333333333333")
>>)
>>(pin "1"
>>>(uuid "34343434-3434-4434-8434-343434343434")
>>)
>>(instances
>>>(project "fixture"
>>>>(path "/11111111-1111-4111-8111-111111111111"
>>>>>(reference "U2")
>>>>>(unit 1)
>>>>)
>>>)
>>)
>)
>(symbol
>>(lib_id "Sensor:Part")
>>(at 148.59 113.03 0)
>>(unit 1)
>>(exclude_from_sim no)
>>(in_bom yes)
>>(on_board yes)
>>(dnp no)
>>(uuid "66666666-6666-4666-8666-666666666666")
>>(property "Reference" "U3"
>>>(at 148.59 100 0)
>>>(effects
>>>>(font
>>>>>(size 1.27 1.27)
>>>>)
>>>)
>>)
>>(pin "24"
>>>(uuid "77777777-7777-4777-8777-777777777777")
>>)
>>(pin "1"
>>>(uuid "78787878-7878-4787-8787-787878787878")
>>)
>>(instances
>>>(project "fixture"
>>>>(path "/11111111-1111-4111-8111-111111111111"
>>>>>(reference "U3")
>>>>>(unit 1)
>>>>)
>>>)
>>)
>)
>(embedded_fonts no)
)
`)
}

func indentKiCad(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		n := 0
		for n < len(line) && line[n] == '>' {
			n++
		}
		if n > 0 {
			lines[i] = strings.Repeat("\t", n) + line[n:]
		}
	}
	return strings.Join(lines, "\n")
}

func mustNM(t *testing.T, raw string) int64 {
	t.Helper()
	value, err := parseNM(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return value
}
