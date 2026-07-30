import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Filter, RefreshCw } from "lucide-react";
import { queryAudit } from "../api";
import { ErrorNotice, PageHeader, StatusBadge, formatDate } from "../components";

export function AuditPage() {
  const [filters, setFilters] = useState(new URLSearchParams());
  const audit = useQuery({ queryKey: ["audit", filters.toString()], queryFn: () => queryAudit(filters) });
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const next = new URLSearchParams();
    for (const key of ["action_id", "actor_id", "decision"]) {
      const value = String(form.get(key) ?? "").trim();
      if (value) next.set(key, value);
    }
    setFilters(next);
  }
  return <div className="page"><PageHeader eyebrow="Governance" title="Audit" meta={`${audit.data?.events.length ?? 0} events`} actions={<button className="icon-button" onClick={() => audit.refetch()} aria-label="Refresh"><RefreshCw size={18} /></button>} />
    <form className="filter-bar" onSubmit={submit}><label>Action<input name="action_id" placeholder="rulary.run.execute" /></label><label>Actor<input name="actor_id" placeholder="user_operator" /></label><label>Decision<select name="decision"><option value="">All</option><option value="allowed">Allowed</option><option value="denied">Denied</option><option value="previewed">Previewed</option></select></label><button className="button" type="submit"><Filter size={17} />Apply</button></form>
    {audit.isError && <ErrorNotice error={audit.error} />}
    <div className="table-wrap audit-table"><table><thead><tr><th>Time</th><th>Action</th><th>Actor</th><th>Channel</th><th>Decision</th><th>Result</th><th>Request</th></tr></thead><tbody>{audit.data?.events.map((event, index) => <tr key={`${event.request_id}-${event.decision}-${index}`}><td>{formatDate(event.finished_at)}</td><td><code>{event.action_id}</code>{event.error_code && <span className="error-code">{event.error_code}</span>}</td><td><strong>{event.actor_id}</strong><span>{event.actor_type}</span></td><td>{event.channel}</td><td><StatusBadge status={event.decision} /></td><td>{event.result_summary || event.reason || "—"}</td><td><code>{event.request_id}</code></td></tr>)}</tbody></table></div>
  </div>;
}
