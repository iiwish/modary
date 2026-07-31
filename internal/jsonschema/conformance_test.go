package jsonschema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	draft7FixtureDirectory = "testdata/json-schema-test-suite/draft7"
	draft7FixtureCount     = 37
	draft7CaseCount        = 257
	draft7TestCount        = 927
	draft7ExclusionCount   = 34
	draft7ExecutedCases    = 223
	draft7ExecutedTests    = 856
	draft7ExcludedTests    = 71
	draft7FixtureDigest    = "7805cf622280dc49e55bf139a6e0ffd58cd580c42eb234d548ec47a64507b72c"
)

type conformanceCaseID struct {
	file        string
	description string
}

type conformanceExclusion struct {
	policy        string
	reason        string
	errorContains string
}

const (
	policyLocalReferencesOnly = "local JSON Pointer references only"
	policyNoSchemaIdentifiers = "schema identifiers and URI bases are prohibited"
)

var draft7ConformanceExclusions = map[conformanceCaseID]conformanceExclusion{
	{"definitions.json", "validate definition against metaschema"}: localReferenceExclusion(
		"the case references the public Draft 7 metaschema by HTTP URI",
	),

	{"ref.json", "$ref prevents a sibling $id from changing the base uri"}: identifierExclusion(
		"the case depends on nested $id base-URI resolution",
	),
	{"ref.json", "remote ref, containing refs itself"}: localReferenceExclusion(
		"the case references the public Draft 7 metaschema by HTTP URI",
	),
	{"ref.json", "Recursive references between schemas"}: identifierExclusion(
		"the recursive graph is connected by absolute identifiers",
	),
	{"ref.json", "Location-independent identifier"}: identifierReferenceExclusion(
		"the case uses a fragment identifier as an anchor",
	),
	{"ref.json", "Reference an anchor with a non-relative URI"}: identifierExclusion(
		"the case uses an absolute identifier and anchor",
	),
	{"ref.json", "Location-independent identifier with base URI change in subschema"}: identifierExclusion(
		"the case changes a base URI and resolves an anchor",
	),
	{"ref.json", "refs with relative uris and defs"}: identifierExclusion(
		"the case resolves relative identifiers across schema resources",
	),
	{"ref.json", "relative refs with absolute uris and defs"}: identifierExclusion(
		"the case resolves absolute identifiers across schema resources",
	),
	{"ref.json", "$id must be resolved against nearest parent, not just immediate parent"}: identifierExclusion(
		"the case exercises nested base-URI resolution",
	),
	{"ref.json", "simple URN base URI with $ref via the URN"}: identifierExclusion(
		"the case resolves a URN schema identifier",
	),
	{"ref.json", "simple URN base URI with JSON pointer"}: identifierExclusion(
		"the case declares a URN base URI",
	),
	{"ref.json", "URN base URI with NSS"}: identifierExclusion(
		"the case declares a URN base URI",
	),
	{"ref.json", "URN base URI with r-component"}: identifierExclusion(
		"the case declares a URN base URI with an r-component",
	),
	{"ref.json", "URN base URI with q-component"}: identifierExclusion(
		"the case declares a URN base URI with a q-component",
	),
	{"ref.json", "URN base URI with URN and JSON pointer ref"}: identifierExclusion(
		"the case resolves a URN plus JSON Pointer",
	),
	{"ref.json", "URN base URI with URN and anchor ref"}: identifierExclusion(
		"the case resolves a URN plus anchor",
	),
	{"ref.json", "ref to if"}: identifierReferenceExclusion(
		"the case identifies an if subschema by absolute URI",
	),
	{"ref.json", "ref to then"}: identifierReferenceExclusion(
		"the case identifies a then subschema by absolute URI",
	),
	{"ref.json", "ref to else"}: identifierReferenceExclusion(
		"the case identifies an else subschema by absolute URI",
	),
	{"ref.json", "ref with absolute-path-reference"}: identifierExclusion(
		"the case resolves an absolute-path reference against a base URI",
	),
	{"ref.json", "$id with file URI still resolves pointers - *nix"}: identifierExclusion(
		"the case declares a file URI base",
	),
	{"ref.json", "$id with file URI still resolves pointers - windows"}: identifierExclusion(
		"the case declares a file URI base",
	),

	{"refRemote.json", "remote ref"}: localReferenceExclusion(
		"the case requires an externally registered HTTP resource",
	),
	{"refRemote.json", "fragment within remote ref"}: localReferenceExclusion(
		"the case requires an externally registered HTTP resource",
	),
	{"refRemote.json", "ref within remote ref"}: localReferenceExclusion(
		"the case requires an externally registered HTTP resource",
	),
	{"refRemote.json", "base URI change"}: identifierExclusion(
		"the case resolves a relative reference after a base-URI change",
	),
	{"refRemote.json", "base URI change - change folder"}: identifierExclusion(
		"the case resolves a remote resource after a base-URI change",
	),
	{"refRemote.json", "base URI change - change folder in subschema"}: identifierExclusion(
		"the case resolves a remote resource after a nested base-URI change",
	),
	{"refRemote.json", "root ref in remote ref"}: identifierExclusion(
		"the case declares a base URI and loads a remote resource",
	),
	{"refRemote.json", "remote ref with ref to definitions"}: identifierExclusion(
		"the case declares a base URI and loads a remote resource",
	),
	{"refRemote.json", "Location-independent identifier in remote ref"}: localReferenceExclusion(
		"the case requires a remote resource containing an anchor",
	),
	{"refRemote.json", "retrieved nested refs resolve relative to their URI not $id"}: identifierExclusion(
		"the case compares retrieval URI and identifier base semantics",
	),
	{"refRemote.json", "$ref to $ref finds location-independent $id"}: localReferenceExclusion(
		"the case requires a remote resource containing an identifier",
	),
}

func localReferenceExclusion(reason string) conformanceExclusion {
	return conformanceExclusion{
		policy:        policyLocalReferencesOnly,
		reason:        reason,
		errorContains: "non-local $ref",
	}
}

func identifierExclusion(reason string) conformanceExclusion {
	return conformanceExclusion{
		policy:        policyNoSchemaIdentifiers,
		reason:        reason,
		errorContains: "prohibited $id",
	}
}

func identifierReferenceExclusion(reason string) conformanceExclusion {
	return conformanceExclusion{
		policy:        policyNoSchemaIdentifiers,
		reason:        reason,
		errorContains: "non-local $ref",
	}
}

type draft7SuiteCase struct {
	Description string            `json:"description"`
	Schema      any               `json:"schema"`
	Tests       []draft7SuiteTest `json:"tests"`
}

type draft7SuiteTest struct {
	Description string `json:"description"`
	Data        any    `json:"data"`
	Valid       bool   `json:"valid"`
}

func TestDraft7MandatoryConformance(t *testing.T) {
	entries, err := os.ReadDir(draft7FixtureDirectory)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})

	digest := sha256.New()
	seenExclusions := make(map[conformanceCaseID]struct{}, len(draft7ConformanceExclusions))
	fixtureCount := 0
	caseCount := 0
	testCount := 0
	executedCaseCount := 0
	executedTestCount := 0
	excludedTestCount := 0

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			t.Fatalf("unexpected Draft 7 fixture entry %q", entry.Name())
		}
		fixtureCount++
		path := filepath.Join(draft7FixtureDirectory, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = digest.Write([]byte(entry.Name()))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(content)
		_, _ = digest.Write([]byte{0})

		cases := decodeDraft7SuiteCases(t, path, content)
		for _, testCase := range cases {
			caseCount++
			testCount += len(testCase.Tests)
			id := conformanceCaseID{file: entry.Name(), description: testCase.Description}
			if exclusion, excluded := draft7ConformanceExclusions[id]; excluded {
				if exclusion.policy == "" || exclusion.reason == "" || exclusion.errorContains == "" {
					t.Fatalf("incomplete exclusion for %s/%s", id.file, id.description)
				}
				seenExclusions[id] = struct{}{}
				excludedTestCount += len(testCase.Tests)
				if _, err := Compile(testCase.Schema); err == nil {
					t.Fatalf("stale exclusion %s/%s: schema now compiles under %s",
						id.file, id.description, exclusion.policy)
				} else if !strings.Contains(err.Error(), exclusion.errorContains) {
					t.Fatalf("exclusion %s/%s failed outside %s: %v",
						id.file, id.description, exclusion.policy, err)
				}
				continue
			}

			executedCaseCount++
			compiled, err := Compile(testCase.Schema)
			if err != nil {
				t.Fatalf("%s/%s: compile: %v", id.file, id.description, err)
			}
			for _, suiteTest := range testCase.Tests {
				executedTestCount++
				valid, err := compiled.ValidateFlag(suiteTest.Data)
				if err != nil {
					t.Fatalf("%s/%s/%s: validate: %v",
						id.file, id.description, suiteTest.Description, err)
				}
				if valid != suiteTest.Valid {
					t.Errorf("%s/%s/%s: got valid=%t, want %t",
						id.file, id.description, suiteTest.Description, valid, suiteTest.Valid)
				}
			}
		}
	}

	for id, exclusion := range draft7ConformanceExclusions {
		if _, seen := seenExclusions[id]; !seen {
			t.Errorf("unused exclusion %s/%s (%s)", id.file, id.description, exclusion.policy)
		}
	}
	if fixtureCount != draft7FixtureCount {
		t.Errorf("fixture count = %d, want %d", fixtureCount, draft7FixtureCount)
	}
	if caseCount != draft7CaseCount {
		t.Errorf("case count = %d, want %d", caseCount, draft7CaseCount)
	}
	if testCount != draft7TestCount {
		t.Errorf("test count = %d, want %d", testCount, draft7TestCount)
	}
	if len(draft7ConformanceExclusions) != draft7ExclusionCount {
		t.Errorf("exclusion count = %d, want %d", len(draft7ConformanceExclusions), draft7ExclusionCount)
	}
	if executedCaseCount != draft7ExecutedCases {
		t.Errorf("executed case count = %d, want %d", executedCaseCount, draft7ExecutedCases)
	}
	if executedTestCount+excludedTestCount != testCount {
		t.Errorf("accounted tests = %d, want %d", executedTestCount+excludedTestCount, testCount)
	}
	if executedTestCount != draft7ExecutedTests {
		t.Errorf("executed test count = %d, want %d", executedTestCount, draft7ExecutedTests)
	}
	if excludedTestCount != draft7ExcludedTests {
		t.Errorf("excluded test count = %d, want %d", excludedTestCount, draft7ExcludedTests)
	}
	if actual := hex.EncodeToString(digest.Sum(nil)); actual != draft7FixtureDigest {
		t.Errorf("fixture digest = %s, want %s", actual, draft7FixtureDigest)
	}
	t.Logf(
		"Draft 7 snapshot: fixtures=%d cases=%d executed_cases=%d excluded_cases=%d tests=%d executed_tests=%d excluded_tests=%d",
		fixtureCount, caseCount, executedCaseCount, len(draft7ConformanceExclusions),
		testCount, executedTestCount, excludedTestCount,
	)
}

func decodeDraft7SuiteCases(t *testing.T, path string, content []byte) []draft7SuiteCase {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var cases []draft7SuiteCase
	if err := decoder.Decode(&cases); err != nil {
		t.Fatalf("%s: decode: %v", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("%s: trailing JSON: %v", path, err)
	}
	for index, testCase := range cases {
		if testCase.Description == "" {
			t.Fatalf("%s: case %d has no description", path, index)
		}
		if len(testCase.Tests) == 0 {
			t.Fatalf("%s/%s has no tests", path, testCase.Description)
		}
		for testIndex, suiteTest := range testCase.Tests {
			if suiteTest.Description == "" {
				t.Fatalf("%s/%s: test %d has no description", path, testCase.Description, testIndex)
			}
		}
	}
	return cases
}
