package profile

// SettingState is the value of a Profile setting. Partial means the settings
// were hand-edited into neither on nor off.
type SettingState string

const (
	SettingOn      SettingState = "on"
	SettingOff     SettingState = "off"
	SettingPartial SettingState = "partial"
)
