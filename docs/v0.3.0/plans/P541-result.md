# P541 Result (draft)

Implementer: Sonnet subagent, stream/p541. Commits: storage and validation, proof diagram flow, Mermaid
window view, UI spec, docs.

Proof steps take view `diagram`: Mermaid text in `data.source`, validated in Go (`proof/diagram.go`:
header allow list, no front matter, directives, click lines, `url(`, `@import`, control characters; 20000
bytes, 1000 lines). Migration `0065_p541_proof_diagram_view.sql` rebuilds the proof tables. The window
renders through `diagramRender.ts`: lazy `mermaid` 12.2.0 and `dompurify` 3.4.17, strict mode, no HTML
labels, private DOMPurify pass, CSP as last layer. Parse errors show the message above the source.

Checks: `TestProofDiagram` and all proofflow tests pass; `proof-diagram`, `proof-chart`, `proof-window` UI
specs pass (23); `checkprompts`, lint, typecheck, `lint:dead`, build pass. Main entry chunk holds no
mermaid code (dynamic import only). Real `claude` proof run: not run, no login or API key in the sandbox.

Deviations: no existing dark-mode resolver exists, so `darkMode` derives from the luminance of the
elevated-background token. Mermaid 12.2.0 cannot draw image nodes under the CSP. Go does not refuse them; the tool text tells agents to send no images. A stored image node shows the error state with its source, so the UI spec asserts that and zero remote requests instead of a drawn image. Header keywords confirmed against the 12.2.0 detectors for classDiagram, erDiagram,
gantt, gitGraph, journey, mindmap, stateDiagram, timeline; flowchart, graph and sequenceDiagram confirmed
by the passing renders.
