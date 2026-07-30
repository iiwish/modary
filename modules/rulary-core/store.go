package rulary_core

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"modary/core/action"
	"modary/core/database"
)

type Store struct{ db *sql.DB }

type RuleSet struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id"`
	Name             string          `json:"name"`
	DraftSpec        json.RawMessage `json:"draft_spec"`
	DraftHash        string          `json:"draft_hash"`
	ValidatedHash    string          `json:"validated_hash,omitempty"`
	State            string          `json:"state"`
	CurrentVersionID string          `json:"current_version_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type RuleVersion struct {
	ID          string          `json:"id"`
	RuleSetID   string          `json:"ruleset_id"`
	WorkspaceID string          `json:"workspace_id"`
	Version     int             `json:"version"`
	Spec        json.RawMessage `json:"spec"`
	SpecHash    string          `json:"spec_hash"`
	PublishedBy string          `json:"published_by"`
	PublishedAt time.Time       `json:"published_at"`
}

type CompanySource struct {
	CompanyID      string `json:"company_id"`
	CompanyName    string `json:"company_name"`
	LicenseAddress string `json:"license_address"`
	UpdatedAt      string `json:"updated_at"`
}

type PlannedLabel struct {
	CompanySource
	Label    AddressLabel `json:"label"`
	Changed  bool         `json:"changed"`
	Rejected bool         `json:"rejected"`
	Reason   string       `json:"reason,omitempty"`
}

type Run struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspace_id"`
	RuleVersionID string         `json:"rule_version_id"`
	Status        string         `json:"status"`
	MatchedRows   int            `json:"matched_rows"`
	WrittenRows   int            `json:"written_rows"`
	RejectedRows  int            `json:"rejected_rows"`
	StartedAt     time.Time      `json:"started_at"`
	FinishedAt    time.Time      `json:"finished_at"`
	ResultOffset  int            `json:"result_offset"`
	ResultLimit   int            `json:"result_limit"`
	Results       []PlannedLabel `json:"results"`
}

func (s *Store) ListRuleSets(ctx context.Context, workspaceID string, limit int) ([]RuleSet, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := database.ExecutorFor(ctx, s.db).QueryContext(ctx, `
		SELECT ruleset_id, workspace_id, name, draft_spec, draft_hash, validated_hash,
		       state, current_version_id, created_at, updated_at
		FROM rulary_ruleset WHERE workspace_id = ? ORDER BY updated_at DESC LIMIT ?`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RuleSet, 0)
	for rows.Next() {
		item, err := scanRuleSet(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetRuleSet(ctx context.Context, workspaceID, id string) (RuleSet, error) {
	return scanRuleSet(database.ExecutorFor(ctx, s.db).QueryRowContext(ctx, `
		SELECT ruleset_id, workspace_id, name, draft_spec, draft_hash, validated_hash,
		       state, current_version_id, created_at, updated_at
		FROM rulary_ruleset WHERE workspace_id = ? AND ruleset_id = ?`, workspaceID, id))
}

type rowScanner interface{ Scan(...any) error }

func scanRuleSet(row rowScanner) (RuleSet, error) {
	var item RuleSet
	var validated, current sql.NullString
	var created, updated string
	if err := row.Scan(&item.ID, &item.WorkspaceID, &item.Name, &item.DraftSpec, &item.DraftHash,
		&validated, &item.State, &current, &created, &updated); err != nil {
		return RuleSet{}, err
	}
	item.ValidatedHash = validated.String
	item.CurrentVersionID = current.String
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return item, nil
}

func (s *Store) CreateRuleSet(ctx context.Context, workspaceID, name string, spec json.RawMessage) (RuleSet, error) {
	hash, err := RuleSpecHash(spec)
	if err != nil {
		return RuleSet{}, err
	}
	id := newID("rs")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		INSERT INTO rulary_ruleset
		(ruleset_id, workspace_id, name, draft_spec, draft_hash, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'draft', ?, ?)`, id, workspaceID, name, []byte(spec), hash, now, now)
	if err != nil {
		return RuleSet{}, err
	}
	return s.GetRuleSet(ctx, workspaceID, id)
}

func (s *Store) UpdateDraft(ctx context.Context, workspaceID, id, expectedHash string, spec json.RawMessage) (RuleSet, error) {
	hash, err := RuleSpecHash(spec)
	if err != nil {
		return RuleSet{}, err
	}
	result, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		UPDATE rulary_ruleset
		SET draft_spec = ?, draft_hash = ?, validated_hash = NULL, state = 'draft', updated_at = ?
		WHERE workspace_id = ? AND ruleset_id = ? AND draft_hash = ?`,
		[]byte(spec), hash, time.Now().UTC().Format(time.RFC3339Nano), workspaceID, id, expectedHash)
	if err != nil {
		return RuleSet{}, err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return RuleSet{}, action.NewError(action.CodePreconditionFailed, "ruleset draft changed after planning")
	}
	return s.GetRuleSet(ctx, workspaceID, id)
}

func (s *Store) MarkValidated(ctx context.Context, workspaceID, id, expectedHash string) error {
	result, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		UPDATE rulary_ruleset SET validated_hash = ?, updated_at = ?
		WHERE workspace_id = ? AND ruleset_id = ? AND draft_hash = ?`,
		expectedHash, time.Now().UTC().Format(time.RFC3339Nano), workspaceID, id, expectedHash)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return action.NewError(action.CodePreconditionFailed, "ruleset draft changed before validation completed")
	}
	return nil
}

func (s *Store) Publish(ctx context.Context, workspaceID, id, actorID, expectedHash string) (RuleVersion, error) {
	ruleset, err := s.GetRuleSet(ctx, workspaceID, id)
	if err != nil {
		return RuleVersion{}, err
	}
	if ruleset.DraftHash != expectedHash {
		return RuleVersion{}, action.NewError(action.CodePlanStale, "ruleset draft changed after publish preview")
	}
	if ruleset.ValidatedHash != ruleset.DraftHash {
		return RuleVersion{}, action.NewError(action.CodePreconditionFailed, "current draft must be validated before publish")
	}
	var version int
	if err := database.ExecutorFor(ctx, s.db).QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_number), 0) + 1 FROM rulary_ruleset_version WHERE ruleset_id = ?`, id).Scan(&version); err != nil {
		return RuleVersion{}, err
	}
	versionID := newID("rv")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		INSERT INTO rulary_ruleset_version
		(version_id, ruleset_id, workspace_id, version_number, spec_json, spec_hash, published_by, published_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, versionID, id, workspaceID, version, []byte(ruleset.DraftSpec), ruleset.DraftHash, actorID, now); err != nil {
		return RuleVersion{}, err
	}
	if _, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		UPDATE rulary_ruleset SET state = 'published', current_version_id = ?, updated_at = ?
		WHERE ruleset_id = ?`, versionID, now, id); err != nil {
		return RuleVersion{}, err
	}
	return s.GetVersion(ctx, workspaceID, versionID)
}

func (s *Store) GetVersion(ctx context.Context, workspaceID, id string) (RuleVersion, error) {
	var version RuleVersion
	var published string
	err := database.ExecutorFor(ctx, s.db).QueryRowContext(ctx, `
		SELECT version_id, ruleset_id, workspace_id, version_number, spec_json, spec_hash, published_by, published_at
		FROM rulary_ruleset_version WHERE workspace_id = ? AND version_id = ?`, workspaceID, id,
	).Scan(&version.ID, &version.RuleSetID, &version.WorkspaceID, &version.Version, &version.Spec,
		&version.SpecHash, &version.PublishedBy, &published)
	if err != nil {
		return RuleVersion{}, err
	}
	version.PublishedAt, _ = time.Parse(time.RFC3339Nano, published)
	return version, nil
}

func (s *Store) LoadSources(ctx context.Context, limit int) ([]CompanySource, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := database.ExecutorFor(ctx, s.db).QueryContext(ctx, `
		SELECT company_id, company_name, license_address, updated_at
		FROM company_license ORDER BY company_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CompanySource, 0)
	for rows.Next() {
		var item CompanySource
		if err := rows.Scan(&item.CompanyID, &item.CompanyName, &item.LicenseAddress, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ExistingLabels(ctx context.Context, companyIDs []string) (map[string]AddressLabel, error) {
	labels := make(map[string]AddressLabel)
	if len(companyIDs) == 0 {
		return labels, nil
	}
	arguments := make([]any, len(companyIDs))
	for index, id := range companyIDs {
		arguments[index] = id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(companyIDs)), ",")
	rows, err := database.ExecutorFor(ctx, s.db).QueryContext(ctx, `
		SELECT company_id, registered_address, business_address, address_note,
		       has_business_address_filing, address_quality_tag, evidence_json
		FROM company_address_labels WHERE company_id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var label AddressLabel
		var hasFiling int
		var evidence []byte
		if err := rows.Scan(&id, &label.RegisteredAddress, &label.BusinessAddress, &label.AddressNote, &hasFiling, &label.AddressQualityTag, &evidence); err != nil {
			return nil, err
		}
		label.HasBusinessAddressFiling = hasFiling == 1
		_ = json.Unmarshal(evidence, &label.Evidence)
		labels[id] = label
	}
	return labels, rows.Err()
}

func (s *Store) WriteRun(ctx context.Context, workspaceID string, version RuleVersion, labels []PlannedLabel) (Run, error) {
	now := time.Now().UTC()
	run := Run{ID: newID("run"), WorkspaceID: workspaceID, RuleVersionID: version.ID, Status: "succeeded", StartedAt: now}
	for _, item := range labels {
		run.MatchedRows++
		if item.Rejected {
			run.RejectedRows++
		} else if item.Changed {
			run.WrittenRows++
		}
	}
	run.FinishedAt = time.Now().UTC()
	if _, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		INSERT INTO rulary_run
		(run_id, workspace_id, rule_version_id, status, matched_rows, written_rows, rejected_rows, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.ID, workspaceID, version.ID, run.Status, run.MatchedRows,
		run.WrittenRows, run.RejectedRows, run.StartedAt.Format(time.RFC3339Nano), run.FinishedAt.Format(time.RFC3339Nano)); err != nil {
		return Run{}, err
	}
	for _, item := range labels {
		if item.Rejected {
			continue
		}
		evidence, _ := json.Marshal(item.Label.Evidence)
		hasFiling := 0
		if item.Label.HasBusinessAddressFiling {
			hasFiling = 1
		}
		processedAt := time.Now().UTC().Format(time.RFC3339Nano)
		if item.Changed {
			_, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
				INSERT INTO company_address_labels
				(company_id, registered_address, business_address, address_note, has_business_address_filing,
				 address_quality_tag, rule_version, run_id, evidence_json, processed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(company_id) DO UPDATE SET
				 registered_address = excluded.registered_address,
				 business_address = excluded.business_address,
				 address_note = excluded.address_note,
				 has_business_address_filing = excluded.has_business_address_filing,
				 address_quality_tag = excluded.address_quality_tag,
				 rule_version = excluded.rule_version,
				 run_id = excluded.run_id,
				 evidence_json = excluded.evidence_json,
				 processed_at = excluded.processed_at`,
				item.CompanyID, item.Label.RegisteredAddress, item.Label.BusinessAddress, item.Label.AddressNote,
				hasFiling, item.Label.AddressQualityTag, version.Version, run.ID, evidence, processedAt)
			if err != nil {
				return Run{}, err
			}
		}
		labelJSON, _ := json.Marshal(item.Label)
		if _, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
			INSERT INTO rulary_label_result
			(result_id, run_id, company_id, label_json, evidence_json, processed_at)
			VALUES (?, ?, ?, ?, ?, ?)`, newID("res"), run.ID, item.CompanyID, labelJSON, evidence, processedAt); err != nil {
			return Run{}, err
		}
	}
	sampleSize := min(len(labels), 20)
	run.Results = append(make([]PlannedLabel, 0, sampleSize), labels[:sampleSize]...)
	return run, nil
}

func (s *Store) GetRun(ctx context.Context, workspaceID, runID string, offset, limit int) (Run, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	run := Run{Results: make([]PlannedLabel, 0)}
	var started, finished string
	err := database.ExecutorFor(ctx, s.db).QueryRowContext(ctx, `
		SELECT run_id, workspace_id, rule_version_id, status, matched_rows, written_rows, rejected_rows, started_at, finished_at
		FROM rulary_run WHERE workspace_id = ? AND run_id = ?`, workspaceID, runID,
	).Scan(&run.ID, &run.WorkspaceID, &run.RuleVersionID, &run.Status, &run.MatchedRows, &run.WrittenRows, &run.RejectedRows, &started, &finished)
	if err != nil {
		return Run{}, err
	}
	run.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	run.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
	run.ResultOffset = offset
	run.ResultLimit = limit
	rows, err := database.ExecutorFor(ctx, s.db).QueryContext(ctx, `
		SELECT lr.company_id, cl.company_name, cl.license_address, cl.updated_at, lr.label_json
		FROM rulary_label_result lr JOIN company_license cl ON cl.company_id = lr.company_id
		WHERE lr.run_id = ? ORDER BY lr.company_id LIMIT ? OFFSET ?`, runID, limit, offset)
	if err != nil {
		return Run{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item PlannedLabel
		var labelJSON []byte
		if err := rows.Scan(&item.CompanyID, &item.CompanyName, &item.LicenseAddress, &item.UpdatedAt, &labelJSON); err != nil {
			return Run{}, err
		}
		_ = json.Unmarshal(labelJSON, &item.Label)
		item.Changed = true
		run.Results = append(run.Results, item)
	}
	return run, rows.Err()
}

func newID(prefix string) string {
	data := make([]byte, 10)
	if _, err := rand.Read(data); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(data)
}
