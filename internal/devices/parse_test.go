package devices

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile("testdata/" + name)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestLegacyInventoryKeepsSparseAndDisconnectedDevices(t *testing.T) {
	inv, err := Parse(fixture(t, "devicectl-legacy.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Devices) != 3 || inv.Tool != "devicectl" || inv.ToolJSONVersion != 2 {
		t.Fatalf("bad inventory: %+v", inv)
	}
	first := inv.Devices[0]
	if first.ID != "11111111-1111-1111-1111-111111111111" || first.UDID != "00000000-0000000000000001" || first.Kind != "physical" || first.Name != "Lab iPhone" || first.OSVersion != "17.5" || first.ConnectionState != "connected" || first.DeveloperMode != "enabled" {
		t.Fatalf("bad device: %+v", first)
	}
	if inv.Devices[1].ConnectionState != "disconnected" || inv.Devices[1].PairingState != "paired" {
		t.Fatal("paired was confused with connected")
	}
	if inv.Devices[2].Kind != "unknown" || inv.Devices[2].ConnectionState != "unknown" || inv.Devices[2].DeveloperMode != "unknown" {
		t.Fatal("missing fields manufactured known state")
	}
	body, _ := json.Marshal(inv)
	for _, secret := range []string{"DO-NOT-EXPORT", "serialNumber", "localHostnames"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("exposed %s", secret)
		}
	}
}

func TestModernPropertiesWinAndSimulatorsAreNotPhysical(t *testing.T) {
	inv, err := Parse(fixture(t, "devicectl-modern.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	d := inv.Devices[0]
	if d.Kind != "simulator" || d.Name != "Simulator from CoreDevice" || d.OSVersion != "27.0" || d.ConnectionState != "disconnected" || d.PairingState != "unpaired" || d.DeveloperMode != "disabled" || d.BootState != "booting" {
		t.Fatalf("wrong precedence: %+v", d)
	}
	if inv.Devices[1].PairingState != "unsupported" || inv.Devices[1].Name != "unknown" {
		t.Fatal("sparse device mishandled")
	}
}

func TestSimctlInventoryIncludesUnavailableAndUnknownStates(t *testing.T) {
	inv, err := Parse(fixture(t, "simctl.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Devices) != 3 || inv.Tool != "simctl" {
		t.Fatalf("bad inventory: %+v", inv)
	}
	d := inv.Devices[0]
	if d.Kind != "simulator" || d.ID != d.UDID || d.Platform != "iOS" || d.Available == nil || !*d.Available || d.BootState != "Booted" || d.ConnectionState != "unknown" {
		t.Fatalf("wrong simulator: %+v", d)
	}
	if inv.Devices[1].Available == nil || *inv.Devices[1].Available || inv.Devices[2].Available != nil || inv.Devices[2].BootState != "Creating" {
		t.Fatal("availability or unknown state lost")
	}
	body, _ := json.Marshal(inv)
	if strings.Contains(string(body), "DO-NOT-EXPORT") {
		t.Fatal("private simulator fields leaked")
	}
}

func TestEmptyListsAndMissingVersion(t *testing.T) {
	for _, tc := range []struct {
		body string
		sim  bool
	}{{`{"info":{"commandType":"devicectl.list.devices","outcome":"success"},"result":{"devices":[]}}`, false}, {`{"devices":{}}`, true}} {
		inv, err := Parse([]byte(tc.body), tc.sim)
		if err != nil || inv.Devices == nil || len(inv.Devices) != 0 {
			t.Fatalf("empty inventory: %+v %v", inv, err)
		}
	}
}

func TestDuplicateJSONKeysCannotHideDevicesOrFailures(t *testing.T) {
	cases := []struct {
		body string
		sim  bool
	}{
		{`{"devices":{"runtime":[{"udid":"device-a"}],"runtime":[]}}`, true},
		{`{"devices":{"runtime":[{"udid":"device-a","udid":"device-b"}]}}`, true},
		{`{"devices":{"runtime":[{"udid":"device-a"}]},"devices":{}}`, true},
		{`{"info":{"commandType":"devicectl.list.devices","outcome":"failed","outcome":"success"},"result":{"devices":[]}}`, false},
		{envelope(`[{"identifier":"device-a","identifier":"device-b"}]`), false},
		{strings.Replace(envelope(`[{"identifier":"a"}]`), `"devices":[{"identifier":"a"}]`, `"devices":[{"identifier":"a"}],"devices":[]`, 1), false},
		{`{"devices":{"runtime":[],"runtim\u0065":[]}}`, true},
		{`{"devices":{"runtime":[{"udid":"device-a"}]},"Devices":{}}`, true},
		{`{"devices":{"runtime":[{"udid":"device-a"}]},"device\u017f":{}}`, true},
	}
	for _, tc := range cases {
		inv, err := Parse([]byte(tc.body), tc.sim)
		if err == nil || len(inv.Devices) != 0 {
			t.Fatalf("ambiguous report accepted: %s -> %+v %v", tc.body, inv, err)
		}
	}
}

func TestModernNullNeverResurrectsLegacyState(t *testing.T) {
	for _, properties := range []string{`{"connection":{"state":null}}`, `{"connection":{"pairingState":null}}`, `{"state":{"name":null}}`, `{"connection":null}`, `{"hardware":null}`, `{"state":null}`, `{"software":null}`, `{"software":{"osVersionNumber":null}}`, `null`} {
		body := envelope(`[{"identifier":"device-a","properties":` + properties + `,"connectionProperties":{"tunnelState":"connected","pairingState":"paired"}}]`)
		if _, err := Parse([]byte(body), false); err == nil {
			t.Fatalf("null accepted: %s", properties)
		}
	}
	for _, tc := range []struct{ connection, want string }{{`{}`, "connected"}, {`{"state":""}`, "unknown"}, {`{"state":"future-state"}`, "future-state"}} {
		body := envelope(`[{"identifier":"device-a","properties":{"connection":` + tc.connection + `},"connectionProperties":{"tunnelState":"connected"}}]`)
		inv, err := Parse([]byte(body), false)
		if err != nil || inv.Devices[0].ConnectionState != tc.want {
			t.Fatalf("precedence %s -> %+v %v", tc.connection, inv, err)
		}
	}
}

func envelope(devices string) string {
	return `{"info":{"commandType":"devicectl.list.devices","outcome":"success","jsonVersion":5},"result":{"devices":` + devices + `}}`
}

func TestRejectMalformedInventoryInsteadOfFalseEmptySuccess(t *testing.T) {
	for _, body := range []string{"", `null`, `{}`, `[]`, `{"info":{"outcome":"success"},"result":{}}`, envelope(`null`), envelope(`{}`), envelope(`[{}]`), envelope(`[null]`), envelope(`[{"identifier":"bad id"}]`), envelope(`[{"identifier":"a"},{"identifier":"A"}]`), envelope(`[{"identifier":"a","deviceProperties":{"name":7}}]`), envelope(`[{"identifier":"a","properties":[]}]`), envelope(`[]`) + `{}`, strings.Replace(envelope(`[]`), `"jsonVersion":5`, `"jsonVersion":99`, 1), strings.Replace(envelope(`[]`), `"success"`, `"failed"`, 1), strings.Replace(envelope(`[]`), `devicectl.list.devices`, `devicectl.device.info.details`, 1)} {
		t.Run(body, func(t *testing.T) {
			inv, err := Parse([]byte(body), false)
			if err == nil || len(inv.Devices) > 0 {
				t.Fatalf("accepted incomplete/invalid report: %+v %v", inv, err)
			}
		})
	}
	for _, body := range []string{`{}`, `{"devices":null}`, `{"devices":[]}`, `{"devices":{"runtime":null}}`, `{"devices":{"runtime":[{"udid":"x","isAvailable":"yes"}]}}`, `{"devices":{"r":[{"udid":"x"},{"udid":"x"}]}}`} {
		if _, err := Parse([]byte(body), true); err == nil {
			t.Fatalf("accepted simctl report: %s", body)
		}
	}
	if _, err := Parse([]byte(strings.Repeat(" ", MaxReportBytes+1)), false); err == nil {
		t.Fatal("oversized JSON accepted")
	}
}

func TestUnknownFieldsAndDisplaySanitization(t *testing.T) {
	body := envelope(`[{"identifier":"test-id","extra":{"secret":"DO-NOT-EXPORT"},"deviceProperties":{"name":"Phone\nFORGED\t\u001b[31m\u202e token=secret-value"},"hardwareProperties":{"reality":"futureKind"},"connectionProperties":{"tunnelState":"futureState"}}]`)
	inv, err := Parse([]byte(body), false)
	if err != nil {
		t.Fatal(err)
	}
	d := inv.Devices[0]
	if d.Kind != "unknown" || d.ConnectionState != "futureState" {
		t.Fatalf("unknown fields lost: %+v", d)
	}
	for _, s := range []string{"\n", "\t", "\x1b", "\u202e", "secret-value"} {
		if strings.Contains(d.Name, s) {
			t.Fatalf("unsafe name %q", d.Name)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"devicectl-legacy.json", "devicectl-modern.json", "simctl.json"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b, name == "simctl.json")
	}
	f.Add([]byte(`null`), false)
	f.Fuzz(func(t *testing.T, b []byte, sim bool) {
		inv, err := Parse(b, sim)
		if err != nil {
			if len(inv.Devices) > 0 {
				t.Fatal("error exposed partial list")
			}
			return
		}
		seen := map[string]bool{}
		for _, d := range inv.Devices {
			key := strings.ToLower(d.ID)
			if d.ID == "" || seen[key] {
				t.Fatal("missing or duplicate ID")
			}
			seen[key] = true
		}
		if _, err := json.Marshal(inv); err != nil {
			t.Fatal(err)
		}
	})
}
