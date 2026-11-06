-- P120: Studio's git-only settings leaves are gone; its log level has its own key.
UPDATE settings SET key = 'advanced.logLevel' WHERE key = 'advanced.gitLogLevel';
DELETE FROM settings
 WHERE key LIKE 'git.%'
    OR key IN ('appearance.inlineBlame', 'appearance.dateFormat');
