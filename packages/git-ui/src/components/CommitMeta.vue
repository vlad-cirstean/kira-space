<script setup lang="ts">
/**
 * Subject + files is `DetailPane.vue`'s whole job — this component renders
 * the subject/facts/actions header and, behind one "Show more" toggle, everything else about the
 * commit (body, author/committer identities, trailers, refs, signature, change request).
 *
 * Every DOM-touching bit (message linkification, ref badges) is built through `linkify.ts`'s
 * `appendLinkifiedText`/`refBadges.ts`'s `buildRefBadges` rather than a template `v-html` — a
 * commit message is untrusted text and this repo's `enableHtmlRendering: false` discipline
 * (`columns.ts`'s own doc comment) applies here just as much as it does inside the grid.
 *
 * `DetailPane.vue` used to mount this component twice — once for the message
 * (with the body/author/trailers behind "Show more"), once more, physically below the file tree,
 * for Refs/Signature/change request. That second, separately-scrolling instance is gone: this file
 * now mounts once and folds Refs/Signature/PR into the same "Show more" region as the body and
 * identities, so there is exactly one expandable region, not two disconnected ones. The collapsed
 * view is now title + a date/SHA facts row + "Open all changes" — no body preview at all (it used
 * to show a 2-line clamp before the toggle); "Show more" reveals the rest. The old copy-message
 * icon button is gone too — SHA/message copy already exist via the row context menu, and the new
 * facts row's SHA button covers the one copy path this pane itself offers.
 */
import { shortSha } from '@kira/git-core';
import type { ChangeRequestLookup, ChangeRequestRecord, GitHostInfo } from '@kira/git-ipc';
import CodiconIcon from '@theme/CodiconIcon.vue';
import AttributeTooltip from '@theme/components/AttributeTooltip.vue';
import KeyValueList from '@theme/components/KeyValueList.vue';
import TextTooltip from '@theme/components/TextTooltip.vue';
import TooltipDetails from '@theme/components/TooltipDetails.vue';
import TooltipIconButton from '@theme/components/TooltipIconButton.vue';
import TruncatedText from '@theme/components/TruncatedText.vue';
import { Badge } from '@theme/components/ui/badge';
import { Button } from '@theme/components/ui/button';
import { FieldDescription } from '@theme/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@theme/components/ui/tooltip';
import { hostWords, type MessageKey, t } from '@theme/i18n';
import { changeRequestTone, TONE_TEXT } from '@theme/lib/statusTone';
import { cn } from '@theme/lib/utils';
import { formatAbsoluteUtc, formatAgeCompact } from '@workbench/behaviour/format';
import { errorText } from '@workbench/bridge/codedError';
import { createOpenExternal } from '@workbench/bridge/openExternal';
import Identifier from '@workbench/components/Identifier.vue';
import ViewToolbar from '@workbench/components/ViewToolbar.vue';
import { computed, nextTick, ref, watch } from 'vue';
import { changeRequestBadgeClass } from '../lib/badgeClass.ts';
import { changeRequestStateLabel } from '../lib/changeRequestStateLabel.ts';
import { appendLinkifiedText } from '../lib/linkify.ts';
import { openAllChangesAnnounced } from '../lib/openAllChangesAnnounced.ts';
import { buildRefBadges } from '../lib/refBadges.ts';
import type { CommitDetail } from '../state/detail.ts';
import type { DetailActions } from '../state/detailActions.ts';

const props = defineProps<{
  detail: CommitDetail | undefined;
  actions: DetailActions;
  /** G24 D12: the currently selected commit's own PR lookup — `ChangeRequestState.selected`, threaded in by
   *  `DetailPane.vue`. `undefined` covers both "nothing selected yet" and the in-flight/debounce
   *  window, same as `detail` itself before it resolves. */
  changeRequestResult?: ChangeRequestLookup;
  /** Any PR this commit is on (`ChangeRequestState.changeRequestForCommit`, threaded by
   *  `DetailPane.vue`) — drives the facts-row icon, distinct from `changeRequestResult` (this exact
   *  commit's own change request row, only ever populated for the selected commit). `undefined`
   *  renders no icon. */
  changeRequestForCommit?: readonly ChangeRequestRecord[];
  /** The repo's code host (`ChangeRequestState.host`); its words name change requests here. */
  host?: GitHostInfo | null;
}>();

const words = computed(() => hostWords(props.host));

const bodyEl = ref<HTMLParagraphElement | null>(null);
const decorationEl = ref<HTMLSpanElement | null>(null);

/** The one "Show more" state for the whole pane now — gates the body, the
 *  identities, the trailers, and Refs/Signature/PR together. Reset to collapsed on every new
 *  commit — an expansion the user made for one commit is not a standing preference carried into
 *  the next. */
const expanded = ref(false);

/** Body lines rendered one `<p>` per blank-line-separated paragraph — `appendLinkifiedText`
 *  handles a paragraph's own line breaks by joining with `\n` inside one paragraph rather than
 *  trying to linkify a single giant string with embedded `<br>`s. */
const bodyParagraphs = computed<string[]>(() => {
  const body = props.detail?.body ?? '';
  if (body.trim() === '') return [];
  return body.split(/\n{2,}/).map((p) => p.trim());
});

function renderBody(): void {
  const container = bodyEl.value;
  if (!container) return;
  container.replaceChildren();
  for (const [index, paragraph] of bodyParagraphs.value.entries()) {
    if (index > 0) container.appendChild(document.createElement('br'));
    if (index > 0) container.appendChild(document.createElement('br'));
    const lines = paragraph.split('\n');
    lines.forEach((line, lineIndex) => {
      if (lineIndex > 0) container.appendChild(document.createElement('br'));
      appendLinkifiedText(container, line, openLink);
    });
  }
}

function renderDecoration(): void {
  const container = decorationEl.value;
  if (!container) return;
  container.replaceChildren();
  const badges = buildRefBadges(props.detail?.decoration ?? []);
  if (badges) container.appendChild(badges);
}

/** `bodyEl` stays mounted (see the template) whether or not the pane is expanded — only its own
 *  visibility toggles — precisely so this watch, and the render it drives, never has to re-run
 *  just because the user clicked "Show more"; it only reacts to an actual commit/body change. */
watch(
  [() => props.detail, bodyEl],
  () => {
    expanded.value = false;
    void nextTick(renderBody);
  },
  { immediate: true },
);
/** `decorationEl`, unlike `bodyEl`, only exists in the DOM while `expanded` — nothing here sets
 *  `expanded` itself, so mounting/unmounting it as the toggle flips is safe and simply re-renders
 *  the badges into place the next time it appears. */
watch([() => props.detail?.decoration, decorationEl], () => void nextTick(renderDecoration), {
  immediate: true,
});

/** "author and committer with both timestamps when they differ" — read as: show one
 *  identity row when author and committer are the very same identity at the very same moment
 *  (the overwhelmingly common case), and both, each with its own timestamp, the moment any part
 *  differs (a rebase, a cherry-pick, an `--author` override) — never silently collapse two
 *  genuinely different facts into one row. */
const committerDiffersFromAuthor = computed(() => {
  const detail = props.detail;
  if (!detail) return false;
  return (
    detail.author.name !== detail.committer.name ||
    detail.author.email !== detail.committer.email ||
    detail.author.timestamp !== detail.committer.timestamp
  );
});

const SIGNATURE_TEXT: Readonly<Record<string, MessageKey>> = {
  G: 'git.signature.good.status',
  B: 'git.signature.bad.status',
  U: 'git.signature.unknownValidity.status',
  X: 'git.signature.expiredSignature.status',
  Y: 'git.signature.expiredKey.status',
  R: 'git.signature.revokedKey.status',
  E: 'git.signature.uncheckable.status',
};

/** `undefined` for an unsigned commit (`status: "N"`) — §6.4/W7: "an unsigned commit shows no
 *  row, not an empty one". */
const signatureText = computed<string | undefined>(() => {
  const signature = props.detail?.signature;
  if (!signature || signature.status === 'N') return undefined;
  const key = SIGNATURE_TEXT[signature.status];
  const text = key === undefined ? signature.status : t(key);
  return signature.signer
    ? t('git.signature.bySigner.status', { status: text, signer: signature.signer })
    : text;
});

/** The one change request row — resolved with a PR (one line per associated PR),
 *  resolved with none ("No change request"), or `unavailable` (`Status.Reason` — the ONLY place in
 *  the app that ever tells a user to log in to the host CLI, per the "inert, never noisy"
 *  rule applying everywhere else but here). `disabled` renders nothing at all — this computed is
 *  `undefined` for both `disabled` and "nothing resolved yet", collapsing them into the same
 *  "no row" template branch below. */

interface ChangeRequestDetailView {
  readonly kind: 'changeRequests' | 'none' | 'unavailable';
  readonly changeRequests: readonly {
    readonly number: number;
    readonly title: string;
    readonly state: string;
    readonly stateLabel: string;
  }[];
  readonly reason: string | undefined;
}

const changeRequestDetail = computed<ChangeRequestDetailView | undefined>(() => {
  const result = props.changeRequestResult;
  if (result === undefined || result.kind === 'disabled') return undefined;
  if (result.kind === 'unavailable') {
    return {
      kind: 'unavailable',
      changeRequests: [],
      reason:
        result.status.reason ?? t('git.changeRequest.noAnswer.hint', { label: words.value.label }),
    };
  }
  if (result.changeRequests.length === 0) {
    return { kind: 'none', changeRequests: [], reason: undefined };
  }
  return {
    kind: 'changeRequests',
    reason: undefined,
    changeRequests: result.changeRequests.map((cr) => ({
      number: cr.number,
      title: cr.title,
      state: cr.state,
      stateLabel: changeRequestStateLabel(cr.state),
    })),
  };
});

/** Whether the Refs/Signature/PR block has anything to show at all — the common case (no
 *  decoration on this commit, unsigned, host off) renders none of it, not an empty padded
 *  box. Reads `detail.decoration.length` directly rather than `decorationEl?.childNodes.length`
 *  (the individual Refs row's own check, unchanged below): this needs a value known synchronously
 *  from props, before `renderDecoration`'s own `nextTick` has populated anything, and before
 *  `decorationEl` even exists in the DOM (it is only mounted while `expanded`) —
 *  `buildRefBadges` only ever returns `null` for an empty decoration array (`refBadges.ts`), so
 *  this tracks the DOM outcome closely enough to gate on. */
const hasDetails = computed(
  () =>
    (props.detail?.decoration.length ?? 0) > 0 ||
    signatureText.value !== undefined ||
    changeRequestDetail.value !== undefined,
);

const COAUTHOR_TOKENS = new Set(['Co-authored-by', 'Signed-off-by']);
const IDENTITY_TRAILER = /^(.*)\s<(.+)>$/;

interface TrailerRow {
  readonly token: string;
  readonly name: string | undefined;
  readonly email: string | undefined;
  readonly raw: string;
}

/** `Co-authored-by`/`Signed-off-by` (the two named trailers) parse their `Name <email>`
 *  shape into separate name/email display so the email can be styled as secondary text; every
 *  other trailer renders its value verbatim — this repo does not try to out-guess git's own
 *  trailer syntax for tokens it has not been told carry an identity. */
const trailerRows = computed<TrailerRow[]>(() =>
  (props.detail?.trailers ?? []).map((trailer) => {
    if (COAUTHOR_TOKENS.has(trailer.token)) {
      const match = IDENTITY_TRAILER.exec(trailer.value);
      if (match) {
        const [, name, email] = match;
        return { token: trailer.token, name: name?.trim(), email, raw: trailer.value };
      }
    }
    return { token: trailer.token, name: undefined, email: undefined, raw: trailer.value };
  }),
);

/** G-UX (item 6): the facts row's own copy path — the only sha-copy affordance this pane offers
 *  now (the old header copy button copied the full *message*, not the sha, and is gone). */
function copySha(): void {
  const sha = props.detail?.sha;
  if (!sha) return;
  props.actions.copy(sha, t('git.copy.fullSha.announce'));
}

/** Replaces the old copy-message header button — "Open all changes", the exact
 *  pattern `ReviewCommitRow.vue`'s own row action already uses. Unlike that row, this pane's
 *  `detail` always already carries the full file list (no lazy per-row fetch to guard against
 *  here), so there is no "expand first" precondition to check. */
async function openAllChanges(): Promise<void> {
  const detail = props.detail;
  if (!detail) return;
  await openAllChangesAnnounced(props.actions, {
    sha: detail.sha,
    parentIndex: detail.parentIndex,
  });
}

/** P74 §3.3: the change request row's own external-open action, and (§4.3) the facts-row icon's
 *  — one open path for both, never two. */
async function openChangeRequest(number: number): Promise<void> {
  try {
    await props.actions.openChangeRequest({ number });
  } catch (err) {
    props.actions.announce(
      t('git.async.openChangeRequest.error', {
        changeRequestNoun: words.value.changeRequestNoun,
        error: errorText(err),
      }),
    );
  }
}

/** The message body's own linkified URLs (`renderBody`, above). */
const openLink = createOpenExternal((url) => props.actions.openExternalLink(url));

/** The facts row's own icon — the first PR `changeRequestForCommit` names (ancestry's own
 *  first-writer-wins already resolved which branch wins when more than one claims a commit;
 *  `bySha`'s own array is realistically always length 1 in practice, never fanned out further
 *  here). `undefined` renders nothing, the same "render nothing" rule every other surface
 *  follows. */
const changeRequestIcon = computed(() => {
  const first = props.changeRequestForCommit?.[0];
  if (!first) return undefined;
  return {
    number: first.number,
    title: first.title,
    state: first.state,
    stateLabel: changeRequestStateLabel(first.state),
  };
});
</script>

<template>
  <div
    v-if="detail"
    :class="
      expanded
        ? 'flex flex-col flex-initial min-h-[min(220px,60%)] max-h-7/10'
        : 'flex flex-col flex-none max-h-1/5 overflow-hidden'
    "
    data-testid="commit-meta"
  >
    <AttributeTooltip :container="decorationEl" />

    <ViewToolbar data-testid="detail-head">
      <span data-testid="meta-subject" class="min-w-0 text-kira-md text-fg"
        ><TruncatedText :text="detail.subject" /></span
      >
      <Tooltip v-if="changeRequestIcon">
        <TooltipTrigger as-child>
          <Button
            variant="toolbar"
            size="inline"
            :class="
              cn('ml-auto leading-none', TONE_TEXT[changeRequestTone(changeRequestIcon!.state)])
            "
            :aria-label="
              t('git.changeRequest.open.aria', {
                changeRequestNoun: words.changeRequestNoun,
                number: String(changeRequestIcon!.number),
                label: words.label,
              })
            "
            data-testid="commit-meta-change-request-icon"
            @click="openChangeRequest(changeRequestIcon!.number)"
          >
            <CodiconIcon name="git-pull-request" :size="12" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          <TooltipDetails
            :title="
              t('git.changeRequest.summary.tooltip', {
                number: String(changeRequestIcon!.number),
                title: changeRequestIcon!.title,
              })
            "
            :rows="[
              { label: t('git.changeRequest.state.label'), value: changeRequestIcon!.stateLabel },
            ]"
          />
        </TooltipContent>
      </Tooltip>
    </ViewToolbar>

    <div :class="['flex min-h-0 flex-col gap-1 overflow-auto px-3', expanded ? 'py-2' : '']">
      <p
        data-testid="meta-body"
        v-if="bodyParagraphs.length > 0"
        ref="bodyEl"
        v-show="expanded"
        class="mt-1 whitespace-normal break-words"
      ></p>

      <div v-if="expanded" class="flex flex-col gap-1 mt-1">
        <FieldDescription data-testid="meta-identity">
          {{ detail.author.name }}
          &lt;{{ detail.author.email }}&gt;
        </FieldDescription>
        <FieldDescription data-testid="meta-identity" v-if="committerDiffersFromAuthor">
          {{ detail.committer.name }}
          &lt;{{ detail.committer.email }}&gt;
          <span>{{ t('git.commit.committer.label') }}</span>
        </FieldDescription>
        <KeyValueList data-testid="meta-trailers" v-if="trailerRows.length > 0" density="compact">
          <template v-for="(row, index) in trailerRows" :key="index">
            <dt>{{ row.token }}</dt>
            <dd v-if="row.name !== undefined">
              {{ row.name }} <span class="text-muted-foreground">&lt;{{ row.email }}&gt;</span>
            </dd>
            <dd v-else>{{ row.raw }}</dd>
          </template>
        </KeyValueList>
        <KeyValueList v-if="hasDetails" density="compact">
          <template v-if="detail.decoration.length > 0">
            <dt>{{ t('git.common.refs.label') }}</dt>
            <dd data-testid="meta-refs" ref="decorationEl" class="flex flex-wrap gap-0.5"></dd>
          </template>
          <template v-if="signatureText">
            <dt>{{ t('git.commit.signature.label') }}</dt>
            <dd>{{ signatureText }}</dd>
          </template>
          <template v-if="changeRequestDetail">
            <dt>{{ words.changeRequest }}</dt>
            <dd v-if="changeRequestDetail.kind === 'changeRequests'" class="flex flex-col gap-0.5">
              <div
                v-for="cr in changeRequestDetail.changeRequests"
                :key="cr.number"
                class="flex items-center gap-0.5"
              >
                <span :class="changeRequestBadgeClass(cr.state)">
                  {{ cr.stateLabel }}
                </span>
                <Button
                  variant="link"
                  size="inline"
                  class="justify-start text-left whitespace-normal"
                  @click="openChangeRequest(cr.number)"
                >
                  #{{ cr.number }} {{ cr.title }}
                </Button>
              </div>
            </dd>
            <dd v-else-if="changeRequestDetail.kind === 'none'">{{
              t('git.commit.noChangeRequest.empty', { changeRequestNoun: words.changeRequestNoun })
            }}</dd>
            <dd v-else class="text-muted-foreground" data-testid="change-request-unavailable">
              {{ changeRequestDetail.reason }}
            </dd>
          </template>
        </KeyValueList>
      </div>
    </div>

    <div class="flex shrink-0 items-center gap-2 px-3 py-2">
      <Button
        data-testid="meta-body-toggle"
        variant="link"
        size="inline"
        class="justify-start"
        @click="expanded = !expanded"
      >
        {{ expanded ? t('common.action.showLess.label') : t('common.action.showMore.label') }}
      </Button>
      <div class="ml-auto flex items-center gap-2">
        <TextTooltip :text="t('git.commit.copySha.tooltip')">
          <Badge
            as="button"
            type="button"
            :aria-label="t('git.commit.copySha.tooltip')"
            data-testid="commit-meta-sha"
            @click="copySha"
          >
            <Identifier :value="detail.sha" :short="shortSha" />
          </Badge>
        </TextTooltip>
        <TextTooltip :text="formatAbsoluteUtc(detail.committer.timestamp)">
          <span class="shrink-0 text-kira-sm text-muted-foreground">{{
            formatAgeCompact(detail.committer.timestamp)
          }}</span>
        </TextTooltip>
        <TooltipIconButton
          icon="diff-multiple"
          :label="t('git.commit.openAllChanges.tooltip')"
          data-testid="open-all-changes-button"
          @click="openAllChanges"
        />
      </div>
    </div>
  </div>
</template>
