package starter

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"go/format"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/api templates/admin/*.tmpl templates/admin/web/*.json templates/admin/web/*.yaml templates/admin/web/*.js templates/admin/web/*.ts templates/admin/web/*.html templates/admin/web/src templates/admin/web/scripts templates/admin/internal/web/dist templates/governed
var profileTemplates embed.FS

type profileTemplate struct {
	source      string
	destination func(normalizedCreateOptions) string
}

var apiTemplates = []profileTemplate{
	{source: "gitignore.tmpl", destination: fixedDestination(".gitignore")},
	{source: "README.md.tmpl", destination: fixedDestination("README.md")},
	{source: "go.mod.tmpl", destination: fixedDestination("go.mod")},
	{source: "main.go.tmpl", destination: func(options normalizedCreateOptions) string { return path.Join("cmd", options.id, "main.go") }},
	{source: "application.go.tmpl", destination: fixedDestination("internal/app/application.go")},
	{source: "application_test.go.tmpl", destination: fixedDestination("internal/app/application_test.go")},
	{source: "ping.go.tmpl", destination: fixedDestination("internal/ping/component.go")},
}

var adminTemplates = []profileTemplate{
	{source: "gitignore.tmpl", destination: fixedDestination(".gitignore")},
	{source: "README.md.tmpl", destination: fixedDestination("README.md")},
	{source: "go.mod.tmpl", destination: fixedDestination("go.mod")},
	{source: "main.go.tmpl", destination: func(options normalizedCreateOptions) string { return path.Join("cmd", options.id, "main.go") }},
	{source: "application.go.tmpl", destination: fixedDestination("internal/app/application.go")},
	{source: "application_test.go.tmpl", destination: fixedDestination("internal/app/application_test.go")},
	{source: "config.go.tmpl", destination: fixedDestination("internal/config/config.go")},
	{source: "records.go.tmpl", destination: fixedDestination("internal/records/component.go")},
	{source: "records.sql.tmpl", destination: fixedDestination("internal/records/migrations/postgres/0001_records.sql")},
	{source: "web.go.tmpl", destination: fixedDestination("internal/web/web.go")},
}

var governedTemplates = []profileTemplate{
	{source: "gitignore.tmpl", destination: fixedDestination(".gitignore")},
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
	ID               string
	SchemaID         string
	QueueSchemaID    string
	Name             string
	ModulePath       string
	ModaryVersion    string
	ModaryReplace    string
	HasModaryReplace bool
}

func fixedDestination(value string) func(normalizedCreateOptions) string {
	return func(normalizedCreateOptions) string { return value }
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
	schemaID := strings.ReplaceAll(options.id, "-", "_")
	data := templateData{
		ID: options.id, SchemaID: schemaID, QueueSchemaID: queueSchemaID(schemaID), Name: options.name, ModulePath: options.modulePath,
		ModaryVersion: options.modaryVersion, ModaryReplace: options.modaryReplace,
		HasModaryReplace: options.modaryReplace != "",
	}
	result := make([]renderedFile, 0, len(templates))
	seen := make(map[string]struct{}, len(templates))
	for _, item := range templates {
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
		result, err = appendStaticTree(ctx, result, seen, "templates/admin/web", "web")
		if err != nil {
			return nil, err
		}
		result, err = appendStaticTree(ctx, result, seen, "templates/admin/internal/web/dist", "internal/web/dist")
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(first, second int) bool { return result[first].path < result[second].path })
	return result, nil
}

func queueSchemaID(applicationSchema string) string {
	const suffix = "_queue"
	const maximumSchemaBytes = 63
	if len(applicationSchema) > maximumSchemaBytes-len(suffix) {
		applicationSchema = applicationSchema[:maximumSchemaBytes-len(suffix)]
	}
	return applicationSchema + suffix
}

func appendStaticTree(ctx context.Context, result []renderedFile, seen map[string]struct{}, sourceRoot, destinationRoot string) ([]renderedFile, error) {
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
