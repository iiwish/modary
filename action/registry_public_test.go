package action_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/iiwish/modary/action"
)

func TestCatalogDoesNotExposeHandlers(t *testing.T) {
	entry := action.CatalogEntry{}
	if _, exposed := reflect.TypeOf(entry).FieldByName("Handler"); exposed {
		t.Fatal("public catalog exposes an Action handler")
	}
}

func TestChannelIsOpenAndPreservesStringWireValues(t *testing.T) {
	if action.ChannelCLI != "cli" || action.ChannelHTTP != "http" || action.ChannelMCP != "mcp" {
		t.Fatalf("standard Channel constants changed: %q %q %q", action.ChannelCLI, action.ChannelHTTP, action.ChannelMCP)
	}

	const desktop action.Channel = "desktop"
	descriptor := action.Descriptor{
		ID:           "prepared.desktop",
		Version:      "1.0.0",
		Title:        "Desktop action",
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(nil).JSON(),
		Permission:   "prepared.desktop",
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []action.Channel{desktop},
	}
	if _, err := action.PrepareDescriptor(descriptor); err != nil {
		t.Fatalf("custom Channel rejected: %v", err)
	}

	requestJSON, err := json.Marshal(action.Request{Channel: action.ChannelHTTP})
	if err != nil {
		t.Fatal(err)
	}
	var requestWire struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(requestJSON, &requestWire); err != nil {
		t.Fatal(err)
	}
	if requestWire.Channel != "http" {
		t.Fatalf("Request channel wire value = %q", requestWire.Channel)
	}
	var decodedRequest action.Request
	if err := json.Unmarshal([]byte(`{"channel":"desktop"}`), &decodedRequest); err != nil {
		t.Fatal(err)
	}
	if decodedRequest.Channel != desktop {
		t.Fatalf("decoded Request channel = %q", decodedRequest.Channel)
	}

	planJSON, err := json.Marshal(action.Plan{Channel: action.ChannelMCP})
	if err != nil {
		t.Fatal(err)
	}
	var planWire struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(planJSON, &planWire); err != nil {
		t.Fatal(err)
	}
	if planWire.Channel != "mcp" {
		t.Fatalf("Plan channel wire value = %q", planWire.Channel)
	}

	descriptorJSON, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	var descriptorWire struct {
		Channels []string `json:"channels"`
	}
	if err := json.Unmarshal(descriptorJSON, &descriptorWire); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(descriptorWire.Channels, []string{"desktop"}) {
		t.Fatalf("Descriptor channel wire values = %#v", descriptorWire.Channels)
	}
}

func TestPreparedDescriptorOwnsStaticContract(t *testing.T) {
	descriptor := action.Descriptor{
		ID:           "prepared.read",
		Version:      "0.1.0",
		Title:        "Prepared read",
		InputSchema:  action.Object(nil).JSON(),
		OutputSchema: action.Object(nil).JSON(),
		Permission:   "prepared.read",
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []action.Channel{action.ChannelHTTP},
	}
	prepared, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	descriptor.Channels[0] = "forged"
	first := prepared.Descriptor()
	first.Channels[0] = "mutated-copy"
	if got := prepared.Descriptor().Channels[0]; got != "http" {
		t.Fatalf("prepared descriptor mutated through caller: %q", got)
	}
}

func TestContractHashIsCanonicalAndBindsGovernanceSemantics(t *testing.T) {
	descriptor := action.Descriptor{
		ID: "prepared.write", Version: "1.0.0", Title: "Prepared write",
		InputSchema:  json.RawMessage(`{ "type": "object", "additionalProperties": false }`),
		OutputSchema: json.RawMessage(`{"additionalProperties":false,"type":"object"}`),
		Permission:   "prepared.write", Preview: action.PreviewNone, AuditLevel: action.AuditDetailed,
		Channels: []action.Channel{action.ChannelMCP, action.ChannelHTTP}, RequiresIdempotency: true,
	}
	first, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	descriptor.InputSchema = json.RawMessage(`{"additionalProperties":false,"type":"object"}`)
	descriptor.Channels = []action.Channel{action.ChannelHTTP, action.ChannelMCP}
	canonical, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContractHash() != canonical.ContractHash() {
		t.Fatalf("equivalent contracts hashed differently: %s != %s", first.ContractHash(), canonical.ContractHash())
	}
	descriptor.AuditLevel = action.AuditMetadata
	changed, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ContractHash() == canonical.ContractHash() {
		t.Fatal("audit governance change did not change the contract hash")
	}
}

func TestContractHashCanonicalizesSchemaNumbersAndDraftDeclaration(t *testing.T) {
	descriptor := action.Descriptor{
		ID:           "prepared.number",
		Version:      "1.0.0",
		Title:        "Prepared number",
		InputSchema:  json.RawMessage(`{"type":"number","minimum":1}`),
		OutputSchema: action.Object(nil).JSON(),
		Permission:   "prepared.number",
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []action.Channel{action.ChannelHTTP},
	}
	variants := []json.RawMessage{
		json.RawMessage(`{"type":"number","minimum":1}`),
		json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","minimum":1.0,"type":"number"}`),
		json.RawMessage(`{"$schema":"https://json-schema.org/draft-07/schema","type":"number","minimum":1e0}`),
	}
	var canonicalHash string
	for index, schema := range variants {
		descriptor.InputSchema = schema
		prepared, err := action.PrepareDescriptor(descriptor)
		if err != nil {
			t.Fatalf("prepare variant %d: %v", index, err)
		}
		if index == 0 {
			canonicalHash = prepared.ContractHash()
		} else if prepared.ContractHash() != canonicalHash {
			t.Fatalf("schema variant %d hashed differently: %s != %s", index, prepared.ContractHash(), canonicalHash)
		}
	}

	descriptor.InputSchema = json.RawMessage(`{"type":"number","minimum":2}`)
	changed, err := action.PrepareDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ContractHash() == canonicalHash {
		t.Fatal("different numeric schema constraint did not change the contract hash")
	}
}
