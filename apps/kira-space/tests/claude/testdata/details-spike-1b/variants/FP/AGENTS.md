# Formats
Finish a step: write handovers/draft-<step>.md (Markdown, <=16 KiB), then call `finish_step`.
proofs/<slug>/proof.md: one file, YAML frontmatter plus optional Markdown body. Step data is inline.
---
kira: proof/1
name: <1-80 chars>
summary: <1-3 sentences>
reason: <required when changing an accepted proof>
steps:
  - {id: <stable>, description: <<=200 chars>, view: table|document|keyvalue|chart|diagram|image, inline: <data>}
---
inline per view: table `{columns:[{name}],rows:[[..]]}`; document `{documents:[{body:{}}]}`; keyvalue `{entries:[{field,value}]}`; diagram Mermaid text (block scalar `|`). An image is a JPEG copied into the folder, referenced as `data: <file>` instead of `inline`.
Changing an accepted proof: keep unchanged step ids; submit answers pending until the user accepts.
