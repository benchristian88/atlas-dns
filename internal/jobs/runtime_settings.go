package jobs

import "github.com/benchristian88/atlas-dns/internal/systemsettings"

type RuntimeSettingsProvider interface {
	RuntimeSettings() systemsettings.RuntimeSettings
}
