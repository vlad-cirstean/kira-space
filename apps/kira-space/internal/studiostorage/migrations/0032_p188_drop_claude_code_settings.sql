-- P188: the keep-awake-with-agents setting moved to Kira Space only.
DELETE FROM settings WHERE key = 'claudeCode.keepAwakeWithAgents';
