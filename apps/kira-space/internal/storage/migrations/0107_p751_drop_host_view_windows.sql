-- P751: the PR and issue view windows are gone; a row left by a crash must not restore as a normal window.
DELETE FROM windows WHERE key LIKE 'hostview-%';
