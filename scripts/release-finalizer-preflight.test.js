'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { spawnSync } = require('node:child_process');
const fs = require('node:fs');
const path = require('node:path');
const { resolveFinalizerActionContext } = require('./release-finalizer-entrypoint');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { resolveFinalizerEvent, inspectFinalizerPreflight } =
  require('./release-finalizer-preflight');

const TAG = 'v3.4.0';
const COMMIT = 'a'.repeat(40);
const REPO = 'zoster81/scripthold';
const API = 'https://api.github.com/repos/zoster81/scripthold';
const TOKEN = 'test_token_never_used_on_network';
const MANIFEST = {
  repository: REPO, tag: TAG, commit: COMMIT,
  assets: expectedReleaseAssetNames(TAG).map((name, i) =>
    ({ name, size: i + 1, sha256: 'f'.repeat(64) })),
};

const tagEvent = () => ({
  eventName: 'push', ref: 'refs/tags/' + TAG, repository: REPO,
});
const dispatchEvent = () => ({
  eventName: 'workflow_dispatch', ref: 'refs/heads/main',
  repository: REPO, inputs: { tag: TAG },
});

function provider({ differentTagObject = false, absent = false,
  changedState = false, changedScorecardRun = false, failOn = null } = {}) {
  const calls = [];
  let tagReads = 0;
  let releaseReads = 0;
  let scorecardReads = 0;
  const tagSha = () => tagReads === 1 || !differentTagObject ?
    'b'.repeat(40) : 'c'.repeat(40);
  const run = (id, path) => ({
    id, run_number: id, run_attempt: 1, head_sha: COMMIT,
    head_branch: 'main', event: 'push', conclusion: 'success',
    path, head_repository: { full_name: REPO },
  });
  const assets = MANIFEST.assets.map((a, i) => ({
    id: 100 + i, name: a.name, size: a.size,
    digest: 'sha256:' + a.sha256, state: 'uploaded',
  }));
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (url === failOn) throw new Error('provider unavailable');
    let result;
    if (url === API + '/git/ref/tags/' + TAG) {
      tagReads++;
      result = { ref: 'refs/tags/' + TAG, object: { type: 'tag', sha: tagSha() } };
    } else if (url === API + '/git/tags/' + 'b'.repeat(40) ||
      url === API + '/git/tags/' + 'c'.repeat(40)) {
      result = { sha: url.slice(-40), tag: TAG, object: { type: 'commit', sha: COMMIT } };
    } else if (url === API + '/git/ref/heads/main') {
      result = { ref: 'refs/heads/main', object: { type: 'commit', sha: COMMIT } };
    } else if (url.includes('/actions/workflows/test.yml/runs?')) {
      result = { total_count: 1, workflow_runs: [run(11, '.github/workflows/test.yml')] };
    } else if (url.includes('/actions/workflows/scorecard.yml/runs?')) {
      scorecardReads++;
      result = { total_count: 1, workflow_runs: [run(
        changedScorecardRun && scorecardReads > 1 ? 13 : 12,
        '.github/workflows/scorecard.yml')] };
    } else if (url.includes('/actions/runs/11/attempts/1/jobs?')) {
      result = { total_count: 1, jobs: [{
        name: 'Release candidate', run_id: 11, run_attempt: 1,
        head_sha: COMMIT, status: 'completed', conclusion: 'success',
      }] };
    } else if (url === API + '/releases?per_page=30&page=1') {
      result = absent ? [] : [{
        id: 123, tag_name: TAG, draft: true, immutable: false,
      }];
    } else if (url === API + '/releases/123') {
      releaseReads++;
      result = { id: 123, tag_name: TAG,
        draft: changedState && releaseReads > 1 ? false : true,
        immutable: false };
    } else if (url === API + '/releases/123/assets?per_page=100&page=1') {
      result = assets;
    } else {
      throw new Error('unexpected test provider URL: ' + url);
    }
    return new Response(JSON.stringify(result), {
      headers: { 'content-type': 'application/json' },
    });
  };
  return { fetchImpl, calls };
}

test('entrypoint normalizes GitHub Actions context without accepting branch or foreign tags', () => {
  const push = resolveFinalizerActionContext({
    GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/tags/' + TAG,
    GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: '',
  });
  const manual = resolveFinalizerActionContext({
    GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/main',
    GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: TAG,
  });
  assert.deepEqual(push, manual);
  assert.equal(push.group, 'scripthold-immutable-release-' + TAG);
  for (const context of [
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/heads/main',
      GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: '' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/feature',
      GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: TAG },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/main',
      GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: '' },
    { GITHUB_EVENT_NAME: 'pull_request', GITHUB_REF: 'refs/tags/' + TAG,
      GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: '' },
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/tags/' + TAG,
      GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: TAG },
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/tags/' + TAG,
      GITHUB_REPOSITORY: 'other/repository', RELEASE_TAG_INPUT: '' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/main',
      GITHUB_REPOSITORY: REPO, RELEASE_TAG_INPUT: 'v3.4.0\ninvalid' },
    {},
  ]) {
    assert.throws(() => resolveFinalizerActionContext(context), Error);
  }
});

test('actual entrypoint process succeeds only for authorized read-only events', () => {
  const entrypoint = path.join(__dirname, 'release-finalizer-entrypoint.js');
  const base = {
    ...process.env,
    GITHUB_REPOSITORY: REPO,
    RELEASE_TAG_INPUT: '',
  };
  for (const env of [
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/tags/' + TAG,
      RELEASE_TAG_INPUT: '' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/main',
      RELEASE_TAG_INPUT: TAG },
  ]) {
    const run = spawnSync(process.execPath, [entrypoint], {
      encoding: 'utf8', env: { ...base, ...env }, timeout: 10000,
    });
    assert.equal(run.status, 0, run.stderr);
    assert.match(run.stdout, /Validated read-only finalizer group:/);
  }
  for (const env of [
    { GITHUB_EVENT_NAME: 'push', GITHUB_REF: 'refs/heads/main',
      RELEASE_TAG_INPUT: '' },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/feature',
      RELEASE_TAG_INPUT: TAG },
    { GITHUB_EVENT_NAME: 'workflow_dispatch', GITHUB_REF: 'refs/heads/main',
      RELEASE_TAG_INPUT: '../escape' },
  ]) {
    const run = spawnSync(process.execPath, [entrypoint], {
      encoding: 'utf8', env: { ...base, ...env }, timeout: 10000,
    });
    assert.notEqual(run.status, 0);
    assert.equal(run.stdout, '');
    assert.match(run.stderr, /Refusing untrusted release preflight event/);
  }
});

test('future single-writer workflow is read-only and has a matching cross-trigger group', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows',
    'release-finalizer-preflight.yml'), 'utf8');
  assert.match(workflow, /^on:\n  push:\n    tags:\n      - 'v\*'\n  workflow_dispatch:/m);
  assert.match(workflow, /^permissions:\n  contents: read\n/m);
  assert.match(workflow, /^concurrency:\n  group: scripthold-immutable-release-\$\{\{ github\.event_name == 'workflow_dispatch' && inputs\.tag \|\| github\.ref_name \}\}\n  cancel-in-progress: false$/m);
  assert.doesNotMatch(workflow, /^\s+[a-z-]+: write\s*$/m);
  assert.doesNotMatch(workflow, /\$\{\{\s*secrets\./);
  assert.doesNotMatch(workflow, /^\s+environment:/m);
  assert.deepEqual([...workflow.matchAll(/^ {6}- name: (.+)$/gm)].map((m) => m[1]), [
    'Check out requested ref without persisted credentials',
    'Set up pinned Node.js',
    'Validate event and canonical concurrency group',
  ]);
  assert.deepEqual([...workflow.matchAll(/^ {8}uses: ([^\s]+).*$/gm)].map((m) => m[1]), [
    'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1',
    'actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1',
  ]);
  assert.equal([...workflow.matchAll(/^ {8}run: \|$/gm)].length, 1);
  assert.match(workflow, /^ {10}persist-credentials: false$/m);
  assert.match(workflow, /^ {10}set -euo pipefail\n {10}node scripts\/release-finalizer-entrypoint\.js$/m);
});

test('normalizes push-tag and manual-main events to one exact concurrency group', () => {
  const push = resolveFinalizerEvent(tagEvent());
  const manual = resolveFinalizerEvent(dispatchEvent());
  assert.deepEqual(push, {
    repository: REPO, tag: TAG, group: 'scripthold-immutable-release-' + TAG,
  });
  assert.deepEqual(manual, push);
  assert.notEqual(resolveFinalizerEvent({
    ...tagEvent(), ref: 'refs/tags/v3.4.1',
  }).group, push.group);
});

test('rejects invalid events, untrusted tag/branch and cross-repository entrypoints', () => {
  for (const event of [
    null, {}, { ...tagEvent(), eventName: 'pull_request' },
    { ...tagEvent(), repository: 'other/scripthold' },
    { ...tagEvent(), ref: 'refs/heads/main' },
    { ...tagEvent(), ref: 'refs/tags/v3.4.0/../../x' },
    { ...tagEvent(), ref: 'refs/tags/V3.4.0' },
    { ...dispatchEvent(), ref: 'refs/heads/feature' },
    { ...dispatchEvent(), inputs: {} },
    { ...dispatchEvent(), inputs: { tag: 'main' } },
    { ...dispatchEvent(), inputs: { tag: 'v3.4.0\nunsafe' } },
    { ...dispatchEvent(), repository: 'other/repo' },
  ]) {
    assert.throws(() => resolveFinalizerEvent(event), Error);
  }
});

test('refreshes GitHub source evidence on both sides of draft discovery, read-only', async () => {
  const fake = provider();
  const result = await inspectFinalizerPreflight({
    event: tagEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: fake.fetchImpl,
  });
  assert.equal(result.group, 'scripthold-immutable-release-' + TAG);
  assert.equal(result.tag, TAG);
  assert.equal(result.authority.commit, COMMIT);
  assert.equal(result.authority.tagObjectSha, 'b'.repeat(40));
  assert.equal(result.discovery.kind, 'candidate');
  assert.equal(result.discovery.releaseId, 123);
  assert.equal(fake.calls.length, 16);
  assert.deepEqual(fake.calls.filter(x => x.url.includes('/git/ref/tags/')).length, 2);
  for (const call of fake.calls) {
    assert.equal(call.init.method, 'GET');
    assert.equal(call.init.redirect, 'error');
    assert.equal(call.init.headers.Authorization, 'Bearer ' + TOKEN);
  }
});

test('fails closed when annotated tag object changes but still resolves to the same commit', async () => {
  const fake = provider({ differentTagObject: true });
  await assert.rejects(inspectFinalizerPreflight({
    event: dispatchEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /changed|source|authority/);
});

test('detects a changed successful Scorecard run between authoritative reads', async () => {
  const fake = provider({ changedScorecardRun: true });
  await assert.rejects(inspectFinalizerPreflight({
    event: tagEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /authority changed/);
  assert.equal(fake.calls.length, 16);
});

test('fails closed instead of returning a usable preflight for an unproven absence', async () => {
  const fake = provider({ absent: true });
  await assert.rejects(inspectFinalizerPreflight({
    event: tagEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /absence is unproven/);
  assert.equal(fake.calls.length, 7);
  assert.ok(fake.calls.every(({ init }) => init.method === 'GET'));
});

test('rejects a changed release state and source event without returning any proposal', async () => {
  for (const args of [
    { event: { ...tagEvent(), ref: 'refs/tags/v3.5.0' } },
    { publisherCommit: 'c'.repeat(40) },
    { token: '' },
  ]) {
    const fake = provider();
    await assert.rejects(inspectFinalizerPreflight({
      event: tagEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
      token: TOKEN, fetchImpl: fake.fetchImpl, ...args,
    }), Error);
  }
  const fake = provider({ changedState: true });
  await assert.rejects(inspectFinalizerPreflight({
    event: tagEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: fake.fetchImpl,
  }), Error);
});

test('stops on provider transport failure without retrying or issuing mutations', async () => {
  const fake = provider({ failOn: API + '/git/ref/heads/main' });
  await assert.rejects(inspectFinalizerPreflight({
    event: tagEvent(), manifest: MANIFEST, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /provider unavailable/);
  assert.equal(fake.calls.length, 3);
});
