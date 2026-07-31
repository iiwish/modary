package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/internal/jsonvalue"
	"github.com/iiwish/modary/internal/safeerr"
)

const (
	maxProtocolJSONDepth = action.MaxJSONNestingDepth * 2
	maxProtocolJSONNodes = action.MaxJSONValueNodes * 2
)

var (
	errBodyTooLarge = errors.New("request body exceeds the configured limit")
	errInvalidJSON  = errors.New("request body must contain exactly one JSON object")
)

func decodeRequestJSON(writer http.ResponseWriter, request *http.Request, limit int64, target any) error {
	if request.ContentLength > limit {
		return errBodyTooLarge
	}
	body := http.MaxBytesReader(writer, request.Body, limit)
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		if _, ok := safeerr.Find[*http.MaxBytesError](err); ok {
			return errBodyTooLarge
		}
		if contextErr := request.Context().Err(); contextErr != nil {
			return contextErr
		}
		return wrapDependencyError("read HTTP request body", err)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || !utf8.Valid(trimmed) || trimmed[0] != '{' {
		return errInvalidJSON
	}
	if err := validateSingleJSONWithin(trimmed, limit); err != nil {
		return errInvalidJSON
	}
	if err := validateExactTopLevelFields(trimmed, target); err != nil {
		return errInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return errInvalidJSON
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errInvalidJSON
	}
	return nil
}

func validateSingleJSON(data []byte) error {
	return validateSingleJSONWithin(data, MaximumBodyBytes)
}

func validateSingleJSONWithin(data []byte, maxBytes int64) error {
	return jsonvalue.Validate(data, protocolJSONLimits(maxBytes))
}

func protocolJSONLimits(maxBytes int64) jsonvalue.Limits {
	return jsonvalue.Limits{
		MaxBytes: maxBytes,
		MaxDepth: maxProtocolJSONDepth,
		MaxNodes: maxProtocolJSONNodes,
		// The protocol envelope must not consume the tighter Action value
		// budget. Once input is extracted, Runtime applies the Action number
		// limit and owns its error classification and rejected audit event.
		MaxNumberBytes: int(maxBytes),
	}
}

func acceptsJSON(request *http.Request) bool {
	values := request.Header.Values("Accept")
	if len(values) == 0 {
		return true
	}
	items, ok := splitHTTPList(values)
	if !ok {
		return false
	}
	bestTypeSpecificity := -1
	bestParameterSpecificity := -1
	bestQuality := 0.0
	for _, item := range items {
		mediaType, parameters, err := mime.ParseMediaType(strings.TrimSpace(item))
		if err != nil {
			return false
		}
		quality := 1.0
		if raw, exists := parameters["q"]; exists {
			quality, err = strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(quality) || math.IsInf(quality, 0) || quality < 0 || quality > 1 {
				return false
			}
			delete(parameters, "q")
		}
		typeSpecificity := -1
		switch mediaType {
		case "application/json":
			typeSpecificity = 2
		case "application/*":
			typeSpecificity = 1
		case "*/*":
			typeSpecificity = 0
		}
		if typeSpecificity < 0 || !matchesJSONParameters(parameters) {
			continue
		}
		parameterSpecificity := len(parameters)
		if typeSpecificity > bestTypeSpecificity ||
			(typeSpecificity == bestTypeSpecificity && parameterSpecificity > bestParameterSpecificity) {
			bestTypeSpecificity = typeSpecificity
			bestParameterSpecificity = parameterSpecificity
			bestQuality = quality
		} else if typeSpecificity == bestTypeSpecificity && parameterSpecificity == bestParameterSpecificity && quality > bestQuality {
			bestQuality = quality
		}
	}
	return bestTypeSpecificity >= 0 && bestQuality > 0
}

func matchesJSONParameters(parameters map[string]string) bool {
	for name, value := range parameters {
		if !strings.EqualFold(name, "charset") || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func splitHTTPList(values []string) ([]string, bool) {
	joined := strings.Join(values, ",")
	items := make([]string, 0, strings.Count(joined, ",")+1)
	start := 0
	quoted := false
	escaped := false
	for index := range len(joined) {
		character := joined[index]
		if escaped {
			escaped = false
			continue
		}
		if quoted && character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			quoted = !quoted
			continue
		}
		if character == ',' && !quoted {
			item := strings.TrimSpace(joined[start:index])
			if item == "" {
				return nil, false
			}
			items = append(items, item)
			start = index + 1
		}
	}
	if quoted || escaped {
		return nil, false
	}
	last := strings.TrimSpace(joined[start:])
	if last == "" {
		return nil, false
	}
	return append(items, last), true
}

func hasJSONContentType(request *http.Request) bool {
	values := request.Header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(values[0])
	if err != nil || mediaType != "application/json" {
		return false
	}
	for name, value := range parameters {
		if !strings.EqualFold(name, "charset") || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func requestHasNoBody(request *http.Request) bool {
	return request.ContentLength == 0 && len(request.TransferEncoding) == 0
}

func validateExactTopLevelFields(data []byte, target any) error {
	targetType := reflect.TypeOf(target)
	if targetType == nil {
		return errors.New("JSON target is required")
	}
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	if targetType.Kind() != reflect.Struct {
		return errors.New("JSON target must point to a struct")
	}
	allowed := make(map[string]struct{})
	collectJSONFieldNames(targetType, allowed)
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	for name := range object {
		if _, ok := allowed[name]; !ok {
			return fmt.Errorf("unknown object member %q", name)
		}
	}
	return nil
}

func collectJSONFieldNames(targetType reflect.Type, names map[string]struct{}) {
	for index := range targetType.NumField() {
		field := targetType.Field(index)
		if !field.IsExported() && !field.Anonymous {
			continue
		}
		tag := field.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && name == "" && fieldType.Kind() == reflect.Struct {
			collectJSONFieldNames(fieldType, names)
			continue
		}
		if name == "" {
			name = field.Name
		}
		names[name] = struct{}{}
	}
}
