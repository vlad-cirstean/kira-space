<script setup lang="ts" generic="T extends Record<string, Record<string, unknown>>">
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter as UiDialogFooter,
} from '@theme/components/ui/dialog';
import { t } from '@theme/i18n';
import { rowVariants } from '@theme/lib/rowVariants';
import type { ComputedRef, Ref } from 'vue';
import { computed, onUnmounted, reactive, ref } from 'vue';
import { errorText } from '../bridge/codedError';

// P103 Part 2 (§5.5): Kira Studio's own workbench/SettingsDialog.vue and Kira Space's, unified —
// the frame (DialogFrame + its "Settings" header), the nav (`sections`/`activeSection`) and the
// whole draft/dirty/reset engine (`cloneSections`/`baseline`/`draft`/`valuesEqual`/`diffSection`/
// `pendingPatch`/`isDirty`/`isAtDefault`/`resetLeaf`/`onDismiss`/`saveError`/`onSave`) were
// line-for-line identical across both files (§1.6's own "confirmed shared" survey) — moved here
// generic over `T`, the app's own settings shape (each app's own `state/settingsDomain.ts` `Settings`
// type, P103 Part 4 §7.3 — three sections for Kira Space, eight for Kira Studio).
//
// Two things the plan's own condensed sketch (§5.5) named that this file does differently, found
// reading both apps' real files side by side rather than trusting the sketch:
//
// 1. "`onDismiss` with the discard-confirm guard" — neither app's `onDismiss` has one. Kira
//    Studio's own comment on it is explicit: "no IPC call, no confirmation (see the plan doc's own
//    D5 for why not)" (P17 D5); Kira Space's is byte-identical in substance. `confirmDialogStore`
//    is real in both files, but gates a *different* action each (Kira Studio: removing a custom
//    script; Kira Space: revoking a paired editor) — never `onDismiss`. Adding a guard neither app
//    has today would be a behaviour change this phase doesn't own; `onDismiss` here stays the
//    plain `emit('close')` both apps already ship.
// 2. The footer markup itself — the Cancel/Save wrapper is byte-identical between the two apps
//    (`class="flex items-center gap-1"`), but `.footer-status` differs: Kira Studio's uses
//    FieldError/FieldDescription (P110 B12's own component swap), Kira Space's keeps plain spans
//    with the equivalent utility classes (see that file's own P110 B12 comment for why) — not
//    documented as a real difference anywhere, only found by diffing the two `<template #footer>`
//    blocks. Rather than pick one app's markup as canonical for both (a real, if small, behaviour
//    change for whichever app didn't have it), the footer stays a `#footer` scoped slot each app
//    fills with its own existing markup, the same "markup that differs stays app-side, state that
//    doesn't moves here" split §5.4 already used for TabStrip's "+" button.
//
// Field validity comes up from the panes rather than this file knowing any app's own leaves: the
// `#pane` slot exposes `registerFieldError`, which each pane calls once (its own component's
// lifetime is the whole dialog's, not just while its own section is the active one — see below)
// with a `ComputedRef<string | null>` for each field it validates. `isValid` is every registered
// error being falsy, AND-ed together — precisely what a single shared script's own flat
// `computed(() => !err1.value && !err2.value && …)` already did.
//
// Every pane stays mounted for the dialog's whole lifetime, not just while its own section is
// active — `v-show`, not `v-if`, on each pane's own root (each app's thin `SettingsDialog.vue`
// wrapper passes `active="s.activeSection === '<Section>'"` down to its own pane components). This
// is not a style choice: in the original single-file component, every section's `computed`
// validators, `watch`es and store reads were already always live regardless of which section's
// `<template v-if>` branch was in the DOM (template conditionals don't tear down a shared script's
// own reactive state) — a v-if-gated pane component would instead destroy its computeds, and
// therefore its registered field error, the moment the user switches away from its tab, silently
// re-enabling Save on an invalid draft in a section that isn't currently visible. `v-show` (paired
// with `display: contents` on each pane's own root, so it doesn't add a flex item to
// `.section-pane`'s own layout) keeps every pane's script — and by extension every registered field
// error and every already-instantiated Pinia store/`useQuery` — alive for as long as the dialog is
// open, exactly matching the original file's own posture where the "Database MCP" mask-rule count
// query doesn't care which section is on screen.
// The patch shape `save` receives, section by section — each present section only its own changed
// leaves (`SettingsPatch`'s own shape, generalised: every field of `settingsPatchSchema` is
// `<Section>.partial().optional()`, never the whole section).
type SectionPatch = { [K in keyof T]?: Partial<T[K]> };

const props = defineProps<{
  sections: readonly string[];
  /** Display label per section id; the id stays the test hook and deep-link key. */
  sectionLabels: Readonly<Record<string, string>>;
  initialSection?: string;
  defaults: T;
  current: T;
  save: (patch: SectionPatch) => Promise<void>;
  /** State kept outside the settings draft (the Prompts section's edits): the one Save runs its
   *  `save` first, then the settings patch. */
  extra?: { isDirty: boolean; isValid: boolean; save: () => Promise<void>; discard: () => void };
}>();

const emit = defineEmits<{ close: [] }>();

// Everything the user touches lives in this draft until
// Save — the store (and therefore every other window and the app's own rendering) sees nothing
// until then. This component is created on open and destroyed on close, which is the draft's whole
// lifetime — no reset logic needed. JSON round-trip rather than structuredClone(): a Pinia store's
// leaves are reactive proxies, and structuredClone's algorithm throws on a Proxy rather than
// cloning the plain data underneath it.
const cloneSections = (s: T): T => JSON.parse(JSON.stringify(s));

// Frozen at runtime (mutation would be a bug); cast back to plain T so diffSection can compare it
// against the mutable draft without a readonly/mutable type mismatch — mirrors both original files'
// own `as Settings`/`as Pick<Settings, …>` cast at this exact spot.
const baseline = Object.freeze(cloneSections(props.current)) as T;
const draft = reactive(cloneSections(props.current)) as T;

// An array-valued leaf (Kira Space's git.protectedBranches) gets its own array object from
// cloneSections even when unedited, so a bare `!==`/`===` would report it changed/non-default on
// every open; a record leaf (memory.models) likewise. Compared by value instead; every primitive
// leaf still takes the cheap `===` path.
function valuesEqual(a: unknown, b: unknown): boolean {
  if (Array.isArray(a) && Array.isArray(b)) {
    return a.length === b.length && a.every((v, i) => v === b[i]);
  }
  if (a && b && typeof a === 'object' && typeof b === 'object') {
    return JSON.stringify(a) === JSON.stringify(b);
  }
  return a === b;
}

// P17 D2: the generic per-leaf diff Save sends. Walking Object.keys(base) rather than a
// hand-maintained leaf list means a future leaf is picked up with no edit here.
function diffSection<S extends object>(base: S, current: S): Partial<S> | undefined {
  const changed: Partial<S> = {};
  let anyChanged = false;
  for (const key of Object.keys(base) as (keyof S)[]) {
    if (!valuesEqual(current[key], base[key])) {
      changed[key] = current[key];
      anyChanged = true;
    }
  }
  return anyChanged ? changed : undefined;
}

const pendingPatch = computed<SectionPatch>(() => {
  const patch: SectionPatch = {};
  for (const key of Object.keys(baseline) as (keyof T)[]) {
    const section = diffSection(baseline[key], draft[key]);
    if (section) patch[key] = section;
  }
  return patch;
});

const isDirty = computed(
  () => Object.keys(pendingPatch.value).length > 0 || props.extra?.isDirty === true,
);

// P85 §10.1: activeSection seeds from the app's own deep-link state (`initialSection`, e.g.
// CookiesPane.vue's "Edit global defaults…" landing on the 'Api' section) — undefined (a plain
// open) falls back to the first section, G12 D9's own default (both apps' first entry is
// "Appearance").
const activeSection = ref<string>(props.initialSection ?? props.sections[0]);

// P28 §2.2: two generic helpers replace an all-or-nothing Revert to Defaults — a future leaf needs
// no edit here either.
function isAtDefault<S extends keyof T, K extends keyof T[S]>(s: S, k: K): boolean {
  return valuesEqual(draft[s][k], props.defaults[s][k]);
}
function resetLeaf<S extends keyof T, K extends keyof T[S]>(s: S, k: K): void {
  draft[s][k] = props.defaults[s][k];
}

// Field validity, up from the panes (see the file-level comment above for why every pane stays
// mounted for this to be live regardless of the active tab).
const fieldErrors = reactive(new Map<string, Ref<string | null> | ComputedRef<string | null>>());
function registerFieldError(
  id: string,
  error: Ref<string | null> | ComputedRef<string | null>,
): void {
  fieldErrors.set(id, error);
}
const isValid = computed(() => {
  if (props.extra && !props.extra.isValid) return false;
  for (const error of fieldErrors.values()) {
    if (error.value) return false;
  }
  return true;
});

// P17 D5 (both apps, identical): Cancel, Escape, the close button and the backdrop all route here —
// the draft dies with the component, no IPC call. Esc, the close button and the backdrop ask
// first while the draft is dirty (DialogContent `dirty`).
function onDismiss(): void {
  emit('close');
}

// Every close route unmounts this component, so one hook discards the extra state for all of them.
onUnmounted(() => props.extra?.discard());

const saveError = ref<string | null>(null);

// P17 D7: a failed Save keeps the dialog open and shows the error; only a successful Save closes
// it. Saving with nothing changed sends no patch at all (D2).
async function onSave(): Promise<void> {
  if (!isValid.value) return;
  saveError.value = null;
  const patch = pendingPatch.value;
  const hasPatch = Object.keys(patch).length > 0;
  if (!hasPatch && !props.extra?.isDirty) {
    emit('close');
    return;
  }
  try {
    await props.extra?.save();
    if (hasPatch) await props.save(patch);
    emit('close');
  } catch (err) {
    saveError.value = errorText(err);
  }
}
</script>

<template>
  <Dialog :open="true" @update:open="(v) => !v && onDismiss()">
    <DialogContent size="xl" fixed-height :dirty="isDirty" data-testid="settings-dialog">
      <DialogHeader close-testid="settings-dialog-close">
        <DialogTitle>{{ t('common.module.settings.label') }}</DialogTitle>
      </DialogHeader>

      <DialogBody flush class="flex-row">
        <nav class="w-44 shrink-0 flex flex-col gap-px py-1.5 px-1">
          <button
            v-for="section in sections"
            :key="section"
            type="button"
            class="text-left border-none"
            :class="rowVariants({ layout: 'nav', selected: activeSection === section })"
            :aria-current="activeSection === section ? 'page' : undefined"
            :data-testid="`settings-section-${section}`"
            @click="activeSection = section"
          >
            {{ sectionLabels[section] }}
          </button>
        </nav>

        <section class="flex-1 overflow-auto flex flex-col p-3 gap-3">
          <slot
            name="pane"
            :active-section="activeSection"
            :draft="draft"
            :is-at-default="isAtDefault"
            :reset-leaf="resetLeaf"
            :register-field-error="registerFieldError"
          />
        </section>
      </DialogBody>

      <UiDialogFooter>
        <slot
          name="footer"
          :is-dirty="isDirty"
          :is-valid="isValid"
          :save-error="saveError"
          :on-save="onSave"
          :on-dismiss="onDismiss"
        />
      </UiDialogFooter>
    </DialogContent>
  </Dialog>
</template>
