#!/usr/bin/env bash
set -euo pipefail

base_url=${1:-http://127.0.0.1:8080}
password=${MODARY_DEMO_PASSWORD:-modary-demo}
cookie_jar=$(mktemp)
trap 'rm -f "$cookie_jar"' EXIT

login=$(curl --silent --show-error --fail-with-body \
  --cookie-jar "$cookie_jar" \
  --header 'Content-Type: application/json' \
  --data "{\"username\":\"admin\",\"password\":\"$password\"}" \
  "$base_url/api/auth/login")
csrf=$(jq -r '.csrf_token' <<<"$login")

call_action() {
  local action_id=$1
  local operation=$2
  local body=$3
  curl --silent --show-error --fail-with-body \
    --cookie "$cookie_jar" \
    --header 'Content-Type: application/json' \
    --header "X-CSRF-Token: $csrf" \
    --data "$body" \
    "$base_url/api/actions/$action_id/$operation"
}

spec='{"schema_version":"rulary.ruleset.f0","id":"company-address","name":"Address labels","source":{"table":"company_license","primary_key":"company_id","field":"license_address"},"operator":{"type":"rulary.address.extract_v1","filing_marker":"经营地址备案","parenthetical_note_target":"address_note"},"output":{"table":"company_address_labels","unique_key":"company_id"}}'
suffix=$(date +%s)
create_body=$(jq -cn --argjson spec "$spec" --arg key "smoke-create-$suffix" '{input:{name:"Release smoke",spec:$spec},idempotency_key:$key}')
created=$(call_action rulary.ruleset.create execute "$create_body")
ruleset_id=$(jq -r '.result.ruleset.id' <<<"$created")

validate_body=$(jq -cn --arg id "$ruleset_id" --arg key "smoke-validate-$suffix" '{input:{ruleset_id:$id},idempotency_key:$key}')
call_action rulary.ruleset.validate execute "$validate_body" >/dev/null

ruleset_input=$(jq -cn --arg id "$ruleset_id" '{ruleset_id:$id}')
preview_body=$(jq -cn --argjson input "$ruleset_input" '{input:($input + {limit:20})}')
previewed=$(call_action rulary.ruleset.preview preview "$preview_body")
registered=$(jq -r '.preview.summary.sample_results[0].label.registered_address' <<<"$previewed")
business=$(jq -r '.preview.summary.sample_results[0].label.business_address' <<<"$previewed")

publish_preview=$(call_action rulary.ruleset.publish preview "$(jq -cn --argjson input "$ruleset_input" '{input:$input}')")
publish_plan=$(jq -r '.preview.plan_hash' <<<"$publish_preview")
publish_body=$(jq -cn --arg id "$ruleset_id" --arg plan "$publish_plan" --arg key "smoke-publish-$suffix" '{input:{ruleset_id:$id},plan_hash:$plan,idempotency_key:$key}')
published=$(call_action rulary.ruleset.publish execute "$publish_body")
version_id=$(jq -r '.result.version.id' <<<"$published")

run_input=$(jq -cn --arg id "$version_id" '{ruleset_version_id:$id,source:{table:"company_license"},target:{table:"company_address_labels"},limit:20}')
run_preview_body=$(jq -cn --argjson input "$run_input" '{input:$input}')
run_preview=$(call_action rulary.run.execute preview "$run_preview_body")
run_plan=$(jq -r '.preview.plan_hash' <<<"$run_preview")
run_body=$(jq -cn --argjson input "$run_input" --arg plan "$run_plan" --arg key "smoke-run-$suffix" '{input:$input,plan_hash:$plan,idempotency_key:$key}')
executed=$(call_action rulary.run.execute execute "$run_body")
run_id=$(jq -r '.result.run.id' <<<"$executed")

run_get_body=$(jq -cn --arg id "$run_id" '{input:{run_id:$id}}')
detail=$(call_action rulary.run.get execute "$run_get_body")
audit=$(curl --silent --show-error --fail-with-body --cookie "$cookie_jar" "$base_url/api/audit")

jq -cn \
  --arg ruleset_id "$ruleset_id" \
  --arg version_id "$version_id" \
  --arg run_id "$run_id" \
  --arg registered "$registered" \
  --arg business "$business" \
  --argjson matched "$(jq '.result.run.matched_rows' <<<"$detail")" \
  --argjson results "$(jq '.result.run.results | length' <<<"$detail")" \
  --argjson audit_events "$(jq '.events | length' <<<"$audit")" \
  '{ruleset_id:$ruleset_id,version_id:$version_id,run_id:$run_id,registered_address:$registered,business_address:$business,matched_rows:$matched,result_rows:$results,audit_events:$audit_events}'
