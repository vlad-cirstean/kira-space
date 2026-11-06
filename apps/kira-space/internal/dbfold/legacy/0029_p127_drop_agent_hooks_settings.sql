-- P127: agent-hooks monitoring left Kira Studio; its two leaves are orphaned.
DELETE FROM settings WHERE key IN ('claudeCode.hooksEnabled', 'claudeCode.hooksPromptDismissed');
