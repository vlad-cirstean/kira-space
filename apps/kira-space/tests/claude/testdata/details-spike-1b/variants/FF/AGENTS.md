# Kira task folders

Every task has a folder with README.md, task.yaml and state.yaml (Kira, read-only), notes/ (user),
handovers/ (you and Kira), proofs/ (you) and links.yaml. Edit files, then call the `submit` tool. Kira
accepts a file, holds it for the user's approval, or rejects it, and tells you which. Read the task's
README.md first.

Handovers: handovers/draft-<step>.md is your hand-over notes to the next stage, Markdown, at most
16 KiB. Name the step you are in. Rewrite to replace. Handed handovers, handovers/NNNN-<from>-to-<to>.md,
are read only. Finish a step by writing the draft, then calling `finish_step`.

Read-only files: README.md, task.yaml, state.yaml. Edits are undone.

## Proofs

Prove functionality and behavior, not code: show what the system does. One folder per proof. proof.md
is YAML frontmatter plus an optional Markdown explanation:

    ---
    kira: proof/1
    name: <unique name, 1-80 characters>
    summary: <1-3 terse sentences, up to 500 characters>
    reason: <what changes and why; required when you change an accepted proof>
    steps:
      - id: <stable id; keep it when you change the step>
        description: <one terse line, at most 200 characters: what the step shows and proves>
        view: table | document | keyvalue | chart | diagram | image
        data: <file in this folder>
        query: <optional: query or command you ran>
        language: <optional: sql, json, shell or text>
        source: <optional: short label of the producing tool>
    ---

Steps show in order, 1 to 100. Chart plots a table and shows no table: add a table step when exact
values matter. Data files:

- table: JSON `{columns:[{name,type?}],rows:[[cell,...]]}`; type number, text, boolean, temporal, json or other.
- document: JSON `{documents:[{id?,body:{...}}]}`. keyvalue: JSON `{entries:[{field,value}]}`.
- chart, bar, line, area or scatter: JSON `{type,x:{type,name?},y?:{name?,unit?},stacked?,horizontal?,series:[{name,points:[[x,y],...]}]}`;
  x.type is category (x a string label), time (ISO-8601 or epoch ms) or value (number). Up to 20 series, 1000 categories, 20000 points.
  Pie: `{type,unit?,slices:[{name,value}]}`, up to 50 slices.
- diagram: a `.mmd` file of Mermaid text whose first line starts with flowchart, graph, sequenceDiagram, classDiagram, stateDiagram,
  erDiagram, gantt, journey, gitGraph, mindmap or timeline. No front matter, directives, click lines, url() or image nodes.
  Up to 20000 bytes, 1000 lines. Quote labels that hold punctuation: A["label (x)"].
- image: a JPEG file in this folder (copy it in; absolute paths are not read). At most 3071 KiB, 16384 pixels a side.

Limits: 10000 rows or documents, 64 KiB per cell, 4 MiB per step, 32 MiB per proof. An accepted proof
you change returns pending until the user accepts; keep unchanged steps' ids and files as they are.
