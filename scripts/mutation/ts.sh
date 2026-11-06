# ts.sh — sourced by run.sh. Per-area StrykerJS loop; see docs/DEV_ENVIRONMENT.md.
# Expects run.sh's SNAP, RUN_DIR, WORKERS, TARGETS, CHANGED, TOOLS_DIR, ROOT_DIR, unit_done.

mutation_ts() {
  require_cmd bun "install it from https://bun.sh"
  require_cmd node "install Node 22+"

  if [ ! -d "$TOOLS_DIR/node_modules" ]; then
    echo "mutation: installing Stryker toolchain"
    (cd "$TOOLS_DIR" && bun install --frozen-lockfile)
  fi

  # Wails bindings are generated and gitignored; the snapshot (tracked files only) lacks them.
  for _app in kira-space; do
    if [ ! -d "$ROOT_DIR/apps/$_app/frontend/bindings" ]; then
      echo "mutation: apps/$_app/frontend/bindings missing — run \`bun run setup\` first" >&2
      exit 1
    fi
    cp -R "$ROOT_DIR/apps/$_app/frontend/bindings" "$SNAP/apps/$_app/frontend/bindings"
  done

  mkdir -p "$RUN_DIR/ts"
  _areas="$TARGETS"
  [ -n "$_areas" ] || _areas="$(jq -r '.areas | keys_unsorted[]' "$TOOLS_DIR/areas.json")"
  _only=""
  if [ -n "$CHANGED" ]; then
    _only="$(git -C "$ROOT_DIR" diff --name-only "$CHANGED" -- '*.ts' | jq -R . | jq -sc .)"
  fi

  cd "$SNAP"
  for _area in $_areas; do
    unit_done ts "$_area" && continue
    echo "mutation[ts]: $_area"
    _t0="$(date +%s)"
    _status=ok
    _reason=""
    _rc=0
    MUTATION_AREA="$_area" MUTATION_REPORT="$RUN_DIR/ts/$_area.json" MUTATION_WORKERS="$WORKERS" \
      MUTATION_ONLY="$_only" \
      node "$TOOLS_DIR/node_modules/@stryker-mutator/core/bin/stryker.js" run \
      "$TOOLS_DIR/stryker.config.mjs" ${MUTATION_STRYKER_ARGS:-} >"$RUN_DIR/ts/$_area.log" 2>&1 || _rc=$?
    if [ "$_rc" = 3 ]; then
      _status=skipped
      _reason="no changed files in scope"
    elif [ "$_rc" != 0 ]; then
      _status=red
      _reason="$(grep -m1 -i -e 'error' -e 'fail' "$RUN_DIR/ts/$_area.log" || echo "stryker exit $_rc")"
    elif [ ! -f "$RUN_DIR/ts/$_area.json" ] && [ "${MUTATION_STRYKER_ARGS:-}" != --dryRunOnly ]; then
      _status=red
      _reason="$(grep -m1 -i -e 'error' -e 'fail' "$RUN_DIR/ts/$_area.log" || echo "stryker exit $_rc")"
    fi
    jq -n --arg area "$_area" --arg status "$_status" --arg reason "$_reason" \
      --argjson sec "$(($(date +%s) - _t0))" \
      '{area: $area, status: $status, reason: $reason, durationSec: $sec}' \
      >"$RUN_DIR/ts/$_area.info.json"
  done
  cd "$ROOT_DIR"
}
