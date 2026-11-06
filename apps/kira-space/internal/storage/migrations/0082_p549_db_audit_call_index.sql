-- The call_id cascade and the per-task trim look up approvals by parent call.
CREATE INDEX ade_db_audit_call ON ade_db_audit (call_id) WHERE call_id IS NOT NULL;
