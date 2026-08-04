package httpkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/module"
)

var (
	contributionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	capabilityPattern     = regexp.MustCompile(`^[a-z][a-z0-9._/-]{0,126}$`)
	adminPathPattern      = regexp.MustCompile(`^/[a-z0-9](?:[a-z0-9/_-]*[a-z0-9])?$`)
	iconPattern           = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	permissionPattern     = regexp.MustCompile(`^[a-z][a-z0-9._:-]{0,126}$`)
)

const (
	maxContributions               = 256
	maxRoutesPerContribution       = 256
	maxRequirementsPerContribution = 64
	maxPlanRoutes                  = 1024
)

var (
	// ErrContributionPanic reports a recovered contribution builder panic.
	ErrContributionPanic = errors.New("HTTP contribution builder panicked")
	// ErrPlanMismatch reports assembly against an application started from a
	// different static definition.
	ErrPlanMismatch = errors.New("HTTP plan does not match application contract")
)

// RouteSpec declares one route before its handler exists.
type RouteSpec struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// AdminDescriptor is the backend-owned navigation and permission metadata for
// one optional Admin surface. Permissions is the complete set of grants the
// surface may inspect, while RequiredPermissions is the subset needed to show
// the surface at all. Both are presentation hints only; every backend route
// must still authorize each request.
type AdminDescriptor struct {
	ID                  string   `json:"id"`
	Label               string   `json:"label"`
	Path                string   `json:"path"`
	Icon                string   `json:"icon"`
	Order               int      `json:"order"`
	Permissions         []string `json:"permissions,omitempty"`
	RequiredPermissions []string `json:"requiredPermissions,omitempty"`
}

// Builder constructs handlers only after the bound application has started.
type Builder func(context.Context, *appkit.Application) ([]Route, error)

// Contribution is one statically inspectable HTTP/Admin component selection.
// Routes and Requires are validated and copied before Build may run.
type Contribution struct {
	ID       string
	Requires []module.Capability
	Routes   []RouteSpec
	Admin    *AdminDescriptor
	Build    Builder
}

// Descriptor is the callback-free snapshot returned by Plan.Contributions.
type Descriptor struct {
	ID       string
	Requires []module.Capability
	Routes   []RouteSpec
	Admin    *AdminDescriptor
}

// Plan is an immutable, preflighted HTTP/Admin composition bound to one
// appkit.Contract.
type Plan struct {
	contract      appkit.Contract
	contributions []Contribution
	descriptors   []Descriptor
	admin         []AdminDescriptor
}

// NewPlan validates the complete application and contribution set without
// invoking Module or contribution callbacks.
func NewPlan(definition appkit.Definition, options appkit.Options, contributions ...Contribution) (*Plan, error) {
	contract, err := appkit.Preflight(definition, options)
	if err != nil {
		return nil, fmt.Errorf("preflight application for HTTP plan: %w", err)
	}
	normalized, descriptors, admin, err := normalizeContributions(contract, contributions)
	if err != nil {
		return nil, err
	}
	return &Plan{contract: contract, contributions: normalized, descriptors: descriptors, admin: admin}, nil
}

// Contributions returns the complete callback-free contribution selection.
func (plan *Plan) Contributions() []Descriptor {
	if plan == nil {
		return nil
	}
	result := make([]Descriptor, len(plan.descriptors))
	for index, descriptor := range plan.descriptors {
		result[index] = cloneDescriptor(descriptor)
	}
	return result
}

// Admin returns permission-bearing Admin descriptors in stable presentation
// order.
func (plan *Plan) Admin() []AdminDescriptor {
	if plan == nil {
		return nil
	}
	result := make([]AdminDescriptor, len(plan.admin))
	for index, descriptor := range plan.admin {
		result[index] = cloneAdmin(descriptor)
	}
	return result
}

// Handler invokes selected builders after startup, verifies their outputs
// exactly match the preflighted route declarations, and composes one handler.
func (plan *Plan) Handler(ctx context.Context, application *appkit.Application) (http.Handler, error) {
	if ctx == nil {
		return nil, appkit.ErrContextRequired
	}
	if plan == nil || !plan.contract.Binds(application) {
		return nil, ErrPlanMismatch
	}
	if !application.Ready() {
		return nil, appkit.ErrApplicationUnavailable
	}
	routes := make([]Route, 0)
	for _, contribution := range plan.contributions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(contribution.Routes) == 0 {
			continue
		}
		built, err := invokeBuilder(ctx, application, contribution)
		if err != nil {
			return nil, err
		}
		if err := matchDeclaredRoutes(contribution, built); err != nil {
			return nil, err
		}
		routes = append(routes, built...)
	}
	observer, err := application.Observability()
	if err != nil && !errors.Is(err, appkit.ErrObservabilityUnavailable) {
		return nil, fmt.Errorf("resolve HTTP observability: %w", err)
	}
	if err == nil {
		for index := range routes {
			routes[index].Handler = observer.WrapHTTP(routes[index].Method, routes[index].Path, routes[index].Handler)
		}
	}
	return NewHandler(routes...)
}

func normalizeContributions(contract appkit.Contract, contributions []Contribution) ([]Contribution, []Descriptor, []AdminDescriptor, error) {
	if len(contributions) > maxContributions {
		return nil, nil, nil, fmt.Errorf("HTTP contribution count exceeds %d", maxContributions)
	}
	normalized := make([]Contribution, len(contributions))
	descriptors := make([]Descriptor, len(contributions))
	admin := make([]AdminDescriptor, 0, len(contributions))
	seenIDs := make(map[string]struct{}, len(contributions))
	seenAdminPaths := make(map[string]string, len(contributions))
	allRoutes := make([]Route, 0)
	dummy := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for index, contribution := range contributions {
		if !contributionIDPattern.MatchString(contribution.ID) {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %d id %q is invalid", index, contribution.ID)
		}
		if _, duplicate := seenIDs[contribution.ID]; duplicate {
			return nil, nil, nil, fmt.Errorf("duplicate HTTP contribution %q", contribution.ID)
		}
		seenIDs[contribution.ID] = struct{}{}
		if len(contribution.Requires) > maxRequirementsPerContribution {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %q requirement count exceeds %d", contribution.ID, maxRequirementsPerContribution)
		}
		if len(contribution.Routes) > maxRoutesPerContribution {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %q route count exceeds %d", contribution.ID, maxRoutesPerContribution)
		}
		if len(allRoutes)+len(contribution.Routes) > maxPlanRoutes {
			return nil, nil, nil, fmt.Errorf("HTTP plan route count exceeds %d", maxPlanRoutes)
		}

		requires, missing, err := normalizeRequirements(contract, contribution.Requires)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %q: %w", contribution.ID, err)
		}
		if len(missing) > 0 {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %q requires unavailable capabilities: %s", contribution.ID, strings.Join(missing, ", "))
		}
		routeSpecs := append([]RouteSpec(nil), contribution.Routes...)
		if len(routeSpecs) > 0 && isNilBuilder(contribution.Build) {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %q builder is required", contribution.ID)
		}
		if len(routeSpecs) == 0 && !isNilBuilder(contribution.Build) {
			return nil, nil, nil, fmt.Errorf("HTTP contribution %q declares a builder without routes", contribution.ID)
		}
		for _, spec := range routeSpecs {
			allRoutes = append(allRoutes, Route{Method: spec.Method, Path: spec.Path, Handler: dummy})
		}

		var adminDescriptor *AdminDescriptor
		if contribution.Admin != nil {
			value, err := normalizeAdmin(contribution.ID, *contribution.Admin)
			if err != nil {
				return nil, nil, nil, err
			}
			if owner, duplicate := seenAdminPaths[value.Path]; duplicate {
				return nil, nil, nil, fmt.Errorf("HTTP contributions %q and %q declare duplicate Admin path %q", owner, contribution.ID, value.Path)
			}
			seenAdminPaths[value.Path] = contribution.ID
			adminDescriptor = &value
			admin = append(admin, value)
		}
		normalized[index] = Contribution{
			ID: contribution.ID, Requires: requires, Routes: routeSpecs, Admin: adminDescriptor, Build: contribution.Build,
		}
		descriptors[index] = Descriptor{
			ID: contribution.ID, Requires: append([]module.Capability(nil), requires...),
			Routes: append([]RouteSpec(nil), routeSpecs...), Admin: cloneAdminPointer(adminDescriptor),
		}
	}
	if _, err := NewHandler(allRoutes...); err != nil {
		return nil, nil, nil, fmt.Errorf("preflight HTTP contribution routes: %w", err)
	}
	slices.SortFunc(admin, func(left, right AdminDescriptor) int {
		if left.Order != right.Order {
			return left.Order - right.Order
		}
		return strings.Compare(left.ID, right.ID)
	})
	return normalized, descriptors, admin, nil
}

func normalizeRequirements(contract appkit.Contract, values []module.Capability) ([]module.Capability, []string, error) {
	result := append([]module.Capability(nil), values...)
	seen := make(map[module.Capability]struct{}, len(result))
	missing := make([]string, 0)
	for _, capability := range result {
		if !capabilityPattern.MatchString(string(capability)) {
			return nil, nil, fmt.Errorf("required capability %q is invalid", capability)
		}
		if _, duplicate := seen[capability]; duplicate {
			return nil, nil, fmt.Errorf("required capability %q is duplicated", capability)
		}
		seen[capability] = struct{}{}
		if _, available := contract.Provider(capability); !available {
			missing = append(missing, string(capability))
		}
	}
	slices.Sort(result)
	slices.Sort(missing)
	return result, missing, nil
}

func normalizeAdmin(id string, descriptor AdminDescriptor) (AdminDescriptor, error) {
	if descriptor.ID != "" && descriptor.ID != id {
		return AdminDescriptor{}, fmt.Errorf("HTTP contribution %q Admin id must be empty or match the contribution", id)
	}
	if descriptor.Label == "" || !utf8.ValidString(descriptor.Label) || utf8.RuneCountInString(descriptor.Label) > 80 ||
		strings.TrimSpace(descriptor.Label) != descriptor.Label || strings.ContainsFunc(descriptor.Label, unicode.IsControl) {
		return AdminDescriptor{}, fmt.Errorf("HTTP contribution %q Admin label is invalid", id)
	}
	if len(descriptor.Path) > maxPathBytes || !adminPathPattern.MatchString(descriptor.Path) {
		return AdminDescriptor{}, fmt.Errorf("HTTP contribution %q Admin path is invalid", id)
	}
	if !iconPattern.MatchString(descriptor.Icon) {
		return AdminDescriptor{}, fmt.Errorf("HTTP contribution %q Admin icon %q is invalid", id, descriptor.Icon)
	}
	if descriptor.Order < 0 || descriptor.Order > 10000 {
		return AdminDescriptor{}, fmt.Errorf("HTTP contribution %q Admin order is invalid", id)
	}
	permissions, err := normalizePermissions(id, "permission", descriptor.Permissions)
	if err != nil {
		return AdminDescriptor{}, err
	}
	required, err := normalizePermissions(id, "required permission", descriptor.RequiredPermissions)
	if err != nil {
		return AdminDescriptor{}, err
	}
	available := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		available[permission] = struct{}{}
	}
	for _, permission := range required {
		if _, ok := available[permission]; !ok {
			return AdminDescriptor{}, fmt.Errorf("HTTP contribution %q Admin required permission %q is not declared in permissions", id, permission)
		}
	}
	descriptor.ID = id
	descriptor.Permissions = permissions
	descriptor.RequiredPermissions = required
	return descriptor, nil
}

func normalizePermissions(id, field string, values []string) ([]string, error) {
	permissions := append([]string(nil), values...)
	if len(permissions) > 64 {
		return nil, fmt.Errorf("HTTP contribution %q Admin %s list exceeds 64 entries", id, field)
	}
	seen := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		if !permissionPattern.MatchString(permission) {
			return nil, fmt.Errorf("HTTP contribution %q Admin %s %q is invalid", id, field, permission)
		}
		if _, duplicate := seen[permission]; duplicate {
			return nil, fmt.Errorf("HTTP contribution %q Admin %s %q is duplicated", id, field, permission)
		}
		seen[permission] = struct{}{}
	}
	slices.Sort(permissions)
	return permissions, nil
}

func invokeBuilder(ctx context.Context, application *appkit.Application, contribution Contribution) (routes []Route, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			routes = nil
			err = fmt.Errorf("build HTTP contribution %q: %w", contribution.ID, ErrContributionPanic)
		}
	}()
	routes, err = contribution.Build(ctx, application)
	returned = true
	if err != nil {
		return nil, &builderError{id: contribution.ID, cause: err}
	}
	return routes, nil
}

func matchDeclaredRoutes(contribution Contribution, routes []Route) error {
	if len(routes) != len(contribution.Routes) {
		return fmt.Errorf("HTTP contribution %q built %d routes, declared %d", contribution.ID, len(routes), len(contribution.Routes))
	}
	for index, route := range routes {
		declared := contribution.Routes[index]
		if route.Method != declared.Method || route.Path != declared.Path {
			return fmt.Errorf("HTTP contribution %q route %d built %s %s, declared %s %s", contribution.ID, index, route.Method, route.Path, declared.Method, declared.Path)
		}
	}
	return nil
}

type builderError struct {
	id    string
	cause error
}

func (err *builderError) Error() string {
	return fmt.Sprintf("build HTTP contribution %q failed", err.id)
}
func (err *builderError) Unwrap() error { return safeerr.Opaque(err.cause) }

func isNilBuilder(builder Builder) bool {
	if builder == nil {
		return true
	}
	value := reflect.ValueOf(builder)
	return value.Kind() == reflect.Func && value.IsNil()
}

func cloneDescriptor(descriptor Descriptor) Descriptor {
	descriptor.Requires = append([]module.Capability(nil), descriptor.Requires...)
	descriptor.Routes = append([]RouteSpec(nil), descriptor.Routes...)
	descriptor.Admin = cloneAdminPointer(descriptor.Admin)
	return descriptor
}

func cloneAdminPointer(descriptor *AdminDescriptor) *AdminDescriptor {
	if descriptor == nil {
		return nil
	}
	clone := cloneAdmin(*descriptor)
	return &clone
}

func cloneAdmin(descriptor AdminDescriptor) AdminDescriptor {
	descriptor.Permissions = append([]string(nil), descriptor.Permissions...)
	descriptor.RequiredPermissions = append([]string(nil), descriptor.RequiredPermissions...)
	return descriptor
}
