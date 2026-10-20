package model

// PlatformInfo describes the host platform and its recommended initial scan path.
type PlatformInfo struct {
	OS              string `json:"os"`
	HomeDirectory   string `json:"homeDirectory"`
	DefaultScanPath string `json:"defaultScanPath"`
}
