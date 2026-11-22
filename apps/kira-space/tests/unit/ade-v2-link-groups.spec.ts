import { describe, expect, test } from 'bun:test';
import { linkGroups } from '../../frontend/src/ade/v2/links/linkGroups';
import type { Artifact, Task } from '../../frontend/src/ade/v2/wire';
import { mkTask } from './support/adeV2Fixtures';

function art(over: Partial<Artifact> & Pick<Artifact, 'kind'>): Artifact {
  return {
    id: '',
    title: '',
    ref: '',
    url: '',
    status: '',
    repo: '',
    source: 'user',
    sessionId: '',
    createdAt: 0,
    ...over,
  };
}

const labels = (groups: ReturnType<typeof linkGroups>, id: string): string[] =>
  groups.find((g) => g.id === id)?.items.map((i) => i.label) ?? [];

const jiraTask = (): Task =>
  mkTask({ id: 'T', jira: { key: 'PAY-1', url: 'https://j.test/browse/PAY-1' } as Task['jira'] });

const userCr = art({
  kind: 'changeRequest',
  id: 'u',
  title: 'user CR',
  url: 'https://h.test/pr/1',
});
const agentCr = art({
  kind: 'changeRequest',
  id: 'g',
  title: 'agent CR',
  url: 'https://h.test/pr/5',
  source: 'agent',
  createdAt: 5,
});

describe('linkGroups order', () => {
  test('A: a stored row matching ownUrls leads, trailing slash ignored', () => {
    const g = linkGroups(mkTask({ id: 'T' }), [userCr, agentCr], {
      ownUrls: ['https://h.test/pr/5/'],
    });
    expect(labels(g, 'changeRequest')).toEqual(['agent CR', 'user CR']);
  });

  test('B: without ownUrls input order holds', () => {
    const g = linkGroups(mkTask({ id: 'T' }), [userCr, agentCr]);
    expect(labels(g, 'changeRequest')).toEqual(['user CR', 'agent CR']);
  });

  test('C: Jira slot, then detected tracker row, then stored; own key row dropped', () => {
    const g = linkGroups(jiraTask(), [
      art({ kind: 'issue', id: 's', title: 'PAY-2', url: 'https://j.test/browse/PAY-2' }),
      art({
        kind: 'issue',
        ref: 'NEW-5',
        title: 'NEW-5',
        url: 'https://j.test/browse/NEW-5',
        createdAt: 10,
      }),
      art({ kind: 'issue', ref: 'PAY-1', title: 'PAY-1', url: 'https://j.test/other/PAY-1' }),
    ]);
    expect(labels(g, 'issue')).toEqual(['PAY-1', 'NEW-5', 'PAY-2']);
  });

  test('D: host slot dedups detected rows; detected branch last, only with branches', () => {
    const task = mkTask({
      id: 'T',
      hostUrl: 'https://h.test/pr/9',
      hostLink: { kind: 'changeRequest', ref: 'repo!9' } as Task['hostLink'],
    });
    const arts = [
      art({ kind: 'changeRequest', title: 'dup', url: 'https://h.test/pr/9/', source: 'host' }),
      art({ kind: 'changeRequest', title: 'det', url: 'https://h.test/pr/10', source: 'host' }),
      art({ kind: 'branch', title: 'feat/x', source: 'git' }),
      art({ kind: 'doc', id: 'd', title: 'Doc' }),
    ];
    const g = linkGroups(task, arts, { branches: true });
    expect(labels(g, 'changeRequest')).toEqual(['repo!9', 'det']);
    expect(labels(g, 'misc')).toEqual(['Doc', 'feat/x']);
    expect(labels(linkGroups(task, arts), 'misc')).toEqual(['Doc']);
  });
});

describe('linkGroups related', () => {
  const foundPr = art({
    kind: 'changeRequest',
    title: 'api!21',
    url: 'https://h.test/pr/21',
    source: 'notes',
    foundIn: [
      { handoverId: 'h1', state: 'note', fromStep: 'Work', toStep: '', fromTask: '', createdAt: 1 },
    ],
  });
  const foundDoc = art({ kind: 'doc', title: 'Spec', url: 'https://d.test/spec', source: 'notes' });

  test('E: found links land in related whatever their kind, never in main', () => {
    const g = linkGroups(mkTask({ id: 'T' }), [userCr, foundPr, foundDoc]);
    expect(labels(g, 'related')).toEqual(['api!21', 'Spec']);
    expect(labels(g, 'changeRequest')).toEqual(['user CR']);
    expect(labels(g, 'misc')).toEqual([]);
    const item = g.find((x) => x.id === 'related')?.items[0];
    expect(item?.origin).toBe('found');
    expect(item?.foundIn?.[0]?.handoverId).toBe('h1');
  });

  test('F: a found link whose URL is a slot is not shown', () => {
    const task = mkTask({ id: 'T', hostUrl: 'https://h.test/pr/21' });
    expect(labels(linkGroups(task, [foundPr]), 'related')).toEqual([]);
  });
});
