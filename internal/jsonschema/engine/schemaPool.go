// Copyright 2015 xeipuuv ( https://github.com/xeipuuv )
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

// author           xeipuuv
// author-github    https://github.com/xeipuuv
// author-mail      xeipuuv@gmail.com
//
// repository-name  gojsonschema
// repository-desc  An implementation of JSON Schema, based on IETF's draft v4 - Go language.
//
// description		Registers one in-memory root document and resolves local
//                  JSON Pointers within it.
//
// created          26-02-2013

package gojsonschema

import (
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonreference"
)

type schemaPoolDocument struct {
	Document interface{}
	Draft    *Draft
}

type schemaPool struct {
	schemaPoolDocuments map[string]*schemaPoolDocument
	autoDetect          *bool
}

func (p *schemaPool) registerRoot(document interface{}) error {
	if _, exists := p.schemaPoolDocuments[""]; exists {
		return fmt.Errorf("JSON Schema root document is already registered")
	}
	var draft *Draft
	if p.autoDetect != nil && *p.autoDetect {
		var err error
		_, draft, err = parseSchemaURL(document)
		if err != nil {
			return err
		}
	}
	p.schemaPoolDocuments[""] = &schemaPoolDocument{Document: document, Draft: draft}
	return nil
}

func (p *schemaPool) GetDocument(reference gojsonreference.JsonReference) (*schemaPoolDocument, error) {
	if internalLogEnabled {
		internalLog("Get Document ( %s )", reference.String())
	}

	parsed := reference.GetUrl()
	if parsed.Scheme != "" || parsed.Host != "" || parsed.Path != "" ||
		parsed.RawPath != "" || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.OmitHost || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Fragment != "" && !strings.HasPrefix(parsed.Fragment, "/") {
		return nil, fmt.Errorf("JSON Schema reference %q is not a local JSON Pointer", reference.String())
	}

	root, exists := p.schemaPoolDocuments[""]
	if !exists {
		return nil, fmt.Errorf("JSON Schema root document is not registered")
	}
	if parsed.Fragment == "" {
		return root, nil
	}

	document, _, err := reference.GetPointer().Get(root.Document)
	if err != nil {
		return nil, fmt.Errorf("resolve local JSON Schema reference %q: %w", reference.String(), err)
	}
	return &schemaPoolDocument{Document: document, Draft: root.Draft}, nil
}
