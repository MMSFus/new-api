package system_setting

import "github.com/QuantumNous/new-api/setting/config"

// ThemeDefaultSettings holds the site-wide color theme preset used when a
// visitor has not picked one. Light and Dark apply to the resolved color
// scheme; an empty value means the frontend's built-in default. The frontend
// validates the value against its preset list, so unknown names fall back.
type ThemeDefaultSettings struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

var defaultThemeDefaultSettings = ThemeDefaultSettings{}

func init() {
	// Not registered as "theme": the retired theme.frontend option lives
	// under that prefix.
	config.GlobalConfig.Register("theme_default", &defaultThemeDefaultSettings)
}

func GetThemeDefaultSettings() *ThemeDefaultSettings {
	return &defaultThemeDefaultSettings
}
