import { createRouter, createWebHistory } from 'vue-router';
import AppShell from './AppShell.vue';
import ActivityScreen from './screens/ActivityScreen.vue';
import LaterScreen from './screens/LaterScreen.vue';
import NeedsScreen from './screens/NeedsScreen.vue';
import PairScreen from './screens/PairScreen.vue';
import PlanScreen from './screens/PlanScreen.vue';
import TerminalScreen from './screens/TerminalScreen.vue';
import { useAuthStore } from './state/auth';

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/pair', name: 'pair', component: PairScreen },
    { path: '/terminal/:sessionId', name: 'terminal', component: TerminalScreen, props: true },
    {
      path: '/',
      name: 'home',
      component: AppShell,
      redirect: '/needs',
      children: [
        { path: 'needs', component: NeedsScreen },
        { path: 'plan', component: PlanScreen },
        { path: 'later', component: LaterScreen },
        { path: 'activity', component: ActivityScreen },
      ],
    },
  ],
});

// Every route but /pair needs a paired device; the first navigation waits for the /api/me check.
router.beforeEach(async (to) => {
  const auth = useAuthStore();
  if (auth.phase === 'checking') await auth.check();
  if (to.name !== 'pair' && auth.phase !== 'paired') return { name: 'pair' };
  if (to.name === 'pair' && auth.phase === 'paired') return { name: 'home' };
  return true;
});
