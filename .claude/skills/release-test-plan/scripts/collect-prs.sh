#!/usr/bin/env bash
# Lists every PR merged to HEAD since BASE and classifies it for triage.
#
# Usage: collect-prs.sh [BASE_TAG] [HEAD_REF]
#   BASE_TAG  default: highest final (non -rc) v* tag
#   HEAD_REF  default: origin/master, falling back to master
#
# Release tags are cut on release-* branches and are usually NOT ancestors of
# master, so the range is merge-base(BASE, HEAD)..HEAD, and commits whose patch
# (or PR number) already exists in BASE are marked "in-base".
#
# Output (stdout): a header comment block, then TSV with columns
#   pr  sha  status  kind  files  lines  areas  subject
# status: analyze | in-base | reverted:by#N | revert:of#N | no-pr
# kind:   deps | ci | docs | test-only | openapi | code
set -euo pipefail

base="${1:-}"
head="${2:-}"

if [[ -z "$base" ]]; then
	base="$(git tag -l 'v*' --sort=-v:refname | grep -v -- '-rc' | head -1)"
fi
if [[ -z "$head" ]]; then
	if git rev-parse -q --verify origin/master >/dev/null; then head=origin/master; else head=master; fi
fi
git rev-parse -q --verify "$base^{commit}" >/dev/null || { echo "unknown base: $base" >&2; exit 1; }
git rev-parse -q --verify "$head^{commit}" >/dev/null || { echo "unknown head: $head" >&2; exit 1; }

mb="$(git merge-base "$base" "$head")"
newer_rcs="$(git tag -l 'v*-rc*' --contains "$mb" --sort=-v:refname | head -3 | tr '\n' ' ')"

# Patch-equivalent commits already shipped in BASE (git cherry prints "-" for them).
in_base_sha="$(git cherry "$base" "$head" "$mb" | awk '$1 == "-" {print $2}')"
# PR numbers referenced by commits on the release branch (cherry-picks often keep "(#N)").
in_base_pr="$(git log --format=%s "$mb..$base" | grep -oE '\(#[0-9]+\)' | tr -dc '0-9\n' || true)"

# Commits that revert another PR ("revert #N"), as "N<TAB>reverting PR" lines.
# Plain text instead of an associative array: macOS ships bash 3.2.
reverts="$(git log --first-parent --format=$'%H\t%s' "$mb..$head" | while IFS=$'\t' read -r sha subject; do
	[[ "$subject" =~ [Rr]evert ]] || continue
	self="$(grep -oE '\(#[0-9]+\)$' <<<"$subject" | tr -dc '0-9' || true)"
	for n in $(grep -oE '#[0-9]+' <<<"$subject" | tr -dc '0-9\n'); do
		[[ "$n" != "$self" ]] && printf '%s\t%s\n' "$n" "${self:-${sha:0:9}}"
	done
done)"
reverted_by() { awk -F'\t' -v n="$1" '$1 == n {print $2; exit}' <<<"$reverts"; }

classify_kind() { # $1 subject, $2 newline-separated file list
	local subject="$1" files="$2"
	if [[ "$subject" =~ ^chore\(deps\) || "$subject" =~ ^(build|chore)\(deps ]]; then echo deps; return; fi
	local non_ci non_docs non_test
	non_ci="$(grep -vE '^(\.github/|\.golangci|Makefile$|\.goreleaser|packaging/|Dockerfile)' <<<"$files" || true)"
	[[ -z "$non_ci" ]] && { echo ci; return; }
	non_docs="$(grep -vE '(\.md$|^docs/|^LICENSE)' <<<"$files" || true)"
	[[ -z "$non_docs" ]] && { echo docs; return; }
	non_test="$(grep -vE '(_test\.go$|/testdata/|/mock/|_fuzz_test\.go$)' <<<"$non_docs" || true)"
	[[ -z "$non_test" ]] && { echo test-only; return; }
	if grep -qE '^openapi/' <<<"$files" && [[ -z "$(grep -vE '^openapi/' <<<"$non_test" || true)" ]]; then echo openapi; return; fi
	echo code
}

areas_of() { # top two path segments of non-test Go/proto/yaml files, deduped
	grep -vE '(_test\.go$|/testdata/|\.pb\.go$|^go\.sum$)' <<<"$1" |
		awk -F/ '{ if ($1 == "pkg" && NF > 2) print $1"/"$2; else if (NF > 1) print $1"/"$2; else print $1 }' |
		sort -u | head -6 | paste -sd, - || true
}

echo "# base=$base head=$head ($(git rev-parse --short "$head")) merge_base=$(git rev-parse --short "$mb")"
echo "# base_on_head=$(git merge-base --is-ancestor "$base" "$head" && echo yes || echo no)"
[[ -n "$newer_rcs" ]] && echo "# rc_tags_near_range: $newer_rcs"
printf 'pr\tsha\tstatus\tkind\tfiles\tlines\tareas\tsubject\n'

git log --first-parent --reverse --format=$'%H\t%s' "$mb..$head" | while IFS=$'\t' read -r sha subject; do
	pr="$(grep -oE '\(#[0-9]+\)$' <<<"$subject" | tr -dc '0-9' || true)"
	files="$(git show --format= --name-only "$sha")"
	nfiles="$(grep -c . <<<"$files" || true)"
	lines="$(git show --format= --numstat "$sha" | awk '$1 != "-" {s += $1 + $2} END {print s + 0}')"
	kind="$(classify_kind "$subject" "$files")"

	rby="$(reverted_by "$pr")"
	status=analyze
	if [[ -z "$pr" ]]; then
		status=no-pr
	elif grep -qxF "$sha" <<<"$in_base_sha" || grep -qxF "$pr" <<<"$in_base_pr"; then
		status=in-base
	elif [[ -n "$rby" ]]; then
		status="reverted:by#$rby"
	elif [[ "$subject" =~ [Rr]evert ]]; then
		status="revert:of#$(grep -oE '#[0-9]+' <<<"$subject" | tr -dc '0-9\n' | grep -vxF "$pr" | paste -sd, -)"
	fi
	areas="$(areas_of "$files")"

	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
		"${pr:--}" "${sha:0:9}" "$status" \
		"$kind" "$nfiles" "$lines" "${areas:--}" "$subject"
done
