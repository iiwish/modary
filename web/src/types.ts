export type RuleSpec = {
  schema_version: string;
  id: string;
  name: string;
  source: { table: string; primary_key: string; field: string };
  operator: { type: string; filing_marker: string; parenthetical_note_target: string };
  output: { table: string; unique_key: string };
};

export type RuleSet = {
  id: string;
  workspace_id: string;
  name: string;
  draft_spec: RuleSpec;
  draft_hash: string;
  validated_hash?: string;
  state: "draft" | "published";
  current_version_id?: string;
  created_at: string;
  updated_at: string;
};

export type Evidence = { field: string; text: string; start: number; end: number };

export type AddressLabel = {
  registered_address: string;
  business_address: string;
  address_note: string;
  has_business_address_filing: boolean;
  address_quality_tag: string;
  evidence: Evidence[];
};

export type PlannedLabel = {
  company_id: string;
  company_name: string;
  license_address: string;
  updated_at: string;
  label: AddressLabel;
  changed: boolean;
  rejected: boolean;
  reason?: string;
};

export type PreviewSummary = {
  matched_rows: number;
  writable_rows: number;
  rejected_rows: number;
  unchanged_rows: number;
  target_table: string;
  sample_results: PlannedLabel[];
  warnings: string[];
};

export type RuleVersion = {
  id: string;
  ruleset_id: string;
  workspace_id: string;
  version: number;
  spec: RuleSpec;
  spec_hash: string;
  published_by: string;
  published_at: string;
};

export type Run = {
  id: string;
  workspace_id: string;
  rule_version_id: string;
  status: string;
  matched_rows: number;
  written_rows: number;
  rejected_rows: number;
  started_at: string;
  finished_at: string;
  result_offset: number;
  result_limit: number;
  results: PlannedLabel[];
};

export const defaultRuleSpec: RuleSpec = {
  schema_version: "rulary.ruleset.f0",
  id: "company-address",
  name: "企业地址标签",
  source: { table: "company_license", primary_key: "company_id", field: "license_address" },
  operator: {
    type: "rulary.address.extract_v1",
    filing_marker: "经营地址备案",
    parenthetical_note_target: "address_note",
  },
  output: { table: "company_address_labels", unique_key: "company_id" },
};
