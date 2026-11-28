#!/bin/sh
# P138: `ade` colours are `--kira-*` tokens; the only literals allowed are data values that encode
# queue state, not theme (P129 Part 1 §2.1): the 10-slot work
# palette (inks and header bands). Tones are theme tokens. Any other colour literal under `ade/`, comments included, fails. So does an allowlist entry with no hit left, keeping the list exactly the kept
# set. A kept value reused as chrome (selection, link, error text) is not visible here; review it.
# Docs: git show bd0c224688d9:docs/v2.0/plans/P138-ade-theme-tokens.md §4.
set -e

cd "$(dirname "$0")/.."

ADE=apps/kira-space/frontend/src/ade
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
export LC_ALL=C

# Lines starting with `;` are group comments.
grep -v '^;' >"$TMP/allow.raw" <<'EOF'
; work palette inks (`PALETTE`, palette.ts)
#dd7577
#09adb4
#cfae42
#af81d6
#7d92e8
#d48140
#cf76ac
#31b182
#81a749
#31a3dd
; task band rest (`TASK_BAND`)
#2d1717
#052325
#241e09
#23192b
#181d2f
#2b1a0d
#2a1722
#0c231a
#19210f
#0c212c
; task band hover (`TASK_BAND_HOVER`)
#341819
#012729
#292105
#281b32
#1a2037
#321b0a
#311827
#08281c
#1b260d
#082534
EOF
sort -u "$TMP/allow.raw" >"$TMP/allow"

grep -rnoiE --include='*.vue' --include='*.ts' --include='*.css' -- \
  '#[0-9a-f]{3,8}\b|(rgba?|hsla?|oklch|hwb|lab|lch)\([^)]*\)' "$ADE" |
  sed -E 's/^([^:]*:[0-9]+):(.*)$/\1	\2/' |
  awk -F'\t' '{ v = tolower($2); gsub(/ /, "", v); print $1 "\t" v }' >"$TMP/hits" || true

STATUS=0

awk -F'\t' 'NR == FNR { ok[$0]; next } !($2 in ok) { print $1 ": " $2 }' "$TMP/allow" "$TMP/hits" >"$TMP/bad"
if [ -s "$TMP/bad" ]; then
  echo "check-ade-colours: colour literal outside the kept set (use a --kira-* token or utility):" >&2
  cat "$TMP/bad" >&2
  STATUS=1
fi

cut -f2 "$TMP/hits" | sort -u >"$TMP/used"
comm -13 "$TMP/used" "$TMP/allow" >"$TMP/stale"
if [ -s "$TMP/stale" ]; then
  echo "check-ade-colours: allowlist entry with no hit left (remove it):" >&2
  cat "$TMP/stale" >&2
  STATUS=1
fi

if [ "$STATUS" -eq 0 ]; then
  echo "check-ade-colours: every colour literal under $ADE is in the kept set."
fi
exit "$STATUS"
