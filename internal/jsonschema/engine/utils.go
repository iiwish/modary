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
// description      Various utility functions.
//
// created          26-02-2013

package gojsonschema

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func isKind(what interface{}, kinds ...reflect.Kind) bool {
	target := what
	if isJSONNumber(what) {
		// JSON Numbers are strings!
		target = *mustBeNumber(what)
	}
	targetKind := reflect.ValueOf(target).Kind()
	for _, kind := range kinds {
		if targetKind == kind {
			return true
		}
	}
	return false
}

func existsMapKey(m map[string]interface{}, k string) bool {
	_, ok := m[k]
	return ok
}

func isStringInSlice(s []string, what string) bool {
	for i := range s {
		if s[i] == what {
			return true
		}
	}
	return false
}

// indexStringInSlice returns the index of the first instance of 'what' in s or -1 if it is not found in s.
func indexStringInSlice(s []string, what string) int {
	for i := range s {
		if s[i] == what {
			return i
		}
	}
	return -1
}

func marshalToJSONString(value interface{}) (*string, error) {

	mBytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	sBytes := string(mBytes)
	return &sBytes, nil
}

func marshalWithoutNumber(value interface{}) (*string, error) {
	return marshalWithoutNumberBudget(value, nil)
}

func marshalWithoutNumberBudget(value interface{}, budget *evaluationBudget) (*string, error) {
	var output strings.Builder
	if err := writeCanonicalValue(&output, value, budget); err != nil {
		return nil, err
	}
	result := output.String()
	return &result, nil
}

func writeCanonicalValue(output *strings.Builder, value interface{}, budget *evaluationBudget) error {
	budget.consumeWork(1)
	switch typed := value.(type) {
	case nil:
		output.WriteByte('z')
	case bool:
		if typed {
			output.WriteString("b1")
		} else {
			output.WriteString("b0")
		}
	case string:
		budget.consumeWork(uint64(len(typed)))
		writeLengthPrefixed(output, 's', typed)
	case json.Number:
		budget.consumeWork(numberWork(typed))
		normalized, err := canonicalNumber(string(typed))
		if err != nil {
			return err
		}
		writeLengthPrefixed(output, 'n', normalized)
	case float64:
		return writeCanonicalNumber(output, strconv.FormatFloat(typed, 'g', -1, 64), budget)
	case float32:
		return writeCanonicalNumber(output, strconv.FormatFloat(float64(typed), 'g', -1, 32), budget)
	case int:
		return writeCanonicalNumber(output, strconv.FormatInt(int64(typed), 10), budget)
	case int8:
		return writeCanonicalNumber(output, strconv.FormatInt(int64(typed), 10), budget)
	case int16:
		return writeCanonicalNumber(output, strconv.FormatInt(int64(typed), 10), budget)
	case int32:
		return writeCanonicalNumber(output, strconv.FormatInt(int64(typed), 10), budget)
	case int64:
		return writeCanonicalNumber(output, strconv.FormatInt(typed, 10), budget)
	case uint:
		return writeCanonicalNumber(output, strconv.FormatUint(uint64(typed), 10), budget)
	case uint8:
		return writeCanonicalNumber(output, strconv.FormatUint(uint64(typed), 10), budget)
	case uint16:
		return writeCanonicalNumber(output, strconv.FormatUint(uint64(typed), 10), budget)
	case uint32:
		return writeCanonicalNumber(output, strconv.FormatUint(uint64(typed), 10), budget)
	case uint64:
		return writeCanonicalNumber(output, strconv.FormatUint(typed, 10), budget)
	case []interface{}:
		output.WriteByte('a')
		output.WriteString(strconv.Itoa(len(typed)))
		output.WriteByte(':')
		for _, nested := range typed {
			if err := writeCanonicalValue(output, nested, budget); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		budget.consumeWork(sortWork(len(names)))
		sort.Strings(names)
		output.WriteByte('o')
		output.WriteString(strconv.Itoa(len(names)))
		output.WriteByte(':')
		for _, name := range names {
			budget.consumeWork(uint64(len(name)))
			writeLengthPrefixed(output, 'k', name)
			if err := writeCanonicalValue(output, typed[name], budget); err != nil {
				return err
			}
		}
	default:
		converted := convertDocumentNode(value)
		if reflect.TypeOf(converted) != reflect.TypeOf(value) {
			return writeCanonicalValue(output, converted, budget)
		}
		return fmt.Errorf("value of type %T is not JSON", value)
	}
	return nil
}

func writeCanonicalNumber(output *strings.Builder, value string, budget *evaluationBudget) error {
	budget.consumeWork(saturatingAdd(1, saturatingMultiply(uint64(len(value)), uint64(len(value)))))
	normalized, err := canonicalNumber(value)
	if err != nil {
		return err
	}
	writeLengthPrefixed(output, 'n', normalized)
	return nil
}

func canonicalNumber(value string) (string, error) {
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	mantissa := value
	exponent := new(big.Int)
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		mantissa = value[:index]
		if _, ok := exponent.SetString(value[index+1:], 10); !ok {
			return "", fmt.Errorf("invalid JSON number exponent")
		}
	}
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	coefficient := strings.TrimLeft(mantissa, "0")
	if coefficient == "" {
		return "0e0", nil
	}
	exponent.Sub(exponent, big.NewInt(int64(fractionDigits)))
	trimmed := strings.TrimRight(coefficient, "0")
	exponent.Add(exponent, big.NewInt(int64(len(coefficient)-len(trimmed))))
	if negative {
		trimmed = "-" + trimmed
	}
	return trimmed + "e" + exponent.String(), nil
}

func writeLengthPrefixed(output *strings.Builder, tag byte, value string) {
	output.WriteByte(tag)
	output.WriteString(strconv.Itoa(len(value)))
	output.WriteByte(':')
	output.WriteString(value)
}

func sortWork(length int) uint64 {
	if length < 2 {
		return uint64(length)
	}
	bits := 0
	for size := length; size > 0; size >>= 1 {
		bits++
	}
	return saturatingMultiply(uint64(length), uint64(bits))
}

func isJSONNumber(what interface{}) bool {

	switch what.(type) {

	case json.Number:
		return true
	}

	return false
}

func checkJSONInteger(what interface{}) (isInt bool) {
	jsonNumber := what.(json.Number)
	normalized, err := canonicalNumber(string(jsonNumber))
	if err != nil || normalized == "0e0" {
		return err == nil
	}
	index := strings.LastIndexByte(normalized, 'e')
	if index < 0 {
		return false
	}
	exponent := new(big.Int)
	if _, ok := exponent.SetString(normalized[index+1:], 10); !ok {
		return false
	}
	return exponent.Sign() >= 0
}

// same as ECMA Number.MAX_SAFE_INTEGER and Number.MIN_SAFE_INTEGER
const (
	maxJSONFloat = float64(1<<53 - 1)  // 9007199254740991.0 	 2^53 - 1
	minJSONFloat = -float64(1<<53 - 1) //-9007199254740991.0	-2^53 - 1
)

func mustBeInteger(what interface{}) *int {
	if !isJSONNumber(what) {
		return nil
	}
	normalized, err := canonicalNumber(string(what.(json.Number)))
	if err != nil {
		return nil
	}
	if normalized == "0e0" {
		zero := 0
		return &zero
	}
	index := strings.LastIndexByte(normalized, 'e')
	if index < 0 {
		return nil
	}
	coefficient := normalized[:index]
	exponent := new(big.Int)
	if _, ok := exponent.SetString(normalized[index+1:], 10); !ok || exponent.Sign() < 0 {
		return nil
	}

	negative := strings.HasPrefix(coefficient, "-")
	digits := strings.TrimPrefix(coefficient, "-")
	maxInt := int(^uint(0) >> 1)
	maxDigits := len(strconv.Itoa(maxInt))
	if !exponent.IsInt64() || exponent.Int64() > int64(maxDigits) {
		return saturatedInteger(negative)
	}
	zeros := int(exponent.Int64())
	if len(digits)+zeros > maxDigits {
		return saturatedInteger(negative)
	}
	token := coefficient + strings.Repeat("0", zeros)
	parsed, err := strconv.ParseInt(token, 10, strconv.IntSize)
	if err != nil {
		return saturatedInteger(negative)
	}
	value := int(parsed)
	return &value
}

func saturatedInteger(negative bool) *int {
	maxInt := int(^uint(0) >> 1)
	value := maxInt
	if negative {
		value = -maxInt - 1
	}
	return &value
}

func mustBeNumber(what interface{}) *big.Rat {

	if isJSONNumber(what) {
		number := what.(json.Number)
		float64Value, success := new(big.Rat).SetString(string(number))
		if success {
			return float64Value
		}
	}

	return nil

}

func convertDocumentNode(val interface{}) interface{} {

	if lval, ok := val.([]interface{}); ok {

		res := []interface{}{}
		for _, v := range lval {
			res = append(res, convertDocumentNode(v))
		}

		return res

	}

	if mval, ok := val.(map[interface{}]interface{}); ok {

		res := map[string]interface{}{}

		for k, v := range mval {
			res[k.(string)] = convertDocumentNode(v)
		}

		return res

	}

	return val
}
