import type { AppearanceSettings } from '@shared/domain/settings';
import { defineStore } from 'pinia';
import { reactive, ref, toRefs } from 'vue';
import { ROW_HEIGHT_PX } from '../lib/rowHeight';
import { hydrateThenSubscribe } from './hydrateThenSubscribe';

// `sections`/`Section` stay per-app (each dialog surfaces a different subset of the one shared
// `Settings` schema — SettingsDialog.vue's own concern, not this store's), so this factory is
// generic over `S`, the app's own Section union.
//
// `Se`/`SeP`, the app's own Settings/SettingsPatch shape, join `S` as explicit type parameters —
// each app now has its own `state/settingsDomain.ts` schema (no more one shared
// `@shared/domain/settings` shape both stores hardcoded). `Se` is bound to
// `{ appearance: AppearanceSettings } & Record<string, Record<string, unknown>>` — the one section
// applyAppearance() below reads directly, genuinely shared by both apps — plus "every section is
// an object", which is what applySettings' generic Object.assign loop needs to stay type-safe
// without listing each app's own section names here.
//
// Kira Studio's own appearanceVersion bump (measured-text re-measure signal, columns.ts) has no
// equivalent in Kira Space (no data grid) — `extend`'s `onApplyAppearance` hook reproduces it
// without Kira Space's applyAppearance() carrying a counter nothing observes. Kira Space's own
// `--kira-graph-font-size` CSS variable is the same idiom, the other way round: real only for
// Space, so it lives in Space's own `onApplyAppearance` instead of this shared function.

type SettingsShape = Record<string, Record<string, unknown>>;

export interface SettingsControl<Se, SeP> {
  settingsGetAll(): Promise<Se>;
  settingsSet(patch: SeP): Promise<Se>;
  onSettingsChanged(cb: (settings: Se) => void): () => void;
}

export interface SettingsStoreActions<Se> {
  settingsState: Se;
}

export interface SettingsStoreExtension<E extends Record<string, unknown>> {
  extra: E;
  /** Called at the end of every applyAppearance(), after every CSS custom property is set. */
  onApplyAppearance?(): void;
}

// Curried on purpose: `S` (the app's Section union), `Se` and `SeP` have nothing but `defaults` to
// infer from, so every call gives them explicitly — `createSettingsStore<Section, Settings,
// SettingsPatch>(defaultSettings)`. Giving `E` explicitly too, alongside these, disables its own
// inference from the real `extend` argument (TypeScript's partial-explicit-type-argument rule
// applies across all of one call's type parameters, not just the ones given), which leaves `E` at
// its default with nothing to infer it from — and that breaks Pinia's own action/state extraction
// for the *whole* store (every instantiation, not just that one), a real TS+Pinia interaction found
// the hard way. The inner call takes no explicit type arguments at all, so
// `E` is always inferred fresh from `extend`'s actual return value.
export function createSettingsStore<S extends string, Se extends SettingsShape, SeP>(
  defaults: Se,
  storeId = 'settings',
) {
  return <E extends Record<string, unknown>>(
    control: SettingsControl<Se, SeP>,
    extend: (actions: SettingsStoreActions<Se>) => SettingsStoreExtension<E>,
  ) =>
    defineStore(storeId, () => {
      // `reactive<Se>(...)` itself types as `Reactive<Se>` (Vue's own `UnwrapNestedRefs<Se>`),
      // which TS can't prove equals `Se` for a generic type parameter — the same gap every other
      // generic Pinia store call site in this codebase casts through. `Se`'s own bound
      // (`Record<string, Record<string, unknown>>`) has no ref/computed leaf for unwrapping to
      // ever change anyway, so the cast is exact, not a widening.
      const settingsState = reactive(structuredClone(defaults)) as Se;
      const settingsOpen = ref(false);

      // null: no deep link pending — SettingsDialog.vue falls back to 'Appearance'. Set by
      // openSettingsAt below, read once on the dialog's own mount, and cleared when it unmounts so
      // a later plain open (TitleBar.vue's gear icon, the command palette) never inherits a stale
      // section.
      const settingsSection = ref<S | null>(null);

      /** Kira Studio's CookiesPane.vue/RequestSettingsPane.vue "Edit global defaults…" — opens
       *  Settings already switched to `section`, api/menus.ts's own `Environments…` precedent
       *  restated for a section instead of a whole dialog. */
      function openSettingsAt(section: S): void {
        settingsSection.value = section;
        settingsOpen.value = true;
      }

      const { extra, onApplyAppearance } = extend({ settingsState });

      // A store without an `appearance` section (the Studio-module settings) owns no theme.
      function applyAppearance(): void {
        const appearance = (settingsState as { appearance?: AppearanceSettings }).appearance;
        if (!appearance) return;
        const root = document.documentElement.style;
        root.setProperty('--kira-font-family', appearance.fontFamily);
        root.setProperty('--kira-font-size', `${appearance.fontSize}px`);
        root.setProperty('--kira-row-height', `${ROW_HEIGHT_PX[appearance.rowDensity]}px`);
        onApplyAppearance?.();
      }

      let unsubscribeChanged: (() => void) | null = null;

      // P103 Part 4 (§7.3): loops every section `Se` actually has instead of naming each app's own
      // eight (Kira Studio) or three (Kira Space) sections — the two stores no longer share one
      // fixed section list to hardcode.
      function applySettings(settings: Se): void {
        for (const key of Object.keys(settingsState) as (keyof Se)[]) {
          Object.assign(settingsState[key], settings[key]);
        }
        applyAppearance();
      }

      async function hydrateSettings(): Promise<void> {
        // Covers a settings change made through any path other than this module's own
        // patchSettings() below (e.g. a direct IPC call) — the same gap connections.ts's
        // onConnectionsChanged closes for the connections list.
        unsubscribeChanged?.();
        unsubscribeChanged = null;
        unsubscribeChanged = await hydrateThenSubscribe({
          snapshot: () => control.settingsGetAll(),
          subscribe: (cb) => control.onSettingsChanged(cb),
          apply: applySettings,
        });
      }

      // Applies `patch` only once the backend confirms it (settingsSet's own resolved value),
      // never pre-applied — see Kira Studio's own git history for the rollback gap that left other
      // windows and the database diverged from a pre-applied, later-rejected patch.
      async function patchSettings(patch: SeP): Promise<void> {
        const updated = await control.settingsSet(patch);
        applySettings(updated);
      }

      return {
        ...toRefs(settingsState),
        settingsOpen,
        settingsSection,
        openSettingsAt,
        hydrateSettings,
        patchSettings,
        ...extra,
      };
    });
}
