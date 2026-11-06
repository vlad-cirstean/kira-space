-- P108 Part 11 F5: the operation history's re-run needs the console path (database/schema) an op
-- actually ran against, not the connection's bare default — re-opening at the wrong path can run
-- unqualified DML against a different database's same-named table. NULL for every op kind that
-- isn't a console execute() batch, and for every pre-existing row (a legacy op has no recorded
-- path to recover).
ALTER TABLE op_log ADD COLUMN path TEXT;
