#!/bin/sh
# P138: `ade` colours are `--kira-*` tokens; the only literals allowed are data values that encode
# queue state, not theme (P129 Part 1 §2.1): tones with their ink and the 20-slot work
# palette. Any other colour literal under `ade/`, comments
# included, fails. So does an allowlist entry with no hit left, keeping the list exactly the kept
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
; tone tint, text, solid (`TONE`, tones.ts)
rgba(203,146,63,0.14)
#e5b473
#cb923f
rgba(223,128,114,0.14)
#f7a599
#df8072
rgba(92,180,123,0.14)
#8ad1a0
#5cb47b
rgba(118,157,233,0.14)
#9cbeff
#769de9
rgba(128,93,192,0.16)
#c5b0f6
#805dc0
#232327
#babec4
#7d8088
; Claude launch button solid (`CLAUDE_SOLID`, tones.ts)
#d97757
; ink on a tone solid (`TONE_INK`)
#15161a
#ffffff
; work palette (`PALETTE`, palette.ts)
#d38261
#d2a155
#b4b04d
#90ba66
#64b477
#57b1a0
#56a6c2
#5a8fd5
#727ed7
#9676d1
#bc72cb
#d36fa4
#d56c7e
#b18765
#95a269
#669d8e
#8092b0
#a182b7
#cc9191
#a4aab2
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
