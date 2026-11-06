-- P11 D12: a gRPC request is a sibling row in http_items, distinguished by `protocol`, not by a
-- third `kind` value -- `kind` stays structural (folder vs. a leaf); `protocol` says which
-- document shape request_json holds for a leaf. Defaulted so every existing row (all HTTP) needs
-- no backfill.
ALTER TABLE http_items ADD COLUMN protocol TEXT NOT NULL DEFAULT 'http';

-- P11 D11: its own table, not a widened http_response_history -- F19's four measurements: the
-- shared `status` column would mean an HTTP status for one protocol and a codes.Code for the
-- other (statusClass(0) === 'err' would silently mis-colour gRPC's OK); storedSnapshot embeds
-- httpclient.Response by value and would become a union; the byte caps have no message-count
-- equivalent; and the two renderer halves cannot be shared anyway (biome.json forbids
-- views/grpcrequest/** from importing views/httprequest/**, F18).
--
-- What IS deliberately mirrored, not abstracted (F19's last point): scope_key's own generated
-- column, the insert-then-trim per-scope cap, the window-function global byte sweep, Adopt and
-- SweepOrphans -- the same five patterns http_response_history/http_variable_history/
-- filter_history already apply, a fourth application following a precedent rather than
-- duplicating an abstraction that exists.
CREATE TABLE grpc_call_history (
  id            TEXT PRIMARY KEY,
  item_id       TEXT REFERENCES http_items(id) ON DELETE CASCADE,
  -- Deliberately NOT a foreign key into `tabs` (P8 F4's own reasoning, applied verbatim) --
  -- TabsRepo.Save deletes and re-inserts a window's whole tab set on a 1 s debounce, so
  -- ON DELETE CASCADE here would erase a scratch tab's history a second after the user typed.
  -- SweepOrphans() at startup is the bound instead.
  tab_id        TEXT NOT NULL,
  scope_key     TEXT GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL,
  called_at     TEXT NOT NULL,
  -- STAGE 1: a secret is still spelled {{name}} -- recorded from the bridge's own unresolved args,
  -- never from the resolver's output (P8 D2's rule verbatim).
  target        TEXT NOT NULL,
  method        TEXT NOT NULL,             -- fully-qualified: pkg.Service/Method
  streaming     TEXT NOT NULL,             -- 'unary' | 'server'
  environment   TEXT NOT NULL DEFAULT '',  -- the environment's NAME at call time, not its id
  code          INTEGER NOT NULL,          -- codes.Code: 0 = OK
  code_name     TEXT NOT NULL,             -- 'OK', 'PermissionDenied', …
  status_message TEXT NOT NULL DEFAULT '',
  elapsed_ms    INTEGER NOT NULL,
  message_count INTEGER NOT NULL,          -- messages actually received
  message_bytes INTEGER NOT NULL,          -- total wire bytes received
  stored_bytes  INTEGER NOT NULL,
  -- model.GrpcCallSnapshot: the stage-1 request, the messages actually kept (D11's own 100-message
  -- cap), header/trailer, and the storage-cap flags. Last column, so SQLite spills the tail into
  -- overflow pages (P4 D2's own projection reasoning).
  snapshot_json TEXT NOT NULL
);

CREATE INDEX grpc_call_history_scope ON grpc_call_history(scope_key, called_at);
CREATE INDEX grpc_call_history_age   ON grpc_call_history(called_at);
CREATE INDEX grpc_call_history_tab   ON grpc_call_history(tab_id);
