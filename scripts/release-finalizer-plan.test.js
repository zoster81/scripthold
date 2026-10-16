'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { normalizeReleaseAssetPage, planReleaseFinalizerStep } =
  require('./release-finalizer-plan');

const COMMIT = 'a'.repeat(40);
const TAG_OBJECT = 'b'.repeat(40);
const TAG = 'v3.4.0';
const REPO = 'zoster81/scripthold';

function fixture() {
  const assets = expectedReleaseAssetNames(TAG).map((name, index) => ({
    name, size: index + 101, sha256: (index + 1).toString(16).padStart(64, '0'),
  }));
  const manifest = { repository: REPO, tag: TAG, commit: COMMIT, assets };
  const run = (id, path) => ({
    id, run_number: id, run_attempt: 1, head_sha: COMMIT, head_branch: 'main',
    event: 'push', conclusion: 'success', path,
    head_repository: { full_name: REPO },
  });
  const authority = {
    repository: REPO, tag: TAG, commit: COMMIT, publisherCommit: COMMIT, manifest,
    tagRef: { ref: 'refs/tags/' + TAG, object: { type: 'tag', sha: TAG_OBJECT } },
    tagObject: { tag: TAG, sha: TAG_OBJECT, object: { type: 'commit', sha: COMMIT } },
    mainRef: { ref: 'refs/heads/main', object: { type: 'commit', sha: COMMIT } },
    testRuns: { workflow_runs: [run(11, '.github/workflows/test.yml')] },
    scorecardRuns: { workflow_runs: [run(12, '.github/workflows/scorecard.yml')] },
    testJobs: { jobs: [{
      name: 'Release candidate', status: 'completed', conclusion: 'success',
      run_id: 11, run_attempt: 1, head_sha: COMMIT,
    }] },
  };
  const remote = assets.map((asset, i) => ({
    id: i + 41, name: asset.name, size: asset.size, state: 'uploaded',
    digest: 'sha256:' + asset.sha256,
  }));
  return { authority, remote };
}

function page(assets, releaseId = 123) {
  return {
    status: 200, request: { releaseId, page: 1, perPage: 100 },
    link: null, assets,
  };
}

function found(assets, flags = {}) {
  return {
    kind: 'present',
    release: { id: 123, tag_name: TAG, draft: true, immutable: false, ...flags },
    assetPage: page(assets),
  };
}

test('accepts a complete bounded GitHub asset page without silently omitting more pages', () => {
  const { remote } = fixture();
  assert.deepEqual(normalizeReleaseAssetPage(123, page(remote)), remote);
  assert.deepEqual(normalizeReleaseAssetPage(123, page([])), []);
  assert.notEqual(normalizeReleaseAssetPage(123, page(remote))[0], remote[0],
    'the plan must not alias mutable API objects');
});

test('rejects partial, ambiguous or oversized GitHub release asset pagination', () => {
  const { remote } = fixture();
  const cases = [
    { ...page(remote), status: 404 },
    { ...page(remote), link: '<https://api.github.com/next>; rel="next"' },
    { ...page(remote), link: '' },
    { ...page(remote), link: undefined },
    { ...page(remote), request: { releaseId: 123, page: 2, perPage: 100 } },
    { ...page(remote), request: { releaseId: 123, page: 1, perPage: 30 } },
    { ...page(remote), request: { releaseId: 124, page: 1, perPage: 100 } },
    { ...page(remote), assets: null },
    { ...page(remote), assets: remote.concat(remote[0]) },
    { ...page(remote), assets: new Array(23) },
    null,
  ];
  for (const candidate of cases) {
    assert.throws(() => normalizeReleaseAssetPage(123, candidate), Error);
  }
});

test('proposes one draft upload at a time in canonical order and publication only at 22/22', () => {
  const { authority, remote } = fixture();
  const initial = planReleaseFinalizerStep(authority, found([]));
  assert.deepEqual(initial, { step: 'upload-candidate', releaseId: 123,
    name: remote[0].name, size: authority.manifest.assets[0].size,
    sha256: authority.manifest.assets[0].sha256, remaining: 22 });
  const partial = planReleaseFinalizerStep(authority, found([remote[2], remote[0]]));
  assert.equal(partial.step, 'upload-candidate');
  assert.equal(partial.name, remote[1].name);
  assert.equal(partial.remaining, 20);
  assert.deepEqual(planReleaseFinalizerStep(authority, found(remote)),
    { step: 'publish-candidate', releaseId: 123, count: 22 });
  assert.deepEqual(planReleaseFinalizerStep(authority,
    found(remote, { draft: false, immutable: true })),
  { step: 'complete', releaseId: 123, count: 22 });
});

test('does not interpret absent releases, failures or uncertain publication as safe automatic retries', () => {
  const { authority, remote } = fixture();
  assert.deepEqual(planReleaseFinalizerStep(authority,
    { kind: 'absent', status: 404 }), { step: 'inspect-absence', tag: TAG });
  assert.deepEqual(planReleaseFinalizerStep(authority,
    { kind: 'absent', status: 404 }, { operation: 'create' }),
  { step: 'halt', reason: 'create-ambiguous' });
  assert.deepEqual(planReleaseFinalizerStep(authority, found(remote),
    { operation: 'publish' }), { step: 'halt', reason: 'publish-ambiguous' });
  assert.deepEqual(planReleaseFinalizerStep(authority, found(remote.slice(0, 1)),
    { operation: 'upload', name: remote[3].name }),
  { step: 'halt', reason: 'upload-ambiguous' });
  assert.deepEqual(planReleaseFinalizerStep(authority, found(remote.slice(0, 4)),
    { operation: 'upload', name: remote[3].name }).step, 'upload-candidate');
  assert.deepEqual(planReleaseFinalizerStep(authority,
    found(remote, { draft: false, immutable: true }), { operation: 'publish' }),
  { step: 'complete', releaseId: 123, count: 22 });
});

test('all 23 partial draft states yield the same deterministic one-step decision', () => {
  const { authority, remote } = fixture();
  const original = JSON.stringify(remote);
  for (let count = 0; count <= remote.length; count++) {
    const observation = found(remote.slice(0, count).reverse());
    const first = planReleaseFinalizerStep(authority, observation);
    const second = planReleaseFinalizerStep(authority, observation);
    assert.deepEqual(first, second, 'planning may not mutate caller data or depend on order');
    if (count === remote.length) {
      assert.equal(first.step, 'publish-candidate');
    } else {
      assert.equal(first.step, 'upload-candidate');
      assert.equal(first.name, remote[count].name);
      assert.equal(first.remaining, remote.length - count);
    }
  }
  assert.equal(JSON.stringify(remote), original, 'planning must not rewrite source metadata');
});

test('an uncertain draft creation is acknowledged only by a separately observed draft', () => {
  const { authority, remote } = fixture();
  assert.equal(planReleaseFinalizerStep(authority, found([]),
    { operation: 'create' }).step, 'upload-candidate');
  assert.deepEqual(planReleaseFinalizerStep(authority, found(remote.slice(0, 3)),
    { operation: 'publish' }), { step: 'halt', reason: 'publish-ambiguous' });
  assert.throws(() => planReleaseFinalizerStep(authority, found(remote, {
    draft: false, immutable: false,
  }), { operation: 'publish' }), /neither an unpublished draft/);
});

test('rejects unknown release states, conflicting assets, wrong publisher and malformed uncertainty', () => {
  const { authority, remote } = fixture();
  for (const observation of [
    null, {}, { kind: 'error', status: 500 },
    { kind: 'absent', status: 403 }, { kind: 'absent', status: 200 },
    found(remote, { draft: false, immutable: false }),
    found(remote, { tag_name: 'v3.3.0' }),
    found(remote.slice(0, 2).map((a, i) => i === 1 ? { ...a, digest: 'sha256:' + 'f'.repeat(64) } : a)),
    found(remote.slice(0, 2).map((a, i) => i === 1 ? { ...a, id: remote[0].id } : a)),
    { ...found(remote), assetPage: page(remote, 124) },
  ]) {
    assert.throws(() => planReleaseFinalizerStep(authority, observation), Error);
  }
  for (const uncertainty of [
    { operation: 'publish', name: remote[0].name },
    { operation: 'upload', name: '../escape' },
    { operation: 'upload' }, { operation: 'delete' }, {},
  ]) {
    assert.throws(() => planReleaseFinalizerStep(authority, found(remote), uncertainty), Error);
  }
  const badAuthority = fixture().authority;
  badAuthority.publisherCommit = 'c'.repeat(40);
  assert.throws(() => planReleaseFinalizerStep(badAuthority, found(remote)), Error);
});
