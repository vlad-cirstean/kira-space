// Pieces of a kube read's args as the approval card draws them.

/** A kind key (v1/pods, apps/v1/deployments) as kubectl's resource and group. */
export function kubeResource(key: string): { name: string; group: string } {
  const parts = key.split('/');
  if (parts.length === 2) return { name: parts[1] ?? key, group: '' };
  if (parts.length === 3) return { name: parts[2] ?? key, group: parts[0] ?? '' };
  return { name: key, group: '' };
}

export interface SelectorToken {
  kind: 'key' | 'op' | 'value';
  text: string;
}

/** A label selector split into key, operator and value tokens; a requirement without an operator is one key. */
export function kubeSelectorTokens(selector: string): SelectorToken[] {
  const out: SelectorToken[] = [];
  splitRequirements(selector).forEach((req, i) => {
    if (i > 0) out.push({ kind: 'op', text: ',' });
    const m = /^([^!=\s]+)(!=|==|=)(.*)$/.exec(req);
    if (!m) {
      out.push({ kind: 'key', text: req });
      return;
    }
    out.push({ kind: 'key', text: m[1] ?? '' }, { kind: 'op', text: m[2] ?? '' });
    if (m[3]) out.push({ kind: 'value', text: m[3] });
  });
  return out;
}

// Commas inside `in (a,b)` belong to the requirement.
function splitRequirements(selector: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < selector.length; i++) {
    const c = selector[i];
    if (c === '(') depth++;
    else if (c === ')') depth = Math.max(0, depth - 1);
    else if (c === ',' && depth === 0) {
      out.push(selector.slice(from, i));
      from = i + 1;
    }
  }
  out.push(selector.slice(from));
  return out.filter((r) => r !== '');
}
