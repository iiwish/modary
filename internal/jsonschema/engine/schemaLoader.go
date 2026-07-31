// Copyright 2018 johandorland ( https://github.com/johandorland )
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gojsonschema

import (
	"fmt"
	"strings"
)

// SchemaLoader is used to load schemas
type SchemaLoader struct {
	pool       *schemaPool
	AutoDetect bool
	Validate   bool
	Draft      Draft
}

// NewSchemaLoader creates a new NewSchemaLoader
func NewSchemaLoader() *SchemaLoader {

	ps := &SchemaLoader{
		pool: &schemaPool{
			schemaPoolDocuments: make(map[string]*schemaPoolDocument),
		},
		AutoDetect: true,
		Validate:   false,
		Draft:      Hybrid,
	}
	ps.pool.autoDetect = &ps.AutoDetect

	return ps
}

func (sl *SchemaLoader) validateMetaschema(documentNode interface{}) error {
	var schema string
	if sl.AutoDetect {
		var err error
		schema, _, err = parseSchemaURL(documentNode)
		if err != nil {
			return err
		}
	}

	// If no explicit "$schema" is used, use the default metaschema associated with the draft used
	if schema == "" {
		if sl.Draft == Hybrid {
			return nil
		}
		schema = drafts.GetSchemaURL(sl.Draft)
	}
	source := drafts.GetMetaSchema(schema)
	draft := drafts.GetDraftVersion(schema)
	if source == "" || draft == nil {
		return fmt.Errorf("JSON Schema metaschema %q is not embedded", schema)
	}
	var (
		metaSchema *Schema
		err        error
	)
	if *draft == Draft7 {
		metaSchema, err = NewDraft7MetaSchema()
	} else {
		var metaDocument interface{}
		metaDocument, err = decodeJSONUsingNumber(strings.NewReader(source))
		if err == nil {
			metaSchema, err = NewLocalSchema(metaDocument, *draft)
		}
	}
	if err != nil {
		return err
	}
	valid, err := metaSchema.ValidateFlag(documentNode, Budget{
		MaxWorkUnits:   64 << 20,
		MaxDiagnostics: 4_096,
		MaxFrames:      DefaultMaxEvaluationFrames,
	})
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("schema does not validate against metaschema %q", schema)
	}
	return nil
}

// Compile loads and compiles a schema
func (sl *SchemaLoader) Compile(rootSchema JSONLoader) (*Schema, error) {

	ref, err := rootSchema.JsonReference()

	if err != nil {
		return nil, err
	}

	d := Schema{}
	d.pool = sl.pool
	d.documentReference = ref
	d.referencePool = newSchemaReferencePool()

	var doc interface{}
	if ref.String() != "" {
		// Get document from schema pool
		spd, err := d.pool.GetDocument(d.documentReference)
		if err != nil {
			return nil, err
		}
		doc = spd.Document
	} else {
		// Load JSON directly
		doc, err = rootSchema.LoadJSON()
		if err != nil {
			return nil, err
		}
		err = sl.pool.registerRoot(doc)
		if err != nil {
			return nil, err
		}
	}

	if sl.Validate {
		if err := sl.validateMetaschema(doc); err != nil {
			return nil, err
		}
	}

	draft := sl.Draft
	if sl.AutoDetect {
		_, detectedDraft, err := parseSchemaURL(doc)
		if err != nil {
			return nil, err
		}
		if detectedDraft != nil {
			draft = *detectedDraft
		}
	}

	err = d.parse(doc, draft)
	if err != nil {
		return nil, err
	}

	return &d, nil
}
