-- P8 D3: one row per response actually received (docs/HISTORY.md (v1.2)'s P8 row). The request half is
-- stored in its STAGE-1 form -- {{$dynamic}} and non-secret {{name}} substituted, a secret still
-- spelled {{name}} -- which is the same line op_log.command already draws (bridge/http.go, P5
-- D6/F3): a secret's plaintext lives in http_variables.secret_value and nowhere else.
CREATE TABLE http_response_history (
  id            TEXT PRIMARY KEY,
  -- The saved request this belongs to, or NULL for a scratch tab's own history. Cascades for real:
  -- db.go's DSN sets _foreign_keys=1 on every connection (P4 F9), so deleting a request -- or the
  -- folder or collection above it -- takes its history with it in one statement.
  item_id       TEXT REFERENCES http_items(id) ON DELETE CASCADE,
  -- The tab that sent it. Deliberately NOT a foreign key into `tabs` (F4): TabsRepo.Save deletes
  -- and re-inserts a window's whole tab set on a 1 s debounce that fires on every keystroke in the
  -- URL field, so ON DELETE CASCADE here would erase a scratch tab's history a second after the
  -- user typed. SweepOrphans() at startup is the bound instead (D7).
  tab_id        TEXT NOT NULL,
  -- The one axis List/trim/Clear key on: the saved request when there is one, else the tab.
  -- GENERATED rather than written, so it cannot disagree with its two sources and so Adopt (D14)
  -- is a single UPDATE of item_id. Verified indexable, and used by the planner, on
  -- modernc.org/sqlite 3.53.3 (F8).
  scope_key     TEXT GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL,
  sent_at       TEXT NOT NULL,
  -- Denormalized out of snapshot_json so the list renders without reading a single body (the same
  -- reason http_items carries method/url). `environment` is the environment's NAME at send time,
  -- not its id: a frozen name still reads correctly after the environment is deleted, which is
  -- exactly when the user wants to know which one it was. '' when none was active.
  method        TEXT NOT NULL,
  url           TEXT NOT NULL,
  environment   TEXT NOT NULL DEFAULT '',
  status        INTEGER NOT NULL,
  status_text   TEXT NOT NULL,
  elapsed_ms    INTEGER NOT NULL,
  -- What the server sent (httpclient.Response.BodyBytes) vs. what this row actually costs. The
  -- second is the byte budget's unit (D6) and is len(snapshot_json), not a body length.
  body_bytes    INTEGER NOT NULL,
  stored_bytes  INTEGER NOT NULL,
  -- model.ResponseHistorySnapshot: the stage-1 request, the full response, and the two storage
  -- flags. Last column, and List never selects it -- SQLite stores columns in declaration order
  -- and spills the tail into overflow pages (P4 D2's own projection reasoning).
  snapshot_json TEXT NOT NULL
);

CREATE INDEX http_response_history_scope ON http_response_history(scope_key, sent_at);
CREATE INDEX http_response_history_age   ON http_response_history(sent_at);
CREATE INDEX http_response_history_tab   ON http_response_history(tab_id);
