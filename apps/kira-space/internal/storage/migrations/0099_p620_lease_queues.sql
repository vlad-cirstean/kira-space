-- P620: user-named resource lease queues in display order. Holds and waits stay in memory.
CREATE TABLE ade_lease_queues (
  name TEXT PRIMARY KEY,
  position INTEGER NOT NULL
) STRICT;
