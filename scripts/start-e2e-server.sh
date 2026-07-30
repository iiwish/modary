#!/usr/bin/env bash
set -euo pipefail

data_dir=${MODARY_E2E_DATA_DIR:-/tmp/modary-e2e}
rm -rf "$data_dir"
mkdir -p "$data_dir"

export MODARY_DATA_DIR="$data_dir"
export MODARY_DATABASE_PATH="$data_dir/modary.db"
export MODARY_DEMO_PASSWORD="e2e-password"
export MODARY_AGENT_TOKEN="e2e-agent-token"

exec go run ../cmd/modary serve --listen 127.0.0.1:18081
