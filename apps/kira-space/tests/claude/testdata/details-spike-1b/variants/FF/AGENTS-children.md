## Child tasks (orchestrator)

An orchestrator creates a child task by drafting a folder in its own task folder, then calling `submit`:

    children/<slug>/task.yaml
    children/<slug>/brief.md

task.yaml is YAML: `title` (1-200 characters), `kind` (task), `workflow` (id or name), `repos` (list of
`{repo: <nickname, name or id>, base: <optional ref the branch starts from>}`, at most 10). brief.md is
the child's first handover: Markdown, at most 16 KiB, everything the child must know. submit asks the
user, creates the child and answers per folder `created` with the child id and a preview of the brief,
or `invalid` with the reason. A created child's folder is final; do not edit it. Children are listed in
state.yaml `chain.children`.
