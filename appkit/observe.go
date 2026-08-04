package appkit

import "github.com/iiwish/modary/observe"

// Observability returns the optional bounded telemetry facade.
func (application *Application) Observability() (observe.Service, error) {
	if application == nil || !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.observability == nil {
		return nil, ErrObservabilityUnavailable
	}
	return application.observability, nil
}
