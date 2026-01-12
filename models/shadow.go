package models

type Shadow struct {
	DeviceUUID string `json:"device_uuid"`
	State      State  `json:"state"`
}

// State contains desired and reported states
type State struct {
	Delta map[string]any `json:"delta,omitempty"`
}
