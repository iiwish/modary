import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, ChevronLeft, ChevronRight, X } from "lucide-react";
import { Link, useParams } from "react-router-dom";
import { executeAction } from "../api";
import { ErrorNotice, PageHeader, StatusBadge, formatDate } from "../components";
import type { PlannedLabel, Run } from "../types";

export function RunPage() {
  const { runId = "" } = useParams();
  const [selected, setSelected] = useState<PlannedLabel | null>(null);
  const [offset, setOffset] = useState(0);
  const pageSize = 50;
  const run = useQuery({ queryKey: ["run", runId, offset], queryFn: async () => (await executeAction<{ run: Run }>("rulary.run.get", { run_id: runId, offset, limit: pageSize })).result.run });
  useEffect(() => {
    if (!run.isSuccess) {
      return;
    }
    const firstFrame = window.requestAnimationFrame(() => {
      window.requestAnimationFrame(() => window.scrollTo({ top: 0, left: 0 }));
    });
    return () => window.cancelAnimationFrame(firstFrame);
  }, [run.isSuccess, runId]);
  if (run.isPending) return <div className="page"><div className="skeleton block" /></div>;
  if (run.isError) return <div className="page"><ErrorNotice error={run.error} /></div>;
  const results = run.data.results ?? [];
  const resultTotal = run.data.matched_rows - run.data.rejected_rows;
  return <div className="page"><Link className="back-link" to="/rulary/rules"><ArrowLeft size={16} />RuleSets</Link><PageHeader eyebrow="RuleRun" title={run.data.id} meta={<><StatusBadge status={run.data.status} /><span>{formatDate(run.data.finished_at)}</span><code>{run.data.rule_version_id}</code></>} />
    <div className="metric-strip run-metrics"><div className="metric"><span>Matched</span><strong>{run.data.matched_rows}</strong></div><div className="metric"><span>Written</span><strong>{run.data.written_rows}</strong></div><div className="metric"><span>Rejected</span><strong>{run.data.rejected_rows}</strong></div></div>
    <div className="table-wrap run-table"><table><thead><tr><th>Company</th><th>Source</th><th>Registered</th><th>Business</th></tr></thead><tbody>{results.map((row) => <tr key={row.company_id} className="clickable-row" onClick={() => setSelected(row)}><td><strong>{row.company_name}</strong><code>{row.company_id}</code></td><td className="truncate-cell">{row.license_address}</td><td>{row.label.registered_address}</td><td>{row.label.business_address || "—"}</td></tr>)}{results.length === 0 && <tr><td colSpan={4}>No result rows</td></tr>}</tbody></table></div>
    {resultTotal > pageSize && <div className="pagination"><span>{offset + 1}–{Math.min(offset + results.length, resultTotal)} of {resultTotal}</span><div><button className="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - pageSize))}><ChevronLeft size={16} />Previous</button><button className="button" disabled={offset + results.length >= resultTotal} onClick={() => setOffset(offset + pageSize)}>Next<ChevronRight size={16} /></button></div></div>}
    {selected && <div className="modal-backdrop" onMouseDown={() => setSelected(null)}><section className="modal" onMouseDown={(event) => event.stopPropagation()}><header><h2>{selected.company_name}</h2><button className="icon-button" onClick={() => setSelected(null)} aria-label="Close"><X size={18} /></button></header><div className="modal-body"><p className="source-text">{selected.license_address}</p><pre className="json-output">{JSON.stringify(selected.label, null, 2)}</pre></div></section></div>}
  </div>;
}
