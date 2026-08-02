#!/bin/sh
set -u

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P) || exit 1
cd "$root" || exit 1

status=0

for tool in git rg find awk; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'neutrality check requires %s\n' "$tool" >&2
		exit 1
	fi
done

# The gate is meaningful only for the complete current Git worktree. Validate
# the repository and its tracked-plus-untracked inventory before scanning the
# filesystem. The NUL-delimited inventory also handles whitespace and newlines
# in pathnames without interpreting them as shell input.
if repository_root=$(git rev-parse --show-toplevel 2>&1); then
	if test "$repository_root" != "$root"; then
		printf 'neutrality check root mismatch: expected %s, found %s\n' "$root" "$repository_root" >&2
		exit 1
	fi
else
	printf 'neutrality repository check failed: %s\n' "$repository_root" >&2
	exit 1
fi
if ! git ls-files --cached --others --exclude-standard -z -- . >/dev/null; then
	printf '%s\n' 'neutrality inventory failed' >&2
	exit 1
fi

scan_forbidden() {
	pattern=$1
	message=$2
	shift 2

	# Caller globs select a surface. Common exclusions are deliberately last so
	# an include glob cannot re-include governance history or the consumer.
	if matches=$(rg -n -i --hidden --no-ignore \
		"$@" \
		--glob '!.git' \
		--glob '!.git/**' \
		--glob '!.ai-platform/evidence/**' \
		--glob '!.ai-platform/specs/**/analysis.md' \
		--glob '!.ai-platform/specs/**/tasks.md' \
		--glob '!.ai-platform/specs/**/packets/**' \
		--glob '!examples/counter/**' \
		--glob '!**/node_modules/**' \
		--glob '!docs/guides/rulary-bootstrap.md' \
		--glob '!scripts/**' \
		--glob '!scripts/check-neutrality.sh' \
		--glob '!scripts/check_neutrality_test.go' \
		-- "$pattern" . 2>&1)
	then
		printf '%s\n' "$matches"
		printf '%s\n' "$message" >&2
		status=1
	else
		result=$?
		if test "$result" -ne 1; then
			printf 'neutrality scan failed: %s\n' "$matches" >&2
			status=1
		fi
	fi
}

# Product vocabulary is forbidden from framework implementation, templates,
# migrations, and assets. Documentation may name an external adoption target
# while explaining the dependency boundary; that does not make it framework
# production code.
domain_pattern='rulary|ruleset|rule_set|rulespec|ws_default|company_(id|license)|company_address|registered_address|modary-rulary|MODARY_DEMO_PASSWORD|MODARY_AGENT_TOKEN'
scan_forbidden "$domain_pattern" 'consumer-domain terms remain in the active framework tree' \
	--glob '*.go' \
	--glob '*.sql' \
	--glob '*.json' \
	--glob '*.toml' \
	--glob '*.yaml' \
	--glob '*.yml' \
	--glob '*.js' \
	--glob '*.jsx' \
	--glob '*.ts' \
	--glob '*.tsx' \
	--glob '*.vue' \
	--glob '*.html' \
	--glob '*.css' \
	--glob '*.tmpl' \
	--glob '!docs/**' \
	--glob '!.ai-platform/**'

# Canonical specifications and plans stay product-neutral. Feature 007 is the
# explicit framework/adoption-boundary decision and may name Rulary only to
# forbid implementing it in this repository.
scan_forbidden "$domain_pattern" 'consumer-domain terms remain in an authoritative framework spec or plan' \
	--glob '.ai-platform/specs/**/spec.md' \
	--glob '.ai-platform/specs/**/plan.md' \
	--glob '!.ai-platform/specs/007-component-framework-refoundation/spec.md' \
	--glob '!.ai-platform/specs/007-component-framework-refoundation/plan.md'

# Authoritative specifications are portable framework contracts. Delivery
# history may retain local execution commands, but canonical specs and plans do
# not name a contributor's home directory or another machine-specific root.
machine_path_pattern='/(Users|home)/[^/[:space:]`]+/|/root/[^/[:space:]`]+/|[[:alpha:]]:\\Users\\'
scan_forbidden "$machine_path_pattern" 'machine-specific absolute path remains in an authoritative framework spec or plan' \
	--glob '.ai-platform/specs/**/spec.md' \
	--glob '.ai-platform/specs/**/plan.md'

# Counter is the external conformance application's domain. It is valid in the
# fixture and in contributor-facing commands, but never in framework runtime
# code or production assets/configuration. Framework tests may use neutral
# Counter examples, so *_test.go files are intentionally outside this scan.
consumer_pattern='example\.com/modary-counter-consumer|counter-console|consumer_counter|counter\.(preview|increment)'
scan_forbidden "$consumer_pattern" 'the conformance consumer leaked into framework production code or configuration' \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '*.sql' \
	--glob '*.json' \
	--glob '*.toml' \
	--glob '*.yaml' \
	--glob '*.yml' \
	--glob '*.js' \
	--glob '*.jsx' \
	--glob '*.ts' \
	--glob '*.tsx' \
	--glob '*.html' \
	--glob '*.css' \
	--glob '!.github/**' \
	--glob '!.ai-platform/**' \
	--glob '!docs/**' \
	--glob '!scripts/**' \
	--glob '!README.md'

read_module_path() {
	awk '
		$1 == "module" {
			count++
			if (NF != 2) invalid = 1
			name = $2
		}
		END {
			if (count != 1 || invalid || name == "") exit 1
			print name
		}
	' "$1"
}

if test ! -f go.mod || test -L go.mod; then
	printf '%s\n' 'framework go.mod is missing or is not a regular owned file' >&2
	status=1
	framework_module=
elif framework_module=$(read_module_path go.mod 2>&1); then
	:
else
	printf 'framework module declaration is invalid: %s\n' "$framework_module" >&2
	status=1
	framework_module=
fi

consumer_root=examples/counter
consumer_mod=$consumer_root/go.mod
if test ! -d "$consumer_root" || test -L "$consumer_root"; then
	printf '%s\n' 'external consumer conformance module is missing or is not an owned directory' >&2
	status=1
elif test ! -f "$consumer_mod" || test -L "$consumer_mod"; then
	printf '%s\n' 'external consumer must own a regular go.mod file' >&2
	status=1
else
	if consumer_module=$(read_module_path "$consumer_mod" 2>&1); then
		if test -z "$framework_module" || test "$consumer_module" = "$framework_module"; then
			printf '%s\n' 'external consumer must declare a module distinct from the framework' >&2
			status=1
		fi
	else
		printf 'external consumer module declaration is invalid: %s\n' "$consumer_module" >&2
		status=1
		consumer_module=
	fi

	if consumer_source=$(find "$consumer_root" -type f -name '*.go' -print -quit 2>&1); then
		if test -z "$consumer_source"; then
			printf '%s\n' 'external consumer contains no Go source' >&2
			status=1
		fi
	else
		printf 'external consumer source inventory failed: %s\n' "$consumer_source" >&2
		status=1
	fi

	if test -n "$framework_module"; then
		if matches=$(rg -n -F --hidden --no-ignore --glob '*.go' -- "$framework_module/internal/" "$consumer_root" 2>&1); then
			printf '%s\n' "$matches"
			printf '%s\n' 'the external consumer imports a private Modary package' >&2
			status=1
		else
			result=$?
			if test "$result" -ne 1; then
				printf 'external-consumer neutrality scan failed: %s\n' "$matches" >&2
				status=1
			fi
		fi
	fi
fi

legacy_import_pattern='github\.com/iiwish/modary/(core|modules|internal/(app|generated|transport|webui))'
scan_forbidden "$legacy_import_pattern" 'an active file references a removed application-owned import path'

for obsolete in core modules web tests internal/app internal/generated internal/transport internal/webui modary.yaml package.json pnpm-lock.yaml pnpm-workspace.yaml Dockerfile.release .dockerignore; do
	if test -e "$obsolete" || test -L "$obsolete"; then
		printf 'obsolete application-owned path remains: %s\n' "$obsolete" >&2
		status=1
	fi
done

if persisted_artifact=$(find . -path './.git' -prune -o -path '*/node_modules' -prune -o -type f \( -name '*.db' -o -name '*.exe' \) -print -quit 2>&1); then
	if test -n "$persisted_artifact"; then
		printf 'runtime or binary artifact remains in the active tree: %s\n' "$persisted_artifact" >&2
		status=1
	fi
else
	printf 'runtime-artifact inventory failed: %s\n' "$persisted_artifact" >&2
	status=1
fi

if unexpected_executable=$(find . -path './.git' -prune -o -path '*/node_modules' -prune -o -type f -perm -111 ! -path './scripts/*.sh' -print -quit 2>&1); then
	if test -n "$unexpected_executable"; then
		printf 'unexpected executable artifact remains in the active tree: %s\n' "$unexpected_executable" >&2
		status=1
	fi
else
	printf 'executable-artifact inventory failed: %s\n' "$unexpected_executable" >&2
	status=1
fi

if test -d internal; then
	if unexpected_internal=$(find internal -mindepth 1 -maxdepth 1 -type d \
		! -name actionpersistence \
		! -name actionruntime \
		! -name callbackcontract \
		! -name databasecontrol \
		! -name filepolicy \
		! -name jsonschema \
		! -name jsonvalue \
		! -name moduleassembly \
		! -name quality \
		! -name runtimecontrol \
		! -name safeerr \
		! -name sqlpolicy \
		! -name testpostgres \
		! -name testsupport \
		! -name transactionoutcome \
		-print -quit 2>&1); then
		if test -n "$unexpected_internal"; then
			printf 'unexpected private framework tree: %s\n' "$unexpected_internal" >&2
			status=1
		fi
	else
		printf 'private-framework inventory failed: %s\n' "$unexpected_internal" >&2
		status=1
	fi
fi

scan_forbidden '^[[:space:]]*package[[:space:]]+main([[:space:]]*(//.*)?)?$' 'the framework production tree contains an application executable' \
	--glob '*.go' \
	--glob '!**/*_test.go' \
	--glob '!cmd/modary/**'

exit "$status"
