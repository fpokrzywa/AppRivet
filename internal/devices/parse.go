package devices

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/fpokrzywa/AppRivet/internal/runner"
)

type hardware struct {
	UDID        *string `json:"udid"`
	Platform    *string `json:"platform"`
	ProductType *string `json:"productType"`
	Reality     *string `json:"reality"`
}
type state struct {
	Name          *string `json:"name"`
	BootState     *string `json:"bootState"`
	DeveloperMode *string `json:"developerModeStatus"`
	OSVersion     *string `json:"osVersionNumber"`
}
type connection struct {
	State        *string `json:"state"`
	TunnelState  *string `json:"tunnelState"`
	PairingState *string `json:"pairingState"`
	Transport    *string `json:"transportType"`
}
type coreDevice struct {
	Identifier string     `json:"identifier"`
	Hardware   hardware   `json:"hardwareProperties"`
	State      state      `json:"deviceProperties"`
	Connection connection `json:"connectionProperties"`
	Properties struct {
		Hardware   hardware   `json:"hardware"`
		State      state      `json:"state"`
		Connection connection `json:"connection"`
		Software   struct {
			OSVersion struct {
				StringValue *string `json:"stringValue"`
			} `json:"osVersionNumber"`
		} `json:"software"`
	} `json:"properties"`
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Parse consumes only the inventory format selected by the caller. Unknown
// additive fields are ignored and are never forwarded as raw device metadata.
func Parse(body []byte, simulators bool) (Inventory, error) {
	if len(body) > MaxReportBytes {
		return Inventory{}, failure("report_too_large", "device report exceeds the 4 MiB limit")
	}
	var inv Inventory
	var err error
	if simulators {
		inv, err = parseSimulators(body)
	} else {
		inv, err = parseCoreDevices(body)
	}
	if err != nil {
		return Inventory{}, err
	}
	if len(inv.Devices) > maxDevices {
		return Inventory{}, failure("invalid_report", "device report exceeds the 4096-record limit")
	}
	seen := map[string]bool{}
	for _, d := range inv.Devices {
		key := strings.ToLower(d.ID)
		if !identifierPattern.MatchString(d.ID) || (d.UDID != "" && !identifierPattern.MatchString(d.UDID)) || seen[key] {
			return Inventory{}, failure("invalid_report", "device report contains a missing, invalid or duplicate identifier")
		}
		seen[key] = true
	}
	sort.Slice(inv.Devices, func(i, j int) bool { return inv.Devices[i].ID < inv.Devices[j].ID })
	inv.Source = "report"
	return inv, nil
}

func parseCoreDevices(body []byte) (Inventory, error) {
	var report struct {
		Info *struct {
			CommandType string `json:"commandType"`
			Outcome     string `json:"outcome"`
			JSONVersion *int   `json:"jsonVersion"`
			Version     string `json:"version"`
		} `json:"info"`
		Result *struct {
			Devices *[]*coreDevice `json:"devices"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &report); err != nil || report.Info == nil {
		return Inventory{}, invalidReport()
	}
	if report.Info.CommandType != "devicectl.list.devices" {
		return Inventory{}, failure("invalid_report", "expected a devicectl.list.devices JSON report")
	}
	if report.Info.Outcome != "success" {
		return Inventory{}, failure("discovery_failed", "devicectl did not report a successful discovery; check Xcode setup and device connectivity")
	}
	if report.Info.JSONVersion != nil && (*report.Info.JSONVersion < 1 || *report.Info.JSONVersion > 5) {
		return Inventory{}, failure("unsupported_schema", "devicectl JSON version is not supported; capture a sanitized report and update the adapter")
	}
	if report.Result == nil || report.Result.Devices == nil {
		return Inventory{}, invalidReport()
	}
	inv := Inventory{Tool: "devicectl", ToolVersion: display(report.Info.Version), Devices: []Device{}}
	if report.Info.Version == "" {
		inv.ToolVersion = ""
	}
	if report.Info.JSONVersion != nil {
		inv.ToolJSONVersion = *report.Info.JSONVersion
	}
	for _, raw := range *report.Result.Devices {
		if raw == nil {
			return Inventory{}, invalidReport()
		}
		h, s, c := raw.Properties.Hardware, raw.Properties.State, raw.Properties.Connection
		kind := "unknown"
		switch pick(h.Reality, raw.Hardware.Reality) {
		case "physical":
			kind = "physical"
		case "simulated":
			kind = "simulator"
		}
		inv.Devices = append(inv.Devices, Device{
			ID: raw.Identifier, UDID: pick(h.UDID, raw.Hardware.UDID), Kind: kind,
			Name: display(pick(s.Name, raw.State.Name)), Platform: display(pick(h.Platform, raw.Hardware.Platform)), Model: display(pick(h.ProductType, raw.Hardware.ProductType)),
			OSVersion: display(pick(raw.Properties.Software.OSVersion.StringValue, raw.State.OSVersion)), BootState: display(pick(s.BootState, raw.State.BootState)),
			ConnectionState: display(pick(c.State, raw.Connection.TunnelState)), PairingState: display(pick(c.PairingState, raw.Connection.PairingState)),
			Transport: display(pick(c.Transport, raw.Connection.Transport)), DeveloperMode: display(pick(s.DeveloperMode, raw.State.DeveloperMode)),
		})
	}
	return inv, nil
}

func parseSimulators(body []byte) (Inventory, error) {
	type simulator struct {
		UDID       string `json:"udid"`
		Name       string `json:"name"`
		State      string `json:"state"`
		Available  *bool  `json:"isAvailable"`
		DeviceType string `json:"deviceTypeIdentifier"`
	}
	var report struct {
		Devices *map[string][]*simulator `json:"devices"`
	}
	if err := json.Unmarshal(body, &report); err != nil || report.Devices == nil {
		return Inventory{}, invalidReport()
	}
	inv := Inventory{Tool: "simctl", Devices: []Device{}}
	for runtime, devices := range *report.Devices {
		if devices == nil {
			return Inventory{}, invalidReport()
		}
		platform := "unknown"
		for _, known := range []string{"iOS", "tvOS", "watchOS", "visionOS", "xrOS"} {
			if strings.HasPrefix(runtime, "com.apple.CoreSimulator.SimRuntime."+known+"-") {
				platform = known
				break
			}
		}
		for _, raw := range devices {
			if raw == nil {
				return Inventory{}, invalidReport()
			}
			inv.Devices = append(inv.Devices, Device{ID: raw.UDID, UDID: raw.UDID, Kind: "simulator", Name: display(raw.Name), Platform: platform, Model: display(raw.DeviceType), OSVersion: "unknown", Runtime: display(runtime), BootState: display(raw.State), Available: raw.Available, ConnectionState: "unknown", PairingState: "unknown", Transport: "unknown", DeveloperMode: "unknown"})
		}
	}
	return inv, nil
}

func invalidReport() error {
	return failure("invalid_report", "device report has malformed JSON or an unsupported inventory structure; no inventory was accepted")
}
func pick(primary, fallback *string) string {
	if primary != nil {
		return *primary
	}
	if fallback != nil {
		return *fallback
	}
	return ""
}
func display(value string) string {
	value = runner.Redact(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	chars := []rune(value)
	if len(chars) > 512 {
		value = string(chars[:512]) + "..."
	}
	if value == "" {
		return "unknown"
	}
	return value
}
