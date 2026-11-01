import type { ScriptKind, ScriptParam } from '@shared/domain/scripts';
import { BUILTIN_VARS } from '@shared/domain/scripts';
import { t } from '@theme/i18n';
import type { TextPart } from '@theme/varText';
import type { Completion } from '../editor/fieldCompletion';
import type { RangeHighlight } from '../editor/ranges';

export interface ScriptVarSource {
  kind: ScriptKind;
  params: readonly ScriptParam[];
  /** Kira Space: built-in task variables exist only there. */
  ade: boolean;
  /** Secret params with a saved value. */
  saved?: readonly string[];
}

const PARAM_ENV_PREFIX = 'KIRA_PARAM_';
const BUILTIN_ENV_PREFIX = 'KIRA_';

export function paramEnvName(name: string): string {
  return PARAM_ENV_PREFIX + name.toUpperCase();
}

function builtinEnvName(name: string): string {
  return BUILTIN_ENV_PREFIX + name.toUpperCase();
}

const PROMPT_VAR_RE = /\{([A-Za-z_][A-Za-z0-9_]*)\}/g;
const ENV_VAR_RE = /\$(?:\{(KIRA_[A-Za-z0-9_]*)\}|(KIRA_[A-Za-z0-9_]*))/g;

function paramDetail(p: ScriptParam): string {
  return p.default.join(', ') || p.type;
}

function builtins(src: ScriptVarSource): readonly string[] {
  return src.ade ? BUILTIN_VARS : [];
}

/** Suggestions for the token under the caret: `{name` in a prompt, `$KIRA_…` in a command. */
export function scriptCandidates(
  src: ScriptVarSource,
): (ctx: { text: string; from: number; word: string }) => Completion[] {
  return ({ text, from, word }) => {
    if (src.kind === 'smart') {
      const closed = text[from + word.length] === '}';
      const close = closed ? '' : '}';
      const out: Completion[] = src.params
        .filter((p) => !p.secret && p.name !== '')
        .map((p) => ({
          label: p.name,
          insert: p.name + close,
          detail: paramDetail(p),
          icon: 'symbol-variable',
        }));
      for (const b of builtins(src))
        out.push({
          label: b,
          insert: b + close,
          detail: t('automations.script.fromTask.status'),
          icon: 'symbol-variable',
        });
      return out;
    }
    const braced = text.slice(from - 2, from) === '${' && text[from + word.length] !== '}';
    const close = braced ? '}' : '';
    const out: Completion[] = src.params
      .filter((p) => p.name !== '')
      .map((p) => ({
        label: paramEnvName(p.name),
        insert: paramEnvName(p.name) + close,
        detail: p.secret ? t('common.secret.label') : paramDetail(p),
        icon: 'symbol-variable',
      }));
    for (const b of builtins(src))
      out.push({
        label: builtinEnvName(b),
        insert: builtinEnvName(b) + close,
        detail: t('automations.script.fromTask.status'),
        icon: 'symbol-variable',
      });
    return out;
  };
}

interface VarRef {
  from: number;
  to: number;
  /** The param or built-in name the reference points at; `null` for an unknown env name. */
  name: string | null;
  param: ScriptParam | null;
  builtin: boolean;
}

function refsIn(text: string, src: ScriptVarSource): VarRef[] {
  const out: VarRef[] = [];
  const paramBy = (n: string) => src.params.find((p) => p.name === n) ?? null;
  const isBuiltin = (n: string) => builtins(src).includes(n);
  if (src.kind === 'smart') {
    for (const m of text.matchAll(PROMPT_VAR_RE)) {
      const name = m[1] ?? '';
      const param = paramBy(name);
      const builtin = !param && isBuiltin(name);
      if (!param && !builtin) continue;
      out.push({ from: m.index, to: m.index + m[0].length, name, param, builtin });
    }
    return out;
  }
  for (const m of text.matchAll(ENV_VAR_RE)) {
    const env = m[1] ?? m[2] ?? '';
    const from = m.index;
    const to = from + m[0].length;
    if (env.startsWith(PARAM_ENV_PREFIX)) {
      const param = src.params.find((p) => paramEnvName(p.name) === env) ?? null;
      out.push({ from, to, name: param?.name ?? null, param, builtin: false });
      continue;
    }
    const b = BUILTIN_VARS.find((v) => builtinEnvName(v) === env);
    if (b) out.push({ from, to, name: src.ade ? b : null, param: null, builtin: src.ade });
  }
  return out;
}

/** Known vars paint as variables; a secret param in a prompt (refused on save) and an unknown
 *  `$KIRA_PARAM_X` (a typo) paint as unknown. Unknown `{word}` in a prompt stays literal text. */
export function scriptHighlights(src: ScriptVarSource): (doc: string) => RangeHighlight[] {
  return (doc) =>
    refsIn(doc, src).map((r) => ({
      from: r.from,
      to: r.to,
      class:
        r.name === null || (src.kind === 'smart' && r.param?.secret)
          ? 'kira-ed-var-unknown'
          : 'kira-ed-var',
    }));
}

export function scriptHover(
  src: ScriptVarSource,
): (text: string, offset: number) => string[] | null {
  return (text, offset) => {
    const ref = refsIn(text, src).find((r) => offset >= r.from && offset <= r.to);
    if (!ref) return null;
    if (ref.builtin) return [t('automations.script.adeSet.tooltip')];
    if (!ref.param) return [t('automations.script.unknownParam.tooltip')];
    const lines = [t('automations.script.paramNamed.label', { name: ref.param.name })];
    if (src.kind === 'smart' && ref.param.secret)
      lines.push(t('automations.script.secretInPrompt.error'));
    else if (ref.param.secret) lines.push(t('common.secret.label'));
    else if (ref.param.default.length > 0)
      lines.push(
        t('automations.script.defaultValue.label', { value: ref.param.default.join(', ') }),
      );
    return lines;
  };
}

/** Names of the params and built-ins the body uses, in order of first use. */
function envVarsUsed(command: string, params: readonly ScriptParam[], ade: boolean): string[] {
  const names: string[] = [];
  for (const r of refsIn(command, { kind: 'script', params, ade })) {
    if (r.name !== null && !names.includes(r.name)) names.push(r.name);
  }
  return names;
}

/** Names a prompt uses in `{…}`, restricted to the ones the run substitutes. */
function promptVarsUsed(prompt: string, params: readonly ScriptParam[], ade: boolean): string[] {
  const names: string[] = [];
  for (const r of refsIn(prompt, { kind: 'smart', params, ade })) {
    if (r.name !== null && !names.includes(r.name)) names.push(r.name);
  }
  return names;
}

function usesValue(name: string, src: ScriptVarSource): string {
  const { params } = src;
  const p = params.find((x) => x.name === name);
  if (!p) return t('automations.script.fromTask.status');
  if (p.secret && src.saved?.includes(name))
    return t('automations.params.secretValue.saved.status');
  return p.default.length > 0 && !p.secret
    ? p.default.join(', ')
    : t('automations.script.askedAtRun.status');
}

/** The "Uses:" line: each used var with what the run gets without any input. */
export function usesParts(src: ScriptVarSource, body: string): TextPart[] {
  const names =
    src.kind === 'smart'
      ? promptVarsUsed(body, src.params, src.ade)
      : envVarsUsed(body, src.params, src.ade);
  const parts: TextPart[] = [`${t('automations.script.uses.label')} `];
  names.forEach((n, i) => {
    if (i > 0) parts.push(', ');
    parts.push({ name: n, value: usesValue(n, src) });
  });
  return parts;
}
