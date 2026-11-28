-- P764: task colour slots shrink from 20 to 10. Slots 10-19 fold onto 0-9.
UPDATE ade_tasks SET color = color % 10 WHERE color >= 10;
