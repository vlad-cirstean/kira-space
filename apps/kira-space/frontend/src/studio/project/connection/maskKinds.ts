import { applyVisible, type MaskKind } from '@shared/domain/mask';
import { type MessageKey, t } from '@theme/i18n';

// One list of the six mask kinds for the Privacy tab and the grid header menu.
export const MASK_KIND_OPTIONS: readonly { value: MaskKind; label: MessageKey }[] = [
  { value: 'name', label: 'common.field.name.label' },
  { value: 'email', label: 'common.field.email.label' },
  { value: 'text', label: 'common.field.text.label' },
  { value: 'number', label: 'common.field.number.label' },
  { value: 'date', label: 'common.field.date.label' },
  { value: 'redact', label: 'studio.project.connectionDialog.redact.label' },
];

export const MASK_KIND_EXPLANATION: Record<MaskKind, MessageKey> = {
  name: 'studio.project.connectionDialog.maskKind.name.description',
  email: 'studio.project.connectionDialog.maskKind.email.description',
  text: 'studio.project.connectionDialog.maskKind.text.description',
  number: 'studio.project.connectionDialog.maskKind.number.description',
  date: 'studio.project.connectionDialog.maskKind.date.description',
  redact: 'studio.project.connectionDialog.maskKind.redact.description',
};

// keepHint's label changes per kind; it is hidden for the three kinds where it means nothing.
export const KEEP_HINT_LABEL: Partial<Record<string, MessageKey>> = {
  name: 'studio.project.connectionDialog.keep.initials.label',
  email: 'studio.project.connectionDialog.keep.domain.label',
  date: 'studio.project.connectionDialog.keep.year.label',
};

const MASK_SAMPLE: Record<MaskKind, MessageKey> = {
  name: 'studio.project.connectionDialog.privacy.sampleName.example',
  email: 'studio.project.connectionDialog.privacy.sampleEmail.example',
  text: 'studio.project.connectionDialog.privacy.sampleText.example',
  number: 'studio.project.connectionDialog.privacy.sampleNumber.example',
  date: 'studio.project.connectionDialog.privacy.sampleDate.example',
  redact: 'studio.project.connectionDialog.privacy.sampleRedact.example',
};

// A number is many-to-one (a bucket) and a date is a poor join key, so neither correlates by default.
export function defaultCorrelateFor(kind: MaskKind): boolean {
  return kind !== 'number' && kind !== 'date';
}

function isMaskKind(kind: string): kind is MaskKind {
  return MASK_KIND_OPTIONS.some((o) => o.value === kind);
}

/** What the agent sees for a fixed sample of `rule.kind`; the tag is a sample, no key is involved. */
export function maskExample(rule: { kind: string; keep_hint?: boolean; correlate?: boolean }): {
  input: string;
  output: string;
} {
  const kind: MaskKind = isMaskKind(rule.kind) ? rule.kind : 'redact';
  const input = t(MASK_SAMPLE[kind]);
  const visible = applyVisible({ kind, keepHint: rule.keep_hint ?? true, correlate: false }, input);
  const tagged = kind !== 'number' && (rule.correlate ?? true);
  return {
    input,
    output: tagged
      ? t('studio.project.connectionDialog.privacy.sampleTagged.example', { masked: visible })
      : visible,
  };
}
