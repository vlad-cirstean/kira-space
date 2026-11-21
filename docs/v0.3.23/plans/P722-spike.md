# P722 spike: drag a tab back into another window

Question: can a tab dragged out of one Kira Space window drop into another native window, and by what
mechanism. Findings only; no production change. Base: `chapter/v0.3.23` at `06f975b5b`.

## Verdict

Feasible with caveats, on macOS, Windows and Linux X11. Not feasible on Linux Wayland (no global
window position). Recommended mechanism: (a), extended onto the existing Sortable drag. Frontend
reports the drop point at `pointerup`. Go hit-tests its own window bounds. No cgo, no native drag,
no new dependency.

## 1. Wails v3 APIs (pinned `github.com/wailsapp/wails/v3 v3.1.0`, `go.mod:58`)

Paths relative to `$GOMODCACHE/github.com/wailsapp/wails/v3@v3.1.0/pkg/`.

- Global cursor position: no public API. Private helpers only:
  - Linux `getMousePosition()` (`application/linux_cgo.go:807`, GTK3 `linux_cgo_gtk3.go:1232`,
    `gdk_device_get_position`), used by the system tray only.
  - Linux `getCursorPositionFromCompositor()` (`application/environment_linux.go:52`): Hyprland and
    Sway IPC only.
  - Windows `w32.GetCursorPos()` (`application/popupmenu_windows.go:392`, `menu_windows.go:188`):
    `w32` is importable (`pkg/w32`), Windows only.
  - macOS `[NSEvent mouseLocation]` in tray code only (`application/systemtray_darwin.m:251`).
- Window under a screen point: none. No z-order API. `App.Window.GetAll()`
  (`application/window_manager.go:124`) plus `Bounds()` per window is the only route.
- Window bounds: `WebviewWindow.Bounds()` (`application/webview_window.go:1100`), `Position()`
  (`:1122`), `SetPosition()` (`:1134`), `GetScreen()` (`:1435`), `IsFocused()` (`:819`).
  - macOS: points, top-left origin of primary screen (`application/webview_window_darwin.go:781`).
    Matches WebKit `screenX/screenY`.
  - Windows: DIP, `PhysicalToDipRect(physicalBounds())` (`application/webview_window_windows.go:903`).
    Frame rect, includes non-client area.
  - Linux: `position()` returns `0,0` off X11 (`application/linux_cgo.go:1249`). Wayland cannot
    report or set a global window position.
- Screen helpers: `ScreenNearestDipPoint`, `PhysicalToDipPoint`, `DipToPhysicalPoint`
  (`application/screenmanager.go:925-992`).
- Window-move events: `events.Common.WindowDidMove` (`events/events.go:16`), already used by
  `shell.Attach` (`internal/shell/window.go:162`). Windows also has `WindowEndMove`.
  `events.Common.WindowFocus` exists for focus-order tracking.
- Mouse events outside the webview: no Go-side mouse event API. Only webview DOM events.
- Native drag: `startDrag()` (`application/webview_window.go:1746`, message `wails:drag` at `:848`)
  is a window-move drag (`--wails-draggable`), not a drag-and-drop session. No API starts an
  `NSDraggingSession`, OLE `DoDragDrop` or GTK drag source. Drop handling is file-only
  (`WindowFilesDropped`, Linux `onDropFiles` at `application/linux_cgo.go:1643`); Kira sets
  `EnableFileDrop: false` (`internal/shell/window.go:81`).
- Per-window notify: `WebviewWindow.EmitEvent` (`application/webview_window.go:260`) reaches the
  target window live.

## 2. HTML5 drag/drop payload across two webview windows

No evidence in Wails source either way; Wails neither starts nor routes HTML5 drags. Engine facts,
all unverified here (only Linux, headless, no Wails display):

- WKWebView: HTML5 drag becomes an `NSDraggingSession` on the pasteboard. `text/plain` crosses
  windows. Custom types go to WebKit's custom pasteboard data, same-origin gated; both Kira windows
  share an origin, so likely readable. Unverified.
- WebView2: OLE drag; Chromium custom types use its own clipboard format, which Chrome reads
  across its own windows. WebView2 may gate external drops (`AllowExternalDrop`). Unverified.
- WebKitGTK: GTK drag source and drop target per webview; same-process cross-window drop expected
  to work for `text/plain`. Wayland drags work (protocol-level). Unverified.

Even where the payload crosses, HTML5 DnD costs more than it saves: it replaces Sortable's
`forceFallback` pointer drag in `useSortableReorder`
(`packages/workbench/src/behaviour/sortableReorder.ts:87`), WebKit's `dragend` coordinates are
unreliable outside the window, and drop outside every window needs `dragend` screen coords anyway.

## 3. Candidate mechanisms

(a) Pointer drag, screen point to Go, Go hit-tests bounds. Recommended.

- Tab drag already runs on pointer events: SortableJS 1.15.7 `forceFallback` binds `pointermove`
  and `pointerup` on `document` (`sortable.esm.js:1313`, `:1383`), no `setPointerCapture`. Its
  `end` event carries `originalEvent` (`sortable.esm.js:2023-2028`). A drop outside the viewport
  (`clientX/clientY` out of bounds) reaches `onEnd` with the pointer position.
- Delivery outside the window rests on OS implicit grab while the button is down: AppKit sends
  `mouseDragged`/`mouseUp` to the window that got `mouseDown`; Win32 Chromium calls `SetCapture`
  on press; GDK keeps an implicit grab on press (X11 and Wayland). Proven in headless Blink and
  WebKit only (section 4). Add `setPointerCapture` on the chip at drag start as belt and braces.
- Go side: new bound call, e.g. `DropTab(srcKey, tabID, screenX, screenY)`. Walk the registry's
  live windows, skip source, keep those whose `Bounds()` contains the point, pick the most recently
  focused (MRU from `WindowFocus`, since Wails has no z-order). Hit: atomic re-key plus
  `EmitEvent` to target. Miss: open new window at the point (`OpenNewWindowKeyed` with bounds),
  seeded with the tab.
- Pros: no cgo, no native API, no new dependency, reuses Sortable. Same code on 3 OSes.
- Cons: no visual outside the window (Sortable's clone stays clipped to the source webview).
  Coordinate spaces differ per OS (see risks). Wayland excluded.
- Effort: low. One bound method, one pure hit-test function, Sortable `onEnd` hook.

(b) Native cursor polling per platform via cgo.

- `NSEvent mouseLocation`, `GetCursorPos` (`pkg/w32` already has it), `gdk_device_get_position`.
- Pros: exact OS cursor, independent of webview events.
- Cons: 3 cgo/purego paths in Kira, polling loop, still needs a start/stop signal from the webview.
  Adds nothing over (a) once the webview reports `pointerup`. Wayland still blind.
- Effort: medium to high. Only fallback if (a) fails on a platform.

(c) HTML5 DnD with Wails drop handling.

- Wails drop handling is file-only; Kira's would be hand-built in DOM `drop` handlers.
- Cons: rewrites TabStrip drag, cross-webview payload unverified on all 3, drop-in-empty-space
  still needs (a)'s screen point. Only gain: native drag image outside the window.
- Effort: high, risk high.

(d) Tear-off only; re-attach via "Move tab to window" menu.

- Pros: trivial, works on Wayland, accessible (keyboard), no coordinate math.
- Cons: not Chrome-like drag-back; misses the user's ask on its own.
- Effort: lowest. Worth shipping alongside (a) as keyboard path and Wayland fallback.

Cheapest mechanism with Chrome-like drag back: (a). Add (d) as the keyboard and Wayland path.

## 4. Proof (Linux, Playwright 1.64.1, headless)

Script: element at viewport 400x300; `pointerdown`, move inside, to `(-150,120)`, to `(900,700)`,
`pointerup`. Run with `PLAYWRIGHT_BROWSERS_PATH=/opt/pw-browsers node capture-proof.cjs`.

```js
const pw = require('playwright-core');
const html = `<div id="tab" style="position:absolute;left:20px;top:20px;width:120px;height:30px">tab</div>
<script>
window.log = [];
const tab = document.getElementById('tab');
const rec = (src) => (e) => log.push({ src, type: e.type, cx: e.clientX, cy: e.clientY, sx: e.screenX, sy: e.screenY });
tab.addEventListener('pointerdown', (e) => { if (window.capture) tab.setPointerCapture(e.pointerId); rec('tab')(e); });
for (const t of ['pointermove', 'pointerup']) { tab.addEventListener(t, rec('tab')); document.addEventListener(t, rec('doc')); }
</script>`;
(async () => {
  for (const name of ['chromium', 'webkit']) for (const capture of [true, false]) {
    const browser = await pw[name].launch();
    const page = await browser.newPage({ viewport: { width: 400, height: 300 } });
    await page.setContent(html);
    await page.evaluate((c) => { window.capture = c; }, capture);
    await page.mouse.move(60, 35); await page.mouse.down();
    await page.mouse.move(200, 150, { steps: 2 });
    await page.mouse.move(-150, 120, { steps: 2 });
    await page.mouse.move(900, 700, { steps: 2 });
    await page.mouse.up();
    const log = await page.evaluate(() => window.log);
    const out = log.filter((e) => e.cx < 0 || e.cy < 0 || e.cx > 400 || e.cy > 300);
    const up = log.filter((e) => e.type === 'pointerup').map((e) => [e.src, e.cx, e.cy, e.sx, e.sy]);
    console.log(name, 'capture=' + capture, 'outside=' + out.length, 'up=' + JSON.stringify(up));
    await browser.close();
  }
})();
```

Result:

```
chromium capture=true  outside=8 up=[["tab",900,700,900,700],["doc",900,700,900,700]]
chromium capture=false outside=4 up=[["doc",900,700,900,700]]
webkit   capture=true  outside=8 up=[["tab",900,700,0,0],["doc",900,700,0,0]]
webkit   capture=false outside=4 up=[["doc",900,700,0,0]]
```

Proves: Blink and WebKit DOM keep dispatching `pointermove`/`pointerup` with out-of-viewport
`clientX/clientY` once a press is under way, with or without `setPointerCapture` (document
listeners, Sortable's setup). `clientX/clientY` stay correct outside the viewport.

Does not prove: OS delivery to a real WKWebView, WebView2 or WebKitGTK window. Playwright injects
input below the OS, so implicit-grab behavior is untested. WebKit `screenX/screenY` read `0,0` under
synthetic input; whether real WKWebView/WebKitGTK fill them is unverified. Hence the frontend sends
`clientX/clientY` too and Go can derive the screen point from the source window's position.

Hit-test needs no separate proof: pure rect containment plus MRU order, written as one function
with a unit test in the phase itself.

## 5. Risks the planner decides

1. Coordinate space. macOS: WebKit `screenX` and Wails `Bounds()` are both points, top-left of
   primary screen; Kira uses `FullSizeContent`, so client origin equals frame origin. Windows:
   `Bounds()` is the DIP frame rect incl. title bar; Chromium `screenX` is DIP but mixed-DPI
   multi-monitor mapping is unverified. Pick: trust `screenX` vs. `Position() + clientX + offset`.
2. Wayland: no global position (`linux_cgo.go:1249`). Drag-back impossible; tear-off opens at
   default or cascade position. Decide: disable drag-out on Wayland, or tear-off only there.
3. Overlap: no z-order API. MRU focus order is a proxy; a never-focused window under another can
   win wrongly. Acceptable?
4. Drop zone: whole target window (append at end) vs. tab strip only (Chrome). Strip-only needs
   each window to report its strip rect or the target to map the screen point locally.
5. Visual feedback: Sortable clone clips at the window edge. Options: none (drop-time window), or
   Chrome-style live window created on leave and moved via `SetPosition` per throttled move.
   Live window risk: new window taking focus may end the source grab on some OS.
6. Non-persistable tabs (terminal, script-run) and `dropResources` page stores: move only
   persistable tabs, or re-attach backend sessions (SPEC constraint).
7. Last tab dragged out of a window: close source window or keep it empty.
8. Esc during drag: existing `dragCancelled` must also cancel the out-of-window drop.

## Manual checklist (macOS and Windows, user)

Run on a real build with two Kira windows side by side, then overlapping.

1. Drag a tab off the strip and outside the window; release over empty desktop. `pointerup` reaches
   the source window (log shows it) and `screenX/screenY` match the cursor.
2. Same, release over the second window. Reported point lies inside that window's `Bounds()`.
3. Release far off any window, on a second monitor (mixed DPI on Windows). Point maps to that
   monitor.
4. Release over the menu bar or Dock (macOS) or taskbar (Windows). Drop still arrives, no crash.
5. Press Esc while outside the window, then release. Nothing moves.
6. Overlapping windows: drop over the overlap. Topmost (last focused) window wins.
7. Fast flick out and release within 100 ms. Drop still arrives.
8. Windows only: window maximized, and window with title bar; compare `screenX` against
   `Bounds()`.
