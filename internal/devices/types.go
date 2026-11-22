// Package devices provides read-only inventories. Discovery never establishes
// installation authorization, pairing permission, or signing readiness.
package devices

const MaxReportBytes = 4 * 1024 * 1024
const maxDevices = 4096

type Device struct {
	ID              string `json:"id"`
	UDID            string `json:"udid,omitempty"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Platform        string `json:"platform"`
	Model           string `json:"model"`
	OSVersion       string `json:"os_version"`
	Runtime         string `json:"runtime,omitempty"`
	BootState       string `json:"boot_state"`
	ConnectionState string `json:"connection_state"`
	PairingState    string `json:"pairing_state"`
	Transport       string `json:"transport"`
	DeveloperMode   string `json:"developer_mode"`
	Available       *bool  `json:"available,omitempty"`
}

type Inventory struct {
	Tool            string   `json:"tool"`
	ToolVersion     string   `json:"tool_version,omitempty"`
	ToolJSONVersion int      `json:"tool_json_version,omitempty"`
	Source          string   `json:"source"`
	Live            bool     `json:"live"`
	Devices         []Device `json:"devices"`
}

type Error struct{ Code, Message string }

func (e *Error) Error() string           { return e.Message }
func failure(code, message string) error { return &Error{Code: code, Message: message} }
