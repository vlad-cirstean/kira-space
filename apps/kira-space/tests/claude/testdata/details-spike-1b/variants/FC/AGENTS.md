# Formats
Finish a step: write your draft as the last section `## draft-<step>` of handovers.md (Markdown, <=16 KiB), then call `finish_step`.
proofs/<slug>/proof.md: YAML frontmatter, optional Markdown body.
---
kira: proof/1
name: <1-80 chars>
summary: <1-3 sentences>
reason: <required when changing an accepted proof>
steps:
  - {id: <stable>, description: <<=200 chars>, view: table|document|keyvalue|chart|diagram|image, data: <file in folder>}
---
Data files: table `{columns:[{name}],rows:[[..]]}`; document `{documents:[{body:{}}]}`; keyvalue `{entries:[{field,value}]}`; diagram `.mmd` Mermaid; image a JPEG copied into the folder.
Changing an accepted proof: keep unchanged step ids and files; submit answers pending until the user accepts.
