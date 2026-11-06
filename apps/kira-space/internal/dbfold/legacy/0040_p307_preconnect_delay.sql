-- P307 Part 2: seconds to wait after the pre-connect script starts before the first connect
-- attempt; 0 = off. First-class column for the same reason as preconnect (not URI-settable).
ALTER TABLE connections ADD COLUMN preconnect_delay_sec INTEGER NOT NULL DEFAULT 0;
