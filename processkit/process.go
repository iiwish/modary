// Package processkit provides a standard-library process boundary for Modary
// applications: deterministic probes, pre-shutdown drain, active-request
// admission, and one shared HTTP server lifecycle. It contains no database,
// container-platform, or telemetry dependency.
package processkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultCheckTimeout bounds one complete readiness evaluation.
	DefaultCheckTimeout = 2 * time.Second
	// MaximumChecks bounds the number of selected readiness dependencies.
	MaximumChecks = 32
)

var checkNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

// Phase is the local admission lifecycle. Draining remains live but rejects
// new application work and reports unready.
type Phase string

const (
	// PhaseStarting rejects traffic before assembly and listener startup finish.
	PhaseStarting Phase = "starting"
	// PhaseReady admits application traffic and enables dependency readiness.
	PhaseReady Phase = "ready"
	// PhaseDraining rejects new work while accepted work completes.
	PhaseDraining Phase = "draining"
	// PhaseStopped is the terminal process state after resources are released.
	PhaseStopped Phase = "stopped"
)

// Check is one bounded readiness dependency. Check functions must be safe for
// concurrent use and honor cancellation. Manager permits at most one in-flight
// invocation per check, preventing a noncooperative dependency from creating an
// unbounded goroutine count across repeated probes.
type Check struct {
	Name string
	Run  func(context.Context) error
}

// Options controls readiness behavior.
type Options struct {
	CheckTimeout time.Duration
	Checks       []Check
}

type checkState struct {
	name string
	run  func(context.Context) error
	gate chan struct{}
}

// Manager owns process readiness and accepted HTTP work.
type Manager struct {
	mu           sync.Mutex
	phase        Phase
	active       int
	drained      chan struct{}
	drainOnce    sync.Once
	checkTimeout time.Duration
	checks       []checkState
}

// New validates and defensively copies readiness checks.
func New(options Options) (*Manager, error) {
	if options.CheckTimeout < 0 || options.CheckTimeout > time.Minute {
		return nil, fmt.Errorf("process readiness timeout must be between zero and one minute")
	}
	if options.CheckTimeout == 0 {
		options.CheckTimeout = DefaultCheckTimeout
	}
	if len(options.Checks) > MaximumChecks {
		return nil, fmt.Errorf("process readiness check count exceeds %d", MaximumChecks)
	}
	checks := make([]checkState, len(options.Checks))
	seen := make(map[string]struct{}, len(options.Checks))
	for index, check := range options.Checks {
		if !checkNamePattern.MatchString(check.Name) || check.Run == nil {
			return nil, fmt.Errorf("process readiness check %d is invalid", index)
		}
		if _, duplicate := seen[check.Name]; duplicate {
			return nil, fmt.Errorf("process readiness check %q is declared more than once", check.Name)
		}
		seen[check.Name] = struct{}{}
		checks[index] = checkState{name: check.Name, run: check.Run, gate: make(chan struct{}, 1)}
	}
	return &Manager{phase: PhaseStarting, drained: make(chan struct{}), checkTimeout: options.CheckTimeout, checks: checks}, nil
}

// Phase returns the current local lifecycle state.
func (manager *Manager) Phase() Phase {
	if manager == nil {
		return PhaseStopped
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.phase
}

// MarkReady enables traffic after application assembly and listener startup.
func (manager *Manager) MarkReady() error {
	if manager == nil {
		return fmt.Errorf("process manager is unavailable")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.phase == PhaseReady {
		return nil
	}
	if manager.phase != PhaseStarting {
		return fmt.Errorf("cannot mark process ready from %s", manager.phase)
	}
	manager.phase = PhaseReady
	return nil
}

// BeginDrain atomically disables readiness and new application admission.
// Repeated calls are safe.
func (manager *Manager) BeginDrain() {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	if manager.phase == PhaseStarting || manager.phase == PhaseReady {
		manager.phase = PhaseDraining
	}
	if manager.phase == PhaseDraining && manager.active == 0 {
		manager.drainOnce.Do(func() { close(manager.drained) })
	}
	manager.mu.Unlock()
}

// Drain begins drain and waits for accepted application requests.
func (manager *Manager) Drain(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("process drain context is required")
	}
	if manager == nil {
		return fmt.Errorf("process manager is unavailable")
	}
	manager.BeginDrain()
	select {
	case <-manager.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// MarkStopped records terminal local shutdown. It is valid only after drain.
func (manager *Manager) MarkStopped() error {
	if manager == nil {
		return fmt.Errorf("process manager is unavailable")
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.phase == PhaseStopped {
		return nil
	}
	if manager.phase != PhaseDraining || manager.active != 0 {
		return fmt.Errorf("cannot stop process from %s with %d active requests", manager.phase, manager.active)
	}
	manager.phase = PhaseStopped
	return nil
}

// Middleware admits application requests only while ready. Exact probe paths
// remain reachable while draining so an orchestrator can observe the transition.
func (manager *Manager) Middleware(next http.Handler) (http.Handler, error) {
	if manager == nil || next == nil {
		return nil, fmt.Errorf("process middleware dependencies are required")
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request != nil && (request.URL.Path == "/livez" || request.URL.Path == "/readyz") {
			next.ServeHTTP(writer, request)
			return
		}
		if !manager.acquire() {
			writeProbe(writer, request, http.StatusServiceUnavailable, "draining")
			return
		}
		defer manager.release()
		next.ServeHTTP(writer, request)
	}), nil
}

func (manager *Manager) acquire() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.phase != PhaseReady {
		return false
	}
	manager.active++
	return true
}

func (manager *Manager) release() {
	manager.mu.Lock()
	manager.active--
	if manager.phase == PhaseDraining && manager.active == 0 {
		manager.drainOnce.Do(func() { close(manager.drained) })
	}
	manager.mu.Unlock()
}

// LivenessHandler reports only local process state and never calls a remote
// dependency.
func (manager *Manager) LivenessHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !validProbeRequest(request) {
			writeProbe(writer, request, http.StatusBadRequest, "invalid_request")
			return
		}
		if manager == nil || manager.Phase() == PhaseStopped {
			writeProbe(writer, request, http.StatusServiceUnavailable, "stopped")
			return
		}
		writeProbe(writer, request, http.StatusOK, "live")
	})
}

// ReadinessHandler runs bounded selected dependency checks only while the local
// lifecycle is ready.
func (manager *Manager) ReadinessHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !validProbeRequest(request) {
			writeProbe(writer, request, http.StatusBadRequest, "invalid_request")
			return
		}
		if manager == nil || manager.Phase() != PhaseReady || !manager.dependenciesReady(request.Context()) {
			writeProbe(writer, request, http.StatusServiceUnavailable, "not_ready")
			return
		}
		writeProbe(writer, request, http.StatusOK, "ready")
	})
}

func (manager *Manager) dependenciesReady(parent context.Context) bool {
	if len(manager.checks) == 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(parent, manager.checkTimeout)
	defer cancel()
	results := make(chan error, len(manager.checks))
	for index := range manager.checks {
		check := &manager.checks[index]
		select {
		case check.gate <- struct{}{}:
			go func() {
				defer func() { <-check.gate }()
				results <- invokeCheck(ctx, check.run)
			}()
		default:
			results <- errors.New("readiness check is already running")
		}
	}
	for range manager.checks {
		select {
		case err := <-results:
			if err != nil {
				return false
			}
		case <-ctx.Done():
			return false
		}
	}
	return true
}

func invokeCheck(ctx context.Context, check func(context.Context) error) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = errors.New("readiness check failed")
		}
	}()
	err = check(ctx)
	returned = true
	return err
}

func validProbeRequest(request *http.Request) bool {
	if request == nil || (request.Method != http.MethodGet && request.Method != http.MethodHead) || request.URL.RawQuery != "" {
		return false
	}
	return request.Body == nil || request.Body == http.NoBody
}

func writeProbe(writer http.ResponseWriter, request *http.Request, status int, value string) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	if request != nil && request.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(writer).Encode(struct {
		Status string `json:"status"`
	}{Status: strings.Clone(value)})
}
