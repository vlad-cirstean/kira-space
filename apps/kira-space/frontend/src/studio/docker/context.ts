import * as DockerService from '@bindings/dockerservice.js';
import { createDockerContext, type DockerBindings, type DockerContext } from '@kira/docker-ui';
import { control } from '../bridge/control';
import { useSettingsStore } from '../state/settings';

let ctx: DockerContext | null = null;

/** One Docker module context per app; created on first use, after Pinia is active. */
export function studioDocker(): DockerContext {
  ctx ??= createDockerContext(DockerService as unknown as DockerBindings, {
    appearance: () => useSettingsStore().appearance,
    openExternal: control.openExternal,
  });
  return ctx;
}
