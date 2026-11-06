-- P312: panel sizes are per module; the one shared value seeds every module.
INSERT OR IGNORE INTO ui_layout (key, value)
SELECT 'modes.' || m.mode || '.project.width', l.value
FROM (SELECT 'studio' AS mode UNION ALL SELECT 'api' AS mode UNION ALL SELECT 'automations' AS mode UNION ALL SELECT 'docker' AS mode) m, ui_layout l WHERE l.key = 'panel.project.width';
INSERT OR IGNORE INTO ui_layout (key, value)
SELECT 'modes.' || m.mode || '.operations.height', l.value
FROM (SELECT 'studio' AS mode UNION ALL SELECT 'api' AS mode UNION ALL SELECT 'automations' AS mode UNION ALL SELECT 'docker' AS mode) m, ui_layout l WHERE l.key = 'panel.operations.height';
DELETE FROM ui_layout WHERE key IN ('panel.project.width', 'panel.operations.height');
