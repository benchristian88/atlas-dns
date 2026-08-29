package jobs

import "github.com/benchristian88/atlas-dns/internal/systemsettings"

type RuntimeSettingsProvider interface {
	RuntimeSettings() systemsettings.RuntimeSettings
}

type runtimeSettingsWatcher interface {
	RuntimeSettingsProvider
	RuntimeSettingsChanged() <-chan struct{}
}

func runtimeSettingsChanged(provider RuntimeSettingsProvider) <-chan struct{} {
	if watcher, ok := provider.(runtimeSettingsWatcher); ok {
		return watcher.RuntimeSettingsChanged()
	}
	return nil
}
