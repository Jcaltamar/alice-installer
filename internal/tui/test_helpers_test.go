package tui

import (
	"github.com/jcaltamar/alice-installer/internal/asterisk"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

func themeDefaultForTest() theme.Theme {
	return theme.Default()
}

func asteriskTestOptions() asterisk.Options {
	return asterisk.Options{
		Enabled:    true,
		ConfigRoot: asterisk.DefaultConfigRoot,
		AMI: asterisk.AMIContract{
			Enabled:  true,
			Host:     asterisk.DefaultAMIHost,
			Port:     asterisk.DefaultAMIPort,
			Username: "alice-guardian",
			Password: "generated-secret",
		},
	}
}
