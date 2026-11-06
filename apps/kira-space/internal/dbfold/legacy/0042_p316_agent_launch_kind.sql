-- P316: the terminal launch kind 'claude-code' is now 'agent'.
UPDATE tabs SET state_json = json_set(state_json, '$.launchKind', 'agent')
WHERE CASE WHEN json_valid(state_json) THEN json_extract(state_json, '$.launchKind') END = 'claude-code';
