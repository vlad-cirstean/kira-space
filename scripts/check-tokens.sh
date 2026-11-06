#!/bin/sh
# P13 D3/OQ-1: every var(--prefix-...) reference in a layer's own source tree must resolve to a
# real definition in that layer's own definition files. A grep-and-comm guard, not a dependency (stylelint is not in this repo's
# toolchain) — its one known blind spot is a var() carrying a fallback (var(--x, red)), which is
# legitimate and is skipped by construction (the pattern below only matches a var() whose closing
# paren directly follows the property name).
#
# P131 Part 3 §6.5: the --kui-* layer (both copies of the retired kira-ui package's token-bridge file, and every
# Kui* consumer that referenced them) is gone — check_layer's own "resolves to a real definition" shape
# no longer applies (there is nothing left to define), so the kui- pass below is a zero-use guard
# instead: any --kui- reference anywhere is a regression, full stop.
set -e

cd "$(dirname "$0")/.."

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
STATUS=0

check_layer() {
  prefix="$1"
  src="$2"
  defs_files="$3"
  out="$TMP/$4"

  grep -rhoE --include='*.vue' --include='*.css' -- "var\(--${prefix}[a-zA-Z0-9-]+\)" $src |
    sed -E "s/^var\((--${prefix}[a-zA-Z0-9-]+)\)\$/\1/" | sort -u >"$out.used"

  grep -hoE -- "--${prefix}[a-zA-Z0-9-]+:" $defs_files |
    sed -E 's/:$//' | sort -u >"$out.defined"

  missing=$(comm -23 "$out.used" "$out.defined")

  if [ -n "$missing" ]; then
    echo "check-tokens: undefined --${prefix}* custom properties referenced in $src:" >&2
    echo "$missing" >&2
    STATUS=1
    return
  fi

  echo "check-tokens: every --${prefix}* reference in $src resolves to a real definition."
}

SPACE_SRC=apps/kira-space/frontend/src
THEME_SRC=packages/theme/src
WORKBENCH_SRC=packages/workbench/src
GIT_UI_SRC=packages/git-ui/src
DOCKER_UI_SRC=packages/docker-ui/src

# P103 (byte-identical tier, folded into P100 Part 2): tokens.css/base.css/primitives.css moved
# out of apps/kira-space/frontend/src/studio/theme into packages/theme/src, shared verbatim by both
# apps — definitions read from their real new location. Usage now scans both apps' own src (kira-
# space is a real --kira-* consumer, not just Studio) plus packages/theme/src itself, whose own
# components/primitives reference the same tokens.
#
# P103 Part 1: packages/workbench/src added to the usage scan too — its own moved ContextMenu.vue/
# AppTooltip.vue/ConfirmDialog.vue reference --kira-* tokens the same way they did inside each app.
check_layer 'kira-' "$SPACE_SRC $THEME_SRC $WORKBENCH_SRC $DOCKER_UI_SRC $GIT_UI_SRC" \
  "$THEME_SRC/tokens.css $THEME_SRC/base.css $THEME_SRC/primitives.css $GIT_UI_SRC/theme/git.css" kira

kui_hits=$(grep -rnE --include='*.vue' --include='*.css' --include='*.ts' \
  -- '--kui-' packages/git-ui/src packages/theme/src packages/workbench/src || true)
if [ -n "$kui_hits" ]; then
  echo "check-tokens: --kui-* is retired (P131 Part 3 §6.5) but still referenced:" >&2
  echo "$kui_hits" >&2
  STATUS=1
else
  echo "check-tokens: no --kui-* reference anywhere."
fi

exit $STATUS
