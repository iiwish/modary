package jsonvalue

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	testMaxDepth       = 256
	testMaxNodes       = 65_536
	testMaxNumberBytes = 4_096
)

func TestDecodeEnforcesOneExactBoundedDocument(t *testing.T) {
	limits := testLimits()
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "invalid UTF-8", data: []byte{'"', 0xff, '"'}},
		{name: "duplicate member", data: []byte(`{"outer":{"id":1,"id":2}}`)},
		{name: "multiple values", data: []byte(`{} {}`)},
		{name: "invalid JSON", data: []byte(`{"open":`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(test.data, limits); err == nil {
				t.Fatalf("Decode(%q) succeeded", test.data)
			}
		})
	}

	value, err := Decode([]byte(`{"integer":9007199254740993}`), limits)
	if err != nil {
		t.Fatalf("Decode(valid): %v", err)
	}
	integer, ok := value.(map[string]any)["integer"].(json.Number)
	if !ok || string(integer) != "9007199254740993" {
		t.Fatalf("decoded integer = %#v", value)
	}
}

func TestDecodeDepthBoundaryCountsContainers(t *testing.T) {
	limits := testLimits()
	document := func(depth int) []byte {
		return []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
	}
	if err := Validate(document(testMaxDepth), limits); err != nil {
		t.Fatalf("%d nested containers: %v", testMaxDepth, err)
	}
	if err := Validate(document(testMaxDepth+1), limits); err == nil {
		t.Fatalf("%d nested containers succeeded", testMaxDepth+1)
	}
}

func TestDecodeNodeBoundaryCountsContainersAndValues(t *testing.T) {
	limits := testLimits()
	document := func(values int) []byte {
		return []byte("[" + strings.TrimSuffix(strings.Repeat("0,", values), ",") + "]")
	}
	if err := Validate(document(testMaxNodes-1), limits); err != nil {
		t.Fatalf("%d value nodes: %v", testMaxNodes, err)
	}
	if err := Validate(document(testMaxNodes), limits); err == nil {
		t.Fatalf("%d value nodes succeeded", testMaxNodes+1)
	}
}

func TestDecodeNumberBoundaryCountsSourceTokenBytes(t *testing.T) {
	limits := testLimits()
	exact := []byte("1" + strings.Repeat("0", testMaxNumberBytes-1))
	if err := Validate(exact, limits); err != nil {
		t.Fatalf("exact number byte limit: %v", err)
	}
	above := append(append([]byte(nil), exact...), '0')
	if err := Validate(above, limits); !IsLimit(err) {
		t.Fatalf("number above byte limit = %v", err)
	}
}

func TestDecodeByteAndConfigurationBoundaries(t *testing.T) {
	limits := Limits{MaxBytes: 2, MaxDepth: 1, MaxNodes: 1, MaxNumberBytes: 1}
	if err := Validate([]byte("0 "), limits); err != nil {
		t.Fatalf("exact byte limit: %v", err)
	}
	if err := Validate([]byte("0  "), limits); err == nil {
		t.Fatal("document above byte limit succeeded")
	}
	for _, invalid := range []Limits{
		{MaxDepth: 1, MaxNodes: 1, MaxNumberBytes: 1},
		{MaxBytes: 1, MaxNodes: 1, MaxNumberBytes: 1},
		{MaxBytes: 1, MaxDepth: 1, MaxNumberBytes: 1},
		{MaxBytes: 1, MaxDepth: 1, MaxNodes: 1},
	} {
		if err := Validate([]byte("0"), invalid); err == nil {
			t.Fatalf("invalid limits %#v succeeded", invalid)
		}
	}
}

func testLimits() Limits {
	return Limits{
		MaxBytes:       1 << 20,
		MaxDepth:       testMaxDepth,
		MaxNodes:       testMaxNodes,
		MaxNumberBytes: testMaxNumberBytes,
	}
}
