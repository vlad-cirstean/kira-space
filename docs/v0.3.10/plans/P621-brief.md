# P621 brief (row added to SPEC.md after P620 lands)

Title: Proof window: save screenshots dropdown and copy-all

- Fix broken "save screenshots" dropdown in the proof window (reproduce, root-cause).
- Default action becomes "copy all screenshots to clipboard" only where the platform can put several images on the clipboard (native, from Go). No stitched single image, ever.
- Where it cannot (mac, if multi-image clipboard is not possible): default stays the existing save-to-folder action.
- Planner verifies per platform (macOS pasteboard, Windows, Linux) what multi-image copy is possible; Chrome's web clipboard API rejects several image items, so native Go path only.
- Tests: backend flow test and UI spec for same scenario.
