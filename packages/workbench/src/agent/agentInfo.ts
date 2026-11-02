import type { AgentEntry, AgentInfo } from '@shared/domain/agent';
import { useQuery } from '@tanstack/vue-query';
import { setMessageDefaults, t } from '@theme/i18n';
import { queryClient } from '@workbench/state/queryClient';
import { computed, type InjectionKey, inject, type MaybeRefOrGetter, toValue, watch } from 'vue';

// Client side of the bound AgentService: the names, models and optional roles of the agent
// provider. Server state; a settings save that can change the provider invalidates it.

/** The calls one app's bridge provides (createCoreControl.ts). */
export interface AgentControl {
  agentInfo(): Promise<AgentInfo>;
  agentInfoFor(id: string): Promise<AgentInfo>;
  agentList(): Promise<AgentEntry[]>;
}

export const agentControlKey: InjectionKey<AgentControl> = Symbol('agentControl');

export function useAgentControl(): AgentControl {
  const control = inject(agentControlKey);
  if (!control) throw new Error('agentControlKey is not provided');
  return control;
}

const INFO_KEY = ['agent', 'info'];
const LIST_KEY = ['agent', 'list'];

/** Shown until the bound call answers: a neutral name, no models. */
export function pendingAgent(): AgentInfo {
  const name = t('memory.common.origin.agent.label');
  return {
    id: '',
    label: name,
    productLabel: name,
    command: name.toLowerCase(),
    headlessLabel: `${name.toLowerCase()} -p`,
    models: [],
    defaultModel: '',
    hooks: false,
    usage: false,
  };
}

/** Drops the cached provider facts; the next read asks Go again. */
export function invalidateAgentInfo(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: ['agent'] });
}

/** The names a catalog message takes as {agent}, {product}, {headless} and {command}. */
export function agentMessageParams(info: AgentInfo): Record<string, string> {
  return {
    agent: info.label,
    product: info.productLabel,
    headless: info.headlessLabel,
    command: info.command,
  };
}

/** The active provider. `info` is never undefined: it is pendingAgent() until the first answer. */
export function useAgentInfo(control: AgentControl = useAgentControl()) {
  const query = useQuery({ queryKey: INFO_KEY, queryFn: () => control.agentInfo() }, queryClient);
  const info = computed(() => query.data.value ?? pendingAgent());
  return { info, query };
}

/** Every registered provider with its CLI's availability, in registration order. */
export function useAgentList(control: AgentControl = useAgentControl()) {
  const query = useQuery({ queryKey: LIST_KEY, queryFn: () => control.agentList() }, queryClient);
  const list = computed(() => query.data.value ?? []);
  return { list, query };
}

/** The provider a stored conversation ran under ("" is the default provider). */
export function useAgentInfoFor(id: MaybeRefOrGetter<string>) {
  const control = useAgentControl();
  const query = useQuery(
    {
      queryKey: computed(() => ['agent', 'infoFor', toValue(id)]),
      queryFn: () => control.agentInfoFor(toValue(id)),
    },
    queryClient,
  );
  const info = computed(() => query.data.value ?? pendingAgent());
  return { info, query };
}

/** Feeds the active provider's names to every catalog message; call once in the app root.
 *  The app root passes its control: it provides it, so cannot inject it. `provider` is the agent.provider
 *  setting: a change refetches the facts. */
export function useAgentMessageDefaults(
  provider: () => string | undefined,
  control: AgentControl = useAgentControl(),
): void {
  const { info } = useAgentInfo(control);
  watch(info, (i) => setMessageDefaults(agentMessageParams(i)), { immediate: true });
  watch(provider, () => void invalidateAgentInfo());
}
