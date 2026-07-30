package rulary_core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	RuleSpecSchema  = "rulary.ruleset.f0"
	AddressOperator = "rulary.address.extract_v1"
	SourceTable     = "company_license"
	TargetTable     = "company_address_labels"
)

type RuleSpec struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        struct {
		Table      string `json:"table"`
		PrimaryKey string `json:"primary_key"`
		Field      string `json:"field"`
	} `json:"source"`
	Operator struct {
		Type                    string `json:"type"`
		FilingMarker            string `json:"filing_marker"`
		ParentheticalNoteTarget string `json:"parenthetical_note_target"`
	} `json:"operator"`
	Output struct {
		Table     string `json:"table"`
		UniqueKey string `json:"unique_key"`
	} `json:"output"`
}

func DefaultRuleSpec() RuleSpec {
	var spec RuleSpec
	spec.SchemaVersion = RuleSpecSchema
	spec.ID = "company-address"
	spec.Name = "Company address labels"
	spec.Source.Table = SourceTable
	spec.Source.PrimaryKey = "company_id"
	spec.Source.Field = "license_address"
	spec.Operator.Type = AddressOperator
	spec.Operator.FilingMarker = "经营地址备案"
	spec.Operator.ParentheticalNoteTarget = "address_note"
	spec.Output.Table = TargetTable
	spec.Output.UniqueKey = "company_id"
	return spec
}

func ParseRuleSpec(data []byte) (RuleSpec, error) {
	var spec RuleSpec
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return RuleSpec{}, fmt.Errorf("invalid RuleSpec: %w", err)
	}
	if err := ValidateRuleSpec(spec); err != nil {
		return RuleSpec{}, err
	}
	return spec, nil
}

func ValidateRuleSpec(spec RuleSpec) error {
	checks := []struct {
		valid   bool
		message string
	}{
		{spec.SchemaVersion == RuleSpecSchema, "schema_version must be rulary.ruleset.f0"},
		{strings.TrimSpace(spec.ID) != "", "id is required"},
		{strings.TrimSpace(spec.Name) != "", "name is required"},
		{spec.Source.Table == SourceTable, "F0 source table must be company_license"},
		{spec.Source.PrimaryKey == "company_id", "F0 source primary key must be company_id"},
		{spec.Source.Field == "license_address", "F0 source field must be license_address"},
		{spec.Operator.Type == AddressOperator, "unsupported address operator"},
		{spec.Operator.FilingMarker != "", "filing_marker is required"},
		{spec.Operator.ParentheticalNoteTarget == "address_note", "parenthetical_note_target must be address_note"},
		{spec.Output.Table == TargetTable, "F0 output table must be company_address_labels"},
		{spec.Output.UniqueKey == "company_id", "F0 output unique key must be company_id"},
	}
	for _, check := range checks {
		if !check.valid {
			return fmt.Errorf("%s", check.message)
		}
	}
	return nil
}

func RuleSpecHash(data []byte) (string, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

type AddressLabel struct {
	RegisteredAddress        string     `json:"registered_address"`
	BusinessAddress          string     `json:"business_address"`
	AddressNote              string     `json:"address_note"`
	HasBusinessAddressFiling bool       `json:"has_business_address_filing"`
	AddressQualityTag        string     `json:"address_quality_tag"`
	Evidence                 []Evidence `json:"evidence"`
}

type Evidence struct {
	Field string `json:"field"`
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

var parentheticalPattern = regexp.MustCompile(`（([^（）]*)）|\(([^()]*)\)`)

func ExtractAddressLabel(input, filingMarker string) AddressLabel {
	label := AddressLabel{Evidence: make([]Evidence, 0)}
	matches := parentheticalPattern.FindAllStringSubmatchIndex(input, -1)
	firstParenStart := len(input)
	if len(matches) > 0 {
		firstParenStart = matches[0][0]
	}
	registered := strings.TrimSpace(input[:firstParenStart])
	registered = strings.TrimRight(registered, "；;，,、 \t\r\n")
	label.RegisteredAddress = registered
	if registered != "" {
		label.Evidence = append(label.Evidence, Evidence{Field: "registered_address", Text: registered, Start: 0, End: len([]rune(registered))})
	}

	for _, match := range matches {
		contentStart, contentEnd := match[2], match[3]
		if contentStart < 0 {
			contentStart, contentEnd = match[4], match[5]
		}
		content := strings.TrimSpace(input[contentStart:contentEnd])
		markerPrefix := filingMarker
		if strings.HasPrefix(content, markerPrefix) {
			business := strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(content, markerPrefix), "：:"))
			if business != "" {
				label.BusinessAddress = business
				label.HasBusinessAddressFiling = true
				label.AddressQualityTag = "含经营地址备案"
				start := runeOffset(input, contentStart) + len([]rune(content)) - len([]rune(business))
				label.Evidence = append(label.Evidence, Evidence{Field: "business_address", Text: business, Start: start, End: start + len([]rune(business))})
			}
			continue
		}
		if label.AddressNote == "" && content != "" {
			label.AddressNote = content
			start := runeOffset(input, contentStart)
			label.Evidence = append(label.Evidence, Evidence{Field: "address_note", Text: content, Start: start, End: start + len([]rune(content))})
		}
	}
	if label.AddressQualityTag == "" {
		label.AddressQualityTag = "仅注册地址"
	}
	return label
}

func runeOffset(value string, byteOffset int) int {
	return len([]rune(value[:byteOffset]))
}
