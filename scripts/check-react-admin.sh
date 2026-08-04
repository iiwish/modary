#!/bin/sh
set -eu

script_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
root=${MODARY_REACT_ADMIN_ROOT:-$script_root}
cd "$root"

admin=starter/templates/admin/web
status=0

fail() {
	printf '%s\n' "$1" >&2
	status=1
}

for tool in find grep rg; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		printf 'React Admin check requires %s\n' "$tool" >&2
		exit 1
	fi
done

for file in \
	"$admin/package.json" \
	"$admin/pnpm-lock.yaml" \
	"$admin/index.html" \
	"$admin/src/main.tsx" \
	"$admin/src/App.tsx" \
	"$admin/src/modules/index.ts" \
	starter/templates/admin/internal/web/dist/index.html
do
	if test ! -f "$file" || test -L "$file" || test ! -s "$file"; then
		fail "required React Admin artifact must be a non-empty regular file: $file"
	fi
done

if ! grep -Fq '<html lang="zh-CN">' "$admin/index.html" || \
	! grep -Fq '<title>管理后台</title>' "$admin/index.html"; then
	fail 'React Admin source must declare Simplified Chinese as its primary document language'
fi

if ! grep -Fq '<html lang="zh-CN">' starter/templates/admin/internal/web/dist/index.html; then
	fail 'embedded React Admin build must preserve the Simplified Chinese document language'
fi

for pattern in 'app-*.js' 'app-*.css'; do
	assets=$(find starter/templates/admin/internal/web/dist/assets -type f -name "$pattern" -print)
	count=$(printf '%s\n' "$assets" | grep -c . || true)
	if test "$count" -ne 1; then
		fail "React Admin build must contain exactly one hashed $pattern asset"
		continue
	fi
	if test -L "$assets" || test ! -s "$assets"; then
		fail "hashed React Admin asset must be a non-empty regular file: $assets"
	fi
done

if ! grep -Eq 'src="/assets/app-[A-Za-z0-9_-]{8,}\.js"' starter/templates/admin/internal/web/dist/index.html || \
	! grep -Eq 'href="/assets/app-[A-Za-z0-9_-]{8,}\.css"' starter/templates/admin/internal/web/dist/index.html; then
	fail 'React Admin index must reference content-hashed script and stylesheet assets'
fi

if legacy_file=$(find "$admin" -path '*/node_modules' -prune -o -type f -name '*.vue' -print -quit); then
	if test -n "$legacy_file"; then
		fail "Vue source remains in the React Admin template: $legacy_file"
	fi
else
	fail 'React Admin source inventory failed'
fi

if legacy_bundle=$(rg -n -i -- 'vue|pinia' starter/templates/admin/internal/web/dist 2>&1); then
	printf '%s\n' "$legacy_bundle" >&2
	fail 'Vue runtime residue remains in the embedded React Admin bundle'
else
	result=$?
	if test "$result" -ne 1; then
		fail "React Admin bundle residue scan failed: $legacy_bundle"
	fi
fi

legacy_pattern="\"vue\"|(^|[/@])vue([/@:[:space:]\"']|$)|@vue/|@vitejs/plugin-vue|vue-router|vue-tsc|pinia|\\.vue([[:space:]\"']|$)"
if legacy=$(rg -n -i --hidden \
	--glob '!node_modules/**' \
	--glob '!internal/web/dist/**' \
	-- "$legacy_pattern" "$admin" 2>&1); then
	printf '%s\n' "$legacy" >&2
	fail 'Vue implementation or dependency residue remains in the React Admin template'
else
	result=$?
	if test "$result" -ne 1; then
		fail "React Admin residue scan failed: $legacy"
	fi
fi

if ! grep -Fq '"react":' "$admin/package.json" || \
	! grep -Fq '"react-dom":' "$admin/package.json" || \
	! grep -Fq '"react-router":' "$admin/package.json"; then
	fail 'React Admin package manifest is missing the required React runtime'
fi

canonical_docs='README.md
SECURITY.md
docs/framework-f0.md
docs/f0-acceptance-report.md
docs/reference/support-matrix.md
docs/getting-started/admin-profile.md
docs/getting-started/choose-profile.md
docs/zh-CN/index.md
docs/zh-CN/getting-started/admin-profile.md
docs/zh-CN/getting-started/choose-profile.md
docs/adr/ADR-004-consumer-owned-surfaces.md
docs/adr/ADR-005-create-only-profiles.md
docs/releases/upgrade-guide.md
docs/releases/versioning.md
.ai-platform/docs/product-design.md
.ai-platform/docs/technology-decision-record.md
starter/templates/admin/README.md.tmpl'

for file in $canonical_docs; do
	if test ! -f "$file" || test -L "$file" || test ! -s "$file"; then
		fail "required React Admin documentation must be a non-empty regular file: $file"
	fi
done

if legacy_docs=$(rg -n -i -- 'Vue 3|Vue Admin|Vue (source|assets|work surface|UI)|Pinia|Vue Router|source-owned Vue|consumer-owned Vue' $canonical_docs 2>&1); then
	printf '%s\n' "$legacy_docs" >&2
	fail 'canonical documentation still describes the current Admin Starter as Vue'
else
	result=$?
	if test "$result" -ne 1; then
		fail "React Admin documentation scan failed: $legacy_docs"
	fi
fi

if ! grep -Fq 'React' README.md || ! grep -Fq 'React' docs/getting-started/admin-profile.md; then
	fail 'canonical onboarding does not identify the React Admin Starter'
fi

exit "$status"
