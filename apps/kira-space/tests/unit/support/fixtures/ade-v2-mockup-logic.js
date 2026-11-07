// Logic-only extract of the frozen P143-149 ade-v2 design mockup's inline <script>
// (formerly the v2.0 ADE mockup, see docs/HISTORY.md (v2.0), stripped of markup/CSS the parity
// oracle never reads). Verbatim design code, not app style — not meant to pass this
// repo's own lint rules (see biome.json's exclusion).

class DCLogic { constructor(props) { this.props = props || {}; } setState(o) { Object.assign(this.state, o); } }

class Component extends DCLogic {
  constructor(props) {
    super(props);
    this.state = {
      view: 'plan', inboxSel: 'i1', inboxDraft: { jira: '', gh: '' }, ghOv: {}, extraRepos: {}, showAllSessions: false, capture: '', inbox: [
        { id: 'i1', text: 'Rate limit the export endpoint per workspace', added: 'today', jira: 'PAY-140', gh: '' },
        { id: 'i2', text: 'Look at flaky e2e on Safari', added: 'yesterday', jira: '', gh: 'https://github.com/acme/web-app/issues/882' },
        { id: 'i3', text: 'Ask Dana about the onboarding copy deadline', added: '2d ago' },
        { id: 'i4', text: 'Dark mode for the billing dashboard?', added: '5d ago' }
      ], repoOn: { 'web-app': true, 'api': true, 'mobile': true },
      panelW: 720, horizon: 14, history: 14, extraDays: [], offDays: [], workWeekend: [], allFilter: 'active', hoverIcon: null, inHistory: false, showHistory: false, pull: 0,
      repoCfg: {}, repoSel: 'web-app', prepOv: {}, extraRepos2: [], repoFolders: [{ path: '~/code/acme', watch: true }], newRepoPath: '', newFolderPath: '', showAllItems: false,
      fetch: { 'web-app': { busy: false, last: '4m ago', note: '' }, 'api': { busy: false, last: '12m ago', note: '' }, 'mobile': { busy: false, last: '1h ago', note: '' } },
      sel: { kind: 'task', id: 'T_bill' }, tab: 'task',
      plan: { day: { T_cart: -1, T_bill: 0, T_auth: 1, T_hooks: 6, T_search: 2, T_notif: 2, T_deps: 3, T_push: 3, T_alerts: 7, T_csv: 8 }, order: ['T_cart', 'T_bill', 'T_auth', 'T_hooks', 'T_search', 'T_notif', 'T_deps', 'T_push', 'T_alerts', 'T_csv'] },
      queuedAfter: {}, rebased: {}, unpushed: {}, merged: { b_cart: true }, intoOv: {},
      wfMode: 'form', wfYaml: {}, wfYamlErr: {}, openOut: null, stageOv: {}, wfSel: 'standard', wfOv: {}, taskWf: {}, runOv: {}, tuiOf: {},
      archived: [], taskStatus: {}, names: {}, est: {}, jiraOv: {}, jiraDraft: '', stepState: {}, stepsAdded: {}, newStep: '',
      newTasks: [], attach: {}, started: {}, newSessions: {}, resumed: {},
      notes: { T_bill: '## Q4 pricing launch\n- [x] Usage metering in api\n- [ ] Invoice lines **before** Oct 1\n- [ ] Align with *Sara* on merge timing\n\nTotals live in `invoice.ts` for now.' },
      rebasing: [], pushing: [], termTab: {}, dragOver: null,
      picker: false, pickerTab: 'new', pickerQ: '', pickerTask: null, nw: { title: '', jira: '', repos: { 'web-app': true }, notes: '' },
      dialog: null, dialogMsg: null, dialogPush: false, dialogOverride: false, dialogBranches: {},
      ctx: null, confirm: null, copied: null, linkOpen: false, linkUrl: ''
    };
    this.palette = ['#e07a4f', '#e3a53c', '#c9c23a', '#8cc152', '#4db86c', '#35b5a0', '#38a8cc', '#4a8ee6', '#6e79ea', '#9a6ee2', '#c566d8', '#e062a8', '#e35f79', '#b88458', '#94a35a', '#58a08e', '#7b92b8', '#a57ec0', '#d58c8c', '#a3aab4'];
    this.colors = {};
  }
  // ---------- mock data. In the app: git (per repo), Jira, GitHub, the Claude Code session registry, and the local task store.
  repos() {
    var base = {
      'web-app': { targets: ['develop'], prepare: 'pnpm install --frozen-lockfile\ncp .env.example .env.local', prepareTimeout: '10m',
        envs: [{ name: 'preview', script: 'vercel inspect "$PREVIEW_URL" --json | jq -r .meta.githubCommitSha' }, { name: 'staging', script: 'curl -s https://staging.acme.dev/version | jq -r .sha' }, { name: 'prod', script: 'curl -s https://acme.dev/version | jq -r .sha' }] },
      'api': { targets: ['develop', 'staging'], prepare: 'go mod download\nmake db-local', prepareTimeout: '15m',
        envs: [{ name: 'staging', script: 'kubectl -n staging get deploy api -o jsonpath="{.metadata.labels.git-sha}"' }, { name: 'prod', script: 'kubectl -n prod get deploy api -o jsonpath="{.metadata.labels.git-sha}"' }] },
      'mobile': { targets: [], prepare: 'yarn install --immutable\nnpx pod-install', prepareTimeout: '20m', envs: [] }
    };
    var meta = {
      'web-app': { full: 'acme-customer-dashboard-web-frontend', path: '~/code/acme/acme-customer-dashboard-web-frontend', nick: 'web-app', source: '~/code/acme' },
      'api': { full: 'acme-platform-core-api-service', path: '~/code/acme/acme-platform-core-api-service', nick: 'api', source: '~/code/acme' },
      'mobile': { full: 'acme-mobile-react-native-app', path: '~/code/acme/acme-mobile-react-native-app', nick: 'mobile', source: '~/code/acme' },
      'infra': { full: 'acme-terraform-infrastructure-live', path: '~/code/acme/acme-terraform-infrastructure-live', nick: 'infra', source: '~/code/acme' },
      'docs': { full: 'acme-public-documentation-site', path: '~/code/acme/acme-public-documentation-site', nick: 'docs', source: '~/code/acme' },
      'dotfiles': { full: 'dotfiles', path: '~/dotfiles', nick: 'dotfiles', source: 'added' }
    };
    Object.keys(meta).forEach(function (k) { base[k] = Object.assign({ targets: [], prepare: '', prepareTimeout: '10m', envs: [] }, base[k] || {}, meta[k]); });
    var st = this.state || {};
    (st.extraRepos2 || []).forEach(function (r) { base[r.id] = Object.assign({ targets: [], prepare: '', prepareTimeout: '10m', envs: [] }, r); });
    var ov = st.repoCfg || {};
    Object.keys(ov).forEach(function (k) { if (base[k]) base[k] = Object.assign({}, base[k], ov[k]); });
    return base;
  }
  branchData() {
    function S(id, state, last, act, mode, step) { return { id: id, state: state, last: last, act: act || 'idle', mode: mode || 'headless', step: step || null }; }
    function PR(repo, num, title, state) { return { num: num, title: title, state: state, url: 'https://github.com/acme/' + repo + '/pull/' + num }; }
    function B(o) { return Object.assign({ kind: 'mine', base: 'main', ahead: 1, behind: 0, files: [], commits: [], dirty: [], sessions: [], pr: null, into: {} }, o); }
    return [
      B({ id: 'b_cart', repo: 'web-app', name: 'fix/cart-flaky', ready: true, deploy: { staging: { st: 'deployed', sha: 'a91f2c0' }, prod: { st: 'deployed', sha: 'a91f2c0' } }, pr: PR('web-app', 1431, 'Stabilise cart total test', 'Merged'), into: { develop: 'merged' }, sessions: [S('c81a', 'stopped', '2d ago', 'idle', 'headless', 'pr')], commits: [['4be21c9', 'Use fake timers in cart test']], files: [['src/cart/cart.test.ts', '+14 −9']] }),
      B({ id: 'b_bill', repo: 'web-app', name: 'feat/usage-billing', ahead: 5, pr: PR('web-app', 1427, 'Usage-based billing', 'Open'), sessions: [S('b1c2', 'running', 'now', 'working', 'headless', 'impl')], commits: [['0de4f71', 'Meter API usage per workspace'], ['6b2c8e9', 'Invoice line items from usage']], files: [['src/payments/client.ts', '+40 −12'], ['src/payments/invoice.ts', '+95 −8'], ['src/metering/usage.ts', '+132']], dirty: [['M', 'src/payments/invoice.ts']] }),
      B({ id: 'b_billdash', repo: 'web-app', name: 'feat/billing-dashboard', base: 'b_bill', ahead: 3, sessions: [S('9d10', 'running', '4m ago', 'input', 'headless', 'impl')], commits: [['7e1c0aa', 'Usage chart'], ['b4402df', 'Upcoming invoice card']], files: [['src/ui/billing/Dashboard.tsx', '+210'], ['src/ui/nav/Sidebar.tsx', '+6 −1']] }),
      B({ id: 'b_meter', repo: 'api', name: 'feat/usage-metering', ahead: 4, deploy: { staging: { st: 'deployed', sha: '33cc44d' } }, pr: PR('api', 218, 'Usage metering endpoints', 'Approved'), into: { develop: 'merged' }, sessions: [S('m111', 'running', '2m ago', 'waiting', 'headless', 'impl')], commits: [['11aa22b', 'Metering table'], ['33cc44d', 'Usage endpoint']], files: [['src/metering/store.ts', '+120'], ['src/routes/usage.ts', '+64']] }),
      B({ id: 'b_auth', repo: 'web-app', name: 'feat/oauth-login', ahead: 7, behind: 3, deploy: { preview: { st: 'stale', sha: '9bc0f13', note: 'preview runs 9bc0f13, from before the last 2 commits' } }, pr: PR('web-app', 1423, 'OAuth login: providers, callback, sessions', 'Open'), into: { develop: 'stale' }, intoNote: { develop: '2 commits since it was merged' }, sessions: [S('9ab0', 'running', '4m ago', 'input', 'tui'), S('51cd', 'stopped', '2d ago', 'idle', 'headless', 'pr')], commits: [['e41a7d2', 'Add OAuth provider config'], ['9bc0f13', 'Callback route + token exchange'], ['1f8e6aa', 'Persist refresh tokens']], files: [['src/auth/oauth.ts', '+214'], ['src/auth/session.ts', '+19 −6']], dirty: [['M', 'src/auth/tokens.ts'], ['??', 'src/auth/crypto.ts']] }),
      B({ id: 'b_authui', repo: 'web-app', name: 'feat/oauth-login-ui', base: 'b_auth', ahead: 4, pr: PR('web-app', 1425, 'Login screen UI', 'Draft'), sessions: [S('d4e7', 'stopped', '1d ago', 'idle', 'headless', 'pr')], commits: [['c7d21e0', 'Login screen with provider buttons']], files: [['src/ui/LoginScreen.tsx', '+176']], dirty: [['M', 'src/ui/LoginScreen.tsx']] }),
      B({ id: 'b_authcb', repo: 'api', name: 'feat/oauth-callback', ahead: 3, deploy: { staging: { st: 'deployed', sha: 'aa01bb2' }, prod: { st: 'stale', sha: '71e0c3a', note: 'prod runs 71e0c3a: 1 commit of this branch is missing' } }, pr: PR('api', 211, 'OAuth callback + token exchange', 'Approved'), into: { develop: 'merged', staging: 'merged' }, sessions: [S('a7a7', 'stopped', 'yesterday', 'idle', 'headless', 'pr')], commits: [['aa01bb2', 'Token exchange endpoint']], files: [['src/auth/callback.ts', '+98']] }),
      B({ id: 'b_hooks', repo: 'api', name: 'feat/webhooks', ahead: 4, deploy: { staging: { st: 'deployed', sha: 'cc33dd4' } }, pr: PR('api', 212, 'Outgoing webhooks', 'Approved'), into: { develop: 'merged', staging: 'stale' }, intoNote: { staging: 'rebased since it was merged' }, sessions: [S('h001', 'running', 'now', 'working', 'headless', 'pr')], commits: [['aa11bb2', 'Webhook registry'], ['cc33dd4', 'Signing']], files: [['src/hooks/registry.ts', '+120'], ['src/hooks/sign.ts', '+40']] }),
      B({ id: 'b_hooksretry', repo: 'api', name: 'feat/webhooks-retry', base: 'b_hooks', ahead: 2, sessions: [S('h002', 'running', '3m ago', 'working', 'headless', 'pr')], commits: [['ee55ff6', 'Exponential backoff']], files: [['src/hooks/retry.ts', '+88']], dirty: [['M', 'src/hooks/retry.ts']] }),
      B({ id: 'b_sara', repo: 'web-app', name: 'sara/payments-refactor', kind: 'review', owner: 'sara', ahead: 9, behind: 1, pr: PR('web-app', 1402, 'Payments: adapter refactor', 'Changes requested'), commits: [['f02b9c4', 'Split payment client into adapters']], files: [['src/payments/client.ts', '+120 −96'], ['src/payments/invoice.ts', '+44 −51']] }),
      B({ id: 'b_li', repo: 'web-app', name: 'li/search-schema', kind: 'review', owner: 'li', ahead: 4, pr: PR('web-app', 1411, 'Search schema v2', 'Approved'), commits: [['9f1a2b3', 'Schema v2 + migration']], files: [['src/search/schema.ts', '+88 −40']] }),
      B({ id: 'b_search', repo: 'web-app', name: 'feat/search-index', base: 'b_li', ahead: 6, pr: PR('web-app', 1426, 'Index on write', 'Open'), sessions: [S('5e3a', 'running', '2m ago', 'waiting', 'headless', 'ci')], commits: [['3d8e1f0', 'Write hook'], ['c2a7b95', 'Backfill job']], files: [['src/search/indexer.ts', '+160'], ['src/search/schema.ts', '+6 −2'], ['package.json', '+1']], dirty: [['M', 'src/search/backfill.ts']] }),
      B({ id: 'b_searchui', repo: 'web-app', name: 'feat/search-ui', base: 'b_search', ahead: 0, prepare: { st: 'failed', took: '1m 12s', log: ['$ pnpm install --frozen-lockfile', 'Lockfile is up to date, resolution step is skipped', 'Packages: +1412', 'ERR_PNPM_FETCH_404  GET https://npm.acme.dev/@acme%2fsearch-sdk: Not Found - 404', 'exit code 1'] }, files: [['src/ui/search/Results.tsx', '+40']] }),
      B({ id: 'b_notif', repo: 'web-app', name: 'feat/notifications', ahead: 4, pr: PR('web-app', 1428, 'In-app notifications', 'Open'), sessions: [S('71b0', 'running', 'now', 'working', 'headless', 'fix')], commits: [['5c9d0e1', 'Notifications model']], files: [['src/notifications/model.ts', '+72'], ['src/ui/nav/Sidebar.tsx', '+3 −1']] }),
      B({ id: 'b_deps', repo: 'web-app', name: 'chore/deps-bump', ciFailing: true, pr: PR('web-app', 1433, 'Bump dependencies', 'Open'), sessions: [S('aa12', 'running', '6m ago', 'input', 'headless', 'ci')], commits: [['0f9e8d7', 'Bump 23 packages']], files: [['package.json', '+23 −23']] }),
      B({ id: 'b_push', repo: 'mobile', name: 'feat/mob-17-push-notification-settings-quiet-hours-and-digests', ahead: 2, prepare: { st: 'running', took: '3m 40s', log: ['$ yarn install --immutable', '➤ YN0000: ┌ Resolution step', '➤ YN0000: └ Completed', '➤ YN0000: ┌ Fetch step', '…'] }, sessions: [S('m7m7', 'running', '1m ago', 'working', 'headless', 'impl')], commits: [['abab123', 'Settings screen']], files: [['app/settings/Push.tsx', '+140']] }),
      B({ id: 'b_pushapi', repo: 'api', name: 'feat/push-tokens', ahead: 2, pr: PR('api', 216, 'Store device push tokens', 'Approved'), into: { develop: 'merged' }, sessions: [S('p0p0', 'running', '1m ago', 'working', 'headless', 'tests')], commits: [['9090aaa', 'Device token table']], files: [['src/push/tokens.ts', '+70']] }),
      B({ id: 'b_spike', repo: 'web-app', name: 'spike/header-redesign', kind: 'parked', ahead: 6, behind: 3, sessions: [S('3c3c', 'stopped', '5d ago', 'idle', 'tui')], files: [['src/ui/Header.tsx', '+120 −80']], dirty: [['M', 'src/ui/Header.tsx']] }),
      // not yet on the plan: offered by Add → Existing branch
      B({ id: 'p_invoice', repo: 'web-app', name: 'fix/invoice-rounding', pool: true, author: 'you', agoMin: 20, files: [['src/payments/round.ts', '+12 −3']] }),
      B({ id: 'p_scopes', repo: 'api', name: 'li/auth-scopes', kind: 'review', owner: 'li', pool: true, author: 'li', agoMin: 60, files: [['src/auth/scopes.ts', '+60']] }),
      B({ id: 'p_csvapi', repo: 'api', name: 'feat/export-endpoint', pool: true, author: 'you', agoMin: 1500 }),
      B({ id: 'p_rate', repo: 'web-app', name: 'omar/rate-limits', kind: 'review', owner: 'omar', pool: true, author: 'omar', agoMin: 2900, files: [['src/api/limits.ts', '+140']] })
    ];
  }
  // Workflows are configured on the Workflows page: ordered stages; a stage is manual (optionally an interactive
  // Claude Code session with its own prompt) or automated (background steps, headless claude -p).
  workflowData() {
    function P(id, name, scope, prompt, onFail, gate, timeout) { return { id: id, name: name, scope: scope, prompt: prompt, onFail: onFail, gate: gate, timeout: timeout }; }
    function M(id, name, tui, prompt, status) { return { id: id, name: name, kind: 'manual', tui: tui, prompt: prompt || '', status: status || 'In progress' }; }
    function A(id, name, steps, status) { return { id: id, name: name, kind: 'auto', steps: steps, status: status || 'In progress' }; }
    function X(id, name, command, scope, onFail, timeout, status) { return { id: id, name: name, kind: 'script', command: command, scope: scope, onFail: onFail, timeout: timeout, status: status || 'In review' }; }
    return [
      { id: 'standard', name: 'Standard feature', stages: [
        M('spec', 'Spec', true, "Let's write the spec for {task} ({jira}). Ask me questions until it is clear. Do not change any code."),
        A('impl', 'Implement', [
          P('plan', 'Plan from spec', 'once', 'Read the task spec and the Jira ticket {jira}. Write an implementation plan per repo. Do not change code yet.', 'stop', 'auto', '20m'),
          P('impl', 'Implement', 'each repo', 'Implement the plan for {repo} on {branch} in {worktree}. Commit in small steps.', 'retry 1', 'auto', '2h'),
          P('tests', 'Write tests', 'each repo', 'Add or update tests for the changes on {branch}. If they fail because of the implementation, report what fails.', 'back:impl', 'auto', '1h'),
          P('ci', 'Make CI green', 'each repo', 'Run the checks for {branch}. Fix failures until they pass.', 'back:impl', 'auto', '1h'),
          P('pr', 'Open PRs', 'each repo', 'Open a draft PR for {branch} and link {jira}.', 'stop', 'approve', '15m')]),
        M('review', 'Review', true, 'Help me go through the review comments on {branch} and address them.', 'In review'),
        X('release', 'Release', './scripts/release.sh --branch {branch}', 'each repo', 'stop', '15m', 'In review') ] },
      { id: 'bugfix', name: 'Bugfix', stages: [
        M('triage', 'Triage', true, 'Help me understand {jira} and find where it happens. Do not change any code.'),
        A('fix', 'Fix', [
          P('repro', 'Reproduce with a failing test', 'each repo', 'Reproduce {jira} with a failing test on {branch}.', 'stop', 'auto', '30m'),
          P('fix', 'Fix', 'each repo', 'Make the failing test pass with the smallest change.', 'retry 1', 'auto', '1h'),
          P('ci', 'Make CI green', 'each repo', 'Run the checks for {branch}. Fix failures until they pass.', 'retry 2', 'auto', '1h'),
          P('pr', 'Open PR', 'each repo', 'Open a draft PR for {branch} and link {jira}.', 'stop', 'approve', '15m')]),
        X('release', 'Release', './scripts/release.sh --branch {branch}', 'each repo', 'stop', '15m', 'In review') ] },
      { id: 'chore', name: 'Maintenance', stages: [
        A('update', 'Update', [
          P('update', 'Apply the change', 'each repo', 'Apply the change described in the task on {branch}.', 'stop', 'auto', '1h'),
          P('ci', 'Make CI green', 'each repo', 'Run the checks for {branch}. Fix failures until they pass.', 'retry 2', 'auto', '1h')]),
        X('release', 'Release', 'make release', 'each repo', 'stop', '10m', 'In review') ] }
    ];
  }
  taskData() {
    function J(key, title, status) { return { key: key, title: title, status: status, url: 'https://acme.atlassian.net/browse/' + key }; }
    function S(id, state, last, act) { return { id: id, state: state, last: last, act: act || 'idle', mode: 'tui', step: null }; }
    return [
      { id: 'T_cart', title: 'Cart total test fails intermittently', jira: J('WEB-311', 'Cart total test fails intermittently', 'In review'), status: 'In review', est: '1h', branches: ['b_cart'],
        workflow: 'bugfix', stage: 'release', run: { repro: 'done', fix: 'done', ci: 'done', pr: 'done', release: { b_cart: { st: 'failed', out: ['$ ./scripts/release.sh --branch fix/cart-flaky', 'tagging web-app v4.18.2', 'pushing tag… ok', 'creating GitHub release… error: 403 Resource not accessible by integration', 'exit code 1'] } } } },
      { id: 'T_bill', title: '', jira: J('PAY-102', 'Usage-based billing', 'In progress'), status: 'In progress', est: '2d', branches: ['b_meter', 'b_bill', 'b_billdash'],
        workflow: 'standard', stage: 'impl', run: { plan: 'done', impl: { b_meter: { st: 'running' }, b_bill: { st: 'running' }, b_billdash: { st: 'running' } } }, specSessions: [S('sp11', 'stopped', '3d ago')] },
      { id: 'T_auth', title: '', jira: J('AUTH-212', 'Sign in with Google and GitHub', 'In review'), status: 'In review', est: '5h', branches: ['b_authcb', 'b_auth', 'b_authui'],
        workflow: 'standard', stage: 'review', run: { plan: 'done', impl: 'done', tests: 'done', ci: 'done', pr: 'done' } },
      { id: 'T_hooks', title: '', jira: J('API-51', 'Outgoing webhooks', 'In review'), status: 'In review', est: '3h', branches: ['b_hooks', 'b_hooksretry'],
        workflow: 'standard', stage: 'impl', run: { plan: 'done', impl: 'done', tests: 'done', ci: 'done', pr: { b_hooks: { st: 'running' }, b_hooksretry: { st: 'running' } } } },
      { id: 'T_search', title: 'Search v2', jira: J('SRCH-41', 'Index documents on write', 'In progress'), status: 'Blocked', est: '3d', branches: ['b_search', 'b_searchui'],
        workflow: 'standard', stage: 'impl', run: { plan: 'done', impl: 'done', tests: 'done', ci: { b_search: { st: 'running' } } } },
      { id: 'T_notif', title: 'In-app notifications', jira: null, status: 'In progress', est: '2h', branches: ['b_notif'], workflow: 'bugfix', stage: 'fix', run: { repro: 'done', fix: { b_notif: { st: 'running' } } } },
      { id: 'T_deps', title: 'Monthly dependency updates', jira: null, status: 'Blocked', est: '1h', branches: ['b_deps'], workflow: 'chore', stage: 'update', run: { update: 'done', ci: { b_deps: { st: 'running' } } } },
      { id: 'T_push', title: '', jira: J('MOB-17', 'Push notification settings with per-channel quiet hours, weekly digests and an opt-out audit trail', 'In progress'), status: 'In progress', est: '2h', branches: ['b_pushapi', 'b_push'],
        workflow: 'standard', stage: 'impl', run: { plan: 'done', impl: { b_pushapi: 'done', b_push: { st: 'running', loops: 1, note: 'sent back by Write tests: 2 failing tests in Push.test.tsx' } }, tests: { b_pushapi: { st: 'running' }, b_push: { st: 'back', note: '2 failing tests → back to Implement (1 of 3)' } } } },
      { id: 'T_alerts', title: 'Usage alerts by email', jira: J('PAY-130', 'Usage alerts by email', 'To do'), status: 'In progress', est: '1d', draftRepos: ['api', 'web-app'],
        workflow: 'standard', stage: 'spec', specActive: true, run: {}, specSessions: [S('sp22', 'running', '2m ago', 'input')] },
      { id: 'T_csv', title: '', jira: J('PAY-121', 'Export invoices as CSV', 'To do'), status: 'To do', est: '4h', draftRepos: ['api', 'web-app'], notes: 'Reuse the invoice line items from usage billing.',
        workflow: 'standard', stage: 'spec', run: {} },
      { id: 'R_sara', kind: 'review', title: '', owner: 'sara', branches: ['b_sara'] },
      { id: 'R_li', kind: 'review', title: '', owner: 'li', branches: ['b_li'] },
      { id: 'T_spike', kind: 'parked', title: 'Header redesign spike', est: '1d', branches: ['b_spike'], workflow: 'standard', stage: 'spec', run: {} }
    ];
  }
  historyData() {
    return [
      { day: -1, title: 'Audit log for admin actions', repos: 'web-app · api', how: 'merged · archived' },
      { day: -4, title: 'Fix login redirect loop', repos: 'web-app', how: 'merged · archived' },
      { day: -4, title: 'Pagination cursor', repos: 'api', how: 'merged · archived' },
      { day: -6, title: 'Team invites', repos: 'web-app · api · mobile', how: 'merged · archived' },
      { day: -12, title: 'Old search spike', repos: 'web-app', how: 'archived' }
    ];
  }
  tone(t) {
    var m = {
      amber: ['rgba(232,163,61,0.14)', '#f0b85c', '#e8a33d'], red: ['rgba(239,107,91,0.14)', '#f28b7d', '#ef6b5b'],
      green: ['rgba(108,197,138,0.14)', '#7fd49b', '#6cc58a'], blue: ['rgba(122,167,255,0.14)', '#93b6ff', '#7aa7ff'],
      purple: ['rgba(163,113,247,0.16)', '#c3a3fb', '#a371f7'], grey: ['#23252b', '#b4b6bd', '#6b6f7a']
    };
    return m[t] || m.grey;
  }
  chip(t) { var c = this.tone(t); return 'font-size: 11px; font-weight: 600; padding: 1px 7px; border-radius: 5px; background: ' + c[0] + '; color: ' + c[1] + '; white-space: nowrap; flex-shrink: 0;'; }
  repoChip(repo) {
    var c = { 'web-app': '#8fb4ff', 'api': '#7fd4b8', 'mobile': '#e6a7d8', 'infra': '#e3c27a', 'docs': '#b9a7f0', 'dotfiles': '#a3aab4' }[repo] || '#b4b6bd';
    return 'font-family: \'IBM Plex Mono\', monospace; font-size: 10.5px; font-weight: 600; padding: 1px 5px; border-radius: 4px; background: ' + c + '1f; color: ' + c + '; border: none; white-space: nowrap; flex-shrink: 0;';
  }
  act(a, size) {
    var z = size || 14;
    var m = {
      input: { label: 'needs input', rank: 0, glyph: '!', style: 'width: ' + z + 'px; height: ' + z + 'px; border-radius: 50%; background: #e8a33d; color: #15161a; font-size: ' + (z - 4) + 'px; font-weight: 800; display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0;' },
      working: { label: 'working', rank: 1, glyph: '', style: 'width: ' + (z - 4) + 'px; height: ' + (z - 4) + 'px; margin: 2px; box-sizing: border-box; border-radius: 50%; background: #6cc58a; box-shadow: 0 0 0 2px rgba(108,197,138,0.28); flex-shrink: 0; display: inline-block;' },
      waiting: { label: 'waiting on monitor', rank: 2, glyph: 'z', style: 'width: ' + z + 'px; height: ' + z + 'px; box-sizing: border-box; border-radius: 50%; border: 1.5px solid #7aa7ff; color: #93b6ff; font-size: ' + (z - 5) + 'px; font-weight: 700; display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0;' },
      idle: { label: 'idle', rank: 3, glyph: '', style: 'width: ' + (z - 4) + 'px; height: ' + (z - 4) + 'px; margin: 2px; box-sizing: border-box; border-radius: 50%; border: 1.5px solid #7c7f88; flex-shrink: 0; display: inline-block;' },
      stopped: { label: 'stopped', rank: 4, glyph: '', style: 'width: ' + (z - 6) + 'px; height: ' + (z - 6) + 'px; margin: 3px; border-radius: 2px; background: #4a4d56; flex-shrink: 0; display: inline-block;' }
    };
    return m[a] || m.idle;
  }
  actSummary(sessions) {
    var self = this, c = { input: 0, working: 0, waiting: 0 };
    sessions.forEach(function (x) { if (x.state === 'running' && c[x.act] !== undefined) c[x.act]++; });
    return ['input', 'working', 'waiting'].filter(function (k) { return c[k] > 0; }).map(function (k) { var a = self.act(k, 12); return { kind: k, style: a.style, glyph: a.glyph, count: c[k], title: c[k] + ' ' + a.label }; });
  }
  // estimates: number + hours|days. Workday = 6h. 1d = full day; ≥2d spans N working days at half a day each.
  parseEst(v) {
    var m = String(v || '').trim().toLowerCase().match(/^(\d+(?:\.\d+)?)\s*(m|h|d|w)$/);
    if (!m) return null;
    var n = parseFloat(m[1]), u = m[2];
    if (u === 'm') return { hours: n / 60, days: 1 };
    if (u === 'h') return { hours: n, days: Math.max(1, Math.ceil(n / 6 - 1e-9)) };
    if (u === 'd') { var dd = Math.max(1, Math.ceil(n)); return { hours: dd === 1 ? 6 : dd * 3, days: dd }; }
    return { hours: Math.ceil(n * 5) * 3, days: Math.max(1, Math.ceil(n * 5)) };
  }
  // plan is per task: day + position
  moveTask(id, before, day) {
    var p = this.state.plan;
    var o = p.order.filter(function (k) { return k !== id; });
    if (before && o.indexOf(before) >= 0) o.splice(o.indexOf(before), 0, id); else o.push(id);
    var dd = Object.assign({}, p.day);
    if (day === null) delete dd[id]; else dd[id] = day;
    this.setState({ plan: { day: dd, order: o } });
  }
  shiftTasks(ids, toDay) {
    var p = this.state.plan;
    var rest = p.order.filter(function (k) { return ids.indexOf(k) < 0; });
    var at = rest.findIndex(function (k) { return p.day[k] === toDay; }); if (at < 0) at = rest.length;
    var dd = Object.assign({}, p.day); ids.forEach(function (k) { dd[k] = toDay; });
    this.setState({ plan: { day: dd, order: rest.slice(0, at).concat(ids).concat(rest.slice(at)) } });
  }
  refresh(repos) {
    var self = this;
    var f = Object.assign({}, this.state.fetch); repos.forEach(function (r) { f[r] = Object.assign({}, f[r], { busy: true }); });
    this.setState({ fetch: f });
    setTimeout(function () {
      var f2 = Object.assign({}, self.state.fetch), ov = Object.assign({}, self.state.intoOv);
      repos.forEach(function (r) {
        if (r === 'web-app') { ov.b_notif = Object.assign({}, ov.b_notif, { develop: 'merged' }); f2[r] = { busy: false, last: 'just now', note: '3 refs changed · 1 merged into develop' }; }
        else f2[r] = { busy: false, last: 'just now', note: 'no changes' };
      });
      self.setState({ fetch: f2, intoOv: ov });
    }, 900);
  }
  forcePush(ids) {
    var self = this;
    this.setState({ pushing: ids });
    setTimeout(function () { var up = Object.assign({}, self.state.unpushed); ids.forEach(function (id) { delete up[id]; }); self.setState({ unpushed: up, pushing: [] }); }, 700);
  }
  archiveTask(id) {
    var sel = this.state.sel;
    this.setState({ archived: this.state.archived.concat([id]), sel: sel.kind === 'task' && sel.id === id ? { kind: 'task', id: null } : sel });
  }
  scrollTop() { var m = document.getElementById('aq-main'); if (m) m.scrollTop = 0; }
  scrollToDay(k) {
    var main = document.getElementById('aq-main');
    var el = document.getElementById('aq-day-' + (k < 0 ? 'm' + (-k) : k));
    if (!main || !el) return;
    main.scrollTop = el.offsetTop - (main.firstElementChild ? main.firstElementChild.offsetHeight : 48) - 2;
  }
  revealHistory() {
    var main = document.getElementById('aq-main'); var before = main ? main.scrollHeight : 0;
    this.setState({ showHistory: true, pull: 0 });
    setTimeout(function () { var m = document.getElementById('aq-main'); if (m) m.scrollTop = m.scrollHeight - before - 60; }, 30);
  }
  openDialog(d) { this.setState({ dialog: d, dialogMsg: null, dialogPush: false, dialogOverride: false, dialogBranches: {}, ctx: null }); }
  sendDialog() {
    var self = this, d = this.state.dialog; if (!d) return;
    if (d.kind === 'rebase' || d.kind === 'queue') {
      var push = this.state.dialogPush;
      this.setState({ rebasing: d.roots, dialog: null });
      setTimeout(function () {
        var rb = Object.assign({}, self.state.rebased), q = Object.assign({}, self.state.queuedAfter), up = Object.assign({}, self.state.unpushed), ov = Object.assign({}, self.state.intoOv);
        d.roots.forEach(function (r) { if (d.onto === 'main') rb[r] = true; else q[r] = d.onto; });
        (d.ids || d.roots).forEach(function (id) {
          if (!push) up[id] = true;
          // rewritten commits are no longer the ones merged into integration branches → stale
          (d.mergedInto[id] || []).forEach(function (t) { ov[id] = Object.assign({}, ov[id]); ov[id][t] = 'stale'; });
        });
        self.setState({ rebased: rb, queuedAfter: q, unpushed: up, intoOv: ov, rebasing: [] });
      }, 900);
      return;
    }
    if (d.kind === 'merge') { var ov2 = Object.assign({}, this.state.intoOv); ov2[d.branch] = Object.assign({}, ov2[d.branch]); ov2[d.branch][d.target] = 'merged'; this.setState({ intoOv: ov2 }); }
    if (d.kind === 'start') {
      var ns = Object.assign({}, this.state.newSessions), stt = Object.assign({}, this.state.started), rs = Object.assign({}, this.state.resumed);
      if (d.resume) rs[d.resume] = true;
      (d.create || []).forEach(function (c) { stt[c.id] = (self.state.dialogBranches[c.id] || '').trim() || c.suggest; });
      (d.sessionOn || []).forEach(function (bid) { ns[bid] = (ns[bid] || []).concat([{ id: Math.random().toString(16).slice(2, 6), state: 'running', last: 'now', act: 'idle', mode: 'tui', step: null }]); });
      var sst = Object.assign({}, this.state.stepState); if (d.step) sst[d.task + ':' + d.step] = 'running';
      this.setState({ newSessions: ns, started: stt, resumed: rs, stepState: sst });
    }
    if (d.kind === 'stage') {
      var ns2 = Object.assign({}, this.state.newSessions); ns2['task:' + d.task] = (ns2['task:' + d.task] || []).concat([{ id: Math.random().toString(16).slice(2, 6), state: 'running', last: 'now', act: 'input', mode: 'tui', step: null }]);
      var tt2 = Object.assign({}, this.state.termTab); this.setState({ newSessions: ns2, dialog: null, sel: { kind: 'task', id: d.task }, tab: 'agents' }); return;
    }
    if (d.kind === 'run') {
      var stt2 = Object.assign({}, this.state.started); (d.create || []).forEach(function (c) { stt2[c.id] = (self.state.dialogBranches[c.id] || '').trim() || c.suggest; });
      var ro2 = Object.assign({}, this.state.runOv); if (d.step) ro2[d.task + ':' + d.step] = 'running';
      var ns3 = Object.assign({}, this.state.newSessions);
      (d.bids || []).forEach(function (bid) { ns3[bid] = (ns3[bid] || []).concat([{ id: Math.random().toString(16).slice(2, 6), state: 'running', last: 'now', act: 'working', mode: 'headless', step: d.step }]); });
      var po = Object.assign({}, this.state.prepOv); (d.create || []).forEach(function (c) { po[c.id] = { st: 'running', took: '0s', log: ['$ (prepare-worktree script)', '…'] }; });
      this.setState({ started: stt2, runOv: ro2, newSessions: ns3, prepOv: po, dialog: null });
      setTimeout(function () { var p2 = Object.assign({}, self.state.prepOv); (d.create || []).forEach(function (c) { p2[c.id] = { st: 'ok', took: '52s', log: ['done'] }; }); self.setState({ prepOv: p2 }); }, 3000);
      return;
    }
    if (d.kind === 'archive') { this.setState({ dialog: null }); this.archiveTask(d.task); return; }
    this.setState({ dialog: null });
  }
  copy(text, key) {
    var self = this;
    try { navigator.clipboard.writeText(text); } catch (e) {}
    this.setState({ copied: key });
    setTimeout(function () { if (self.state.copied === key) self.setState({ copied: null }); }, 1200);
  }
  renderVals() {
    var self = this, s = this.state;
    var REPOS = this.repos(), repoNames = Object.keys(REPOS);
    var BR = this.branchData(), HIST = this.historyData();
    var busy = s.rebasing.length > 0, WORKDAY = 6, CAP = WORKDAY;
    var FINISH = 'When this step is done, call the finish_step tool with status "done" and a one-line summary. If it failed, call it with status "failed" and say what failed. If you need a decision from me, call it with status "needs_input" and your question.';
    function slug(t) { return String(t || 'new-work').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 40) || 'new-work'; }
    function jiraKey(v) { var m = String(v || '').match(/([A-Z][A-Z0-9]+-\d+)/); return m ? m[1] : null; }
    function nick(r) { return (REPOS[r] && REPOS[r].nick) || r; }
    function abbr(t) { return { develop: 'dev', staging: 'stg', release: 'rel' }[t] || t; }

    // ---------- tasks and their branches (across repos)
    var allTasks = this.taskData().concat(s.newTasks.map(function (t) { return Object.assign({}, t); }));
    var byB = {}; BR.forEach(function (b) { byB[b.id] = Object.assign({}, b); });
    allTasks.forEach(function (t) {
      t.branches = (t.branches || []).slice();
      Object.keys(s.attach).forEach(function (bid) { if (s.attach[bid] === t.id) t.branches.push(bid); });
      (t.draftRepos || []).concat(s.extraRepos[t.id] || []).forEach(function (r) {
        var id = t.id + ':' + r, started = s.started[id], key = t.jira ? t.jira.key : null;
        byB[id] = { id: id, repo: r, name: started || '', draft: !started, kind: 'mine', base: 'main', ahead: 0, behind: 0, files: [], commits: [], dirty: [], sessions: [], pr: null, into: {},
          suggest: 'feat/' + slug((key ? key.toLowerCase() + '-' : '') + (t.title || (t.jira && t.jira.title) || 'new-work')) };
        t.branches.push(id);
      });
    });
    var taskOf = {}; allTasks.forEach(function (t) { t.branches.forEach(function (bid) { taskOf[bid] = t.id; }); });
    var isArchived = function (tid) { return s.archived.indexOf(tid) >= 0; };
    var tasks = allTasks.filter(function (t) { return !isArchived(t.id); });
    var tById = {}; allTasks.forEach(function (t) { tById[t.id] = t; });
    function sessionsOf(b) {
      var arch = isArchived(taskOf[b.id]);
      return b.sessions.map(function (x) {
        if (arch && x.state === 'running') return { id: x.id, state: 'stopped', last: 'just now', act: 'idle' };
        return s.resumed[x.id] ? { id: x.id, state: 'running', last: 'now', act: 'working' } : x;
      }).concat(arch ? [] : (s.newSessions[b.id] || []));
    }
    function running(b) { return sessionsOf(b).filter(function (x) { return x.state === 'running'; }); }
    function jiraOf(t) {
      var ov = s.jiraOv[t.id];
      if (ov !== undefined) { var k = jiraKey(ov); return k ? { key: k, title: '', status: '', url: /^https?:/.test(ov) ? ov : 'https://acme.atlassian.net/browse/' + k } : null; }
      return t.jira || null;
    }
    function titleOf(t) {
      if (s.names[t.id]) return s.names[t.id];
      if (t.title) return t.title;
      var j = jiraOf(t); if (j && j.title) return j.title;
      var b0 = byB[t.branches[0]]; if (b0 && b0.pr) return b0.pr.title;
      return (b0 && b0.name) || (j ? j.key : 'New task');
    }
    function defaultTitleOf(t) { var j = jiraOf(t); return t.title || (j && j.title) || (byB[t.branches[0]] && (byB[t.branches[0]].pr ? byB[t.branches[0]].pr.title : byB[t.branches[0]].name)) || 'New task'; }
    function estOf(t) { return self.parseEst(s.est[t.id] !== undefined ? s.est[t.id] : t.est); }
    var WF = {}; this.workflowData().forEach(function (w) { WF[w.id] = s.wfOv[w.id] || w; }); Object.keys(s.wfOv).forEach(function (k) { if (!WF[k]) WF[k] = s.wfOv[k]; });
    function wfOf(t) { return WF[s.taskWf[t.id] || t.workflow] || null; }
    function stageIdxOf(t) { var w = wfOf(t); if (!w) return 0; var id = s.stageOv[t.id] || t.stage || (w.stages[0] && w.stages[0].id); if (id === 'done') return w.stages.length; var i = -1; w.stages.forEach(function (x, k) { if (x.id === id) i = k; }); return i < 0 ? 0 : i; }
    function curStage(t) { var w = wfOf(t), i = stageIdxOf(t); return w && i < w.stages.length ? w.stages[i] : null; }
    function isFinished(t) { var w = wfOf(t); return !!w && stageIdxOf(t) >= w.stages.length; }
    function specSessions(t) {
      var arch = isArchived(t.id);
      return (t.specSessions || []).map(function (x) { return arch && x.state === 'running' ? Object.assign({}, x, { state: 'stopped', last: 'just now', act: 'idle' }) : x; }).concat(arch ? [] : (s.newSessions['task:' + t.id] || []));
    }
    function taskSessions(t) {
      var out = specSessions(t).map(function (x) { return { x: x, b: null }; });
      t.branches.forEach(function (bid) { sessionsOf(byB[bid]).forEach(function (x) { out.push({ x: x, b: byB[bid] }); }); });
      return out;
    }
    function targetsOf(t, scope) {
      var mine = t.branches.filter(function (bid) { return byB[bid].kind === 'mine'; });
      if (scope === 'once') return mine.slice(0, 1);
      if (/^only /.test(scope || '')) { var rr = scope.slice(5); return mine.filter(function (bid) { return byB[bid].repo === rr; }); }
      return mine;
    }
    function runOf(t, stepId, bid) {
      var ov = s.runOv[t.id + ':' + stepId + ':' + bid] || s.runOv[t.id + ':' + stepId];
      if (ov) return typeof ov === 'string' ? { st: ov } : ov;
      var r = (t.run || {})[stepId]; if (!r) return { st: 'pending' };
      if (typeof r === 'string') return { st: r };
      var x = r[bid]; if (!x) return { st: 'pending' };
      return typeof x === 'string' ? { st: x } : x;
    }
    function stageSteps(t, stage) {
      if (!stage || stage.kind === 'manual') return [];
      var list = stage.kind === 'script' ? [{ id: stage.id, name: stage.name, scope: stage.scope, onFail: stage.onFail, gate: 'auto', timeout: stage.timeout, command: stage.command, isScript: true }] : stage.steps;
      var ts = taskSessions(t), prevDone = true;
      return list.map(function (st, i) {
        var runs = targetsOf(t, st.scope).map(function (bid) {
          var r = runOf(t, st.id, bid);
          var live = ts.filter(function (o) { return o.b && o.b.id === bid && o.x.state === 'running' && o.x.mode === 'headless' && o.x.step === st.id; });
          var state = r.st; if (state === 'running' && live.some(function (o) { return o.x.act === 'input'; })) state = 'stuck';
          return { b: byB[bid], state: state, todo: r.todo || null, loops: r.loops || 0, note: r.note || '', out: r.out || null, live: live };
        });
        var sts = runs.map(function (r) { return r.state; });
        var agg = !runs.length ? 'pending' : sts.indexOf('stuck') >= 0 ? 'stuck' : sts.indexOf('failed') >= 0 ? 'failed' : (sts.indexOf('running') >= 0 || sts.indexOf('back') >= 0) ? 'running' : sts.every(function (x) { return x === 'done'; }) ? 'done' : 'pending';
        var live = []; runs.forEach(function (r) { live = live.concat(r.live); });
        var approval = agg === 'pending' && st.gate === 'approve' && i > 0 && prevDone;
        prevDone = agg === 'done';
        var frac = runs.length ? runs.reduce(function (acc, r) { return acc + (r.state === 'done' ? 1 : r.todo ? r.todo[0] / r.todo[1] : 0); }, 0) / runs.length : 0;
        return Object.assign({}, st, { n: i + 1, state: agg, live: live, approval: approval, runs: runs, frac: frac });
      });
    }
    // task status follows the workflow
    function taskStatusOf(t) {
      if (t.kind === 'review') return 'In review';
      if (t.kind === 'parked') return 'To do';
      var w = wfOf(t); if (!w) return 'To do';
      if (isFinished(t)) return 'Done';
      var steps = stepsOf(t), cur = curStage(t);
      if (steps.some(function (x) { return x.state === 'stuck' || x.state === 'failed'; })) return 'Blocked';
      if (t.branches.some(function (bid) { var pr = prepOf(byB[bid]); return pr && pr.st === 'failed'; })) return 'Blocked';
      if (stageIdxOf(t) === 0 && !taskSessions(t).length && !t.specActive && steps.every(function (x) { return x.state === 'pending'; })) return 'To do';
      return (cur && cur.status) || 'In progress';
    }
    function runScript(t, st) {
      var ro = Object.assign({}, self.state.runOv); ro[t.id + ':' + st.id] = 'running'; self.setState({ runOv: ro });
      setTimeout(function () { var r2 = Object.assign({}, self.state.runOv); r2[t.id + ':' + st.id] = { st: 'done', out: ['$ ' + (st.command || ''), 'released', 'exit code 0'] }; self.setState({ runOv: r2 }); }, 2000);
    }
    function stepsOf(t) { return stageSteps(t, curStage(t)); }
    function prepOf(b) { if (b.draft) return null; return s.prepOv[b.id] || b.prepare || { st: 'ok', took: '', log: [] }; }
    function deployOf(b) { return b.deploy || {}; }
    function retryPrepare(bid) {
      var po = Object.assign({}, self.state.prepOv); po[bid] = { st: 'running', took: '0s', log: ['$ ' + String(REPOS[byB[bid].repo].prepare || '').split('\n')[0], '…'] }; self.setState({ prepOv: po });
      setTimeout(function () { var p2 = Object.assign({}, self.state.prepOv); p2[bid] = { st: 'ok', took: '48s', log: ['$ ' + String(REPOS[byB[bid].repo].prepare || '').split('\n')[0], 'done'] }; self.setState({ prepOv: p2 }); }, 2500);
    }
    function intoOf(b) { return Object.assign({}, b.into, s.intoOv[b.id]); }

    // ---------- colors: per task; first free palette slot, else least used; never reassigned
    allTasks.forEach(function (t) {
      if (self.colors[t.id] !== undefined) return;
      var used = {}; Object.keys(self.colors).forEach(function (k) { if (!isArchived(k)) used[self.colors[k]] = (used[self.colors[k]] || 0) + 1; });
      var pick = -1; for (var i = 0; i < self.palette.length; i++) if (!used[i]) { pick = i; break; }
      if (pick < 0) { var mn = 1e9; for (var j = 0; j < self.palette.length; j++) if ((used[j] || 0) < mn) { mn = used[j] || 0; pick = j; } }
      self.colors[t.id] = pick;
    });
    function colorOf(tid) { return self.palette[self.colors[tid] % self.palette.length]; }

    // ---------- git facts, per repo (branches of every task on the plan)
    var planB = []; tasks.forEach(function (t) { t.branches.forEach(function (bid) { planB.push(byB[bid]); }); });
    var parentOf = {}, kids = {};
    planB.forEach(function (b) { kids[b.id] = []; });
    planB.forEach(function (b) {
      var p = s.queuedAfter[b.id] || (b.base !== 'main' && byB[b.base] && taskOf[b.base] && !isArchived(taskOf[b.base]) ? b.base : null);
      if (p) { parentOf[b.id] = p; if (kids[p]) kids[p].push(b.id); }
    });
    var ancestors = function (id) { var a = [], p = parentOf[id]; while (p) { a.push(p); p = parentOf[p]; } return a; };
    var rootOf = function (id) { var a = ancestors(id); return a.length ? a[a.length - 1] : id; };
    var behind = function (id) { return (s.rebased[rootOf(id)] && !parentOf[id]) ? 0 : byB[id].behind; };
    var files = function (id) { return byB[id].files.map(function (f) { return f[0]; }); };
    var sharedF = function (a, b) { var fb = files(b); return files(a).filter(function (f) { return fb.indexOf(f) >= 0; }); };
    var conflicts = {};
    planB.forEach(function (b) {
      if (b.kind !== 'mine' || s.merged[b.id] || b.draft) return;
      planB.forEach(function (r) {
        if (r.kind !== 'review' || r.repo !== b.repo || ancestors(b.id).indexOf(r.id) >= 0) return;
        var sh = sharedF(b.id, r.id);
        if (sh.length) { (conflicts[b.id] = conflicts[b.id] || []).push({ with: r.id, files: sh }); (conflicts[r.id] = conflicts[r.id] || []).push({ with: b.id, files: sh }); }
      });
    });

    // ---------- days
    var LATER = 9999;
    var TODAY = new Date(2026, 8, 22);
    var WD = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'], MO = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    function dateOf(k) { var d0 = new Date(TODAY.getTime()); d0.setDate(d0.getDate() + k); return d0; }
    function offsetOf(dt) { return Math.round((dt.getTime() - TODAY.getTime()) / 86400000); }
    function isWeekend(k) { var g = dateOf(k).getDay(); return g === 0 || g === 6; }
    function isDayOff(k) { return s.offDays.indexOf(k) >= 0; }
    function isOff(k) { return isDayOff(k) || (isWeekend(k) && s.workWeekend.indexOf(k) < 0); }
    function nextWork(k) { var n2 = k + 1; while (isOff(n2)) n2++; return n2; }
    function firstWork(k) { return isOff(k) ? nextWork(k) : k; }
    function dayLabel(k) { if (k === LATER) return 'Later'; if (k === 0) return 'Today'; var dt = dateOf(k); return WD[dt.getDay()] + ' ' + dt.getDate(); }
    function dayLong(k) { if (k === LATER) return 'Later'; var dt = dateOf(k); return (k === 0 ? 'Today, ' : '') + WD[dt.getDay()] + ' ' + dt.getDate() + ' ' + MO[dt.getMonth()]; }
    function spanDays(start, n2) { var out = [start], k = start; while (out.length < n2) { k = nextWork(k); out.push(k); } return out; }

    // ---------- task order: my tasks by (merge day, position); review items sit right above what depends on / conflicts with them
    var mineT = tasks.filter(function (t) { return t.kind !== 'review'; });
    var revT = tasks.filter(function (t) { return t.kind === 'review'; });
    mineT.forEach(function (t) {
      t.day = s.plan.day[t.id] !== undefined ? s.plan.day[t.id] : LATER;
      t.pos = s.plan.order.indexOf(t.id); if (t.pos < 0) t.pos = 999;
      var e = estOf(t); t.span = t.day === LATER ? 1 : (e ? e.days : 1);
      t.days = t.day === LATER ? [LATER] : spanDays(t.day, t.span); t.end = t.days[t.days.length - 1];
    });
    mineT.sort(function (a, b) { return (a.end - b.end) || (a.day - b.day) || (a.pos - b.pos); });
    var seq = [];
    mineT.forEach(function (t) {
      revT.forEach(function (r) {
        if (r.placed) return;
        var rb = r.branches[0];
        var hit = t.branches.some(function (bid) { return ancestors(bid).indexOf(rb) >= 0 || (conflicts[bid] || []).some(function (c) { return c.with === rb; }); });
        if (hit) { r.placed = true; r.day = t.day; r.end = t.end; r.days = [t.day]; seq.push(r); }
      });
      seq.push(t);
    });
    revT.forEach(function (r) { if (!r.placed) { r.day = LATER; r.end = LATER; r.days = [LATER]; seq.push(r); } });
    var seqIdx = {}; seq.forEach(function (t, i) { seqIdx[t.id] = i; });
    function taskBranchOrder(t) {
      var inT = function (id) { return taskOf[id] === t.id; };
      var out = [];
      var rs = t.branches.slice().sort(function (a, b) { return byB[a].repo < byB[b].repo ? -1 : byB[a].repo > byB[b].repo ? 1 : 0; });
      rs.forEach(function (bid) {
        if (parentOf[bid] && inT(parentOf[bid])) return;
        (function walk(id, dep) { out.push({ id: id, dep: dep }); (kids[id] || []).forEach(function (c) { if (inT(c)) walk(c, dep + 1); }); })(bid, 0);
      });
      return out;
    }
    // shares files with mine work (another task, same repo) that merges earlier → must rebase after it
    var after = {}, order = [];
    seq.forEach(function (t) { taskBranchOrder(t).forEach(function (x) { order.push(x.id); }); });
    order.forEach(function (bid, i) {
      var b = byB[bid];
      if (b.kind !== 'mine' || b.draft || s.merged[bid]) return;
      if (parentOf[bid]) return;   // stacked on my work, or on someone else's branch (can't be rebased away from it)
      for (var j = i - 1; j >= 0; j--) {
        var a = byB[order[j]];
        if (a.repo !== b.repo || a.kind !== 'mine' || a.draft || s.merged[a.id] || taskOf[a.id] === taskOf[bid]) continue;
        var sh = sharedF(a.id, bid);
        if (sh.length) { after[bid] = { id: a.id, file: sh[0].split('/').pop() }; break; }
      }
    });

    function gitStatus(id) {
      var b = byB[id];
      if (b.draft) return ['not created', 'grey'];
      var pr0 = prepOf(b);
      if (pr0 && pr0.st === 'running') return ['preparing', 'blue'];
      if (pr0 && pr0.st === 'failed') return ['setup failed', 'red'];
      if (s.merged[id]) return ['merged', 'purple'];
      if (b.kind === 'parked') return ['not merging', 'grey'];
      if (b.kind === 'review') return conflicts[id] ? ['conflict', 'red'] : ['review', 'blue'];
      if (s.rebasing.indexOf(rootOf(id)) >= 0) return ['rebasing', 'amber'];
      if (s.pushing.indexOf(id) >= 0) return ['pushing', 'amber'];
      if (conflicts[id]) return ['conflict', 'red'];
      if (s.unpushed[id]) return ['not pushed', 'amber'];
      if (ancestors(id).some(function (x) { return conflicts[x]; })) return ['base conflict', 'amber'];
      if (behind(id) > 0) return ['↓' + behind(id) + ' behind', 'amber'];
      if (ancestors(id).some(function (x) { return behind(x) > 0; })) return ['base behind', 'amber'];
      if (b.ciFailing) return ['CI failing', 'red'];
      if (parentOf[id] && byB[rootOf(id)].kind === 'review') return ['waiting', 'blue'];
      return ['up to date', 'green'];
    }
    function deployChips(b) {
      if (b.draft || b.kind !== 'mine') return [];
      var d = deployOf(b);
      return (REPOS[b.repo].envs || []).filter(function (e) { return d[e.name]; }).map(function (e) {
        var x = d[e.name];
        return { label: '▲' + e.name + (x.st === 'deployed' ? ' ✓' : ' ⚠'), tip: x.st === 'deployed' ? 'deployed: ' + e.name + ' runs ' + x.sha + ', which contains this branch' : 'stale on ' + e.name + ': ' + (x.note || e.name + ' runs ' + x.sha + ', missing changes of this branch'),
          style: 'font-family: \'IBM Plex Mono\', monospace; font-size: 10.5px; font-weight: 500; white-space: nowrap; flex-shrink: 0; color: ' + (x.st === 'deployed' ? '#6b7a8f' : '#c9944a') + ';' };
      });
    }
    function intoChips(b) {
      if (b.draft || b.kind !== 'mine') return [];
      var io = intoOf(b);
      return (REPOS[b.repo].targets || []).filter(function (t) { return io[t]; }).map(function (t) {
        var st = io[t], tt = self.tone(st === 'merged' ? 'green' : 'amber');
        return { label: abbr(t) + (st === 'merged' ? ' ✓' : ' ⚠'), tip: st === 'merged' ? 'merged into ' + t + ', up to date' : 'stale in ' + t + ((b.intoNote && b.intoNote[t]) ? ': ' + b.intoNote[t] : ': has changes not in ' + t) + '. Right-click to re-merge.',
          style: 'font-family: \'IBM Plex Mono\', monospace; font-size: 10.5px; font-weight: 500; white-space: nowrap; flex-shrink: 0; color: ' + (st === 'merged' ? '#6b7f72' : '#c9944a') + ';' };
      });
    }
    function wtOf(b) { return '~/kira-space-wts/' + b.repo + '/' + (b.name || b.suggest || '').split('/').pop(); }
    // ---------- dialog openers
    function agentTargets(ids) { return ids.map(function (id) { var r = running(byB[id]); return { branch: id, choice: r.length ? r[0].id : 'new' }; }); }
    function stackIds(rootId) { var out = [rootId]; (function down(x) { (kids[x] || []).forEach(function (c) { if (byB[c].kind === 'mine' && !byB[c].draft) { out.push(c); down(c); } }); })(rootId); return out; }
    function rebaseDialog(roots, onto, title) {
      var ids = []; roots.forEach(function (r) { ids = ids.concat(stackIds(r)); });
      var mergedInto = {}; ids.forEach(function (id) { var io = intoOf(byB[id]); mergedInto[id] = Object.keys(io).filter(function (t) { return io[t] === 'merged'; }); });
      self.openDialog({ kind: onto !== 'main' && byB[onto] && byB[onto].kind === 'review' ? 'queue' : 'rebase', roots: roots, onto: onto, ids: ids, mergedInto: mergedInto, targets: agentTargets(roots), title: title });
    }
    function mergeDialog(bid, target) {
      var st = intoOf(byB[bid])[target];
      self.openDialog({ kind: 'merge', branch: bid, target: target, targets: agentTargets([bid]), title: (st === 'stale' ? 'Re-merge into ' : 'Merge into ') + target });
    }
    function openStageDialog(t, st) { self.openDialog({ kind: 'stage', task: t.id, stage: st.id, title: st.name + ' · interactive Claude Code' }); }
    function runPipelineDialog(t) {
      var drafts = t.branches.filter(function (bid) { return byB[bid].draft; });
      var first = stepsOf(t).filter(function (x) { return x.state === 'pending'; })[0];
      var mineB2 = t.branches.filter(function (bid) { return byB[bid].kind === 'mine'; });
      self.openDialog({ kind: 'run', task: t.id, step: first ? first.id : null, bids: first && first.scope === 'once' ? mineB2.slice(0, 1) : mineB2, create: drafts.map(function (bid) { return { id: bid, repo: byB[bid].repo, suggest: byB[bid].suggest }; }), title: 'Run ' + (curStage(t) ? curStage(t).name : '') + ' in the background' });
    }
    function setRun(t, stepId, state) { var ro = Object.assign({}, self.state.runOv); ro[t.id + ':' + stepId] = state; return ro; }
    function headlessOn(bids, stepId) {
      var ns = Object.assign({}, self.state.newSessions);
      bids.forEach(function (bid) { ns[bid] = (ns[bid] || []).concat([{ id: Math.random().toString(16).slice(2, 6), state: 'running', last: 'now', act: 'working', mode: 'headless', step: stepId }]); });
      return ns;
    }
    function approveStep(t, st) { self.setState({ runOv: setRun(t, st.id, 'running'), newSessions: headlessOn(t.branches.filter(function (bid) { return byB[bid].kind === 'mine' && !byB[bid].draft; }), st.id) }); }
    // Take over: spawn an interactive Claude Code session that resumes the headless one (claude --resume <id>)
    function openTui(o) {
      var key = o.b ? o.b.id : 'task:' + taskOf[o.b && o.b.id];
      var ns = Object.assign({}, self.state.newSessions), tid = o.x.id + 't';
      var exists = (ns[key] || []).some(function (x) { return x.id === tid; });
      if (!exists) ns[key] = (ns[key] || []).concat([{ id: tid, state: 'running', last: 'now', act: 'idle', mode: 'tui', step: o.x.step, resumes: o.x.id }]);
      var tt = Object.assign({}, self.state.termTab); tt['b:' + key] = tid; tt['t:' + taskOf[key]] = tid;
      self.setState({ newSessions: ns, termTab: tt, sel: { kind: 'branch', id: key }, tab: 'agents', hoverIcon: null });
    }
    function stuckOf(t) { var st = stepsOf(t).filter(function (x) { return x.state === 'stuck' || x.state === 'failed'; })[0]; if (!st) return null; var o = st.live.filter(function (q) { return q.x.act === 'input'; })[0] || st.live[0]; return { step: st, o: o }; }
    function advance(t) { var w = wfOf(t), i = stageIdxOf(t), so = Object.assign({}, self.state.stageOv); so[t.id] = i + 1 < w.stages.length ? w.stages[i + 1].id : 'done'; self.setState({ stageOv: so }); }
    function phaseAction(t) {
      var w = wfOf(t); if (!w) return null;
      var i = stageIdxOf(t); if (i >= w.stages.length) return null;
      var st = w.stages[i], last = i === w.stages.length - 1, next = last ? null : w.stages[i + 1];
      var done = { label: last ? 'Finish ✓' : 'Done ›', tip: last ? 'finish the workflow' : st.name + ' is done; move to ' + next.name, run: function () { advance(t); }, tone: 'green' };
      if (st.kind === 'manual') {
        var talking = taskSessions(t).some(function (o) { return o.x.state === 'running' && o.x.mode === 'tui'; });
        if (st.tui && !talking && !(t.specActive && st.id === (t.stage || ''))) return { label: '▶ ' + st.name, tip: 'open an interactive Claude Code session for ' + st.name, run: function () { openStageDialog(t, st); }, tone: 'claude' };
        return done;
      }
      var steps = stepsOf(t);
      if (st.kind === 'script') {
        var sc = steps[0];
        if (sc && sc.state === 'failed') return { label: 'Retry', tip: st.name + ' failed: see its output in the Task tab', run: function () { runScript(t, sc); }, tone: 'red' };
        if (sc && sc.state === 'pending') return { label: '▶ Run', tip: 'run: ' + (st.command || ''), run: function () { runScript(t, sc); }, tone: 'claude' };
        if (sc && sc.state === 'done') return done;
        return null;
      }
      var sk = stuckOf(t); if (sk && sk.o) return { label: 'Take over', tip: sk.step.name + ' needs you: continue it yourself in Claude Code', run: function () { openTui(sk.o); }, tone: 'red' };
      var ap = steps.filter(function (x) { return x.approval; })[0]; if (ap) return { label: 'Approve', tip: 'run "' + ap.name + '"', run: function () { approveStep(t, ap); }, tone: 'amber' };
      if (steps.length && steps.every(function (x) { return x.state === 'pending'; })) return { label: '▶ Run', tip: 'run ' + st.name + ' with background agents (claude -p)', run: function () { runPipelineDialog(t); }, tone: 'claude' };
      if (steps.length && steps.every(function (x) { return x.state === 'done'; })) return done;
      return null;
    }
    function phasePills(t) {
      var w = wfOf(t); if (!w) return { segs: [], label: '', tip: '', labelStyle: '', showBar: false };
      var idx = stageIdxOf(t), cur = curStage(t), steps = stepsOf(t), done = steps.filter(function (x) { return x.state === 'done'; }).length;
      var bad = steps.some(function (x) { return x.state === 'stuck' || x.state === 'failed'; });
      var segs = w.stages.map(function (x, i) { var st = i < idx ? 'done' : i === idx ? 'current' : 'todo';
        return 'width: ' + (x.kind === 'auto' ? 14 : 9) + 'px; height: 6px; border-radius: 2px; background: ' + (st === 'done' ? '#6cc58a' : st === 'current' ? (bad ? '#ef6b5b' : '#e8a33d') : '#3a3e48') + ';'; });
      var label = !cur ? 'Done' : cur.name + (cur.kind === 'auto' ? ' ' + done + '/' + steps.length : '');
      var frac = steps.length ? steps.reduce(function (acc, x) { return acc + x.frac; }, 0) / steps.length : 0, showBar = !!cur && cur.kind !== 'manual';
      var c = !cur ? ['rgba(108,197,138,0.14)', '#7fd49b'] : bad ? ['#ef6b5b', '#15161a'] : ['rgba(232,163,61,0.16)', '#f0b85c'];
      return { segs: segs, label: label, showBar: showBar, pct: Math.round(frac * 100) + '%',
        barFill: 'display: block; height: 100%; width: ' + Math.round(frac * 100) + '%; background: ' + (bad ? '#ef6b5b' : '#e8a33d') + ';',
        barTip: steps.map(function (x) { return x.n + '. ' + x.name + ': ' + x.runs.map(function (r) { return nick(r.b.repo) + ' ' + (r.state === 'done' ? '✓' : r.todo ? r.todo[0] + '/' + r.todo[1] : r.state); }).join(', '); }).join('\n'),
        tip: w.name + ': ' + w.stages.map(function (x, i) { return (i < idx ? '✓ ' : i === idx ? '▸ ' : '') + x.name + (x.kind === 'auto' ? ' (agent)' : x.kind === 'script' ? ' (script)' : ' (user)'); }).join(' › '),
        labelStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 11px; font-weight: 700; padding: 1px 6px; border-radius: 5px; white-space: nowrap; flex-shrink: 0; max-width: 150px; overflow: hidden; text-overflow: ellipsis; background: ' + c[0] + '; color: ' + c[1] + ';' };
    }
    function startBranchDialog(bid) { self.openDialog({ kind: 'start', task: taskOf[bid], branchOnly: bid, sessionOn: [bid], create: [], title: 'Start Claude Code (interactive)' }); }
    function atRisk(t) {
      var out = [];
      t.branches.forEach(function (bid) { var b = byB[bid]; if (b.draft || b.kind === 'review') return; var d = b.dirty.map(function (x) { return x[1]; }), u = s.merged[bid] ? 0 : b.ahead; if (d.length || u) out.push({ id: bid, dirty: d, unmerged: u }); });
      return out;
    }
    function requestArchive(t) {
      var risk = atRisk(t);
      if (!risk.length) { self.archiveTask(t.id); return; }
      self.openDialog({ kind: 'archive', task: t.id, risk: risk, targets: agentTargets(risk.map(function (r) { return r.id; })), title: 'Archive: work would be lost' });
    }
    var archiveTip = 'Stop its agents, delete its worktrees and hide it. Branches, notes and links are kept; it stays in history.';
    function selectTask(id) { self.setState({ sel: { kind: 'task', id: id }, tab: s.tab === 'agents' ? 'agents' : 'task' }); }
    function selectBranch(id) { self.setState({ sel: { kind: 'branch', id: id }, tab: s.tab === 'agents' ? 'agents' : 'details' }); }
    function openSession(bid, sid) { var tt = Object.assign({}, s.termTab); tt['b:' + bid] = sid; self.setState({ sel: { kind: 'branch', id: bid }, tab: 'agents', termTab: tt, hoverIcon: null }); }
    function agentIconsFor(bids) {
      var list = [];
      bids.forEach(function (bid) { running(byB[bid]).forEach(function (x) { list.push({ b: byB[bid], x: x }); }); });
      list.sort(function (p, q) { return self.act(p.x.act).rank - self.act(q.x.act).rank; });
      return list.map(function (o) {
        var a = self.act(o.x.act, 13), hk = o.b.id + ':' + o.x.id;
        return { style: a.style, glyph: a.glyph, label: 'Open claude ' + o.x.id, hover: s.hoverIcon === hk,
          tipTitle: o.b.repo + ' · claude ' + o.x.id, tipState: a.label, tipLast: o.x.last,
          tipStateStyle: 'color: ' + (o.x.act === 'input' ? '#f0b85c' : o.x.act === 'working' ? '#7fd49b' : o.x.act === 'waiting' ? '#93b6ff' : '#9a9ca5') + ';',
          enter: function () { if (self.state.hoverIcon !== hk) self.setState({ hoverIcon: hk }); },
          leave: function () { if (self.state.hoverIcon === hk) self.setState({ hoverIcon: null }); },
          open: function (e) { if (e && e.stopPropagation) e.stopPropagation(); openSession(o.b.id, o.x.id); } };
      });
    }
    // only what needs me: sessions waiting for input (interactive) or stuck (background)
    function attnFor(items) {
      var need = items.filter(function (o) { return o.x.state === 'running' && o.x.act === 'input'; });
      if (!need.length) return { has: false, label: '', tip: '', open: function () {} };
      var o = need[0];
      return { has: true, label: need.length > 1 ? need.length + ' need you' : 'needs you',
        tip: need.map(function (q) { return (q.x.mode === 'headless' ? 'background run stuck' : 'Claude Code waiting for you') + (q.b ? ' · ' + q.b.repo + ' · ' + q.b.name : ' · spec'); }).join('\n'),
        open: function (e) { if (e && e.stopPropagation) e.stopPropagation(); if (o.x.mode === 'headless') openTui(o); else if (o.b) openSession(o.b.id, o.x.id); else { var tt = Object.assign({}, self.state.termTab); tt['t:' + o.tid] = o.x.id; self.setState({ sel: { kind: 'task', id: o.tid }, tab: 'agents', termTab: tt }); } } };
    }
    var rootEl = function () { return document.getElementById('aq-root').getBoundingClientRect(); };
    function branchMenu(bid) {
      return function (e) {
        var b = byB[bid]; if (b.draft || b.kind !== 'mine') return;
        e.preventDefault(); e.stopPropagation();
        var items = [], io = intoOf(b);
        (REPOS[b.repo].targets || []).forEach(function (t) {
          if (io[t] === 'stale') items.push({ label: 'Re-merge into ' + t + ' (stale)', run: function () { mergeDialog(bid, t); } });
          else if (!io[t]) items.push({ label: 'Merge into ' + t, run: function () { mergeDialog(bid, t); } });
        });
        if (behind(rootOf(bid)) > 0 && byB[rootOf(bid)].kind === 'mine') items.push({ label: 'Rebase onto main', run: function () { rebaseDialog([rootOf(bid)], 'main', 'Rebase onto main'); } });
        if (after[bid]) items.push({ label: 'Rebase onto ' + byB[after[bid].id].name, run: function () { rebaseDialog([bid], after[bid].id, 'Rebase onto ' + byB[after[bid].id].name); } });
        if (s.unpushed[bid]) items.push({ label: 'Force push', run: function () { self.forcePush([bid]); } });
        if (!items.length) items.push({ label: 'Nothing to fix', run: function () {} });
        var R0 = rootEl();
        self.setState({ ctx: { x: e.clientX - R0.left, y: e.clientY - R0.top, title: b.repo + ' · ' + b.name, items: items } });
      };
    }

    // ---------- rows + left action cells for one task box
    function branchProg(t, bid) {
      var cur = curStage(t); if (!cur || cur.kind === 'manual') return null;
      var steps = stepsOf(t), segs = [], label = '', tone = 'amber', pos = null;
      steps.forEach(function (x) {
        var r = x.runs.filter(function (q) { return q.b.id === bid; })[0]; if (!r) return;
        segs.push('width: 7px; height: 5px; border-radius: 2px; background: ' + ({ done: '#6cc58a', running: '#e8a33d', back: '#e8a33d', stuck: '#ef6b5b', failed: '#ef6b5b' }[r.state] || '#3a3e48') + ';');
        if (!pos && r.state !== 'done') pos = { x: x, r: r };
      });
      if (!segs.length) return null;
      if (!pos) label = cur.name + ' ✓';
      else if (pos.r.state === 'back') label = pos.x.name + ' ↩ sent back';
      else label = pos.x.name + (pos.r.todo ? ' ' + pos.r.todo[0] + '/' + pos.r.todo[1] : '') + (pos.r.loops ? ' · fix ' + pos.r.loops : '') + (pos.r.state === 'stuck' ? ' · stuck' : pos.r.state === 'failed' ? ' · failed' : pos.r.state === 'pending' ? ' · waiting' : '');
      if (pos && (pos.r.state === 'stuck' || pos.r.state === 'failed')) tone = 'red'; else if (!pos) tone = 'green'; else if (pos.r.state === 'pending') tone = 'grey';
      return { segs: segs, label: label, tip: steps.map(function (x) { var r = x.runs.filter(function (q) { return q.b.id === bid; })[0]; return r ? x.n + '. ' + x.name + ': ' + r.state + (r.todo ? ' ' + r.todo[0] + '/' + r.todo[1] : '') + (r.note ? ' (' + r.note + ')' : '') : ''; }).filter(Boolean).join('\n'),
        style: 'font-size: 11px; font-weight: 600; white-space: nowrap; flex-shrink: 0; color: ' + { amber: '#f0b85c', red: '#f28b7d', green: '#7fd49b', grey: '#9a9ca5' }[tone] + ';' };
    }
    var btnS = function (bg, fg, dis) { return 'height: 22px; padding: 0 9px; flex-shrink: 0; white-space: nowrap; border-radius: 5px; border: none; background: ' + bg + '; color: ' + fg + '; font-size: 11px; font-weight: 600; cursor: pointer; opacity: ' + (dis ? 0.5 : 1) + ';'; };
    var tagS = function (tt) { return 'max-width: 120px; box-sizing: border-box; font-size: 11px; font-weight: 600; padding: 2px 7px; border-radius: 5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex-shrink: 1; color: ' + tt[1] + '; background: ' + tt[0] + ';'; };
    var STAT = { 'To do': 'grey', 'In progress': 'amber', 'In review': 'blue', 'Blocked': 'red', 'Done': 'green' };
    var STEPC = { done: '#6cc58a', running: '#e8a33d', pending: '#3a3e48', failed: '#ef6b5b' };
    var selKey = s.sel.kind + ':' + s.sel.id;
    function taskBox(t) {
      var color = colorOf(t.id), steps = stepsOf(t);
      var rows = [], cells = [];
      var mineBs = t.branches.filter(function (bid) { return byB[bid].kind === 'mine'; });
      var allMerged = mineBs.length > 0 && mineBs.every(function (bid) { return s.merged[bid]; });
      var anyDraft = t.branches.some(function (bid) { return byB[bid].draft; });
      var tStatus = taskStatusOf(t);
      var tsel = selKey === 'task:' + t.id;
      var isRev = t.kind === 'review';
      var jr = jiraOf(t);
      var repos = []; t.branches.forEach(function (bid) { if (repos.indexOf(byB[bid].repo) < 0) repos.push(byB[bid].repo); });
      if (!isRev) {
        rows.push({ isTask: true, isBranch: false, isReview: false, hasInto: false, into: [], deploys: [], hasDeploys: false, hasLine2: true, hasProg: false, prog: { segs: [], label: '', tip: '', style: '' }, attn: attnFor(taskSessions(t).map(function (o) { return Object.assign({ tid: t.id }, o); })), label: titleOf(t),
          dot: 'width: 12px; height: 12px; border-radius: 3px; flex-shrink: 0; box-sizing: border-box; ' + (t.kind === 'parked' ? 'border: 2px dashed ' + color + ';' : 'background: ' + color + ';'),
          status: t.kind === 'parked' ? 'not merging' : tStatus, statusStyle: self.chip(t.kind === 'parked' ? 'grey' : STAT[tStatus] || 'grey'),
          hasPhase: t.kind !== 'parked', phase: t.kind === 'parked' ? { segs: [], label: '', tip: '', labelStyle: '', showBar: false } : phasePills(t),
          stepsTip: steps.map(function (x) { return (x.state === 'done' ? '✓ ' : x.state === 'running' ? '● ' : x.state === 'stuck' || x.state === 'failed' ? '! ' : '○ ') + x.n + '. ' + x.name; }).join('\n'),
          title: titleOf(t), hasSub: true, sub: (t.days && t.days.length > 1 ? t.span + 'd → ' + dayLabel(t.end) + ' · ' : '') + (jr ? jr.key + ' · ' : '') + repos.join(' · '),
          titleStyle: 'display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; padding: 0; border: none; background: transparent; text-align: left; cursor: pointer; font-family: inherit; font-size: 13px; font-weight: 700; line-height: 16px; word-break: break-word; color: ' + (t.kind === 'parked' ? '#b4b6bd' : '#e8e6e1') + ';',
          subStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 11px; line-height: 14px; color: #7c7f88; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0;',
          indentStyle: 'width: 0; flex-shrink: 0;', elbow: 'display: none;',
          style: 'height: 68px; width: 100%; box-sizing: border-box; padding: 7px 10px; border-bottom: 1px solid #2a2d35; border-left: 3px solid ' + (tsel ? '#e8a33d' : 'transparent') + '; background: ' + (tsel ? '#2a2b31' : color + '1c') + '; color: #e8e6e1; display: flex; flex-direction: column; justify-content: center; gap: 4px; cursor: pointer;',
          pick: function () { selectTask(t.id); }, menu: function () {} });
        var c0 = { hasTag: true, tag: t.kind === 'parked' ? 'not merging' : tStatus, tagStyle: tagS(self.tone(t.kind === 'parked' ? 'grey' : STAT[tStatus] || 'grey')), tip: 'task status', hasInfo: false, info: '', btns: [] };
        if (isFinished(t) || (allMerged && !wfOf(t))) { c0.tag = '✓ merged'; c0.tagStyle = tagS(self.tone('purple')); c0.tip = 'every branch landed on main'; c0.btns.push({ label: 'Archive', run: function () { requestArchive(t); }, disabled: false, tip: archiveTip, style: btnS('#a371f7', '#ffffff', false) }); }
        else if (t.kind !== 'parked') {
          var pa = phaseAction(t);
          if (pa) { var tcol = { claude: ['#d97757', '#1a0f0a'], red: ['#ef6b5b', '#15161a'], amber: ['#e8a33d', '#15161a'], green: ['#6cc58a', '#15161a'], blue: ['#7aa7ff', '#15161a'] }[pa.tone]; c0.btns.push({ label: pa.label, run: pa.run, disabled: false, tip: pa.tip, style: btnS(tcol[0], tcol[1], false) }); }
        }
        cells.push(c0);
      }
      taskBranchOrder(t).forEach(function (x) {
        var b = byB[x.id], st = gitStatus(x.id), bsel = selKey === 'branch:' + x.id;
        var p = parentOf[x.id], pOutside = p && taskOf[p] !== t.id;
        var dep = isRev ? 0 : x.dep + 1;
        rows.push({ isTask: false, isBranch: true, isReview: b.kind === 'review', owner: b.owner || '', label: b.name || 'new branch',
          hasBase: !!p, baseLabel: p ? (pOutside ? '⑂ ' + byB[p].name.replace(/^[^/]+\//, '') : '⑂') : '', baseTip: p ? 'starts from ' + byB[p].name + (byB[p].kind === 'review' ? ' (' + byB[p].owner + '\u2019s branch)' : taskOf[p] !== t.id ? ' (another task)' : '') + ', not main' : '',
          baseStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 10.5px; font-weight: 600; padding: 1px 5px; border-radius: 4px; max-width: 110px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex-shrink: 0; ' + (p && byB[p].kind === 'review' ? 'color: #93b6ff; background: rgba(122,167,255,0.12);' : 'color: #c9c7c2; background: #2a2c33;'),
          repo: nick(b.repo), repoStyle: self.repoChip(b.repo), into: intoChips(b), hasInto: intoChips(b).length + deployChips(b).length > 0, deploys: deployChips(b), hasDeploys: deployChips(b).length > 0, prog: branchProg(t, x.id) || { segs: [], label: '', tip: '', style: '' }, hasProg: !!branchProg(t, x.id), hasLine2: !!branchProg(t, x.id) || (b.draft || isRev) || intoChips(b).length + deployChips(b).length > 0,
          attn: attnFor(running(b).map(function (x2) { return { x: x2, b: b }; })),
          title: b.draft ? 'new branch' : b.name, hasSub: b.draft || isRev, sub: b.draft ? 'no branch yet · from ' + (p ? byB[p].name : 'main') : (b.pr ? b.pr.title : ''),
          titleStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 12px; font-weight: 600; line-height: 16px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: ' + (b.kind === 'review' ? '#93b6ff' : b.draft ? '#9a9ca5' : '#d8d6d1') + ';' + (b.draft ? ' font-style: italic;' : ''),
          subStyle: 'font-size: 11px; line-height: 14px; color: #7c7f88; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;' + (b.draft ? ' font-style: italic;' : ''),
          indentStyle: 'position: relative; flex-shrink: 0; align-self: stretch; width: ' + (dep * 16) + 'px;',
          elbow: dep ? 'position: absolute; right: 2px; top: -4px; width: 8px; height: 18px; box-sizing: border-box; border-left: 2px solid ' + (pOutside && byB[p].kind === 'review' ? '#7aa7ff' : '#4a4d56') + '; border-bottom: 2px solid ' + (pOutside && byB[p].kind === 'review' ? '#7aa7ff' : '#4a4d56') + '; border-bottom-left-radius: 4px;' : 'display: none;',
          style: 'height: 40px; width: 100%; box-sizing: border-box; padding: 0 10px; border-left: 3px solid ' + (bsel ? '#e8a33d' : 'transparent') + '; background: ' + (bsel ? '#26272d' : s.merged[x.id] ? 'rgba(163,113,247,0.08)' : b.kind === 'review' ? 'repeating-linear-gradient(135deg, rgba(122,167,255,0.07) 0 8px, rgba(122,167,255,0.03) 8px 16px)' : 'transparent') + '; color: #e8e6e1; display: flex; align-items: center; gap: 7px; cursor: pointer;',
          pick: function () { selectBranch(x.id); }, menu: branchMenu(x.id) });
        // left cell for this branch
        var c = { hasTag: true, tag: '', tagStyle: '', tip: '', hasInfo: false, info: '', btns: [] };
        var conf = conflicts[x.id] && conflicts[x.id][0], rootB = !p || byB[p].kind !== 'mine';
        var tone;
        var prep = prepOf(b);
        if (b.draft) { tone = 'grey'; c.tag = 'not created'; c.tip = 'Claude creates it when the task starts'; }
        else if (prep && prep.st === 'running') { tone = 'blue'; c.tag = '⚙ preparing ' + prep.took; c.tip = 'running the prepare-worktree script for ' + b.repo; }
        else if (prep && prep.st === 'failed') { tone = 'red'; c.tag = '✕ setup failed'; c.tip = 'the prepare-worktree script failed'; c.btns.push({ label: 'See error', run: function () { self.setState({ sel: { kind: 'branch', id: x.id }, tab: 'details' }); }, disabled: false, tip: 'open the setup log', style: btnS('#ef6b5b', '#15161a', false) }); }
        else if (s.merged[x.id]) { tone = 'purple'; c.tag = '✓ merged'; c.tip = 'landed on main'; }
        else if (b.kind === 'parked') { tone = 'grey'; c.tag = 'not merging'; c.tip = 'kept out of the merge order'; }
        else if (b.kind === 'review') { tone = conf ? 'red' : 'blue'; c.tag = conf ? '✕ conflict' : 'review'; c.tip = conf ? 'overlaps ' + byB[conf.with].name + ': ' + conf.files.join(', ') : 'someone else\'s branch'; }
        else if (conf) { tone = 'red'; c.tag = '✕ conflict'; c.tip = 'conflicts with ' + byB[conf.with].name + ': ' + conf.files.join(', '); c.btns.push({ label: 'Queue after', run: function () { rebaseDialog([rootOf(x.id)], conf.with, 'Queue after ' + byB[conf.with].name); }, disabled: false, tip: 'rebase onto ' + byB[conf.with].name, style: btnS('#ef6b5b', '#15161a', false) }); }
        else if (s.unpushed[x.id]) { tone = 'amber'; c.tag = '↑ not pushed'; c.tip = 'rebased locally; the remote still has the old commits'; c.btns.push({ label: s.pushing.length ? 'Pushing…' : 'Force push', run: function () { self.forcePush([x.id]); }, disabled: s.pushing.length > 0, tip: 'git push --force-with-lease', style: btnS('#e8a33d', '#15161a', s.pushing.length > 0) }); }
        else if (rootB && behind(x.id) > 0) { tone = 'amber'; c.tag = '↓' + behind(x.id) + ' main'; c.tip = behind(x.id) + ' commits behind main'; c.btns.push({ label: 'Rebase', run: function () { rebaseDialog([x.id], 'main', 'Rebase onto main'); }, disabled: busy, tip: 'rebase onto main', style: btnS('#e8a33d', '#15161a', busy) }); }
        else if (after[x.id]) { tone = 'amber'; c.tag = '↻ ' + byB[after[x.id].id].name.replace(/^[^/]+\//, ''); c.tip = 'shares ' + after[x.id].file + ' with ' + byB[after[x.id].id].name + '; merges after it'; c.btns.push({ label: 'Rebase', run: function () { rebaseDialog([x.id], after[x.id].id, 'Rebase onto ' + byB[after[x.id].id].name); }, disabled: busy, tip: 'rebase onto ' + byB[after[x.id].id].name, style: btnS('#e8a33d', '#15161a', busy) }); }
        else if (p && byB[p].kind === 'review') { tone = 'blue'; c.tag = '⏳ ' + byB[p].owner; c.tip = 'based on ' + byB[p].name; }
        else if (st[0] === 'base behind' || st[0] === 'base conflict') { tone = 'amber'; c.tag = st[0]; c.tip = 'the branch it is stacked on needs attention'; }
        else if (b.ciFailing) { tone = 'red'; c.tag = 'CI failing'; c.tip = 'checks fail on the PR'; }
        else { tone = 'green'; c.tag = '✓ clean'; c.tip = 'no shared files with work merging before it'; }
        if (!b.draft && b.kind === 'mine' && sessionsOf(b).length === 0 && !s.merged[x.id] && (!prep || prep.st === 'ok')) c.btns.push({ label: '▶ Start', run: function () { startBranchDialog(x.id); }, disabled: false, tip: 'start Claude Code in its worktree', style: btnS('#d97757', '#1a0f0a', false) });
        c.tagStyle = tagS(self.tone(tone));
        cells.push(c);
      });
      cells.forEach(function (c, ci) { c.cellStyle = 'height: ' + (ci === 0 && !isRev ? 68 : 40) + 'px; display: flex; align-items: center; justify-content: flex-end; gap: 6px; min-width: 0;'; });
      return {
        id: t.id, rows: rows, cells: cells,
        draggable: !isRev,
        dragStart: function (e) { self.dragTask = t.id; try { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', t.id); } catch (err) {} },
        dropOn: function (e) { e.preventDefault(); e.stopPropagation(); var k2 = self.dragTask; self.dragTask = null; self.setState({ dragOver: null }); if (k2 && k2 !== t.id && !isRev) self.moveTask(k2, t.id, t.day === LATER ? null : t.day); },
        boxStyle: 'flex-grow: 1; min-width: 0; max-width: 600px; display: flex; flex-direction: column; border-radius: 10px; overflow: visible; border: 1px ' + (t.kind === 'parked' ? 'dashed #3a3e48' : allMerged ? 'solid rgba(163,113,247,0.6)' : 'solid #3a3e48') + '; border-left: 4px ' + (t.kind === 'parked' ? 'dashed ' : 'solid ') + (isRev ? '#7aa7ff' : color) + '; background: ' + (t.kind === 'parked' ? 'repeating-linear-gradient(135deg, #17181c 0 7px, #1c1d22 7px 14px)' : '#17181c') + '; box-shadow: 0 2px 0 rgba(0,0,0,0.35), 0 6px 16px rgba(0,0,0,0.18); cursor: ' + (isRev ? 'default' : 'grab') + ';',
        _day: t.day, _t: t
      };
    }
    var visible = function (t) { return t.branches.length === 0 || t.branches.some(function (bid) { return s.repoOn[byB[bid].repo] !== false; }); };
    var LIMIT = 10, visSeq = seq.filter(visible);
    var shownIds = {}; visSeq.forEach(function (t, i) { if (s.showAllItems || i < LIMIT) shownIds[t.id] = true; });
    var hiddenCount = visSeq.length - Object.keys(shownIds).length;
    var lastShownDay = -1e9; visSeq.forEach(function (t) { if (shownIds[t.id] && t.day !== LATER) lastShownDay = Math.max(lastShownDay, t.end); });
    var boxes = visSeq.filter(function (t) { return shownIds[t.id]; }).map(taskBox);
    // ---------- hours, overflow, bands
    var hoursOn = {}, taskHoursOn = {};
    seq.forEach(function (t) {
      if (t.kind === 'review') return;
      var e = estOf(t); if (!e) return;
      if (e.days > 1 && t.day !== LATER) t.days.forEach(function (d2) { hoursOn[d2] = (hoursOn[d2] || 0) + e.hours / e.days; if (d2 === t.day) taskHoursOn[t.id] = e.hours / e.days; });
      else { hoursOn[t.day] = (hoursOn[t.day] || 0) + e.hours; taskHoursOn[t.id] = e.hours; }
    });
    function doneTask(t) { var m = t.branches.filter(function (bid) { return byB[bid].kind === 'mine'; }); return m.length && m.every(function (bid) { return s.merged[bid]; }); }
    function overflowOf(dk) {
      var total = hoursOn[dk] || 0; if (dk === LATER || total <= CAP + 0.01) return [];
      var starts = seq.filter(function (t) { return t.day === dk && t.kind !== 'review' && !doneTask(t); });
      var out = [], over = total - CAP;
      for (var i = starts.length - 1; i >= 0 && over > 0.01; i--) { out.unshift(starts[i].id); over -= taskHoursOn[starts[i].id] || 0; }
      return out;
    }
    var keys = {};
    if (s.showHistory) for (var h0 = -s.history; h0 < 0; h0++) keys[h0] = true;
    for (var k0 = 0; k0 < s.horizon; k0++) if (s.showAllItems || !hiddenCount || k0 <= lastShownDay) keys[k0] = true;
    seq.forEach(function (t) { if (!shownIds[t.id]) return; (t.days || []).forEach(function (d2) { if (d2 !== LATER) keys[d2] = true; }); });
    s.extraDays.forEach(function (k) { keys[k] = true; }); s.offDays.forEach(function (k) { keys[k] = true; });
    var dayKeys = Object.keys(keys).map(Number).sort(function (a2, b2) { return a2 - b2; }).concat(!hiddenCount || boxes.some(function (b) { return b._day === LATER; }) ? [LATER] : []);
    var prevMonth = null;
    var bands = dayKeys.map(function (dk) {
      var bl = boxes.filter(function (b) { return b._day === dk; });
      var spans = [];
      seq.forEach(function (t) {
        if (!t.days || t.days.length < 2 || !shownIds[t.id]) return;
        var idx = t.days.indexOf(dk); if (idx <= 0) return;
        var isEnd = idx === t.days.length - 1, col = colorOf(t.id);
        spans.push({ title: titleOf(t), note: 'day ' + (idx + 1) + '/' + t.days.length + (isEnd ? ' · merges' : ''), tip: 'continues from ' + dayLabel(t.day) + '; click to open',
          noteStyle: 'font-size: 11px; color: ' + (isEnd ? '#f0b85c' : '#7c7f88') + '; white-space: nowrap; flex-shrink: 0;',
          style: 'margin-left: 218px; max-width: 600px; height: 28px; box-sizing: border-box; padding: 0 10px; border-radius: 6px; border: 1px dashed #34373f; border-left: 3px solid ' + col + '; background: transparent; color: #9a9ca5; font-size: 12px; display: flex; align-items: center; gap: 8px; cursor: pointer; text-align: left;',
          pick: function () { selectTask(t.id); self.scrollToDay(t.day); } });
      });
      var hrs = Math.round((hoursOn[dk] || 0) * 10) / 10;
      var hItems = s.showHistory ? HIST.filter(function (x) { return x.day === dk; }) : [];
      var over = s.dragOver === dk, empty = bl.length === 0 && spans.length === 0 && hItems.length === 0;
      var isToday = dk === 0, isPast = dk < 0 && dk !== LATER, weekend = dk !== LATER && isWeekend(dk) && s.workWeekend.indexOf(dk) < 0, dayOff = dk !== LATER && isDayOff(dk), greyed = weekend || dayOff;
      var monday = dk !== LATER && dateOf(dk).getDay() === 1;
      var overdue = isPast ? seq.filter(function (t) { return t.day === dk && t.kind !== 'review' && t.end < 0 && !doneTask(t); }).map(function (t) { return t.id; }) : [];
      var overflow = !isPast ? overflowOf(dk) : [], nxt = dk === LATER ? LATER : nextWork(dk);
      var label = dayLabel(dk);
      if (dk !== LATER && dk !== 0) { var dt = dateOf(dk); if (prevMonth !== null && dt.getMonth() !== prevMonth) label += ' ' + MO[dt.getMonth()]; prevMonth = dt.getMonth(); } else if (dk === 0) prevMonth = TODAY.getMonth();
      return {
        domId: 'aq-day-' + (dk === LATER ? 'later' : dk < 0 ? 'm' + (-dk) : dk), label: label, sub: dayOff ? 'off' : (hrs ? hrs + 'h' : ''),
        tasks: bl, spans: spans, history: hItems, isLater: dk === LATER,
        isOverdue: overdue.length > 0, overdueNote: overdue.length + (overdue.length === 1 ? ' task' : ' tasks') + ' not merged', rollover: function () { self.shiftTasks(overdue, firstWork(0)); },
        isOver: overflow.length > 0 && !greyed, overNote: Math.round((hrs - CAP) * 10) / 10 + 'h over ' + CAP + 'h',
        overLabel: 'Move to ' + dayLabel(nxt) + ' · ' + (overflow.length === 1 ? titleOf(tById[overflow[0]]) : overflow.length + ' tasks'), overflowMove: function () { self.shiftTasks(overflow, nxt); },
        rowStyle: 'display: flex; border-top: 1px ' + (monday || dk === LATER || isToday ? 'solid #34373f' : 'dashed #202227') + ';' + (greyed ? ' background: repeating-linear-gradient(135deg, rgba(255,255,255,0.018) 0 6px, transparent 6px 12px);' : overdue.length ? ' background: rgba(232,163,61,0.04);' : isPast ? ' background: rgba(255,255,255,0.012);' : ''),
        labelStyle: 'font-size: ' + (empty ? 11 : 12) + 'px; font-weight: ' + (empty ? 500 : 600) + '; color: ' + (greyed ? '#4f525b' : overdue.length ? '#f0b85c' : empty || isPast ? '#6b6f7a' : '#e8e6e1') + '; white-space: nowrap;' + (dayOff ? ' text-decoration: line-through;' : ''),
        subStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 11px; color: ' + (greyed ? '#4f525b' : hrs > CAP && dk !== LATER ? '#f28b7d' : '#9a9ca5') + ';',
        rulerStyle: 'width: 60px; flex-shrink: 0; box-sizing: border-box; border-right: 2px solid ' + (isToday ? '#e8a33d' : isPast || greyed ? '#2a2d35' : '#3a3e48') + '; padding: ' + (empty ? '2px' : '6px') + ' 10px ' + (empty ? '2px' : '6px') + ' 0; text-align: right; position: relative;',
        tick: 'position: absolute; right: -' + (empty ? 4 : 6) + 'px; top: ' + (empty ? 6 : 10) + 'px; width: ' + (empty ? 6 : 10) + 'px; height: ' + (empty ? 6 : 10) + 'px; border-radius: 50%; box-sizing: border-box; background: ' + (isToday ? '#e8a33d' : '#121316') + '; border: 2px solid ' + (isToday ? '#e8a33d' : greyed ? '#2a2d35' : '#3a3e48') + ';',
        dropStyle: 'flex-grow: 1; min-width: 0; min-height: ' + (empty ? (greyed ? 16 : 22) : 0) + 'px; box-sizing: border-box; padding: ' + (empty ? '0' : '8px') + ' 0 ' + (empty ? '0' : '10px') + ' 8px; display: flex; flex-direction: column; gap: 10px; border-radius: 6px; background: ' + (over && !dayOff ? 'rgba(232,163,61,0.07)' : 'transparent') + '; outline: ' + (over && !dayOff ? '1px dashed #e8a33d' : 'none') + ';',
        over: function (e) { if (dayOff || isPast) return; e.preventDefault(); if (self.state.dragOver !== dk) self.setState({ dragOver: dk }); },
        drop: function (e) { e.preventDefault(); var k2 = self.dragTask; self.dragTask = null; self.setState({ dragOver: null }); if (k2 && !dayOff && !isPast) self.moveTask(k2, null, dk === LATER ? null : dk); },
        menu: function (e) {
          if (dk === LATER || isPast) return; e.preventDefault();
          var R0 = rootEl(), wkd = isWeekend(dk), off = isDayOff(dk), wkWork = s.workWeekend.indexOf(dk) >= 0;
          var startsHere = seq.filter(function (t) { return t.day === dk && t.kind !== 'review'; }).map(function (t) { return t.id; });
          var item = off ? { label: 'Mark as working day', run: function () { self.setState({ offDays: self.state.offDays.filter(function (k) { return k !== dk; }), ctx: null }); } }
            : wkd ? { label: wkWork ? 'Mark as weekend (off)' : 'Work this day', run: function () { self.setState({ workWeekend: wkWork ? self.state.workWeekend.filter(function (k) { return k !== dk; }) : self.state.workWeekend.concat([dk]), ctx: null }); } }
            : { label: 'Mark as day off', run: function () {
                var offs = self.state.offDays.concat([dk]);
                if (!startsHere.length) { self.setState({ offDays: offs, ctx: null }); return; }
                var to = dk + 1; while (offs.indexOf(to) >= 0 || (isWeekend(to) && self.state.workWeekend.indexOf(to) < 0)) to++;
                self.setState({ offDays: offs, ctx: null, confirm: { title: dayLong(dk) + ' is a day off', text: 'Move ' + startsHere.length + (startsHere.length === 1 ? ' task' : ' tasks') + ' planned for that day to ' + dayLong(to) + '?', keys: startsHere, to: to } });
              } };
          self.setState({ ctx: { x: e.clientX - R0.left, y: e.clientY - R0.top, title: dayLong(dk), items: [item] } });
        }
      };
    });
    var dayCtl = {
      moreWeek: function () { self.setState({ horizon: self.state.horizon + 7 }); },
      pickDate: function (e) { var v = e.target.value; if (!v) return; var p = v.split('-'); var k = offsetOf(new Date(+p[0], +p[1] - 1, +p[2])); if (k > 0 && self.state.extraDays.indexOf(k) < 0) self.setState({ extraDays: self.state.extraDays.concat([k]) }); }
    };
    var ctx = { open: false, style: '', title: '', items: [] };
    if (s.ctx) ctx = { open: true, title: s.ctx.title, items: s.ctx.items.map(function (it) { return { label: it.label, run: function (e) { if (e && e.stopPropagation) e.stopPropagation(); self.setState({ ctx: null }); it.run(); } }; }),
      style: 'position: absolute; left: ' + s.ctx.x + 'px; top: ' + s.ctx.y + 'px; z-index: 60; min-width: 220px; max-width: 320px; box-sizing: border-box; padding: 4px; border-radius: 8px; border: 1px solid #3a3e48; background: #1f2127; box-shadow: 0 12px 30px rgba(0,0,0,0.5);' };
    var confirm = { open: false, title: '', text: '', yesLabel: '', noLabel: '', yes: function () {}, no: function () {} };
    if (s.confirm) { var CF = s.confirm; confirm = { open: true, title: CF.title, text: CF.text, yesLabel: 'Move to ' + dayLabel(CF.to), noLabel: 'Leave it', yes: function () { self.shiftTasks(CF.keys, CF.to); self.setState({ confirm: null }); }, no: function () { self.setState({ confirm: null }); } }; }
    // ---------- Add: new task (any repos) or existing branch (any repo, newest first)
    var nw = s.nw;
    var tabS = function (on) { return 'height: 34px; padding: 0 10px; border: none; border-bottom: 2px solid ' + (on ? '#e8a33d' : 'transparent') + '; background: transparent; color: ' + (on ? '#e8e6e1' : '#9a9ca5') + '; font-size: 12px; font-weight: ' + (on ? 600 : 500) + '; cursor: pointer;'; };
    var nwRepos = repoNames.filter(function (r) { return nw.repos[r]; });
    var canAdd = !!((nw.title.trim() || jiraKey(nw.jira)) && nwRepos.length);
    function setNw(k, v) { var o = Object.assign({}, self.state.nw); o[k] = v; self.setState({ nw: o }); }
    var q = (s.pickerQ || '').toLowerCase();
    function ago(m) { return m < 60 ? m + 'm ago' : m < 1440 ? Math.round(m / 60) + 'h ago' : Math.round(m / 1440) + 'd ago'; }
    var pool = BR.filter(function (b) { return b.pool && !s.attach[b.id] && (!q || (b.repo + ' ' + b.name).toLowerCase().indexOf(q) >= 0); }).sort(function (a, b) { return a.agoMin - b.agoMin; });
    var picker = {
      open: s.picker, isNew: s.pickerTab === 'new', isBranch: s.pickerTab === 'branch',
      tabNew: function () { self.setState({ pickerTab: 'new' }); }, tabBranch: function () { self.setState({ pickerTab: 'branch' }); },
      tabNewStyle: tabS(s.pickerTab === 'new'), tabBranchStyle: tabS(s.pickerTab === 'branch'),
      targetNote: s.pickerTask && tById[s.pickerTask] ? 'adding to: ' + titleOf(tById[s.pickerTask]) : '',
      nwTitle: nw.title, nwJira: nw.jira, nwNotes: nw.notes,
      onTitle: function (e) { setNw('title', e.target.value); }, onJira: function (e) { setNw('jira', e.target.value); }, onNotes: function (e) { setNw('notes', e.target.value); },
      repoOpts: repoNames.map(function (r) { var on = !!nw.repos[r]; return { name: nick(r), on: on, toggle: function () { var rr = Object.assign({}, self.state.nw.repos); rr[r] = !rr[r]; setNw('repos', rr); },
        style: self.repoChip(r) + ' height: 26px; padding: 0 10px; font-size: 11.5px; cursor: pointer; background: ' + (on ? 'rgba(255,255,255,0.08)' : 'transparent') + '; opacity: ' + (on ? 1 : 0.55) + ';' }; }),
      cantAdd: !canAdd,
      addStyle: 'height: 28px; padding: 0 12px; flex-shrink: 0; white-space: nowrap; border-radius: 6px; border: none; background: ' + (canAdd ? '#e8a33d' : '#2a2c33') + '; color: ' + (canAdd ? '#15161a' : '#7c7f88') + '; font-size: 12px; font-weight: 600; cursor: ' + (canAdd ? 'pointer' : 'not-allowed') + ';',
      addNew: function () {
        if (!canAdd) return;
        var id = 'T_' + Math.random().toString(16).slice(2, 7), k = jiraKey(nw.jira);
        var t = { id: id, title: nw.title.trim(), jira: k ? { key: k, title: '', status: '', url: /^https?:/.test(nw.jira) ? nw.jira.trim() : 'https://acme.atlassian.net/browse/' + k } : null, status: 'To do', est: '', draftRepos: nwRepos, notes: nw.notes.trim(), workflow: 'standard', stage: 'spec', run: {} };
        self.setState({ view: 'plan', newTasks: self.state.newTasks.concat([t]), sel: { kind: 'task', id: id }, tab: 'task', picker: false, nw: { title: '', jira: '', repos: { 'web-app': true }, notes: '' } });
      },
      q: s.pickerQ, onQ: function (e) { self.setState({ pickerQ: e.target.value }); }, empty: pool.length === 0,
      items: pool.map(function (b) {
        var mine = b.author === 'you';
        return { name: b.name, repo: nick(b.repo), repoStyle: self.repoChip(b.repo), who: mine ? 'you' : b.author, ago: ago(b.agoMin),
          whoStyle: 'font-size: 11px; padding: 0 6px; border-radius: 4px; flex-shrink: 0; ' + (mine ? 'background: #23252b; color: #c9c7c2;' : 'background: rgba(122,167,255,0.14); color: #93b6ff;'),
          add: function () {
            var at = Object.assign({}, self.state.attach);
            if (self.state.pickerTask) { at[b.id] = self.state.pickerTask; self.setState({ view: 'plan', attach: at, picker: false, pickerQ: '', sel: { kind: 'branch', id: b.id }, tab: 'details' }); return; }
            var id = 'T_' + Math.random().toString(16).slice(2, 7);
            at[b.id] = id;
            self.setState({ attach: at, newTasks: self.state.newTasks.concat([{ id: id, kind: b.kind === 'review' ? 'review' : 'task', owner: b.owner, title: '', jira: null, status: 'To do', est: '', branches: [], steps: [] }]), picker: false, pickerQ: '', sel: { kind: 'task', id: id }, tab: 'task' });
          } };
      })
    };

    // ---------- selection
    var selT = null, selB = null;
    if (s.sel.kind === 'branch' && byB[s.sel.id] && taskOf[s.sel.id] && !isArchived(taskOf[s.sel.id])) { selB = byB[s.sel.id]; selT = tById[taskOf[s.sel.id]]; }
    else { selT = tById[s.sel.id] && !isArchived(s.sel.id) ? tById[s.sel.id] : seq.filter(function (t) { return t.kind !== 'review'; })[0]; }
    var inboxSelItem = s.view === 'inbox' ? (s.inbox.filter(function (it) { return it.id === s.inboxSel; })[0] || s.inbox[0] || null) : null;
    var noteKey = inboxSelItem ? inboxSelItem.id : (selT ? selT.id : null);
    var btnP = function (t) { return 'height: 26px; padding: 0 10px; border-radius: 5px; border: none; background: ' + self.tone(t)[2] + '; color: ' + (t === 'purple' ? '#ffffff' : '#15161a') + '; font-size: 12px; font-weight: 600; cursor: pointer;'; };
    var btnG = 'height: 26px; padding: 0 10px; border-radius: 5px; border: 1px solid #3a3e48; background: transparent; color: #e8e6e1; font-size: 12px; cursor: pointer;';
    var startBtn = 'height: 26px; padding: 0 10px; border-radius: 5px; border: none; background: #d97757; color: #1a0f0a; font-size: 12px; font-weight: 600; cursor: pointer;';
    // ---------- notes: WYSIWYG rich text; stored as Markdown (the user never sees Markdown)
    function setNote(v) { var nn = Object.assign({}, self.state.notes); nn[noteKey] = v; self.setState({ notes: nn }); }
    function esc(t) { return String(t).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'); }
    function mdInline(t) {
      return esc(t)
        .replace(/`([^`]+)`/g, '<code>$1</code>')
        .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
        .replace(/\*([^*]+)\*/g, '<i>$1</i>')
        .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    }
    function mdToHtml(md) {
      var out = [], list = null;
      function close() { if (list) { out.push(list === 'ol' ? '</ol>' : '</ul>'); list = null; } }
      function open(kind, tag) { if (list !== kind) { close(); out.push(tag); list = kind; } }
      String(md || '').split('\n').forEach(function (l) {
        var m;
        if ((m = l.match(/^- \[( |x)\] (.*)$/i))) { open('task', '<ul data-tasks="1">'); out.push('<li data-done="' + (m[1].toLowerCase() === 'x' ? 1 : 0) + '">' + mdInline(m[2]) + '</li>'); return; }
        if ((m = l.match(/^- (.*)$/))) { open('ul', '<ul>'); out.push('<li>' + mdInline(m[1]) + '</li>'); return; }
        if ((m = l.match(/^\d+\. (.*)$/))) { open('ol', '<ol>'); out.push('<li>' + mdInline(m[1]) + '</li>'); return; }
        close();
        if ((m = l.match(/^#{1,3} (.*)$/))) { out.push('<h3>' + mdInline(m[1]) + '</h3>'); return; }
        if (l.trim()) out.push('<p>' + mdInline(l) + '</p>');
      });
      close();
      return out.join('');
    }
    function inlineMd(node) {
      var s2 = '';
      node.childNodes.forEach(function (c) {
        if (c.nodeType === 3) { s2 += c.nodeValue; return; }
        if (c.nodeType !== 1) return;
        var tg = c.tagName, inner = inlineMd(c);
        if (tg === 'B' || tg === 'STRONG') s2 += inner.trim() ? '**' + inner + '**' : inner;
        else if (tg === 'I' || tg === 'EM') s2 += inner.trim() ? '*' + inner + '*' : inner;
        else if (tg === 'CODE') s2 += '`' + c.textContent + '`';
        else if (tg === 'A') s2 += '[' + inner + '](' + c.getAttribute('href') + ')';
        else if (tg === 'BR') s2 += '';
        else s2 += inner;
      });
      return s2.replace(/\u00a0/g, ' ');
    }
    function htmlToMd(root) {
      var lines = [];
      (function walk(parent) { parent.childNodes.forEach(function (c) {
        if (c.nodeType === 3) { if (c.nodeValue.trim()) lines.push(c.nodeValue.trim()); return; }
        if (c.nodeType !== 1) return;
        var tg = c.tagName;
        if (/^H[1-6]$/.test(tg)) lines.push('## ' + inlineMd(c));
        else if (tg === 'UL' || tg === 'OL') {
          var tasks = c.hasAttribute('data-tasks'), i = 0;
          c.querySelectorAll(':scope > li').forEach(function (li) { i++; lines.push(tasks ? '- [' + (li.getAttribute('data-done') === '1' ? 'x' : ' ') + '] ' + inlineMd(li) : tg === 'OL' ? i + '. ' + inlineMd(li) : '- ' + inlineMd(li)); });
        }
        else if ((tg === 'P' || tg === 'DIV') && c.querySelector('ul,ol,h1,h2,h3,p,div')) walk(c);
        else { var t = inlineMd(c); lines.push(t); }
      }); })(root);
      return lines.join('\n').replace(/\n{3,}/g, '\n\n').trim();
    }
    function editor() { return document.getElementById('notes-rt'); }
    function syncFromEditor() { var el = editor(); if (el) setNote(htmlToMd(el)); }
    // fill the editor when the selected branch changes (or the tab re-mounts it); never re-render it while typing
    setTimeout(function () {
      var el = editor();
      if (el && el.getAttribute('data-for') !== noteKey) { el.innerHTML = mdToHtml(self.state.notes[noteKey] || ''); el.setAttribute('data-for', noteKey); }
    }, 0);
    function cmd(name, val) { return function () { var el = editor(); if (!el) return; el.focus(); try { document.execCommand(name, false, val); } catch (e) {} syncFromEditor(); }; }
    function toggleBlock(tag) {
      return function () {
        var el = editor(); if (!el) return; el.focus();
        var cur = ''; try { cur = String(document.queryCommandValue('formatBlock') || '').toLowerCase(); } catch (e) {}
        try { document.execCommand('formatBlock', false, cur === tag ? 'p' : tag); } catch (e) {}
        syncFromEditor();
      };
    }
    function insertCode() {
      var el = editor(); if (!el) return; el.focus();
      var t = String(window.getSelection() || '') || 'code';
      try { document.execCommand('insertHTML', false, '<code>' + esc(t) + '</code>&#8203;'); } catch (e) {}
      syncFromEditor();
    }
    function listAtCaret() { var el = editor(), s4 = window.getSelection(), n4 = s4 && s4.anchorNode; while (n4 && n4 !== el && n4.tagName !== 'UL' && n4.tagName !== 'OL') n4 = n4.parentNode; return n4 && n4 !== el ? n4 : null; }
    function markTasks(ul) { ul.setAttribute('data-tasks', '1'); ul.querySelectorAll('li').forEach(function (li) { if (!li.hasAttribute('data-done')) li.setAttribute('data-done', '0'); }); }
    function checklist() {
      var el = editor(); if (!el) return; el.focus();
      var cur = listAtCaret();
      if (cur && cur.tagName === 'UL' && cur.hasAttribute('data-tasks')) { cur.removeAttribute('data-tasks'); cur.querySelectorAll('li').forEach(function (li) { li.removeAttribute('data-done'); }); }
      else if (cur && cur.tagName === 'UL') markTasks(cur);
      else { try { document.execCommand('insertUnorderedList'); } catch (e) {} var made = listAtCaret(); if (made) markTasks(made); }
      syncFromEditor();
    }
    function openLink() {
      var sel2 = window.getSelection();
      self.savedRange = sel2 && sel2.rangeCount ? sel2.getRangeAt(0).cloneRange() : null;
      self.setState({ linkOpen: true, linkUrl: '' });
    }
    function applyLink() {
      var url = (self.state.linkUrl || '').trim();
      if (!/^https?:\/\//.test(url)) { self.setState({ linkOpen: false }); return; }
      var el = editor(); if (!el) return; el.focus();
      var sel2 = window.getSelection();
      if (self.savedRange) { sel2.removeAllRanges(); sel2.addRange(self.savedRange); }
      if (String(sel2) ) { try { document.execCommand('createLink', false, url); } catch (e) {} }
      else { try { document.execCommand('insertHTML', false, '<a href="' + esc(url) + '">' + esc(url) + '</a>&nbsp;'); } catch (e) {} }
      el.querySelectorAll('a').forEach(function (a) { a.setAttribute('target', '_blank'); a.setAttribute('rel', 'noopener'); });
      self.setState({ linkOpen: false, linkUrl: '' });
      syncFromEditor();
    }
    function noteToolbar() {
      var btn = 'height: 24px; min-width: 26px; padding: 0 5px; border-radius: 4px; border: none; background: transparent; color: #c9c7c2; font-size: 12px; cursor: pointer;';
      return [
        ['Bold', 'B', cmd('bold'), btn + ' font-weight: 800;'],
        ['Italic', 'I', cmd('italic'), btn + ' font-style: italic; font-family: Georgia, serif;'],
        ['Code', '</>', insertCode, btn + ' font-family: \'IBM Plex Mono\', monospace; font-size: 10px;'],
        ['Heading', 'H', toggleBlock('h3'), btn + ' font-weight: 700;'],
        ['Bulleted list', '•', cmd('insertUnorderedList'), btn],
        ['Numbered list', '1.', cmd('insertOrderedList'), btn + ' font-size: 11px;'],
        ['Checklist', '☐', checklist, btn],
        ['Link', '🔗', openLink, btn + ' font-size: 11px;']
      ].map(function (f) { return { label: f[0], glyph: f[1], run: f[2], style: f[3], keep: function (e) { e.preventDefault(); } }; });
    }


    // terminals for a set of branches
    function stepNameOf(o) { var t2 = tById[o.b ? taskOf[o.b.id] : null]; var w = t2 ? wfOf(t2) : null, found = null; if (w) w.stages.forEach(function (sg) { (sg.steps || []).forEach(function (z) { if (z.id === o.x.step) found = z; }); }); return found ? found.name : o.x.step || ''; }
    function termFor(items, key, cwdFallback) {
      var list = items.filter(function (o) { return o.x.state === 'running'; });
      list.sort(function (p2, q2) { return (p2.x.mode === 'tui' ? 0 : 1) - (q2.x.mode === 'tui' ? 0 : 1) || self.act(p2.x.act).rank - self.act(q2.x.act).rank; });
      var curId = s.termTab[key], cur = 0; list.forEach(function (o, i) { if (o.x.id === curId) cur = i; });
      var tabs2 = list.map(function (o, i) {
        var on = i === cur, a = self.act(o.x.act, 12), hl = o.x.mode === 'headless';
        return { name: hl ? 'run · ' + stepNameOf(o) + (o.b ? ' · ' + o.b.repo : '') : (o.b ? o.b.repo : 'spec') + ' · claude ' + o.x.id + (o.x.resumes ? ' (resumed)' : ''),
          iconStyle: a.style, glyph: a.glyph, badge: hl ? 'claude -p' : 'TUI',
          badgeStyle: 'font-size: 9.5px; font-weight: 700; padding: 0 4px; border-radius: 3px; ' + (hl ? 'border: 1px dashed #5c606b; color: #9a9ca5;' : 'background: rgba(217,119,87,0.2); color: #e8a07f;'),
          pick: function () { var tt = Object.assign({}, self.state.termTab); tt[key] = o.x.id; self.setState({ termTab: tt }); },
          style: 'height: 30px; padding: 0 12px; flex-shrink: 0; border: none; border-right: 1px solid #22252c; background: ' + (on ? '#0b0c0e' : 'transparent') + '; color: ' + (on ? '#e8e6e1' : '#9a9ca5') + '; font-family: \'IBM Plex Mono\', monospace; font-size: 11px; display: flex; align-items: center; gap: 6px; cursor: pointer; white-space: nowrap; box-shadow: ' + (on ? 'inset 0 2px 0 #d97757' : 'none') + ';' };
      });
      var c = list[cur], term = [], ca = c ? self.act(c.x.act, 14) : null, hl = c && c.x.mode === 'headless';
      var cwd = c ? (c.b ? wtOf(c.b) : cwdFallback) : '';
      if (c && hl) {
        term = [['claude -p --output-format stream-json · session ' + c.x.id, '#9a9ca5'], ['cwd ' + cwd, '#9a9ca5'], ['step: ' + stepNameOf(c), '#c9c7c2']]
          .concat(c.b ? c.b.commits.map(function (cm) { return ['› git commit ' + cm[0] + ' ' + cm[1], '#c9c7c2']; }) : [])
          .concat(c.b && c.b.files[0] ? [['› edit ' + c.b.files[0][0], '#c9c7c2']] : [])
          .concat(c.x.act === 'input' ? [['! stuck: needs a decision it cannot make headless', '#f0b85c']] : c.x.act === 'waiting' ? [['… waiting for CI monitor to report', '#93b6ff']] : [['● working…', '#7fd49b']]);
      } else if (c) {
        term = [['$ claude' + (c.x.resumes ? ' --resume ' + c.x.resumes : ''), '#d97757'], ['cwd ' + cwd, '#9a9ca5']]
          .concat(c.x.act === 'input' ? [['? Which providers should the spec cover first? (Google, GitHub, both)', '#f0b85c']] : [['> ', '#9a9ca5']]);
      }
      var stopped = items.filter(function (o) { return o.x.state !== 'running'; });
      return {
        termTabs: tabs2, hasRunning: !!c, noRunning: !c, isTui: !!c && !hl, isHeadless: !!hl,
        curActStyle: ca ? ca.style : '', curActGlyph: ca ? ca.glyph : '',
        curActLabel: c ? (hl ? 'headless run · step ' + stepNameOf(c) + ' · ' + (c.x.act === 'input' ? 'stuck · needs you' : ca.label) + ' · ' + c.x.last : 'interactive · ' + ca.label + ' · ' + c.x.last) : '',
        curActBar: 'display: flex; align-items: center; gap: 8px; padding: 6px 12px; font-size: 12px; border-bottom: 1px solid #22252c; color: ' + (c && c.x.act === 'input' ? '#f0b85c' : '#9a9ca5') + '; background: ' + (c && c.x.act === 'input' ? 'rgba(232,163,61,0.08)' : 'transparent') + ';',
        openTuiHere: function () { if (c) openTui(c); },
        term: term.map(function (l) { return { text: l[0], style: 'color: ' + l[1] + '; white-space: pre-wrap;' }; }),
        hasStopped: stopped.length > 0,
        stopped: stopped.map(function (o) { return { last: o.x.last, label: (o.x.mode === 'headless' ? 'run · ' + stepNameOf(o) : 'TUI') + ' · ' + (o.b ? o.b.repo + ' · ' + o.b.name : 'spec') + ' · ' + o.x.id,
          resume: function () { if (o.b) self.openDialog({ kind: 'start', task: taskOf[o.b.id], resume: o.x.id, resumeB: o.b.id, sessionOn: [], create: [], title: 'Resume in Claude Code (interactive)', askWt: isArchived(taskOf[o.b.id]), wt: 'new', noSame: isArchived(taskOf[o.b.id]) }); } }; }),
        count: list.length
      };
    }
    var notesUI = {
      fmt: noteToolbar(),
      onRichInput: function () { syncFromEditor(); },
      onRichClick: function (e) {
        var li = e.target && e.target.closest ? e.target.closest('ul[data-tasks] > li') : null;
        if (li && e.offsetX < 18) { li.setAttribute('data-done', li.getAttribute('data-done') === '1' ? '0' : '1'); syncFromEditor(); }
        var a = e.target && e.target.closest ? e.target.closest('a') : null;
        if (a && (e.metaKey || e.ctrlKey)) window.open(a.href, '_blank', 'noopener');
      },
      linkOpen: !!s.linkOpen, linkUrl: s.linkUrl || '',
      onLinkUrl: function (e) { self.setState({ linkUrl: e.target.value }); },
      onLinkKey: function (e) { if (e.key === 'Enter') { e.preventDefault(); applyLink(); } if (e.key === 'Escape') self.setState({ linkOpen: false }); },
      applyLink: applyLink, keep: function (e) { e.preventDefault(); },
    };
    var JT = { 'To do': 'grey', 'In progress': 'amber', 'In review': 'blue', 'Done': 'green' };
    var PRT = { 'Draft': 'grey', 'Open': 'green', 'Approved': 'green', 'Changes requested': 'red', 'Merged': 'purple', 'Closed': 'grey' };
    var sel = {}, tabs = [];
    if (selT && !selB) {
      var t = selT, steps = stepsOf(t), tStatus = taskStatusOf(t), jr = jiraOf(t);
      var repos2 = []; t.branches.forEach(function (bid) { if (repos2.indexOf(byB[bid].repo) < 0) repos2.push(byB[bid].repo); });
      var mineBs = t.branches.filter(function (bid) { return byB[bid].kind === 'mine'; });
      var allMerged = mineBs.length > 0 && mineBs.every(function (bid) { return s.merged[bid]; });
      var anyDraft = t.branches.some(function (bid) { return byB[bid].draft; });
      var everStarted = t.branches.some(function (bid) { return sessionsOf(byB[bid]).length > 0; });
      var acts = [];
      var finished = isFinished(t) || (allMerged && !wfOf(t));
      if (finished) acts.push({ label: 'Archive', tip: archiveTip, disabled: false, style: btnP('purple'), run: function () { requestArchive(t); } });
      var pa2 = (t.kind === 'task' || !t.kind) && !finished ? phaseAction(t) : null;
      if (pa2) acts.push({ label: pa2.label, tip: pa2.tip, disabled: false, style: pa2.tone === 'claude' ? startBtn : btnP(pa2.tone), run: pa2.run });
      if (!finished) acts.push({ label: 'Archive', tip: archiveTip, disabled: false, style: btnG, run: function () { requestArchive(t); } });
      var tf = termFor(taskSessions(t), 't:' + t.id, '~/code/' + (repos2[0] || 'web-app'));
      var estRaw = s.est[t.id] !== undefined ? s.est[t.id] : (t.est || ''), estM = String(estRaw).match(/^(\d+(?:\.\d+)?)\s*([hd])$/i), estP = self.parseEst(estRaw);
      var estNum = estM ? estM[1] : '', estUnit = estM ? estM[2].toLowerCase() : 'h';
      var setEst = function (num, unit) { var es = Object.assign({}, self.state.est); es[t.id] = num === '' ? '' : num + unit; self.setState({ est: es }); };
      var segOn = 'height: 20px; padding: 0 9px; border-radius: 4px; border: none; background: #2f323b; color: #e8e6e1; font-size: 11px; font-weight: 600; cursor: pointer;', segOff = 'height: 20px; padding: 0 9px; border-radius: 4px; border: none; background: transparent; color: #9a9ca5; font-size: 11px; cursor: pointer;';
      var isRev = t.kind === 'review';
      sel = Object.assign({
        isTask: true, isBranch: false, isReview: isRev, reviewNote: isRev ? (t.owner || 'Someone') + '’s work. Read-only here: you can run agents on it and keep your own notes.' : '',
        title: titleOf(t), headStyle: 'margin: 0; font-size: 14px; font-weight: 700; flex-grow: 1; min-width: 0; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; line-height: 18px; word-break: break-word;',
        dot: 'width: 12px; height: 12px; border-radius: 3px; flex-shrink: 0; background: ' + colorOf(t.id) + ';',
        status: isRev ? 'review' : tStatus, chip: self.chip(isRev ? 'blue' : STAT[tStatus] || 'grey'),
        statusWhy: isFinished(t) ? 'workflow finished' : tStatus === 'Blocked' ? 'a step is stuck or failed' : tStatus === 'To do' ? 'not started' : (curStage(t) ? curStage(t).name + ' stage' : ''),
        facts: (t.day === undefined || t.day === LATER ? 'Later' : dayLabel(t.day) + (t.days && t.days.length > 1 ? '–' + dayLabel(t.end) : '')) + ' · ' + t.branches.length + (t.branches.length === 1 ? ' branch' : ' branches') + (repos2.length ? ' in ' + repos2.join(', ') : '') + (steps.length ? ' · ' + steps.filter(function (x) { return x.state === 'done'; }).length + '/' + steps.length + ' steps' : ''),
        hasActions: acts.length > 0, actions: acts,
        customName: s.names[t.id] || '', defaultTitle: defaultTitleOf(t),
        onName: function (e) { var nm = Object.assign({}, self.state.names); nm[t.id] = e.target.value; self.setState({ names: nm }); },
        statusOpts: ['To do', 'In progress', 'In review', 'Blocked', 'Done'].map(function (o) { var on = o === tStatus, tt = self.tone(STAT[o]);
          return { label: o, on: on, pick: function () { var ts = Object.assign({}, self.state.taskStatus); ts[t.id] = o; self.setState({ taskStatus: ts }); },
            style: 'height: 22px; padding: 0 8px; border-radius: 5px; font-size: 11px; font-weight: 600; cursor: pointer; border: 1px solid ' + (on ? tt[2] : '#2f323b') + '; background: ' + (on ? tt[0] : 'transparent') + '; color: ' + (on ? tt[1] : '#9a9ca5') + ';' }; }),
        jira: jr ? { has: true, none: false, key: jr.key, url: jr.url, title: jr.title || 'title after sync', status: jr.status || 'syncing', statusStyle: self.chip(JT[jr.status] || 'grey') + ' min-width: 84px; text-align: center; box-sizing: border-box;', copyIcon: s.copied === 'jira:' + t.id ? '✓' : '⧉', copy: function () { self.copy(jr.url, 'jira:' + t.id); } }
          : { has: false, none: true, draft: s.jiraDraft, onDraft: function (e) { self.setState({ jiraDraft: e.target.value }); }, save: function () { var jo = Object.assign({}, self.state.jiraOv); jo[t.id] = (self.state.jiraDraft || '').trim(); self.setState({ jiraOv: jo, jiraDraft: '' }); } },
        gh: (function () { var u = s.ghOv[t.id], g = u ? (String(u).match(/github\.com\/([^/]+)\/([^/]+)\/(pull|issues)\/(\d+)/)) : null;
          return g ? { has: true, none: false, ref: (g[3] === 'pull' ? 'PR ' : 'issue ') + g[2] + '#' + g[4], url: u, status: g[3] === 'pull' ? 'PR' : 'issue', statusStyle: self.chip('grey') + ' min-width: 84px; text-align: center; box-sizing: border-box;', copyIcon: s.copied === 'tg:' + t.id ? '✓' : '⧉', copy: function () { self.copy(u, 'tg:' + t.id); } }
            : { has: false, none: true, draft: s.inboxDraft.gh, onDraft: function (e) { self.setState({ inboxDraft: Object.assign({}, self.state.inboxDraft, { gh: e.target.value }) }); }, save: function () { var go = Object.assign({}, self.state.ghOv); go[t.id] = (self.state.inboxDraft.gh || '').trim(); self.setState({ ghOv: go, inboxDraft: Object.assign({}, self.state.inboxDraft, { gh: '' }) }); } }; })(),
        estNum: estNum, onEstNum: function (e) { setEst(e.target.value, estUnit); }, unitH: function () { setEst(estNum, 'h'); }, unitD: function () { setEst(estNum, 'd'); },
        unitHStyle: estUnit === 'h' ? segOn : segOff, unitDStyle: estUnit === 'd' ? segOn : segOff, estHint: estP && estP.days > 1 ? 'spans ' + estP.days + ' days' : '',
        phaseBlocks: (isRev || t.kind === 'parked') ? [] : (function () {
          var w = wfOf(t); if (!w) return [];
          var idx = stageIdxOf(t), pa3 = phaseAction(t);
          var talking = taskSessions(t).some(function (o) { return o.x.state === 'running' && o.x.mode === 'tui'; });
          return w.stages.map(function (stg, i) {
            var st = i < idx ? 'done' : i === idx ? 'current' : 'todo';
            var cc = st === 'done' ? 'green' : st === 'current' ? 'amber' : 'grey';
            var btns = [];
            if (st === 'current' && pa3) btns.push({ label: pa3.label, tip: pa3.tip, run: pa3.run, style: 'height: 22px; padding: 0 9px; flex-shrink: 0; border-radius: 5px; border: none; background: ' + ({ claude: '#d97757', red: '#ef6b5b', amber: '#e8a33d', green: '#6cc58a', blue: '#7aa7ff' }[pa3.tone]) + '; color: #15161a; font-size: 11px; font-weight: 600; cursor: pointer;' });
            if (stg.kind === 'manual' && st === 'current' && talking) btns.unshift({ label: 'Open session', tip: '', run: function () { self.setState({ tab: 'agents' }); }, style: 'height: 22px; padding: 0 9px; flex-shrink: 0; border-radius: 5px; border: 1px solid #3a3e48; background: transparent; color: #e8e6e1; font-size: 11px; cursor: pointer;' });
            var steps = stageSteps(t, stg), done = steps.filter(function (x) { return x.state === 'done'; }).length;
            return { key: stg.id, title: stg.name, mode: stg.kind === 'auto' ? 'agent · background claude -p' : stg.kind === 'script' ? 'script' : stg.tui ? 'user · interactive Claude Code' : 'user',
              state: st === 'done' ? 'done' : st === 'current' ? 'now' : 'next', stateStyle: self.chip(cc) + ' min-width: 40px; text-align: center; box-sizing: border-box;',
              boxStyle: 'display: flex; flex-direction: column; gap: 4px; padding: 8px 10px; border-radius: 8px; border: 1px solid ' + (st === 'current' ? '#5a4420' : '#2a2d35') + '; background: ' + (st === 'current' ? 'rgba(232,163,61,0.05)' : '#17181c') + ';',
              btns: btns, isImpl: stg.kind !== 'manual', count: stg.kind === 'auto' ? done + '/' + steps.length : '',
              steps: steps.map(function (x) {
                var sc = { done: ['✓', '#6cc58a', '#15161a', 'done'], running: ['●', 'transparent', '#e8a33d', 'running'], stuck: ['!', '#e8a33d', '#15161a', 'stuck · needs you'], failed: ['✕', '#ef6b5b', '#15161a', 'failed'], pending: ['', 'transparent', '#5c606b', x.approval ? 'waiting for approval' : 'pending'] }[x.state];
                var head = [];
                if (x.approval) head.push({ label: 'Approve', run: function () { approveStep(t, x); }, style: 'height: 20px; padding: 0 8px; flex-shrink: 0; border-radius: 4px; border: none; background: #e8a33d; color: #15161a; font-size: 11px; font-weight: 600; cursor: pointer;' });
                var fail = String(x.onFail || '');
                var failTxt = /^back:/.test(fail) ? '↩ on failure: back to ' + ((stg.steps || []).filter(function (z) { return z.id === fail.slice(5); })[0] || { name: fail.slice(5) }).name : fail && fail !== 'stop' ? 'on failure: ' + fail : '';
                return { n: x.n + '.', name: x.name, statusText: sc[3], glyph: sc[0], scope: x.isScript ? (x.command || '') : x.scope, gate: x.gate === 'approve' ? 'needs approval' : '', hasGate: x.gate === 'approve', failTxt: failTxt, hasFail: !!failTxt, btns: head,
                  statusStyle: 'font-size: 11px; white-space: nowrap; flex-shrink: 0; color: ' + (x.state === 'stuck' || x.state === 'failed' ? '#f28b7d' : x.state === 'running' ? '#f0b85c' : x.state === 'done' ? '#7fd49b' : '#7c7f88') + ';',
                  iconStyle: 'width: 16px; height: 16px; flex-shrink: 0; border-radius: 4px; box-sizing: border-box; font-size: 10px; font-weight: 800; display: flex; align-items: center; justify-content: center; background: ' + sc[1] + '; color: ' + sc[2] + '; border: ' + (x.state === 'done' || x.state === 'stuck' || x.state === 'failed' ? 'none' : '1.5px solid ' + (x.state === 'running' ? '#e8a33d' : '#5c606b')) + ';',
                  nameStyle: 'font-size: 12px; font-weight: 600; flex-grow: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: ' + (x.state === 'done' ? '#7c7f88' : '#e8e6e1') + ';',
                  scopeStyle: 'font-size: 10.5px; color: #7c7f88; white-space: nowrap; flex-shrink: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis;' + (x.isScript ? ' font-family: \'IBM Plex Mono\', monospace;' : ''),
                  hasRuns: x.runs.some(function (r) { return r.state !== 'pending' || r.note; }) && (x.runs.length > 1 || x.isScript || x.runs.some(function (r) { return r.state !== x.state || r.todo || r.note || r.out; })),
                  runs: x.runs.map(function (r) {
                    var live = r.live[0], stuckO = r.live.filter(function (o) { return o.x.act === 'input'; })[0] || live, a = live ? self.act(live.x.act, 11) : null;
                    var rc = { done: ['✓', '#7fd49b'], running: ['●', '#f0b85c'], back: ['↩', '#f0b85c'], stuck: ['!', '#f28b7d'], failed: ['✕', '#f28b7d'], pending: ['○', '#7c7f88'] }[r.state] || ['○', '#7c7f88'];
                    var bt = [];
                    if (live) bt.push({ label: 'Log', run: function () { var tt = Object.assign({}, self.state.termTab); tt['t:' + t.id] = live.x.id; self.setState({ termTab: tt, tab: 'agents' }); }, style: 'height: 18px; padding: 0 7px; flex-shrink: 0; border-radius: 4px; border: 1px solid #3a3e48; background: transparent; color: #e8e6e1; font-size: 10.5px; cursor: pointer;' });
                    if (stuckO && !x.isScript) bt.push({ label: 'Take over', run: function () { openTui(stuckO); }, style: 'height: 18px; padding: 0 7px; flex-shrink: 0; border-radius: 4px; border: none; background: ' + (r.state === 'stuck' ? '#ef6b5b' : '#2f323b') + '; color: ' + (r.state === 'stuck' ? '#15161a' : '#e8e6e1') + '; font-size: 10.5px; font-weight: 600; cursor: pointer;' });
                    if (r.out) bt.push({ label: s.openOut === t.id + x.id + r.b.id ? 'Hide output' : 'Output', run: function () { self.setState({ openOut: self.state.openOut === t.id + x.id + r.b.id ? null : t.id + x.id + r.b.id }); }, style: 'height: 18px; padding: 0 7px; flex-shrink: 0; border-radius: 4px; border: 1px solid #3a3e48; background: transparent; color: #e8e6e1; font-size: 10.5px; cursor: pointer;' });
                    if (r.state === 'failed') bt.push({ label: 'Retry', run: function () { if (x.isScript) runScript(t, x); else approveStep(t, x); }, style: 'height: 18px; padding: 0 7px; flex-shrink: 0; border-radius: 4px; border: none; background: #d97757; color: #1a0f0a; font-size: 10.5px; font-weight: 600; cursor: pointer;' });
                    var pct = r.state === 'done' ? 100 : r.todo ? Math.round(100 * r.todo[0] / r.todo[1]) : 0;
                    return { glyph: rc[0], glyphStyle: 'width: 14px; text-align: center; font-size: 11px; font-weight: 800; flex-shrink: 0; color: ' + rc[1] + ';', repo: nick(r.b.repo), repoStyle: self.repoChip(r.b.repo),
                      todo: r.todo ? r.todo[0] + '/' + r.todo[1] : r.state === 'done' ? 'done' : r.state === 'back' ? 'sent back' : r.state, todoStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 10.5px; color: ' + rc[1] + '; width: 64px; flex-shrink: 0; white-space: nowrap;',
                      bar: 'display: block; height: 100%; width: ' + pct + '%; background: ' + rc[1] + ';',
                      hasAgent: !!a, agentStyle: a ? a.style : '', agentGlyph: a ? a.glyph : '', btns: bt,
                      note: r.note || (r.loops ? 'fix round ' + r.loops + ' of 3' : ''), loops: r.loops,
                      showOut: !!r.out && s.openOut === t.id + x.id + r.b.id, out: (r.out || []).map(function (l) { return { text: l, style: 'white-space: pre-wrap; color: ' + (/error|exit code [1-9]/i.test(l) ? '#f28b7d' : '#c9c7c2') + ';' }; }) };
                  }) };
              }),
              isRelease: stg.id === 'release' && st !== 'todo',
              rel: stg.id === 'release' ? t.branches.filter(function (bid) { return byB[bid].kind === 'mine' && !byB[bid].draft; }).map(function (bid) {
            var b = byB[bid];
            return { repo: nick(b.repo), repoStyle: self.repoChip(b.repo), name: b.name, into: intoChips(b), main: s.merged[bid] ? 'main ✓' : 'main —', mainStyle: self.chip(s.merged[bid] ? 'purple' : 'grey'), pick: function () { selectBranch(bid); } };
          }) : [] };
          });
        })(),
        wfValue: wfOf(t) ? wfOf(t).id : '', wfOptions: Object.keys(WF).map(function (k) { return { v: k, l: WF[k].name }; }),
        onWf: function (e) { var tw = Object.assign({}, self.state.taskWf); tw[t.id] = e.target.value; var so = Object.assign({}, self.state.stageOv); delete so[t.id]; self.setState({ taskWf: tw, stageOv: so }); },
        editWf: function () { self.setState({ view: 'pipelines', wfSel: wfOf(t) ? wfOf(t).id : 'standard' }); },
        isFlow: !isRev && t.kind !== 'parked',
        canAddBranch: !isRev, addBranch: function () { self.setState({ picker: true, pickerTab: 'branch', pickerTask: t.id, pickerQ: '' }); },
        canAddRepo: !isRev, addRepoOpts: [{ v: '', l: '+ Add repo…' }].concat(repoNames.filter(function (r) { return repos2.indexOf(r) < 0; }).map(function (r) { return { v: r, l: nick(r) + ' · ' + REPOS[r].full }; })),
        onAddRepo: function (e) { var r = e.target.value; if (!r) return; var er = Object.assign({}, self.state.extraRepos); er[t.id] = (er[t.id] || []).concat([r]); self.setState({ extraRepos: er }); },
        branches: t.branches.map(function (bid) { var b = byB[bid], gs = gitStatus(bid);
          return { pick: function () { selectBranch(bid); }, status: gs[0], statusStyle: self.chip(gs[1]) + ' min-width: 84px; text-align: center; box-sizing: border-box;', repo: nick(b.repo), repoStyle: self.repoChip(b.repo), into: intoChips(b),
            name: b.draft ? 'new branch · from main' : b.name, nameStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 12px; flex-grow: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: ' + (b.draft ? '#9a9ca5' : '#e8e6e1') + ';' + (b.draft ? ' font-style: italic;' : '') }; }),

      }, notesUI, tf);
      tabs = [['task', 'Task', -1], ['notes', 'Notes', -1], ['agents', 'Sessions', tf.count]];
    } else if (selB) {
      var b = selB, gs = gitStatus(b.id), tb = selT, root = rootOf(b.id), io = intoOf(b), conf = conflicts[b.id] && conflicts[b.id][0];
      var acts2 = [];
      if (s.unpushed[b.id]) acts2.push({ label: s.pushing.length ? 'Pushing…' : 'Force push', tip: 'git push --force-with-lease', disabled: s.pushing.length > 0, style: btnP('amber'), run: function () { self.forcePush([b.id]); } });
      if (b.kind === 'mine' && !b.draft && !s.merged[b.id]) {
        if (byB[root].kind === 'mine' && behind(root) > 0) acts2.push({ label: busy ? 'Rebasing…' : 'Rebase onto main', tip: '', disabled: busy, style: btnP('amber'), run: function () { rebaseDialog([root], 'main', 'Rebase onto main'); } });
        if (after[b.id]) acts2.push({ label: 'Rebase onto ' + byB[after[b.id].id].name.replace(/^[^/]+\//, ''), tip: '', disabled: busy, style: btnP('amber'), run: function () { rebaseDialog([b.id], after[b.id].id, 'Rebase onto ' + byB[after[b.id].id].name); } });
        if (conf) acts2.push({ label: 'Queue after ' + byB[conf.with].name.replace(/^[^/]+\//, ''), tip: '', disabled: false, style: btnP('red'), run: function () { rebaseDialog([root], conf.with, 'Queue after ' + byB[conf.with].name); } });
        (REPOS[b.repo].targets || []).forEach(function (tg) { if (io[tg] === 'stale') acts2.push({ label: 'Re-merge into ' + tg, tip: '', disabled: false, style: btnP('amber'), run: function () { mergeDialog(b.id, tg); } }); });
        var prS = prepOf(b);
        if (prS && prS.st === 'failed') acts2.unshift({ label: 'Retry setup', tip: 'run the prepare-worktree script again', disabled: false, style: btnP('red'), run: function () { retryPrepare(b.id); } });
        else if (sessionsOf(b).length === 0 && (!prS || prS.st === 'ok')) acts2.push({ label: '▶ Start agent', tip: '', disabled: false, style: startBtn, run: function () { startBranchDialog(b.id); } });
      }
      if (b.draft) acts2.push({ label: 'Created when the pipeline runs', tip: '', disabled: true, style: btnG + ' opacity: 0.6; cursor: default;', run: function () {} });
      var tf2 = termFor(sessionsOf(b).map(function (x) { return { x: x, b: b }; }), 'b:' + b.id, wtOf(b));
      var confFiles = {}; (conflicts[b.id] || []).forEach(function (c) { c.files.forEach(function (f) { confFiles[f] = true; }); });
      var dirtyN = b.dirty.length;
      sel = Object.assign({
        isTask: false, isBranch: true, isReview: b.kind === 'review', reviewNote: b.kind === 'review' ? b.owner + '’s work. Read-only here: you can run agents on it.' : '',
        taskTitle: titleOf(tb), backToTask: function () { selectTask(tb.id); },
        title: b.draft ? 'new branch (' + b.repo + ')' : b.name, headStyle: 'margin: 0; font-family: \'IBM Plex Mono\', monospace; font-size: 13px; font-weight: 600; flex-grow: 1; min-width: 0; word-break: break-all; line-height: 18px;',
        dot: 'width: 12px; height: 12px; border-radius: 3px; flex-shrink: 0; box-sizing: border-box; ' + (b.kind === 'review' ? 'border: 2px solid #7aa7ff;' : 'border: 2px solid ' + colorOf(tb.id) + ';'),
        status: gs[0], chip: self.chip(gs[1]),
        facts: nick(b.repo) + ' · base ' + (parentOf[b.id] ? byB[parentOf[b.id]].name : 'main') + ' · ↑' + b.ahead + ' ↓' + behind(b.id),
        hasActions: acts2.length > 0, actions: acts2,
        links: [
          { label: 'Branch', status: gs[0], statusStyle: self.chip(gs[1]) + ' min-width: 84px; text-align: center; box-sizing: border-box;', hasLink: !b.draft, ref: b.name, url: 'https://github.com/acme/' + b.repo + '/tree/' + b.name, title: '', copyIcon: s.copied === 'b:' + b.id ? '✓' : '⧉', copy: function () { self.copy(b.name, 'b:' + b.id); } },
          { label: 'PR', status: b.pr ? b.pr.state : 'none', statusStyle: self.chip(b.pr ? PRT[b.pr.state] || 'grey' : 'grey') + ' min-width: 84px; text-align: center; box-sizing: border-box;', hasLink: !!b.pr, ref: b.pr ? '#' + b.pr.num : '', url: b.pr ? b.pr.url : '#', title: b.pr ? b.pr.title : '', copyIcon: s.copied === 'pr:' + b.id ? '✓' : '⧉', copy: function () { if (b.pr) self.copy(b.pr.url, 'pr:' + b.id); } }
        ],
        into: (REPOS[b.repo].targets || []).map(function (tg) {
          var st = io[tg], tone = st === 'merged' ? 'green' : st === 'stale' ? 'amber' : 'grey';
          var btn = b.kind === 'mine' && !b.draft ? (st === 'stale' ? 'Re-merge' : !st ? 'Merge' : '') : '';
          return { target: tg, status: st === 'merged' ? 'merged' : st === 'stale' ? 'stale' : 'not merged', statusStyle: self.chip(tone) + ' min-width: 84px; text-align: center; box-sizing: border-box;',
            detail: st === 'merged' ? 'up to date' : st === 'stale' ? ((b.intoNote && b.intoNote[tg]) || 'has changes not in ' + tg) : '',
            hasBtn: !!btn, btn: btn, run: function () { mergeDialog(b.id, tg); },
            btnStyle: 'height: 20px; padding: 0 8px; flex-shrink: 0; border-radius: 4px; border: none; background: ' + (st === 'stale' ? '#e8a33d' : '#2f323b') + '; color: ' + (st === 'stale' ? '#15161a' : '#e8e6e1') + '; font-size: 11px; font-weight: 600; cursor: pointer;' };
        }),
        noTargets: !(REPOS[b.repo].targets || []).length,
        setup: (function () { var pr = prepOf(b) || { st: 'none', took: '', log: [] }; var tone = pr.st === 'ok' ? 'green' : pr.st === 'running' ? 'blue' : pr.st === 'failed' ? 'red' : 'grey';
          return { status: pr.st === 'ok' ? 'ready' : pr.st === 'running' ? 'preparing' : pr.st === 'failed' ? 'failed' : 'not created', statusStyle: self.chip(tone) + ' min-width: 84px; text-align: center; box-sizing: border-box;',
            detail: (pr.took ? pr.took + ' · ' : '') + 'prepare-worktree script of ' + b.repo, canRetry: pr.st === 'failed', retry: function () { retryPrepare(b.id); },
            hasLog: (pr.log || []).length > 0 && pr.st !== 'ok', log: (pr.log || []).map(function (l) { return { text: l, style: 'white-space: pre-wrap; color: ' + (/ERR|exit code [1-9]|error/i.test(l) ? '#f28b7d' : '#c9c7c2') + ';' }; }) }; })(),
        deploys: (REPOS[b.repo].envs || []).map(function (e) {
          var x = deployOf(b)[e.name], st = x ? x.st : 'none';
          return { env: e.name, status: st === 'deployed' ? 'deployed' : st === 'stale' ? 'stale' : 'not deployed', statusStyle: self.chip(st === 'deployed' ? 'green' : st === 'stale' ? 'amber' : 'grey') + ' min-width: 84px; text-align: center; box-sizing: border-box;',
            detail: x ? (st === 'deployed' ? 'runs ' + x.sha + ', contains this branch' : (x.note || 'runs ' + x.sha)) : '' };
        }),
        noEnvs: !(REPOS[b.repo].envs || []).length,
        base: parentOf[b.id] ? byB[parentOf[b.id]].name : 'main', ahead: b.ahead, behind: behind(b.id),
        wt: b.draft ? 'created on Start' : (b.kind === 'review' ? 'read-only · ' : '') + wtOf(b) + (dirtyN ? ' · ' + dirtyN + ' uncommitted' : ''),
        wtStyle: 'font-family: \'IBM Plex Mono\', monospace; font-size: 11px; color: ' + (dirtyN ? '#f0b85c' : '#c9c7c2') + ';',
        hasConflict: !!conf, conflict: conf ? byB[conf.with].name + ' · ' + conf.files.map(function (f) { return f.split('/').pop(); }).join(', ') : '',
        hasShared: !!after[b.id], shared: after[b.id] ? byB[after[b.id].id].name + ' · ' + after[b.id].file : '',
        hasDirty: dirtyN > 0, dirty: b.dirty.map(function (x) { return { code: x[0], path: x[1], style: 'width: 16px; color: ' + (x[0] === 'M' ? '#f0b85c' : '#7fd49b') + ';' }; }),
        commits: b.commits.map(function (c) { return { sha: c[0], msg: c[1] }; }),
        files: b.files.map(function (f) { var hot = confFiles[f[0]]; return { path: f[0], delta: f[1], style: 'display: flex; justify-content: space-between; gap: 8px; font-family: \'IBM Plex Mono\', monospace; font-size: 11px; padding: 2px 6px; border-radius: 4px; color: ' + (hot ? '#f28b7d' : '#c9c7c2') + '; background: ' + (hot ? '#2a1917' : 'transparent') + ';' }; })
      }, tf2);
      tabs = [['details', 'Details', -1], ['changes', 'Changes', -1], ['agents', 'Sessions', tf2.count]];
    }
    var curTab = s.tab;
    if (!tabs.some(function (x) { return x[0] === curTab; })) curTab = tabs.length ? tabs[0][0] : 'task';
    var tabList = tabs.map(function (tt) {
      var on = curTab === tt[0];
      return { label: tt[1], hasCount: tt[2] >= 0, count: tt[2],
        countStyle: 'margin-left: 6px; font-size: 11px; padding: 0 6px; border-radius: 999px; background: ' + (tt[2] ? 'rgba(217,119,87,0.18)' : '#23252b') + '; color: ' + (tt[2] ? '#e8a07f' : '#9a9ca5') + ';',
        pick: function () { self.setState({ tab: tt[0] }); },
        style: 'height: 34px; padding: 0 10px; border: none; border-bottom: 2px solid ' + (on ? '#e8a33d' : 'transparent') + '; background: transparent; color: ' + (on ? '#e8e6e1' : '#9a9ca5') + '; font-size: 12px; font-weight: ' + (on ? 600 : 500) + '; cursor: pointer; display: flex; align-items: center;' };
    });
    // ---------- Claude dialog: one editable message built only from repos, branches, worktrees, Jira and what the user typed
    var dlg = { open: false, blocked: false, busyShown: false, busy: [], title: '', targets: [], askWt: false, wtOptions: [], sendLabel: '', message: '', edited: false, hasBranchNames: false, branchNames: [], canPush: false, pushOn: false, isArchive: false, riskText: '' };
    if (s.dialog) {
      var D = s.dialog, lines = [];
      var pushLine = s.dialogPush ? 'Then push each rebased branch with: git push --force-with-lease' : 'Do not push.';
      var ask = 'If anything is unclear, ask me before changing anything.';
      if (D.kind === 'rebase' || D.kind === 'queue') {
        var ontoB = D.onto === 'main' ? null : byB[D.onto];
        var ontoRef = !ontoB ? 'origin/main' : ontoB.kind === 'review' ? 'origin/' + ontoB.name : ontoB.name;
        D.roots.forEach(function (rid, i) {
          var b = byB[rid], chain = [];
          (function down(x) { (kids[x] || []).forEach(function (c) { if (byB[c].kind === 'mine' && !byB[c].draft) { chain.push([c, x]); down(c); } }); })(rid);
          if (i) lines.push('');
          lines.push('Rebase ' + b.name + ' (repo ' + b.repo + ') onto ' + (ontoB ? ontoB.name : 'main') + (chain.length ? ', then restack the branches built on it:' : '.'));
          lines.push('1. In ' + wtOf(b) + ': git fetch origin && git rebase ' + ontoRef);
          chain.forEach(function (cp, ci) { lines.push((ci + 2) + '. In ' + wtOf(byB[cp[0]]) + ': git rebase ' + byB[cp[1]].name); });
        });
        if (ontoB && ontoB.kind === 'review') lines.push('Do not modify ' + ontoB.name + '.');
        lines.push('Resolve any conflicts. ' + pushLine); lines.push(ask);
      } else if (D.kind === 'merge') {
        var mb = byB[D.branch], twt = '~/kira-space-wts/' + mb.repo + '/_' + D.target;
        lines.push('Merge ' + mb.name + ' into ' + D.target + ' (repo ' + mb.repo + ').');
        lines.push('1. git fetch origin');
        lines.push('2. Use the ' + D.target + ' worktree at ' + twt + ' (create it if missing: git worktree add ' + twt + ' origin/' + D.target + ' -B ' + D.target + ')');
        lines.push('3. In ' + twt + ': git merge ' + mb.name);
        lines.push('Resolve any conflicts. ' + (s.dialogPush ? 'Then push: git push origin ' + D.target : 'Do not push.')); lines.push(ask);
      } else if (D.kind === 'archive') {
        var at = tById[D.task];
        lines.push('Task "' + titleOf(at) + '" is being archived and its worktrees will be deleted.');
        D.risk.forEach(function (r) { var b = byB[r.id]; lines.push('- ' + b.repo + ' · ' + b.name + ' · ' + wtOf(b) + (r.dirty.length ? ' · uncommitted: ' + r.dirty.join(', ') : '') + (r.unmerged ? ' · commits not merged into main: ' + r.unmerged : '')); });
        lines.push('Before they are deleted: ');
      } else if (D.kind === 'stage') {
        var sp = tById[D.task], spj = jiraOf(sp), spw = wfOf(sp), sgO = spw && spw.stages.filter(function (z) { return z.id === D.stage; })[0];
        var spRepos = []; sp.branches.forEach(function (bid) { if (spRepos.indexOf(byB[bid].repo) < 0) spRepos.push(byB[bid].repo); });
        var realB = sp.branches.filter(function (bid) { return !byB[bid].draft && byB[bid].kind === 'mine'; });
        lines.push((sgO ? sgO.name : 'Stage') + ': ' + titleOf(sp));
        if (spj) lines.push('- Jira: ' + spj.key + ' ' + spj.url);
        if (realB.length) realB.forEach(function (bid) { var b = byB[bid]; lines.push('- Repo: ' + b.repo + ' · Branch: ' + b.name + ' · Worktree: ' + wtOf(b)); });
        else spRepos.forEach(function (r) { lines.push('- Repo: ' + r + ' (read only, in ~/code/' + r + ')'); });
        if (sp.notes) lines.push('- Notes: ' + sp.notes);
        if (sgO && sgO.prompt) lines.push(sgO.prompt.replace(/\{task\}/g, titleOf(sp)).replace(/\{jira\}/g, spj ? spj.key : 'no Jira ticket').replace(/\{repo\}/g, spRepos.join(', ')).replace(/\{branch\}/g, realB.map(function (bid) { return byB[bid].name; }).join(', ') || 'the new branches').replace(/\{worktree\}/g, realB.map(function (bid) { return wtOf(byB[bid]); }).join(', ') || '~/code'));
      } else if (D.kind === 'run') {
        var rt = tById[D.task], rP = curStage(rt), rSt = rP && rP.steps.filter(function (z) { return z.id === D.step; })[0], rj = jiraOf(rt);
        lines.push('Task: ' + titleOf(rt));
        if (rj) lines.push('- Jira: ' + rj.key + ' ' + rj.url);
        if (D.create.length) {
          lines.push('- Create a branch from origin/main in each repo, each in its own new worktree:');
          D.create.forEach(function (c) { var nm = (s.dialogBranches[c.id] || '').trim(); lines.push('  - ' + c.repo + ': ' + (nm ? 'branch ' + nm + ', worktree ~/kira-space-wts/' + c.repo + '/' + nm.split('/').pop() : 'pick a short descriptive branch name; worktree under ~/kira-space-wts/' + c.repo + '/')); });
        }
        (D.bids || []).filter(function (bid) { return !byB[bid].draft; }).forEach(function (bid) { var b = byB[bid]; lines.push('- Repo: ' + b.repo + ' · Branch: ' + b.name + ' · Worktree: ' + wtOf(b)); });
        if (rSt) {
          lines.push('Step ' + (rP.steps.indexOf(rSt) + 1) + '/' + rP.steps.length + ': ' + rSt.name);
          lines.push(rSt.prompt.replace(/\{jira\}/g, rj ? rj.key : 'the task').replace(/\{repo\}/g, 'each repo').replace(/\{branch\}/g, 'its branch').replace(/\{worktree\}/g, 'its worktree').replace(/\{task\}/g, titleOf(rt)));
          lines.push(''); lines.push(FINISH);
        }
      } else if (D.kind === 'start' && D.resume) {
        var rb = byB[D.resumeB];
        lines.push('Resume session ' + D.resume + '.'); lines.push('- Repo: ' + rb.repo + ' · Branch: ' + rb.name); lines.push('- Worktree: ' + (D.askWt && D.wt === 'new' ? 'create a new worktree for it' : wtOf(rb)));
      } else if (D.kind === 'start') {
        var st0 = tById[D.task], sj = jiraOf(st0), stepO = D.stepObj || (D.step ? stepsOf(st0).filter(function (x) { return x.id === D.step; })[0] : null);
        lines.push('Task: ' + titleOf(st0));
        if (sj) lines.push('- Jira: ' + sj.key + ' ' + sj.url);
        if (st0.notes && D.create.length) lines.push('- Notes: ' + st0.notes);
        if (D.create.length) {
          lines.push('- Create a branch from origin/main in each repo, each in its own new worktree:');
          D.create.forEach(function (c) { var nm = (s.dialogBranches[c.id] || '').trim(); lines.push('  - ' + c.repo + ': ' + (nm ? 'branch ' + nm + ', worktree ~/kira-space-wts/' + c.repo + '/' + nm.split('/').pop() : 'pick a short descriptive branch name; worktree under ~/kira-space-wts/' + c.repo + '/')); });
        }
        var others = (D.sessionOn || []).filter(function (bid) { return !byB[bid].draft; });
        others.forEach(function (bid) { var b = byB[bid]; lines.push('- Repo: ' + b.repo + ' · Branch: ' + b.name + ' · Worktree: ' + wtOf(b)); });
        if (stepO) lines.push('Step: ' + stepO.name);
      }
      var busyList = [];
      if (D.kind === 'rebase' || D.kind === 'queue') (D.ids || D.roots).forEach(function (id) { running(byB[id]).forEach(function (x) { if (x.act === 'working' || x.act === 'waiting') busyList.push({ b: byB[id], x: x }); }); });
      var chipStyle = function (on) { return 'height: 26px; padding: 0 10px; border-radius: 6px; border: 1px solid ' + (on ? '#d97757' : '#3a3e48') + '; background: ' + (on ? 'rgba(217,119,87,0.16)' : 'transparent') + '; color: ' + (on ? '#e8a07f' : '#c9c7c2') + '; font-family: \'IBM Plex Mono\', monospace; font-size: 11px; cursor: pointer;'; };
      var ovOn = busyList.length > 0 && s.dialogOverride;
      dlg = {
        open: true, title: D.title,
        busyShown: busyList.length > 0, blocked: busyList.length > 0 && !s.dialogOverride,
        busyTitle: s.dialogOverride ? 'Override on: this will be sent even though these agents are busy.' : 'Agents are busy on branches this would rewrite. Wait until they\'re idle or need input.',
        overrideLabel: s.dialogOverride ? 'Undo override' : 'Override…', toggleOverride: function () { self.setState({ dialogOverride: !self.state.dialogOverride }); },
        busy: busyList.map(function (o) { var a = self.act(o.x.act, 12); return { text: o.b.repo + ' · ' + o.b.name + ' · claude ' + o.x.id + ' · ' + a.label, iconStyle: a.style, glyph: a.glyph }; }),
        message: s.dialogMsg !== null ? s.dialogMsg : lines.join('\n'), edited: s.dialogMsg !== null,
        onMessage: function (e) { self.setState({ dialogMsg: e.target.value }); }, resetMessage: function () { self.setState({ dialogMsg: null }); },
        hasBranchNames: !!(D.create && D.create.length),
        branchNames: (D.create || []).map(function (c) { return { repo: nick(c.repo), repoStyle: self.repoChip(c.repo), inputId: 'bn-' + c.repo, value: s.dialogBranches[c.id] || '', onChange: function (e) { var o = Object.assign({}, self.state.dialogBranches); o[c.id] = e.target.value; self.setState({ dialogBranches: o }); } }; }),
        canPush: D.kind === 'rebase' || D.kind === 'queue' || D.kind === 'merge', pushOn: !!s.dialogPush,
        pushLabel: D.kind === 'merge' ? 'Also push ' + D.target : 'Also force-push after rebasing',
        togglePush: function () { self.setState({ dialogPush: !self.state.dialogPush }); },
        pushTrack: 'width: 30px; height: 18px; border-radius: 999px; position: relative; flex-shrink: 0; background: ' + (s.dialogPush ? '#d97757' : '#3a3e48') + ';',
        pushKnob: 'position: absolute; top: 2px; left: ' + (s.dialogPush ? 14 : 2) + 'px; width: 14px; height: 14px; border-radius: 50%; background: #e8e6e1;',
        isArchive: D.kind === 'archive',
        riskText: D.kind === 'archive' ? D.risk.map(function (r) { return byB[r.id].repo + ': ' + [r.dirty.length ? r.dirty.length + ' uncommitted' : '', r.unmerged ? r.unmerged + ' unmerged commits' : ''].filter(Boolean).join(', '); }).join(' · ') : '',
        justDelete: function () { var id = D.task; self.setState({ dialog: null }); self.archiveTask(id); },
        sendLabel: ovOn ? 'Send anyway' : D.kind === 'run' ? 'Run in background' : D.kind === 'stage' ? 'Open session' : D.kind === 'start' ? 'Start' : D.kind === 'archive' ? 'Send to Claude, then archive' : 'Send to Claude',
        sendStyle: 'height: 30px; padding: 0 12px; border-radius: 6px; border: none; font-size: 12px; font-weight: 600; ' + (busyList.length && !s.dialogOverride ? 'background: #2a2c33; color: #7c7f88; cursor: not-allowed;' : ovOn ? 'background: #ef6b5b; color: #1a0f0a; cursor: pointer;' : 'background: #d97757; color: #1a0f0a; cursor: pointer;'),
        send: function () { if (busyList.length && !self.state.dialogOverride) return; self.sendDialog(); },
        cancel: function () { self.setState({ dialog: null }); },
        targets: (D.kind === 'start' || D.kind === 'run' || D.kind === 'stage' ? [] : (D.targets || [])).map(function (tg, ti) {
          var b = byB[tg.branch], rs = running(b);
          var opts = rs.length ? rs.map(function (x) { return { id: x.id, label: 'claude ' + x.id }; }) : [{ id: 'new', label: 'new session' }];
          return { repo: nick(b.repo), repoStyle: self.repoChip(b.repo), branch: b.name,
            options: opts.map(function (o) { return { label: o.label + (rs.length === 1 ? ' (only agent)' : ''), style: chipStyle(tg.choice === o.id), pick: function () { var nd = Object.assign({}, self.state.dialog); nd.targets = nd.targets.map(function (x, xi) { return xi === ti ? Object.assign({}, x, { choice: o.id }) : x; }); self.setState({ dialog: nd }); } }; }) };
        }),
        askWt: !!D.askWt,
        wtOptions: [['same', 'same worktree'], ['new', 'new worktree']].filter(function (w) { return !(D.noSame && w[0] === 'same'); }).map(function (w) { return { label: w[1], style: chipStyle(D.wt === w[0]), pick: function () { self.setState({ dialog: Object.assign({}, self.state.dialog, { wt: w[0] }) }); } }; })
      };
    }

    // ---------- All agents: grouped by task
    var allSess = []; allTasks.forEach(function (t) { taskSessions(t).forEach(function (o) { allSess.push(o.x); }); });
    var activeN = allSess.filter(function (x) { return x.state === 'running'; }).length, wantActive = s.allFilter === 'active';
    var groups = allTasks.map(function (t) {
      var rows = [];
      taskSessions(t).forEach(function (o) {
        var b = o.b, x = o.x, bid = b ? b.id : null;
        var isRun = x.state === 'running'; if (isRun !== wantActive) return;
        var a = self.act(isRun ? x.act : 'stopped', 14), hl = x.mode === 'headless';
        rows.push({ _rank: a.rank, sid: x.id, repo: b ? b.repo : 'spec', repoStyle: b ? self.repoChip(b.repo) : self.chip('grey'),
          step: (hl ? 'claude -p · ' + stepNameOf(o) : 'TUI · ' + (b ? 'interactive' : 'spec session')), branch: b ? b.name : '', wt: b ? wtOf(b) : '~/code',
          last: x.last, iconStyle: a.style, glyph: a.glyph, state: isArchived(t.id) ? 'stopped · archived' : (hl && x.act === 'input' ? 'stuck · needs you' : a.label),
          stateStyle: 'font-size: 12px; color: ' + (isRun && x.act === 'input' ? '#f0b85c' : isRun && x.act === 'working' ? '#7fd49b' : isRun && x.act === 'waiting' ? '#93b6ff' : '#9a9ca5') + ';',
          rowStyle: 'display: grid; grid-template-columns: 20px 130px 76px 92px 90px 64px minmax(0, 1fr); column-gap: 12px; align-items: center; padding: 6px 12px; border-radius: 8px; background: ' + (isRun && x.act === 'input' ? 'rgba(232,163,61,0.07)' : 'transparent') + ';',
          btn: isRun ? (hl ? 'Take over' : 'Open') : 'Take over',
          btnStyle: isRun && !hl ? 'height: 26px; border-radius: 6px; border: 1px solid #3a3e48; background: transparent; color: #e8e6e1; font-size: 12px; cursor: pointer;' : 'height: 26px; border-radius: 6px; border: none; background: #d97757; color: #1a0f0a; font-size: 12px; font-weight: 600; cursor: pointer;',
          run: isRun ? (hl ? function () { self.setState({ view: 'plan' }); openTui(o); } : function () { var tt = Object.assign({}, s.termTab); if (b) { tt['b:' + bid] = x.id; self.setState({ view: 'plan', sel: { kind: 'branch', id: bid }, tab: 'agents', termTab: tt }); } else { tt['t:' + t.id] = x.id; self.setState({ view: 'plan', sel: { kind: 'task', id: t.id }, tab: 'agents', termTab: tt }); } })
            : function () { if (b) self.openDialog({ kind: 'start', task: t.id, resume: x.id, resumeB: bid, sessionOn: [], create: [], askWt: true, wt: isArchived(t.id) ? 'new' : 'same', noSame: isArchived(t.id), title: 'Resume in Claude Code (interactive)' }); } });
      });
      rows.sort(function (p, q2) { return p._rank - q2._rank; });
      return { title: titleOf(t), swatch: 'width: 10px; height: 10px; border-radius: 3px; flex-shrink: 0; background: ' + colorOf(t.id) + ';', rows: rows, has: rows.length > 0, rank: rows.length ? rows[0]._rank : 9 };
    }).filter(function (g) { return g.has; }).sort(function (a, b) { return a.rank - b.rank; });
    var segS = function (on) { return 'height: 28px; padding: 0 12px; border-radius: 6px; border: none; background: ' + (on ? '#2a2c33' : 'transparent') + '; color: ' + (on ? '#e8e6e1' : '#9a9ca5') + '; font-size: 12px; font-weight: ' + (on ? 600 : 500) + '; cursor: pointer;'; };
    var allView = { groups: groups, empty: groups.length === 0, activeLabel: 'Active ' + activeN, olderLabel: 'Older ' + (allSess.length - activeN),
      activeStyle: segS(wantActive), olderStyle: segS(!wantActive), showActive: function () { self.setState({ allFilter: 'active' }); }, showOlder: function () { self.setState({ allFilter: 'older' }); },
      acts: wantActive ? self.actSummary(allSess) : [] };

    // ---------- Needs you: one list across all tasks, oldest first
    var needItems = [];
    var ageRank = function (t) { var m = String(t || '').match(/(\d+)\s*(m|h|d)/); if (!m) return t === 'yesterday' ? 1440 : 0; return +m[1] * (m[2] === 'm' ? 1 : m[2] === 'h' ? 60 : 1440); };
    tasks.forEach(function (t) {
      var col = colorOf(t.id);
      var base = function (o) { return Object.assign({ task: titleOf(t), swatch: 'display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 4px; background: ' + col + ';' }, o); };
      taskSessions(t).forEach(function (o) {
        if (o.x.state !== 'running' || o.x.act !== 'input') return;
        var hl = o.x.mode === 'headless';
        needItems.push(base({ _age: ageRank(o.x.last), age: o.x.last, kind: hl ? 'stuck run' : 'question', tone: hl ? 'red' : 'amber',
          what: hl ? 'Step "' + stepNameOf(o) + '" is stuck' + (o.b ? ' on ' + o.b.name : '') : 'Claude Code is waiting for your answer' + (o.b ? ' on ' + o.b.name : ' (spec)'),
          repo: o.b ? nick(o.b.repo) : 'spec', btn: hl ? 'Take over' : 'Open', run: function () { self.setState({ view: 'plan' }); if (hl) openTui(o); else if (o.b) openSession(o.b.id, o.x.id); else { var tt = Object.assign({}, self.state.termTab); tt['t:' + t.id] = o.x.id; self.setState({ sel: { kind: 'task', id: t.id }, tab: 'agents', termTab: tt }); } } }));
      });
      stepsOf(t).forEach(function (st) {
        if (st.approval) needItems.push(base({ _age: 5, age: 'now', kind: 'approval', tone: 'amber', what: 'Approve step "' + st.name + '"', repo: st.scope, btn: 'Approve', run: function () { approveStep(t, st); } }));
        if (st.state === 'failed') needItems.push(base({ _age: 30, age: '', kind: 'failed', tone: 'red', what: 'Step "' + st.name + '" failed', repo: st.scope, btn: 'Retry', run: function () { approveStep(t, st); } }));
      });
      t.branches.forEach(function (bid) {
        var b = byB[bid], io = intoOf(b), pr = prepOf(b);
        if (pr && pr.st === 'failed') needItems.push(base({ _age: 60, age: pr.took ? 'took ' + pr.took : '', kind: 'setup failed', tone: 'red', what: 'Worktree setup failed for ' + b.name, repo: nick(b.repo), btn: 'See error', run: function () { self.setState({ view: 'plan', sel: { kind: 'branch', id: bid }, tab: 'details' }); } }));
        Object.keys(io).forEach(function (tg) { if (io[tg] === 'stale') needItems.push(base({ _age: 2, age: '', kind: 'stale merge', tone: 'grey', what: b.name + ' is stale in ' + tg, repo: nick(b.repo), btn: 'Re-merge', run: function () { mergeDialog(bid, tg); } })); });
      });
    });
    needItems.sort(function (a, b) { var pr = { red: 0, amber: 1, grey: 2 }; return (pr[a.tone] - pr[b.tone]) || (b._age - a._age); });
    var runningAll = []; allTasks.forEach(function (t) { taskSessions(t).forEach(function (o) { if (o.x.state === 'running') runningAll.push(o.x); }); });
    var bgRun = runningAll.filter(function (x) { return x.mode === 'headless'; });
    var needs = {
      items: needItems.map(function (n2) { var tt = self.tone(n2.tone); return Object.assign(n2, {
        kindStyle: 'font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 5px; text-align: center; white-space: nowrap; background: ' + tt[0] + '; color: ' + tt[1] + ';',
        repoStyle: n2.repoId ? self.repoChip(n2.repoId) : /^(web-app|api|mobile)$/.test(n2.repo) ? self.repoChip(n2.repo) : 'font-size: 10.5px; color: #7c7f88; white-space: nowrap;',
        btnStyle: 'height: 26px; border-radius: 6px; border: none; font-size: 12px; font-weight: 600; cursor: pointer; background: ' + (n2.btn === 'Take over' ? '#d97757' : n2.tone === 'grey' ? '#2f323b' : tt[2]) + '; color: ' + (n2.tone === 'grey' ? '#e8e6e1' : '#15161a') + ';',
        rowStyle: 'display: grid; grid-template-columns: 92px 60px 92px 70px minmax(0, 1fr); column-gap: 12px; align-items: center; padding: 8px 12px; border-radius: 8px; border: 1px solid #2a2d35; background: ' + (n2.tone === 'red' ? 'rgba(239,107,91,0.06)' : n2.tone === 'amber' ? 'rgba(232,163,61,0.05)' : '#17181c') + ';' }); }),
      empty: needItems.length === 0,
      summary: needItems.length ? needItems.length + ' items · oldest first, most urgent on top' : '',
      load: bgRun.length + ' background runs · ' + bgRun.filter(function (x) { return x.act === 'waiting'; }).length + ' waiting on CI · ' + runningAll.filter(function (x) { return x.mode === 'tui'; }).length + ' interactive sessions',
      showAll: s.showAllSessions, toggleLabel: s.showAllSessions ? 'Hide all sessions' : 'All sessions',
      toggleAll: function () { self.setState({ showAllSessions: !self.state.showAllSessions }); }
    };
    // ---------- Inbox: quick capture, prioritise later, turn into tasks
    function capture(text) { var v = (text || '').trim(); if (!v) return false; self.setState({ inbox: [{ id: 'i' + Math.random().toString(16).slice(2, 7), text: v, added: 'just now' }].concat(self.state.inbox), capture: '', captured: Date.now() }); return true; }
    function ghParse(u) { var m = String(u || '').match(/github\.com\/([^/]+)\/([^/]+)\/(pull|issues)\/(\d+)/); return m ? { ref: (m[3] === 'pull' ? 'PR ' : 'issue ') + m[2] + '#' + m[4], url: u.trim(), kind: m[3] } : null; }
    function patchItem(id, patch) { self.setState({ inbox: self.state.inbox.map(function (y) { return y.id === id ? Object.assign({}, y, patch) : y; }) }); }
    function itemToTask(it) {
      var id = 'T_' + Math.random().toString(16).slice(2, 7), k = jiraKey(it.jira || it.text);
      var nt = { id: id, title: it.text, jira: k ? { key: k, title: '', status: '', url: /^https?:/.test(it.jira || '') ? it.jira : 'https://acme.atlassian.net/browse/' + k } : null, status: 'To do', est: '', draftRepos: [], notes: '', workflow: 'standard', stage: 'spec', run: {} };
      var nn = Object.assign({}, self.state.notes); if (nn[it.id]) { nn[id] = nn[it.id]; delete nn[it.id]; }
      var gh = Object.assign({}, self.state.ghOv); if (it.gh) gh[id] = it.gh;
      var rest = self.state.inbox.filter(function (y) { return y.id !== it.id; });
      self.setState({ newTasks: self.state.newTasks.concat([nt]), inbox: rest, notes: nn, ghOv: gh, inboxSel: rest[0] ? rest[0].id : null, view: 'plan', sel: { kind: 'task', id: id }, tab: 'task' });
    }
    var inboxPanel = { has: false };
    if (inboxSelItem) {
      var IT = inboxSelItem, ijk = jiraKey(IT.jira), igh = ghParse(IT.gh);
      inboxPanel = Object.assign({
        has: true, title: IT.text, added: 'captured ' + IT.added,
        onTitle: function (e) { patchItem(IT.id, { text: e.target.value }); },
        toTask: function () { itemToTask(IT); }, remove: function () { var rest = self.state.inbox.filter(function (y) { return y.id !== IT.id; }); self.setState({ inbox: rest, inboxSel: rest[0] ? rest[0].id : null }); },
        jira: ijk ? { has: true, none: false, key: ijk, url: /^https?:/.test(IT.jira) ? IT.jira : 'https://acme.atlassian.net/browse/' + ijk, status: 'syncing', statusStyle: self.chip('grey') + ' min-width: 84px; text-align: center; box-sizing: border-box;', title: 'title after sync', copyIcon: s.copied === 'ij:' + IT.id ? '✓' : '⧉', copy: function () { self.copy(/^https?:/.test(IT.jira) ? IT.jira : 'https://acme.atlassian.net/browse/' + ijk, 'ij:' + IT.id); }, clear: function () { patchItem(IT.id, { jira: '' }); } }
          : { has: false, none: true, draft: s.inboxDraft.jira, onDraft: function (e) { self.setState({ inboxDraft: Object.assign({}, self.state.inboxDraft, { jira: e.target.value }) }); }, save: function () { patchItem(IT.id, { jira: (self.state.inboxDraft.jira || '').trim() }); self.setState({ inboxDraft: Object.assign({}, self.state.inboxDraft, { jira: '' }) }); } },
        gh: igh ? { has: true, none: false, ref: igh.ref, url: igh.url, status: igh.kind === 'pull' ? 'PR' : 'issue', statusStyle: self.chip('grey') + ' min-width: 84px; text-align: center; box-sizing: border-box;', copyIcon: s.copied === 'ig:' + IT.id ? '✓' : '⧉', copy: function () { self.copy(igh.url, 'ig:' + IT.id); }, clear: function () { patchItem(IT.id, { gh: '' }); } }
          : { has: false, none: true, draft: s.inboxDraft.gh, onDraft: function (e) { self.setState({ inboxDraft: Object.assign({}, self.state.inboxDraft, { gh: e.target.value }) }); }, save: function () { patchItem(IT.id, { gh: (self.state.inboxDraft.gh || '').trim() }); self.setState({ inboxDraft: Object.assign({}, self.state.inboxDraft, { gh: '' }) }); } }
      }, notesUI);
    }
    var inboxView = {
      empty: s.inbox.length === 0,
      items: s.inbox.map(function (it, i) {
        function mv(d) { return function () { var arr = self.state.inbox.slice(), j = i + d; if (j < 0 || j >= arr.length) return; var x = arr[i]; arr[i] = arr[j]; arr[j] = x; self.setState({ inbox: arr }); }; }
        var on = inboxSelItem && inboxSelItem.id === it.id, ik = jiraKey(it.jira), ig = ghParse(it.gh);
        return { text: it.text, added: it.added, inputId: 'ib-' + it.id, up: mv(-1), down: mv(1),
          pick: function () { self.setState({ inboxSel: it.id }); },
          links: [ik ? ik : '', ig ? ig.ref : '', self.state.notes[it.id] ? 'notes' : ''].filter(Boolean).join(' · '),
          rowStyle: 'display: flex; align-items: center; gap: 8px; min-height: 40px; padding: 4px 8px; border-radius: 8px; border: 1px solid ' + (on ? '#e8a33d' : '#2a2d35') + '; background: ' + (on ? '#202127' : '#17181c') + '; cursor: pointer;',
          onText: function (e) { var v = e.target.value; self.setState({ inbox: self.state.inbox.map(function (y) { return y.id === it.id ? Object.assign({}, y, { text: v }) : y; }) }); },
          remove: function () { self.setState({ inbox: self.state.inbox.filter(function (y) { return y.id !== it.id; }) }); },
          toTask: function (e) { if (e && e.stopPropagation) e.stopPropagation(); itemToTask(it); },
          _old: function () {
            var id = 'T_' + Math.random().toString(16).slice(2, 7), k = jiraKey(it.text);
            var nt = { id: id, title: it.text, jira: k ? { key: k, title: '', status: '', url: 'https://acme.atlassian.net/browse/' + k } : null, status: 'To do', est: '', draftRepos: [], notes: '', workflow: 'standard', stage: 'spec', run: {} };
            self.setState({ newTasks: self.state.newTasks.concat([nt]), inbox: self.state.inbox.filter(function (y) { return y.id !== it.id; }), view: 'plan', sel: { kind: 'task', id: id }, tab: 'task' });
          } };
      })
    };

    // ---------- nav + repo chips + refresh
    var navDefs = [['inbox', 'Backlog', s.inbox.length, 'grey'], ['needs', 'Needs you', needItems.length, 'amber'], ['plan', 'Plan', 0, ''], ['pipelines', 'Workflows', 0, ''], ['repos', 'Repos', 0, '']];
    var navTabs = navDefs.map(function (n) {
      var on = s.view === n[0], ct = self.tone(n[3]);
      return { label: n[1], hasCount: n[2] > 0, count: n[2], countStyle: 'min-width: 18px; height: 18px; padding: 0 5px; box-sizing: border-box; border-radius: 9px; display: inline-flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 800; background: ' + (n[3] === 'amber' ? '#e8a33d' : '#2f323b') + '; color: ' + (n[3] === 'amber' ? '#15161a' : '#c9c7c2') + ';',
        pick: function () { self.setState({ view: n[0], picker: false, ctx: null }); },
        style: 'height: 40px; padding: 0 16px; border: none; border-right: 1px solid ' + '#1e2026' + '; background: ' + (on ? '#121316' : 'transparent') + '; color: ' + (on ? '#e8e6e1' : '#9a9ca5') + '; font-size: 12px; font-weight: ' + (on ? 600 : 500) + '; display: flex; align-items: center; gap: 7px; cursor: pointer; box-shadow: ' + (on ? 'inset 0 2px 0 #e8a33d' : 'none') + ';' };
    });
    var fetchOf = function (r) { return s.fetch[r] || { busy: false, last: 'never', note: '' }; };
    var anyBusy = repoNames.some(function (r) { return fetchOf(r).busy; });
    // the plan only shows repos its tasks actually touch
    var planRepos = repoNames.filter(function (r) { return tasks.some(function (t) { return t.branches.some(function (bid) { return byB[bid].repo === r; }); }); });
    var repoChips = planRepos.map(function (r) {
      var F = fetchOf(r), on = s.repoOn[r] !== false;
      return { name: nick(r), on: on, busy: F.busy, note: F.busy ? 'fetching…' : (F.note ? F.last + ' · ' + F.note : F.last), tip: (F.busy ? 'fetching' : 'fetched ' + F.last) + (F.note ? ' · ' + F.note : '') + ' · autofetch off',
        toggle: function () { var ro = Object.assign({}, self.state.repoOn); ro[r] = ro[r] === false; self.setState({ repoOn: ro }); },
        refresh: function () { if (!self.state.fetch[r].busy) self.refresh([r]); }, fullName: REPOS[r].full,
        style: 'display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 2px 0 4px; border-radius: 7px; border: 1px solid #2f323b; background: ' + (on ? '#1b1d22' : 'transparent') + '; opacity: ' + (on ? 1 : 0.5) + '; max-width: 320px;',
        nameStyle: self.repoChip(r) + ' height: 20px; cursor: pointer; background: transparent;',
        noteStyle: 'font-size: 11px; color: ' + (F.note && F.note !== 'no changes' ? '#c9c7c2' : '#7c7f88') + '; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0;' };
    });
    var hist = HIST;
    // ---------- Repos: per-repo config (prepare script, integration branches, environments)
    var curRepo = s.repoSel && REPOS[s.repoSel] ? s.repoSel : repoNames[0], RC = REPOS[curRepo];
    function setCfg(patch) { var rc = Object.assign({}, self.state.repoCfg); rc[curRepo] = Object.assign({}, rc[curRepo], patch); self.setState({ repoCfg: rc }); }
    var IMPORTS = { '~/code/oss': [{ id: 'oss-cli', full: 'acme-open-source-cli', nick: 'oss-cli' }, { id: 'oss-site', full: 'acme-open-source-website', nick: 'oss-site' }] };
    function baseName(pth) { return String(pth).replace(/\/+$/, '').split('/').pop(); }
    var reposView = {
      folders: s.repoFolders.map(function (f, i) {
        var n = repoNames.filter(function (r) { return REPOS[r].source === f.path; }).length;
        return { path: f.path, meta: n + ' repos', watch: f.watch,
          toggleWatch: function () { self.setState({ repoFolders: self.state.repoFolders.map(function (x, j) { return j === i ? Object.assign({}, x, { watch: !x.watch }) : x; }) }); },
          watchTrack: 'width: 26px; height: 16px; border-radius: 999px; position: relative; flex-shrink: 0; background: ' + (f.watch ? '#6cc58a' : '#3a3e48') + ';',
          watchKnob: 'position: absolute; top: 2px; left: ' + (f.watch ? 12 : 2) + 'px; width: 12px; height: 12px; border-radius: 50%; background: #e8e6e1;',
          remove: function () { self.setState({ repoFolders: self.state.repoFolders.filter(function (x, j) { return j !== i; }) }); } };
      }),
      newFolder: s.newFolderPath, onNewFolder: function (e) { self.setState({ newFolderPath: e.target.value }); },
      addFolder: function () {
        var pth = (self.state.newFolderPath || '').trim(); if (!pth) return;
        var found = (IMPORTS[pth] || [{ id: baseName(pth) + '-app', full: baseName(pth) + '-app', nick: baseName(pth) + '-app' }]).map(function (r) { return Object.assign({ path: pth + '/' + r.full, source: pth }, r); });
        self.setState({ repoFolders: self.state.repoFolders.concat([{ path: pth, watch: true }]), extraRepos2: self.state.extraRepos2.concat(found.filter(function (r) { return !REPOS[r.id]; })), newFolderPath: '' });
      },
      newRepo: s.newRepoPath, onNewRepo: function (e) { self.setState({ newRepoPath: e.target.value }); },
      addRepo: function () {
        var pth = (self.state.newRepoPath || '').trim(); if (!pth) return; var nm = baseName(pth);
        if (REPOS[nm]) { self.setState({ repoSel: nm, newRepoPath: '' }); return; }
        self.setState({ extraRepos2: self.state.extraRepos2.concat([{ id: nm, full: nm, nick: nm, path: pth, source: 'added' }]), repoSel: nm, newRepoPath: '' });
      },
      nick: RC.nick || curRepo, full: RC.full || curRepo, path: RC.path || '', source: RC.source === 'added' ? 'added individually' : 'imported from ' + RC.source,
      onNick: function (e) { setCfg({ nick: e.target.value }); },
      inPlan: tasks.some(function (t) { return t.branches.some(function (bid) { return byB[bid].repo === curRepo; }); }) ? 'used by tasks on the plan' : 'not on the plan',
      list: repoNames.map(function (r) { var on = r === curRepo; return { name: nick(r), full: REPOS[r].full, nameStyle: self.repoChip(r), meta: (REPOS[r].envs || []).length + ' envs · ' + (REPOS[r].targets || []).length + ' integration branches', pick: function () { self.setState({ repoSel: r }); },
        style: 'width: 100%; box-sizing: border-box; padding: 8px 10px; border: none; border-left: 3px solid ' + (on ? '#e8a33d' : 'transparent') + '; border-radius: 6px; background: ' + (on ? '#23242a' : 'transparent') + '; color: #e8e6e1; text-align: left; cursor: pointer; display: flex; flex-direction: column; gap: 4px; align-items: flex-start;' }; }),
      name: nick(curRepo), nameStyle: self.repoChip(curRepo) + ' font-size: 13px; padding: 2px 8px;',
      prepare: RC.prepare || '', onPrepare: function (e) { setCfg({ prepare: e.target.value }); },
      prepareTimeout: RC.prepareTimeout || '', onPrepTimeout: function (e) { setCfg({ prepareTimeout: e.target.value }); },
      targets: (RC.targets || []).join(', '), onTargets: function (e) { setCfg({ targets: e.target.value.split(',').map(function (x) { return x.trim(); }).filter(Boolean) }); },
      envs: (RC.envs || []).map(function (en, i) {
        function set(k) { return function (e) { var arr = (REPOS[curRepo].envs || []).map(function (z) { return Object.assign({}, z); }); arr[i][k] = e.target.value; setCfg({ envs: arr }); }; }
        return { name: en.name, script: en.script, onName: set('name'), onScript: set('script'), idName: 'env-name-' + i, idScript: 'env-script-' + i,
          remove: function () { var arr = (REPOS[curRepo].envs || []).slice(); arr.splice(i, 1); setCfg({ envs: arr }); } };
      }),
      addEnv: function () { setCfg({ envs: (REPOS[curRepo].envs || []).concat([{ name: 'new-env', script: '' }]) }); }
    };
    // ---------- finish tool: appended to every background step so the app knows when it ends
    // ---------- workflows are YAML files (portable): serialise / parse a small, strict subset
    function yq(v) { v = String(v == null ? '' : v); return (/^[\w./{}() -]+$/.test(v) && !/^[-\s]|\s$|: /.test(v) && !/^(true|false|null|yes|no|\d+)$/i.test(v)) ? v : JSON.stringify(v); }
    function yblock(key, text, pad) { var lines = String(text || '').split('\n'); return pad + key + ': |\n' + lines.map(function (l) { return pad + '  ' + l; }).join('\n') + '\n'; }
    function toYaml(w) {
      var o = 'id: ' + yq(w.id) + '\nname: ' + yq(w.name) + '\nstages:\n';
      w.stages.forEach(function (sg) {
        o += '  - id: ' + yq(sg.id) + '\n    name: ' + yq(sg.name) + '\n    kind: ' + (sg.kind === 'auto' ? 'agent' : sg.kind === 'manual' ? 'user' : sg.kind) + '\n    status: ' + yq(sg.status || 'In progress') + '\n';
        if (sg.kind === 'manual') { o += '    session: ' + (sg.tui ? 'true' : 'false') + '\n'; if (sg.tui && sg.prompt) o += yblock('prompt', sg.prompt, '    '); }
        if (sg.kind === 'script') { o += '    runs_on: ' + yq(sg.scope || 'each repo') + '\n    on_failure: ' + yq(sg.onFail || 'stop') + '\n    timeout: ' + yq(sg.timeout || '10m') + '\n' + yblock('command', sg.command, '    '); }
        if (sg.kind === 'auto') {
          o += '    steps:\n';
          (sg.steps || []).forEach(function (st) {
            o += '      - id: ' + yq(st.id) + '\n        name: ' + yq(st.name) + '\n        runs_on: ' + yq(st.scope) + '\n        before: ' + (st.gate === 'approve' ? 'approval' : 'auto') + '\n        on_failure: ' + yq(st.onFail) + '\n        timeout: ' + yq(st.timeout) + '\n' + yblock('prompt', st.prompt, '        ');
          });
        }
      });
      return o;
    }
    function fromYaml(text) {
      var L = String(text).replace(/\t/g, '  ').split('\n').map(function (raw, k) { return { raw: raw, n: k + 1 }; }), i = 0;
      function ind(o) { return o.raw.match(/^ */)[0].length; }
      function blank(o) { return !o.raw.trim() || /^\s*#/.test(o.raw); }
      function skip() { while (i < L.length && blank(L[i])) i++; }
      function fail(o, msg) { var e = new Error((o ? 'line ' + o.n + ': ' : '') + msg); e.yaml = true; throw e; }
      function scalar(v) { v = v.trim(); if (/^".*"$/.test(v)) { try { return JSON.parse(v); } catch (e) { fail(L[i - 1], 'bad quoted string'); } } if (/^'.*'$/.test(v)) return v.slice(1, -1).replace(/''/g, "'"); if (v === 'true') return true; if (v === 'false') return false; return v; }
      function block(base) { var out = [], bi = null; while (i < L.length && (blank(L[i]) || ind(L[i]) > base)) { if (!blank(L[i]) && bi === null) bi = ind(L[i]); out.push(blank(L[i]) ? '' : L[i].raw.slice(bi)); i++; } while (out.length && !out[out.length - 1]) out.pop(); return out.join('\n'); }
      function node(d) { skip(); if (i >= L.length) return null; return L[i].raw.trim().indexOf('- ') === 0 ? list(ind(L[i])) : map(ind(L[i])); }
      function map(d) {
        var m = {};
        while (true) {
          skip(); if (i >= L.length) break;
          var o = L[i], di = ind(o), tx = o.raw.trim();
          if (di < d || tx.indexOf('- ') === 0) break;
          if (di > d) fail(o, 'unexpected indentation');
          var mm = tx.match(/^([A-Za-z_][\w-]*):(?:\s+(.*))?$/); if (!mm) fail(o, 'expected "key: value"');
          i++;
          if (mm[2] === undefined || mm[2] === '') { skip(); m[mm[1]] = (i < L.length && ind(L[i]) > d) ? node(ind(L[i])) : null; }
          else if (mm[2] === '|' || mm[2] === '|-') m[mm[1]] = block(d);
          else m[mm[1]] = scalar(mm[2]);
        }
        return m;
      }
      function list(d) {
        var arr = [];
        while (true) {
          skip(); if (i >= L.length) break;
          var o = L[i]; if (ind(o) !== d || o.raw.trim().indexOf('-') !== 0) break;
          var rest = o.raw.trim().slice(1).trim();
          if (!/^[A-Za-z_][\w-]*:/.test(rest)) { arr.push(scalar(rest)); i++; continue; }
          L[i] = { raw: o.raw.replace('-', ' '), n: o.n };
          arr.push(map(d + 2));
        }
        return arr;
      }
      var y = node(0);
      skip(); if (i < L.length) fail(L[i], 'unexpected content here (check the indentation)');
      if (!y || typeof y !== 'object' || Array.isArray(y)) fail(null, 'expected a workflow (id, name, stages)');
      if (!y.id) fail(L[0], 'missing "id"'); if (!y.name) fail(L[0], 'missing "name"');
      if (!Array.isArray(y.stages) || !y.stages.length) fail(null, '"stages" must be a non-empty list');
      return { id: String(y.id), name: String(y.name), stages: y.stages.map(function (sg, k) {
        var kind = (sg.kind === 'agent' || sg.kind === 'automated') ? 'auto' : (sg.kind === 'user' || sg.kind === 'manual') ? 'manual' : sg.kind;
        if (['manual', 'auto', 'script'].indexOf(kind) < 0) fail(null, 'stage ' + (k + 1) + ': kind must be user, agent or script');
        var out = { id: String(sg.id || 's' + (k + 1)), name: String(sg.name || 'Stage ' + (k + 1)), kind: kind, status: sg.status || 'In progress' };
        if (kind === 'manual') { out.tui = sg.session === true; out.prompt = sg.prompt || ''; }
        if (kind === 'script') { out.command = sg.command || ''; out.scope = sg.runs_on || 'each repo'; out.onFail = sg.on_failure || 'stop'; out.timeout = sg.timeout || '10m'; }
        if (kind === 'auto') {
          if (!Array.isArray(sg.steps)) fail(null, 'stage "' + out.name + '": agent stages need a "steps" list');
          out.steps = sg.steps.map(function (st, j) { return { id: String(st.id || 'x' + (j + 1)), name: String(st.name || 'Step ' + (j + 1)), scope: st.runs_on || 'each repo', gate: st.before === 'approval' ? 'approve' : 'auto', onFail: st.on_failure || 'stop', timeout: st.timeout || '1h', prompt: st.prompt || '' }; });
        }
        return out;
      }) };
    }

    // ---------- Workflows: ordered stages (manual / automated); automated stages hold background steps
    var wfIds = Object.keys(WF), curWf = WF[s.wfSel] || WF[wfIds[0]];
    function editWf(fn) { var cp = JSON.parse(JSON.stringify(curWf)); fn(cp); var wo = Object.assign({}, self.state.wfOv); wo[cp.id] = cp; var ya = Object.assign({}, self.state.wfYaml); delete ya[cp.id]; self.setState({ wfOv: wo, wfYaml: ya }); }
    var selS = 'height: 26px; box-sizing: border-box; padding: 0 6px; border-radius: 5px; border: 1px solid #3a3e48; background: #121316; color: #e8e6e1; font-size: 12px;';
    function rid() { return Math.random().toString(16).slice(2, 6); }
    var curYaml = curWf ? (s.wfYaml[curWf.id] !== undefined ? s.wfYaml[curWf.id] : toYaml(curWf)) : '';
    var wfView = {
      isForm: s.wfMode !== 'yaml', isYaml: s.wfMode === 'yaml',
      formTab: function () { self.setState({ wfMode: 'form' }); }, yamlTab: function () { self.setState({ wfMode: 'yaml' }); },
      formTabStyle: 'height: 24px; padding: 0 10px; border-radius: 5px; border: none; font-size: 12px; cursor: pointer; background: ' + (s.wfMode !== 'yaml' ? '#2f323b' : 'transparent') + '; color: ' + (s.wfMode !== 'yaml' ? '#e8e6e1' : '#9a9ca5') + ';',
      yamlTabStyle: 'height: 24px; padding: 0 10px; border-radius: 5px; border: none; font-size: 12px; cursor: pointer; background: ' + (s.wfMode === 'yaml' ? '#2f323b' : 'transparent') + '; color: ' + (s.wfMode === 'yaml' ? '#e8e6e1' : '#9a9ca5') + ';',
      file: curWf ? '~/.config/agent-planner/workflows/' + curWf.id + '.yaml' : '',
      yaml: curYaml, yamlErr: curWf && s.wfYamlErr[curWf.id] ? s.wfYamlErr[curWf.id] : '', yamlOk: !(curWf && s.wfYamlErr[curWf.id]),
      onYaml: function (e) {
        var txt = e.target.value, id = curWf.id, ya = Object.assign({}, self.state.wfYaml), er = Object.assign({}, self.state.wfYamlErr); ya[id] = txt;
        try { var w2 = fromYaml(txt); w2.id = id; var wo = Object.assign({}, self.state.wfOv); wo[id] = w2; delete er[id]; self.setState({ wfYaml: ya, wfYamlErr: er, wfOv: wo }); }
        catch (err) { er[id] = err.message; self.setState({ wfYaml: ya, wfYamlErr: er }); }
      },
      copyYaml: function () { self.copy(curYaml, 'yaml:' + (curWf ? curWf.id : '')); }, copyLabel: s.copied === 'yaml:' + (curWf ? curWf.id : '') ? 'Copied ✓' : 'Copy YAML',
      importYaml: function () { var id = 'w' + rid(); var tmpl = 'id: ' + id + '\nname: Imported workflow\nstages:\n  - id: spec\n    name: Spec\n    kind: manual\n    status: In progress\n    session: true\n    prompt: |\n      Paste your workflow YAML over this.\n'; var wo = Object.assign({}, self.state.wfOv); wo[id] = fromYaml(tmpl); var ya = Object.assign({}, self.state.wfYaml); ya[id] = tmpl; self.setState({ wfOv: wo, wfYaml: ya, wfSel: id, wfMode: 'yaml' }); },
      finishNote: FINISH,
      list: wfIds.map(function (k) {
        var W = WF[k], on = curWf && k === curWf.id, used = tasks.filter(function (t) { return wfOf(t) && wfOf(t).id === k; }).length;
        return { name: W.name, meta: W.stages.map(function (x) { return x.name; }).join(' › '), used: 'used by ' + used, pick: function () { self.setState({ wfSel: k }); },
          style: 'width: 100%; box-sizing: border-box; padding: 8px 10px; border: none; border-left: 3px solid ' + (on ? '#e8a33d' : 'transparent') + '; border-radius: 6px; background: ' + (on ? '#23242a' : 'transparent') + '; color: #e8e6e1; text-align: left; cursor: pointer; display: flex; flex-direction: column; gap: 2px;' };
      }),
      addWf: function () { var id = 'w' + rid(); var wo = Object.assign({}, self.state.wfOv); wo[id] = { id: id, name: 'New workflow', stages: [{ id: 's' + rid(), name: 'Spec', kind: 'manual', tui: true, prompt: '' }, { id: 's' + rid(), name: 'Implement', kind: 'auto', steps: [{ id: 'x' + rid(), name: 'Implement', scope: 'each repo', prompt: 'Implement the task on {branch} in {worktree}.', onFail: 'stop', gate: 'auto', timeout: '1h' }] }, { id: 's' + rid(), name: 'Release', kind: 'manual', tui: false, prompt: '' }] }; self.setState({ wfOv: wo, wfSel: id }); },
      name: curWf ? curWf.name : '', onName: function (e) { var v = e.target.value; editWf(function (cp) { cp.name = v; }); },
      stages: curWf ? curWf.stages.map(function (sg, i) {
        function setS(k, val) { return function (e) { var v = val !== undefined ? val : e.target.value; editWf(function (cp) { cp.stages[i][k] = v; if (k === 'kind' && v === 'auto' && !cp.stages[i].steps) cp.stages[i].steps = []; if (k === 'kind' && v === 'script') { cp.stages[i].command = cp.stages[i].command || ''; cp.stages[i].scope = cp.stages[i].scope || 'each repo'; cp.stages[i].onFail = cp.stages[i].onFail || 'stop'; cp.stages[i].timeout = cp.stages[i].timeout || '10m'; } }); }; }
        return { n: (i + 1) + '.', name: sg.name, kind: sg.kind, isAuto: sg.kind === 'auto', isManual: sg.kind === 'manual', isScript: sg.kind === 'script', tui: !!sg.tui, prompt: sg.prompt || '',
          status: sg.status || 'In progress', onStatus: setS('status'), idStatus: 'sg-status-' + i,
          command: sg.command || '', onCommand: setS('command'), idCommand: 'sg-cmd-' + i, sScope: sg.scope || 'each repo', onSScope: setS('scope'), idSScope: 'sg-scope-' + i,
          sFail: sg.onFail || 'stop', onSFail: setS('onFail'), idSFail: 'sg-fail-' + i, sTimeout: sg.timeout || '10m', onSTimeout: setS('timeout'), idSTime: 'sg-time-' + i,
          idName: 'sg-name-' + i, idKind: 'sg-kind-' + i, idPrompt: 'sg-prompt-' + i,
          onName: setS('name'), onKind: setS('kind'), onPrompt: setS('prompt'), toggleTui: function () { editWf(function (cp) { cp.stages[i].tui = !cp.stages[i].tui; }); },
          tuiTrack: 'width: 30px; height: 18px; border-radius: 999px; position: relative; flex-shrink: 0; background: ' + (sg.tui ? '#d97757' : '#3a3e48') + ';',
          tuiKnob: 'position: absolute; top: 2px; left: ' + (sg.tui ? 14 : 2) + 'px; width: 14px; height: 14px; border-radius: 50%; background: #e8e6e1;',
          kindBadge: sg.kind === 'auto' ? 'agent' : sg.kind === 'script' ? 'script' : 'user', kindBadgeStyle: self.chip(sg.kind === 'auto' ? 'amber' : sg.kind === 'script' ? 'green' : 'blue'),
          up: function () { if (i > 0) editWf(function (cp) { var x = cp.stages[i]; cp.stages[i] = cp.stages[i - 1]; cp.stages[i - 1] = x; }); },
          down: function () { editWf(function (cp) { if (i < cp.stages.length - 1) { var x = cp.stages[i]; cp.stages[i] = cp.stages[i + 1]; cp.stages[i + 1] = x; } }); },
          remove: function () { editWf(function (cp) { cp.stages.splice(i, 1); }); },
          addStep: function () { editWf(function (cp) { cp.stages[i].steps = (cp.stages[i].steps || []).concat([{ id: 'x' + rid(), name: 'New step', scope: 'each repo', prompt: '', onFail: 'stop', gate: 'auto', timeout: '1h' }]); }); },
          selStyle: selS,
          steps: (sg.steps || []).map(function (st, j) {
            function set(k) { return function (e) { var v = e.target.value; editWf(function (cp) { cp.stages[i].steps[j][k] = v; }); }; }
            return { n: (i + 1) + '.' + (j + 1), name: st.name, prompt: st.prompt, scope: st.scope, onFail: st.onFail, gate: st.gate, timeout: st.timeout,
              onNameS: set('name'), onPrompt: set('prompt'), onScope: set('scope'), onFailS: set('onFail'), onGate: set('gate'), onTimeout: set('timeout'),
              idName: 'ps-name-' + i + '-' + j, idPrompt: 'ps-prompt-' + i + '-' + j, idScope: 'ps-scope-' + i + '-' + j, idFail: 'ps-fail-' + i + '-' + j, idGate: 'ps-gate-' + i + '-' + j, idTime: 'ps-time-' + i + '-' + j,
              up: function () { if (j > 0) editWf(function (cp) { var a3 = cp.stages[i].steps; var x = a3[j]; a3[j] = a3[j - 1]; a3[j - 1] = x; }); },
              down: function () { editWf(function (cp) { var a3 = cp.stages[i].steps; if (j < a3.length - 1) { var x = a3[j]; a3[j] = a3[j + 1]; a3[j + 1] = x; } }); },
              remove: function () { editWf(function (cp) { cp.stages[i].steps.splice(j, 1); }); }, selStyle: selS,
              failOpts: [{ v: 'stop', l: 'stop' }, { v: 'retry 1', l: 'retry 1' }, { v: 'retry 2', l: 'retry 2' }].concat((sg.steps || []).slice(0, j).map(function (z) { return { v: 'back:' + z.id, l: '↩ send back to ' + z.name }; })) };
          })
        };
      }) : [],
      addStage: function () { editWf(function (cp) { cp.stages.push({ id: 's' + rid(), name: 'New stage', kind: 'manual', tui: false, prompt: '' }); }); },
      kindOpts: [{ v: 'manual', l: 'user' }, { v: 'auto', l: 'agent (background)' }, { v: 'script', l: 'script' }],
      statusOpts: ['To do', 'In progress', 'In review', 'Done'], scriptFailOpts: ['stop', 'retry 1', 'retry 2'],
      scopeOpts: ['once', 'each repo'].concat(repoNames.map(function (r) { return 'only ' + r; })),
      failOpts: ['stop', 'retry 1', 'retry 2'], gateOpts: [{ v: 'auto', l: 'start automatically' }, { v: 'approve', l: 'wait for my approval' }]
    };
    var pullPct = Math.min(100, Math.round(s.pull / 4));

    return {
      inboxPanel: inboxPanel, navTabs: navTabs.slice(0, 3), navRight: navTabs.slice(3), isNeeds: s.view === 'needs' || s.view === 'agents', isInbox: s.view === 'inbox', isPlan: s.view === 'plan', needs: needs, inboxView: inboxView,
      captureValue: s.capture, onCapture: function (e) { self.setState({ capture: e.target.value }); },
      onCaptureKey: function (e) { if (e.key === 'Enter') { e.preventDefault(); capture(self.state.capture); } },
      captureNote: s.captured && Date.now() - s.captured < 2500 ? 'added to backlog' : '', captureNoteStyle: 'font-size: 11px; color: #7fd49b; margin: 0 8px; white-space: nowrap; width: 84px;', isPipes: s.view === 'pipelines', isRepos: s.view === 'repos', reposView: reposView, wfView: wfView, allView: allView,
      repoChips: repoChips, refreshAll: function () { if (!anyBusy) self.refresh(planRepos); }, refreshBusy: anyBusy, refreshLabel: anyBusy ? 'Refreshing…' : 'Refresh all',
      refreshStyle: 'height: 28px; padding: 0 14px; flex-shrink: 0; border-radius: 7px; border: none; background: ' + (anyBusy ? '#2a2c33' : '#e8e6e1') + '; color: ' + (anyBusy ? '#9a9ca5' : '#15161a') + '; font-size: 12px; font-weight: 600; cursor: pointer; display: flex; align-items: center; gap: 7px;',
      picker: picker, togglePicker: function () { self.setState({ picker: !s.picker, pickerQ: '', pickerTask: null, pickerTab: 'new' }); },
      bands: bands, dayCtl: dayCtl, ctx: ctx, confirm: confirm, dialog: dlg,
      sel: sel, tabs: tabList, tab: { notes: curTab === 'notes', task: curTab === 'task', details: curTab === 'details', changes: curTab === 'changes', agents: curTab === 'agents' },
      panelStyle: 'width: ' + s.panelW + 'px; flex-shrink: 0; display: flex; flex-direction: column; background: #16171b; min-height: 0;',
      startResize: function (e) { e.preventDefault(); var x0 = e.clientX, w0 = self.state.panelW; function mv(ev) { self.setState({ panelW: Math.max(340, Math.min(1080, w0 - (ev.clientX - x0))) }); } function up() { document.removeEventListener('mousemove', mv); document.removeEventListener('mouseup', up); } document.addEventListener('mousemove', mv); document.addEventListener('mouseup', up); },
      dragEnd: function () { if (self.state.dragOver !== null) self.setState({ dragOver: null }); },
      rootClick: function () { if (self.state.ctx) self.setState({ ctx: null }); },
      inHistory: s.inHistory && s.showHistory, showPull: !s.showHistory,
      pullText: 'Load history · ' + hist.length + ' archived tasks in the last 2 weeks',
      hasMore: hiddenCount > 0, moreText: 'Load all items · ' + hiddenCount + ' more', loadAll: function () { self.setState({ showAllItems: true }); },
      canCollapse: s.showAllItems && visSeq.length > LIMIT, collapse: function () { self.setState({ showAllItems: false }); setTimeout(function () { self.scrollTop(); }, 30); },
      pullBar: 'position: absolute; left: 0; top: 0; bottom: 0; width: ' + pullPct + '%; background: rgba(163,113,247,0.18); transition: width 0.1s;',
      openHistory: function () { self.revealHistory(); },
      hideHistory: function () { self.setState({ showHistory: false, inHistory: false }); setTimeout(function () { self.scrollTop(); }, 30); },
      backToNow: function () { var first = seq.filter(function (t) { return t.day !== LATER; })[0]; if (first && first.day < 0) self.scrollToDay(first.day); else self.scrollToDay(0); },
      onMainWheel: function (e) {
        return;
        var main = document.getElementById('aq-main');
        if (!main || main.scrollTop > 0 || e.deltaY >= 0) { if (self.state.pull) self.setState({ pull: 0 }); return; }
        var p = self.state.pull + Math.min(120, -e.deltaY); clearTimeout(self.pullTimer);
        if (p >= 400) { self.revealHistory(); return; }
        self.setState({ pull: p }); self.pullTimer = setTimeout(function () { self.setState({ pull: 0 }); }, 700);
      },
      onMainScroll: function (e) {
        var today = document.getElementById('aq-day-0'); if (!today) return;
        var header = e.target.firstElementChild ? e.target.firstElementChild.offsetHeight : 48;
        var h2 = e.target.scrollTop + header < today.offsetTop - 30;
        if (h2 !== self.state.inHistory) self.setState({ inHistory: h2 });
      },
      goToDate: function (e) {
        var v = e.target.value; if (!v) return; var p = v.split('-'); var k = offsetOf(new Date(+p[0], +p[1] - 1, +p[2]));
        if (k < -self.state.history) self.setState({ history: -k + 2 });
        if (k >= self.state.horizon && self.state.extraDays.indexOf(k) < 0) self.setState({ extraDays: self.state.extraDays.concat([k]) });
        setTimeout(function () { self.scrollToDay(k); }, 40);
      }
    };
  }
}

