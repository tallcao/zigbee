package models

type zigbeeBridgeDeviceDefinition struct {
	Model  string `json:"model"`
	Vendor string `json:"vendor"`
}

type ZigbeeBridgeDevice struct {
	Addr       string                       `json:"ieee_address"`
	Name       string                       `json:"friendly_name"`
	Type       string                       `json:"type"`
	Disabled   bool                         `json:"disabled"`
	Definition zigbeeBridgeDeviceDefinition `json:"definition"`
}

type ConnectionState struct {
	// online, offline
	State string `json:"state"`
}
