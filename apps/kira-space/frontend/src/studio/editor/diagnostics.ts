// P18 addendum D24: pure text in, diagnostics out — CodeMirrorHost.vue owns wrapping this in
// @codemirror/lint's linter()/compartment/theming, so a caller never sees an EditorView or a
// @codemirror/lint import.
export interface ConsoleDiagnostic {
  from: number;
  to: number;
  severity: 'error' | 'warning';
  message: string;
}
