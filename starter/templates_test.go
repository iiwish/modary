package starter

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestDerivedQueueSchemaIDRespectsRiverLimitAndAvoidsTruncationCollisions(t *testing.T) {
	t.Parallel()
	const riverSchemaLimit = 46

	firstQueue := deriveProjectSchemaIDs(strings.Repeat("a", 62) + "b").queue
	secondQueue := deriveProjectSchemaIDs(strings.Repeat("a", 62) + "c").queue

	if len(firstQueue) > riverSchemaLimit {
		t.Fatalf("queue schema length = %d, want at most %d: %q", len(firstQueue), riverSchemaLimit, firstQueue)
	}
	if firstQueue == secondQueue {
		t.Fatalf("distinct application schemas produced the same queue schema %q", firstQueue)
	}
}

func TestDerivedProjectSchemaIDsRespectBackendLimits(t *testing.T) {
	t.Parallel()

	short := deriveProjectSchemaIDs("sample-admin")
	wantShort := projectSchemaIDs{
		application:     "modary_app_sample_admin",
		queue:           "modary_queue_sample_admin",
		testApplication: "modary_test_app_sample_admin",
		testQueue:       "modary_test_queue_sample_admin",
	}
	if short != wantShort {
		t.Fatalf("short schema IDs = %#v, want %#v", short, wantShort)
	}

	first := deriveProjectSchemaIDs(strings.Repeat("a", 62) + "b")
	second := deriveProjectSchemaIDs(strings.Repeat("a", 62) + "c")
	validSchema := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for name, test := range map[string]struct {
		value string
		limit int
	}{
		"application":      {value: first.application, limit: maxPostgreSQLSchemaBytes},
		"queue":            {value: first.queue, limit: maxRiverQueueSchemaBytes},
		"test application": {value: first.testApplication, limit: maxPostgreSQLSchemaBytes},
		"test queue":       {value: first.testQueue, limit: maxRiverQueueSchemaBytes},
	} {
		if len(test.value) > test.limit {
			t.Errorf("%s schema length = %d, want at most %d: %q", name, len(test.value), test.limit, test.value)
		}
		if !validSchema.MatchString(test.value) {
			t.Errorf("%s schema is not a PostgreSQL identifier: %q", name, test.value)
		}
	}
	if first.queue == second.queue || first.testApplication == second.testApplication || first.testQueue == second.testQueue {
		t.Fatalf("distinct long project IDs collide: first=%#v second=%#v", first, second)
	}
	if deriveProjectSchemaIDs(strings.Repeat("a", 62)+"b") != first {
		t.Fatal("schema derivation is not deterministic")
	}
}

func TestDerivedProjectSchemaIDsAvoidReservedPostgreSQLNames(t *testing.T) {
	t.Parallel()

	for _, projectID := range []string{"public", "information-schema", "pg", "pg-events"} {
		projectID := projectID
		t.Run(projectID, func(t *testing.T) {
			t.Parallel()
			schemaIDs := deriveProjectSchemaIDs(projectID)
			for role, schema := range map[string]string{
				"application":      schemaIDs.application,
				"queue":            schemaIDs.queue,
				"test application": schemaIDs.testApplication,
				"test queue":       schemaIDs.testQueue,
			} {
				if schema == "public" || schema == "information_schema" || strings.HasPrefix(schema, "pg_") {
					t.Errorf("%s schema derived from %q is reserved: %q", role, projectID, schema)
				}
			}
		})
	}
}

func TestLongProjectIDTemplatesUseBoundedRuntimeAndTestSchemas(t *testing.T) {
	t.Parallel()

	projectID := strings.Repeat("a", 63)
	schemaIDs := deriveProjectSchemaIDs(projectID)
	tests := []struct {
		name           string
		options        normalizedCreateOptions
		requiredByFile map[string][]string
	}{
		{
			name: "admin tasks",
			options: normalizedCreateOptions{
				id: projectID, modulePath: "example.com/long-admin", name: "Long Admin",
				profile: ProfileAdmin, modaryVersion: "v0.1.0-alpha.3", components: []Component{ComponentTasks},
			},
			requiredByFile: map[string][]string{
				"internal/config/config.go": {
					fmt.Sprintf("config.Schema = %q", schemaIDs.application),
					fmt.Sprintf("config.QueueSchema = %q", schemaIDs.queue),
				},
				"internal/app/application_test.go": {
					fmt.Sprintf("runtimeConfig.Schema = %q", schemaIDs.testApplication),
					fmt.Sprintf("runtimeConfig.QueueSchema = %q", schemaIDs.testQueue),
				},
			},
		},
		{
			name: "governed",
			options: normalizedCreateOptions{
				id: projectID, modulePath: "example.com/long-governed", name: "Long Governed",
				profile: ProfileGoverned, modaryVersion: "v0.1.0-alpha.3",
			},
			requiredByFile: map[string][]string{
				"internal/config/config.go": {
					fmt.Sprintf("config.ApplicationSchema = %q", schemaIDs.application),
					fmt.Sprintf("config.QueueSchema = %q", schemaIDs.queue),
				},
				"internal/project/application_test.go": {
					fmt.Sprintf("environmentOr(\"MODARY_APPLICATION_SCHEMA\", %q)", schemaIDs.testApplication),
					fmt.Sprintf("environmentOr(\"MODARY_QUEUE_SCHEMA\", %q)", schemaIDs.testQueue),
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files, err := renderProfile(context.Background(), test.options)
			if err != nil {
				t.Fatal(err)
			}
			for fileName, required := range test.requiredByFile {
				data := renderedFileContent(t, files, fileName)
				for _, value := range required {
					if !strings.Contains(data, value) {
						t.Errorf("%s is missing %q", fileName, value)
					}
				}
			}
		})
	}
}

func renderedFileContent(t *testing.T, files []renderedFile, name string) string {
	t.Helper()
	for _, file := range files {
		if file.path == name {
			return string(file.data)
		}
	}
	t.Fatalf("rendered file %q not found", name)
	return ""
}
