import type { ScriptKind } from '@shared/domain/scripts';
import { defineStore } from 'pinia';
import { ref } from 'vue';

// A New / New Alternate shortcut asks for a script; the panel's own create flow answers. The
// panel is v-if'd on the project panel, so a request outlives it until the panel mounts and takes it.
export const useNewScriptRequestStore = defineStore('newScriptRequest', () => {
  const requested = ref<ScriptKind | null>(null);

  function request(kind: ScriptKind): void {
    requested.value = kind;
  }

  function take(): ScriptKind | null {
    const kind = requested.value;
    requested.value = null;
    return kind;
  }

  return { requested, request, take };
});
