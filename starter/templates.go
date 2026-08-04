package starter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"go/format"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/api templates/admin/*.tmpl templates/admin/web/*.json templates/admin/web/*.yaml templates/admin/web/*.js templates/admin/web/*.ts templates/admin/web/*.html templates/admin/web/src templates/admin/web/scripts templates/admin/internal/web/dist* templates/governed
var profileTemplates embed.FS

type profileTemplate struct {
	source      string
	destination func(normalizedCreateOptions) string
	condition   func(normalizedCreateOptions) bool
}

var apiTemplates = []profileTemplate{
	{source: "gitignore.tmpl", destination: fixedDestination(".gitignore")},
	{source: "dockerignore.tmpl", destination: fixedDestination(".dockerignore")},
	{source: "Dockerfile.tmpl", destination: fixedDestination("Dockerfile")},
	{source: "README.md.tmpl", destination: fixedDestination("README.md")},
	{source: "go.mod.tmpl", destination: fixedDestination("go.mod")},
	{source: "main.go.tmpl", destination: func(options normalizedCreateOptions) string { return path.Join("cmd", options.id, "main.go") }},
	{source: "application.go.tmpl", destination: fixedDestination("internal/app/application.go")},
	{source: "application_test.go.tmpl", destination: fixedDestination("internal/app/application_test.go")},
	{source: "ping.go.tmpl", destination: fixedDestination("internal/ping/component.go")},
}

var adminTemplates = []profileTemplate{
	{source: "gitignore.tmpl", destination: fixedDestination(".gitignore")},
	{source: "dockerignore.tmpl", destination: fixedDestination(".dockerignore")},
	{source: "Dockerfile.tmpl", destination: fixedDestination("Dockerfile")},
	{source: "compose.yaml.tmpl", destination: fixedDestination("compose.yaml")},
	{source: "README.md.tmpl", destination: fixedDestination("README.md")},
	{source: "go.mod.tmpl", destination: fixedDestination("go.mod")},
	{source: "main.go.tmpl", destination: func(options normalizedCreateOptions) string { return path.Join("cmd", options.id, "main.go") }},
	{source: "application.go.tmpl", destination: fixedDestination("internal/app/application.go")},
	{source: "application_test.go.tmpl", destination: fixedDestination("internal/app/application_test.go")},
	{source: "config.go.tmpl", destination: fixedDestination("internal/config/config.go")},
	{source: "adminapi.go.tmpl", destination: fixedDestination("internal/adminapi/adminapi.go")},
	{source: "records.go.tmpl", destination: fixedDestination("internal/records/component.go")},
	{source: "records.sql.tmpl", destination: fixedDestination("internal/records/migrations/postgres/0001_records.sql")},
	{source: "tasks.go.tmpl", destination: fixedDestination("internal/tasks/component.go"), condition: withComponent(ComponentTasks)},
	{source: "audit.go.tmpl", destination: fixedDestination("internal/auditlog/component.go"), condition: withComponent(ComponentAudit)},
	{source: "web-package.json.tmpl", destination: fixedDestination("web/package.json")},
	{source: "web-vite.config.ts.tmpl", destination: fixedDestination("web/vite.config.ts")},
	{source: "web-check-assets.mjs.tmpl", destination: fixedDestination("web/scripts/check-assets.mjs")},
	{source: "modules.ts.tmpl", destination: fixedDestination("web/src/modules/active.ts")},
	{source: "modules.test.ts.tmpl", destination: fixedDestination("web/src/modules/active.test.ts")},
	{source: "web.go.tmpl", destination: fixedDestination("internal/web/web.go")},
	{source: "web/src/views/LoginView.oidc.tsx", destination: fixedDestination("web/src/views/LoginView.tsx"), condition: withComponent(ComponentOIDC)},
	{source: "login-oidc.test.tsx.tmpl", destination: fixedDestination("web/src/views/LoginView.test.tsx"), condition: withComponent(ComponentOIDC)},
	{source: "web/src/stores/auth.oidc.tsx", destination: fixedDestination("web/src/stores/auth.tsx"), condition: withComponent(ComponentOIDC)},
	{source: "auth-oidc.test.tsx.tmpl", destination: fixedDestination("web/src/stores/auth.test.tsx"), condition: withComponent(ComponentOIDC)},
}

var governedTemplates = []profileTemplate{
	{source: "gitignore.tmpl", destination: fixedDestination(".gitignore")},
	{source: "dockerignore.tmpl", destination: fixedDestination(".dockerignore")},
	{source: "Dockerfile.tmpl", destination: fixedDestination("Dockerfile")},
	{source: "compose.yaml.tmpl", destination: fixedDestination("compose.yaml")},
	{source: "README.md.tmpl", destination: fixedDestination("README.md")},
	{source: "go.mod.tmpl", destination: fixedDestination("go.mod")},
	{source: "main.go.tmpl", destination: func(options normalizedCreateOptions) string { return path.Join("cmd", options.id, "main.go") }},
	{source: "worker.go.tmpl", destination: func(options normalizedCreateOptions) string { return path.Join("cmd", options.id+"-worker", "main.go") }},
	{source: "config.go.tmpl", destination: fixedDestination("internal/config/config.go")},
	{source: "project.go.tmpl", destination: fixedDestination("internal/project/project.go")},
	{source: "limits.go.tmpl", destination: fixedDestination("internal/limits/module.go")},
	{source: "worker_handler.go.tmpl", destination: fixedDestination("internal/limits/worker.go")},
	{source: "limits.sql.tmpl", destination: fixedDestination("internal/limits/migrations/postgres/0001_limits.sql")},
	{source: "application_test.go.tmpl", destination: fixedDestination("internal/project/application_test.go")},
}

type templateData struct {
	ID                string
	SchemaID          string
	QueueSchemaID     string
	TestSchemaID      string
	TestQueueSchemaID string
	Name              string
	ModulePath        string
	ModaryVersion     string
	ModaryReplace     string
	PostgresReplace   string
	GovernedReplace   string
	OIDCReplace       string
	OTelReplace       string
	HasModaryReplace  bool
	HasTasks          bool
	HasAudit          bool
	HasOIDC           bool
	HasOTel           bool
}

func fixedDestination(value string) func(normalizedCreateOptions) string {
	return func(normalizedCreateOptions) string { return value }
}

func withComponent(component Component) func(normalizedCreateOptions) bool {
	return func(options normalizedCreateOptions) bool { return options.hasComponent(component) }
}

func renderProfile(ctx context.Context, options normalizedCreateOptions) ([]renderedFile, error) {
	templates := apiTemplates
	directory := "templates/api"
	switch options.profile {
	case ProfileAdmin:
		templates = adminTemplates
		directory = "templates/admin"
	case ProfileGoverned:
		templates = governedTemplates
		directory = "templates/governed"
	case ProfileAPI:
	default:
		return nil, fmt.Errorf("%w: unsupported normalized profile %q", ErrInvalidOptions, options.profile)
	}
	schemaIDs := deriveProjectSchemaIDs(options.id)
	data := templateData{
		ID:                options.id,
		SchemaID:          schemaIDs.application,
		QueueSchemaID:     schemaIDs.queue,
		TestSchemaID:      schemaIDs.testApplication,
		TestQueueSchemaID: schemaIDs.testQueue,
		Name:              options.name,
		ModulePath:        options.modulePath,
		ModaryVersion:     options.modaryVersion,
		ModaryReplace:     options.modaryReplace,
		HasModaryReplace:  options.modaryReplace != "",
		HasTasks:          options.hasComponent(ComponentTasks),
		HasAudit:          options.hasComponent(ComponentAudit),
		HasOIDC:           options.hasComponent(ComponentOIDC),
		HasOTel:           options.hasComponent(ComponentOTel),
	}
	if options.modaryReplace != "" {
		data.PostgresReplace = filepath.Join(options.modaryReplace, "components", "postgres")
		data.GovernedReplace = filepath.Join(options.modaryReplace, "components", "governedpostgres")
		data.OIDCReplace = filepath.Join(options.modaryReplace, "components", "oidc")
		data.OTelReplace = filepath.Join(options.modaryReplace, "components", "otel")
	}
	result := make([]renderedFile, 0, len(templates))
	seen := make(map[string]struct{}, len(templates))
	for _, item := range templates {
		if item.condition != nil && !item.condition(options) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		destination := item.destination(options)
		if _, exists := seen[destination]; exists {
			return nil, fmt.Errorf("duplicate Starter template destination %s", destination)
		}
		seen[destination] = struct{}{}
		source, err := profileTemplates.ReadFile(path.Join(directory, item.source))
		if err != nil {
			return nil, fmt.Errorf("read Starter template %s: %w", item.source, err)
		}
		parsed, err := template.New(item.source).Option("missingkey=error").Funcs(template.FuncMap{
			"quote": strconv.Quote,
		}).Parse(string(source))
		if err != nil {
			return nil, fmt.Errorf("parse Starter template %s: %w", item.source, err)
		}
		var rendered bytes.Buffer
		if err := parsed.Execute(&rendered, data); err != nil {
			return nil, fmt.Errorf("render Starter template %s: %w", item.source, err)
		}
		content := rendered.Bytes()
		if strings.HasSuffix(destination, ".go") {
			content, err = format.Source(content)
			if err != nil {
				return nil, fmt.Errorf("format Starter template %s: %w", item.source, err)
			}
		}
		result = append(result, renderedFile{path: destination, data: append([]byte(nil), content...)})
	}
	if options.profile == ProfileAdmin {
		var err error
		result, err = appendStaticTreeFiltered(ctx, result, seen, "templates/admin/web", "web", func(relative string) bool {
			switch {
			case relative == "package.json" || relative == "vite.config.ts":
				return false
			case relative == "src/modules/active.ts":
				return false
			case relative == "scripts/build-variants.mjs" || relative == "scripts/check-assets.mjs":
				return false
			case strings.HasPrefix(relative, "scripts/selections/"):
				return false
			case strings.Contains(relative, ".oidc."):
				return false
			case options.hasComponent(ComponentOIDC) && (relative == "src/views/LoginView.tsx" || relative == "src/views/LoginView.test.tsx" || relative == "src/stores/auth.tsx" || relative == "src/stores/auth.test.tsx" || relative == "src/App.test.tsx"):
				return false
			case strings.HasPrefix(relative, "src/modules/tasks/") && !options.hasComponent(ComponentTasks):
				return false
			case strings.HasPrefix(relative, "src/modules/audit/") && !options.hasComponent(ComponentAudit):
				return false
			default:
				return true
			}
		})
		if err != nil {
			return nil, err
		}
		result, err = appendStaticTree(ctx, result, seen, adminDistRoot(options), "internal/web/dist")
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(first, second int) bool { return result[first].path < result[second].path })
	return result, nil
}

func adminDistRoot(options normalizedCreateOptions) string {
	switch {
	case options.hasComponent(ComponentOIDC) && options.hasComponent(ComponentTasks) && options.hasComponent(ComponentAudit):
		return "templates/admin/internal/web/dist-oidc-operations"
	case options.hasComponent(ComponentOIDC) && options.hasComponent(ComponentTasks):
		return "templates/admin/internal/web/dist-oidc-tasks"
	case options.hasComponent(ComponentOIDC) && options.hasComponent(ComponentAudit):
		return "templates/admin/internal/web/dist-oidc-audit"
	case options.hasComponent(ComponentOIDC):
		return "templates/admin/internal/web/dist-oidc"
	case options.hasComponent(ComponentTasks) && options.hasComponent(ComponentAudit):
		return "templates/admin/internal/web/dist-operations"
	case options.hasComponent(ComponentTasks):
		return "templates/admin/internal/web/dist-tasks"
	case options.hasComponent(ComponentAudit):
		return "templates/admin/internal/web/dist-audit"
	default:
		return "templates/admin/internal/web/dist"
	}
}

const (
	maxPostgreSQLSchemaBytes = 63
	maxRiverQueueSchemaBytes = 46
	schemaHashBytes          = 8
)

type projectSchemaIDs struct {
	application     string
	queue           string
	testApplication string
	testQueue       string
}

func deriveProjectSchemaIDs(projectID string) projectSchemaIDs {
	stem := strings.ReplaceAll(projectID, "-", "_")
	return projectSchemaIDs{
		application:     boundedSchemaID("modary_app_"+stem, maxPostgreSQLSchemaBytes),
		queue:           boundedSchemaID("modary_queue_"+stem, maxRiverQueueSchemaBytes),
		testApplication: boundedSchemaID("modary_test_app_"+stem, maxPostgreSQLSchemaBytes),
		testQueue:       boundedSchemaID("modary_test_queue_"+stem, maxRiverQueueSchemaBytes),
	}
}

func boundedSchemaID(candidate string, maximumBytes int) string {
	if len(candidate) <= maximumBytes {
		return candidate
	}
	// Project IDs are lowercase ASCII, so byte truncation preserves identifier encoding.
	digest := sha256.Sum256([]byte(candidate))
	hash := fmt.Sprintf("_%x", digest[:schemaHashBytes])
	prefixBytes := maximumBytes - len(hash)
	return candidate[:prefixBytes] + hash
}

func appendStaticTree(ctx context.Context, result []renderedFile, seen map[string]struct{}, sourceRoot, destinationRoot string) ([]renderedFile, error) {
	return appendStaticTreeFiltered(ctx, result, seen, sourceRoot, destinationRoot, func(string) bool { return true })
}

func appendStaticTreeFiltered(ctx context.Context, result []renderedFile, seen map[string]struct{}, sourceRoot, destinationRoot string, include func(string) bool) ([]renderedFile, error) {
	err := fs.WalkDir(profileTemplates, sourceRoot, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative := strings.TrimPrefix(name, sourceRoot+"/")
		if !include(relative) {
			return nil
		}
		destination := path.Join(destinationRoot, relative)
		if _, duplicate := seen[destination]; duplicate {
			return fmt.Errorf("duplicate Starter template destination %s", destination)
		}
		data, err := profileTemplates.ReadFile(name)
		if err != nil {
			return err
		}
		seen[destination] = struct{}{}
		result = append(result, renderedFile{path: destination, data: append([]byte(nil), data...)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read static Starter tree %s: %w", sourceRoot, err)
	}
	return result, nil
}
