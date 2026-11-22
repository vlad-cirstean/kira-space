import { defineStore } from 'pinia';
import { ref } from 'vue';

// P520: the approvals surface's view state. Items live in TanStack (approvals/approvalsQueries.ts);
// this holds only what the person did to the view.
export const useApprovalsStore = defineStore('approvals', () => {
  const minimised = ref(false);
  const focusId = ref<string | null>(null);
  const answering = ref<string[]>([]);
  const errors = ref<Record<string, string>>({});
  // Items whose kind code threw on the payload: the card shows an error and Deny only.
  const broken = ref<string[]>([]);
  // Per-item choices a kind body collected (branch.request: itemIds, remoteItemIds).
  const options = ref<Record<string, unknown>>({});
  // Values the person typed for an item's declared inputs (item id, then input name). Never persisted
  // and never in TanStack: sent with Approve only, dropped once the item settles.
  const inputs = ref<Record<string, Record<string, string>>>({});

  function minimise(): void {
    minimised.value = true;
  }

  function open(id?: string): void {
    minimised.value = false;
    focusId.value = id ?? null;
  }

  function setOption(id: string, value: unknown): void {
    options.value = { ...options.value, [id]: value };
  }

  // A blank value is no value: a stored input then keeps what Kira holds, a needed one blocks Approve.
  function setInput(id: string, name: string, value: string): void {
    const { [name]: _old, ...rest } = inputs.value[id] ?? {};
    inputs.value = {
      ...inputs.value,
      [id]: value.trim() === '' ? rest : { ...rest, [name]: value },
    };
  }

  function clearInputs(id: string): void {
    const { [id]: _gone, ...rest } = inputs.value;
    inputs.value = rest;
  }

  function markBroken(id: string): void {
    if (!broken.value.includes(id)) broken.value = [...broken.value, id];
  }

  function isBroken(id: string): boolean {
    return broken.value.includes(id);
  }

  function begin(ids: string[]): void {
    answering.value = [...new Set([...answering.value, ...ids])];
    errors.value = Object.fromEntries(
      Object.entries(errors.value).filter(([k]) => !ids.includes(k)),
    );
  }

  // An answered item that stays listed (a partially answered branch.request) is answerable again.
  function done(id: string): void {
    answering.value = answering.value.filter((a) => a !== id);
    clearInputs(id);
  }

  function fail(id: string, message: string): void {
    answering.value = answering.value.filter((a) => a !== id);
    errors.value = { ...errors.value, [id]: message };
  }

  // The queue's push is the source of truth: forget state for items it no longer lists. An empty
  // queue also resets `minimised`, so the next batch opens the dialog again.
  function prune(live: string[]): void {
    answering.value = answering.value.filter((a) => live.includes(a));
    errors.value = Object.fromEntries(
      Object.entries(errors.value).filter(([k]) => live.includes(k)),
    );
    options.value = Object.fromEntries(
      Object.entries(options.value).filter(([k]) => live.includes(k)),
    );
    inputs.value = Object.fromEntries(
      Object.entries(inputs.value).filter(([k]) => live.includes(k)),
    );
    broken.value = broken.value.filter((b) => live.includes(b));
    if (focusId.value && !live.includes(focusId.value)) focusId.value = null;
    if (live.length === 0) minimised.value = false;
  }

  return {
    minimised,
    focusId,
    answering,
    errors,
    options,
    inputs,
    broken,
    minimise,
    open,
    setOption,
    setInput,
    clearInputs,
    markBroken,
    isBroken,
    begin,
    done,
    fail,
    prune,
  };
});
