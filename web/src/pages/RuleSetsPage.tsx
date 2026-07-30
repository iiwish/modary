import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Braces, FilePlus2, Plus, RefreshCw } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { executeAction, newIdempotencyKey, type Actor } from "../api";
import { EmptyState, ErrorNotice, Modal, PageHeader, StatusBadge, formatDate } from "../components";
import { defaultRuleSpec, type RuleSet, type RuleSpec } from "../types";

export function RuleSetsPage({ actor }: { actor: Actor }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const canCreate = actor.permissions.includes("rulary.ruleset.create");
  const rulesets = useQuery({
    queryKey: ["rulesets"],
    queryFn: async () => (await executeAction<{ rulesets: RuleSet[] }>("rulary.ruleset.list", { limit: 100 })).result.rulesets,
  });
  const create = useMutation({
    mutationFn: async ({ name, spec }: { name: string; spec: RuleSpec }) =>
      (await executeAction<{ ruleset: RuleSet }>("rulary.ruleset.create", { name, spec }, { idempotencyKey: newIdempotencyKey("create") })).result.ruleset,
    onSuccess: (ruleset) => {
      queryClient.invalidateQueries({ queryKey: ["rulesets"] });
      setCreateOpen(false);
      navigate(`/rulary/rules/${ruleset.id}`);
    },
  });
  function submitCreate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      create.mutate({ name: String(form.get("name")), spec: JSON.parse(String(form.get("spec"))) });
    } catch {
      create.reset();
    }
  }
  return <div className="page">
    <PageHeader eyebrow="Rulary" title="RuleSets" meta={`${rulesets.data?.length ?? 0} rules`} actions={<>
      <button className="icon-button" onClick={() => rulesets.refetch()} aria-label="Refresh" title="Refresh"><RefreshCw size={18} /></button>
      {canCreate && <button className="button primary" onClick={() => setCreateOpen(true)}><Plus size={17} />New RuleSet</button>}
    </>} />
    {rulesets.isError && <ErrorNotice error={rulesets.error} />}
    {rulesets.data?.length === 0 && <EmptyState icon={<Braces size={26} />} title="No RuleSets" action={canCreate ? <button className="button" onClick={() => setCreateOpen(true)}><FilePlus2 size={17} />Create RuleSet</button> : undefined} />}
    {!!rulesets.data?.length && <div className="table-wrap"><table><thead><tr><th>Name</th><th>Status</th><th>Version</th><th>Updated</th><th /></tr></thead><tbody>{rulesets.data.map((ruleset) => <tr key={ruleset.id} onClick={() => navigate(`/rulary/rules/${ruleset.id}`)} className="clickable-row"><td><strong>{ruleset.name}</strong><code>{ruleset.id}</code></td><td><StatusBadge status={ruleset.state} /></td><td>{ruleset.current_version_id ? <code>{ruleset.current_version_id}</code> : "—"}</td><td>{formatDate(ruleset.updated_at)}</td><td className="row-action">›</td></tr>)}</tbody></table></div>}
    {createOpen && <Modal title="New RuleSet" onClose={() => setCreateOpen(false)} footer={<><button className="button" onClick={() => setCreateOpen(false)}>Cancel</button><button className="button primary" type="submit" form="create-ruleset" disabled={create.isPending}><Plus size={17} />Create</button></>}>
      <form id="create-ruleset" className="stack-form" onSubmit={submitCreate}>
        <label>Name<input name="name" defaultValue="企业地址标签" required autoFocus /></label>
        <label>RuleSpec<textarea name="spec" className="code-editor compact" defaultValue={JSON.stringify(defaultRuleSpec, null, 2)} spellCheck={false} required /></label>
        {create.isError && <ErrorNotice error={create.error} />}
      </form>
    </Modal>}
  </div>;
}
