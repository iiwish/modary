import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Check, Eye, Play, Save, Send, ShieldAlert } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { executeAction, newIdempotencyKey, previewAction, type Actor, type Preview } from "../api";
import { ErrorNotice, Modal, PageHeader, StatusBadge, SuccessNotice, formatDate } from "../components";
import type { PlannedLabel, PreviewSummary, RuleSet, RuleVersion, Run, RuleSpec } from "../types";

type PublishSummary = { ruleset_id: string; draft_hash: string; previous_version_id?: string; change: string };

export function RuleSetPage({ actor }: { actor: Actor }) {
  const { rulesetId = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [editor, setEditor] = useState("");
  const [notice, setNotice] = useState("");
  const [preview, setPreview] = useState<Preview<PreviewSummary> | null>(null);
  const [publishPlan, setPublishPlan] = useState<Preview<PublishSummary> | null>(null);
  const [runPlan, setRunPlan] = useState<Preview<PreviewSummary> | null>(null);
  const [runLimit, setRunLimit] = useState(100);
  const [lastRun, setLastRun] = useState<Run | null>(null);
  const [selectedResult, setSelectedResult] = useState<PlannedLabel | null>(null);
  const [activeError, setActiveError] = useState<unknown>(null);

  const ruleset = useQuery({
    queryKey: ["ruleset", rulesetId],
    queryFn: async () => (await executeAction<{ ruleset: RuleSet }>("rulary.ruleset.get", { ruleset_id: rulesetId })).result.ruleset,
  });
  useEffect(() => {
    if (ruleset.data) setEditor(JSON.stringify(ruleset.data.draft_spec, null, 2));
  }, [ruleset.data?.draft_hash]);
  const spec = useMemo(() => {
    try { return JSON.parse(editor) as RuleSpec; } catch { return null; }
  }, [editor]);
  const isDirty = !!ruleset.data && !!spec && JSON.stringify(spec) !== JSON.stringify(ruleset.data.draft_spec);
  const canEdit = actor.permissions.includes("rulary.ruleset.edit");
  const canPreview = actor.permissions.includes("rulary.ruleset.preview");
  const canPublish = actor.permissions.includes("rulary.ruleset.publish");
  const canRun = actor.permissions.includes("rulary.run.execute");

  const save = useMutation({
    mutationFn: async () => (await executeAction<{ ruleset: RuleSet }>("rulary.ruleset.update_draft", { ruleset_id: rulesetId, spec }, { idempotencyKey: newIdempotencyKey("save") })).result.ruleset,
    onSuccess: () => { setNotice("Draft saved"); setPreview(null); queryClient.invalidateQueries({ queryKey: ["ruleset", rulesetId] }); queryClient.invalidateQueries({ queryKey: ["rulesets"] }); },
    onError: setActiveError,
  });
  const validate = useMutation({
    mutationFn: () => executeAction<{ valid: boolean; spec_hash: string; errors: string[] }>("rulary.ruleset.validate", { ruleset_id: rulesetId }, { idempotencyKey: newIdempotencyKey("validate") }),
    onSuccess: () => { setNotice("Draft validated"); queryClient.invalidateQueries({ queryKey: ["ruleset", rulesetId] }); },
    onError: setActiveError,
  });
  const loadPreview = useMutation({
    mutationFn: () => previewAction<PreviewSummary>("rulary.ruleset.preview", { ruleset_id: rulesetId, limit: 100 }),
    onSuccess: (value) => { setPreview(value); setNotice(""); },
    onError: setActiveError,
  });
  const preparePublish = useMutation({
    mutationFn: () => previewAction<PublishSummary>("rulary.ruleset.publish", { ruleset_id: rulesetId }),
    onSuccess: setPublishPlan,
    onError: setActiveError,
  });
  const publish = useMutation({
    mutationFn: () => executeAction<{ version: RuleVersion }>("rulary.ruleset.publish", { ruleset_id: rulesetId }, { planHash: publishPlan?.plan_hash, idempotencyKey: newIdempotencyKey("publish") }),
    onSuccess: () => { setPublishPlan(null); setNotice("Version published"); queryClient.invalidateQueries({ queryKey: ["ruleset", rulesetId] }); queryClient.invalidateQueries({ queryKey: ["rulesets"] }); },
    onError: setActiveError,
  });
  const prepareRun = useMutation({
    mutationFn: () => previewAction<PreviewSummary>("rulary.run.execute", runInput(ruleset.data?.current_version_id ?? "", runLimit)),
    onSuccess: setRunPlan,
    onError: setActiveError,
  });
  const executeRun = useMutation({
    mutationFn: () => executeAction<{ run: Run }>("rulary.run.execute", runInput(ruleset.data?.current_version_id ?? "", runLimit), { planHash: runPlan?.plan_hash, idempotencyKey: newIdempotencyKey("run") }),
    onSuccess: (response) => { setLastRun(response.result.run); setRunPlan(null); setNotice("Run completed"); },
    onError: setActiveError,
  });

  if (ruleset.isPending) return <div className="page"><div className="skeleton heading" /><div className="skeleton block" /></div>;
  if (ruleset.isError) return <div className="page"><ErrorNotice error={ruleset.error} /></div>;
  const item = ruleset.data;
  return <div className="page">
    <Link className="back-link" to="/rulary/rules"><ArrowLeft size={16} />RuleSets</Link>
    <PageHeader eyebrow="RuleSet" title={item.name} meta={<><StatusBadge status={item.state} /><code>{item.id}</code><span>Updated {formatDate(item.updated_at)}</span></>} actions={<>
      {canEdit && <button className="button" disabled={!isDirty || !spec || save.isPending} onClick={() => save.mutate()}><Save size={17} />Save</button>}
      {canPreview && <button className="button" disabled={isDirty || validate.isPending} onClick={() => validate.mutate()}><Check size={17} />Validate</button>}
      {canPreview && <button className="button" disabled={isDirty || loadPreview.isPending} onClick={() => loadPreview.mutate()}><Eye size={17} />Preview</button>}
      {canPublish && <button className="button primary" disabled={isDirty || item.validated_hash !== item.draft_hash || preparePublish.isPending} onClick={() => preparePublish.mutate()}><Send size={17} />Publish</button>}
    </>} />
    {notice && <SuccessNotice>{notice}</SuccessNotice>}
    {activeError !== null && <ErrorNotice error={activeError} />}
    <div className="workbench-grid">
      <section className="editor-section">
        <div className="section-heading"><div><h2>Draft</h2><span>{item.validated_hash === item.draft_hash ? "Validated" : "Validation required"}</span></div><code>{item.draft_hash.slice(0, 20)}…</code></div>
        <textarea className="code-editor" value={editor} onChange={(event) => { setEditor(event.target.value); setNotice(""); }} readOnly={!canEdit} spellCheck={false} aria-label="RuleSpec JSON" />
        {!spec && <div className="inline-error">RuleSpec is not valid JSON</div>}
      </section>
      <section className="preview-section">
        <div className="section-heading"><div><h2>Data preview</h2><span>{preview ? `${preview.summary.matched_rows} matched` : "No current preview"}</span></div>{preview && <code>{preview.plan_hash.slice(0, 18)}…</code>}</div>
        {preview ? <PreviewTable summary={preview.summary} onSelect={setSelectedResult} /> : <div className="preview-placeholder"><Eye size={24} /><span>Preview pending</span></div>}
      </section>
    </div>
    <section className="run-band">
      <div className="section-heading"><div><h2>Manual run</h2><span>{item.current_version_id ? `Version ${item.current_version_id}` : "Publish a version first"}</span></div></div>
      <div className="run-controls"><label>Row limit<input type="number" min={1} max={1000} value={runLimit} onChange={(event) => setRunLimit(Number(event.target.value))} /></label><button className="button" disabled={!canRun || !item.current_version_id || prepareRun.isPending} onClick={() => prepareRun.mutate()}><ShieldAlert size={17} />Preview run</button></div>
      {lastRun && <div className="run-result"><div><strong>{lastRun.status}</strong><span>{lastRun.written_rows} written · {lastRun.matched_rows} matched</span></div><button className="button" onClick={() => navigate(`/rulary/runs/${lastRun.id}`)}>Open run</button></div>}
    </section>
    {publishPlan && <Modal title="Publish version" onClose={() => setPublishPlan(null)} footer={<><button className="button" onClick={() => setPublishPlan(null)}>Cancel</button><button className="button primary" onClick={() => publish.mutate()} disabled={publish.isPending}><Send size={17} />Publish</button></>}><dl className="impact-list"><div><dt>Change</dt><dd>{publishPlan.summary.change}</dd></div><div><dt>Draft hash</dt><dd><code>{publishPlan.summary.draft_hash}</code></dd></div><div><dt>Expires</dt><dd>{formatDate(publishPlan.expires_at)}</dd></div></dl></Modal>}
    {runPlan && <Modal title="Execute run" onClose={() => setRunPlan(null)} footer={<><button className="button" onClick={() => setRunPlan(null)}>Cancel</button><button className="button danger" onClick={() => executeRun.mutate()} disabled={executeRun.isPending}><Play size={17} />Execute</button></>}><div className="impact-metrics"><Metric label="Matched" value={runPlan.summary.matched_rows} /><Metric label="Writable" value={runPlan.summary.writable_rows} /><Metric label="Unchanged" value={runPlan.summary.unchanged_rows} /><Metric label="Rejected" value={runPlan.summary.rejected_rows} /></div><dl className="impact-list"><div><dt>Target</dt><dd><code>{runPlan.summary.target_table}</code></dd></div><div><dt>Plan</dt><dd><code>{runPlan.plan_hash}</code></dd></div></dl></Modal>}
    {selectedResult && <ResultDrawer result={selectedResult} onClose={() => setSelectedResult(null)} />}
  </div>;
}

function runInput(version: string, limit: number) {
  return { ruleset_version_id: version, source: { table: "company_license" }, target: { table: "company_address_labels" }, limit };
}

function PreviewTable({ summary, onSelect }: { summary: PreviewSummary; onSelect: (result: PlannedLabel) => void }) {
  return <><div className="metric-strip"><Metric label="Matched" value={summary.matched_rows} /><Metric label="Writable" value={summary.writable_rows} /><Metric label="Unchanged" value={summary.unchanged_rows} /><Metric label="Rejected" value={summary.rejected_rows} /></div><div className="table-wrap preview-table"><table><thead><tr><th>Company</th><th>Registered address</th><th>Business filing</th><th>Change</th></tr></thead><tbody>{summary.sample_results.map((row) => <tr key={row.company_id} className="clickable-row" onClick={() => onSelect(row)}><td><strong>{row.company_name}</strong><code>{row.company_id}</code></td><td>{row.label.registered_address}</td><td>{row.label.business_address || "—"}</td><td>{row.rejected ? "Rejected" : row.changed ? "Write" : "No change"}</td></tr>)}</tbody></table></div></>;
}

function Metric({ label, value }: { label: string; value: number }) {
  return <div className="metric"><span>{label}</span><strong>{value}</strong></div>;
}

function ResultDrawer({ result, onClose }: { result: PlannedLabel; onClose: () => void }) {
  return <div className="drawer-scrim" onMouseDown={onClose}><aside className="drawer" onMouseDown={(event) => event.stopPropagation()}><header><div><span className="eyebrow">Result trace</span><h2>{result.company_name}</h2><code>{result.company_id}</code></div><button className="icon-button" onClick={onClose} aria-label="Close">×</button></header><section><h3>Source</h3><p className="source-text">{result.license_address}</p></section><section><h3>Output</h3><dl className="result-fields"><div><dt>Registered address</dt><dd>{result.label.registered_address}</dd></div><div><dt>Business address</dt><dd>{result.label.business_address || "—"}</dd></div><div><dt>Address note</dt><dd>{result.label.address_note || "—"}</dd></div><div><dt>Quality tag</dt><dd>{result.label.address_quality_tag}</dd></div></dl></section><section><h3>Evidence</h3><div className="evidence-list">{result.label.evidence.map((item) => <div key={`${item.field}-${item.start}`}><code>{item.field}</code><span>{item.text}</span><small>{item.start}–{item.end}</small></div>)}</div></section></aside></div>;
}
