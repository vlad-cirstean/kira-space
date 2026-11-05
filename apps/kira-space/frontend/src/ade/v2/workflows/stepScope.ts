import { type InjectionKey, inject, provide } from 'vue';

/** Where an inspector edits a step: the library, or a step of one workflow. Library steps cannot
 *  skip, name another step as a route target, or pick a review implementer. */
export type StepScope = 'library' | 'workflow';

const KEY: InjectionKey<StepScope> = Symbol('adeStepScope');

export function provideStepScope(scope: StepScope): void {
  provide(KEY, scope);
}

export function useStepScope(): StepScope {
  return inject(KEY, 'workflow');
}
