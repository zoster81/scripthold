'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { readPublicationAuthority } = require('./release-finalizer-authority');

const REPO = 'zoster81/scripthold';
const TAG = 'v3.4.0';
const COMMIT = 'a'.repeat(40);
const TAG_SHA = 'b'.repeat(40);
const TOKEN = 'test_only_private_token';
const API = 'https://api.github.com/repos/zoster81/scripthold';

function fixture() {
  const manifest = {
    repository: REPO, tag: TAG, commit: COMMIT,
    assets: expectedReleaseAssetNames(TAG).map((name, i) =>
      ({ name, size: i + 1, sha256: 'f'.repeat(64) })),
  };
  const run = (id, path) => ({
    id, run_number: id, run_attempt: 2, event: 'push',
    head_branch: 'main', head_sha: COMMIT, conclusion: 'success',
    path, head_repository: { full_name: REPO },
  });
  const suffix = '?event=push&branch=main&head_sha=' + COMMIT + '&per_page=100&page=1';
  const testUrl = API + '/actions/workflows/test.yml/runs' + suffix;
  const scoreUrl = API + '/actions/workflows/scorecard.yml/runs' + suffix;
  const jobUrl = API + '/actions/runs/111/attempts/2/jobs?per_page=100&page=1';
  const urls = [
    API + '/git/ref/tags/' + TAG,
    API + '/git/tags/' + TAG_SHA,
    API + '/git/ref/heads/main',
    testUrl, scoreUrl, jobUrl,
  ];
  const data = [
    { ref: 'refs/tags/' + TAG, object: { type: 'tag', sha: TAG_SHA } },
    { sha: TAG_SHA, tag: TAG, object: { type: 'commit', sha: COMMIT } },
    { ref: 'refs/heads/main', object: { type: 'commit', sha: COMMIT } },
    { total_count: 1, workflow_runs: [run(111, '.github/workflows/test.yml')] },
    { total_count: 1, workflow_runs: [run(222, '.github/workflows/scorecard.yml')] },
    { total_count: 1, jobs: [{
      name: 'Release candidate', status: 'completed', conclusion: 'success',
      run_id: 111, run_attempt: 2, head_sha: COMMIT,
    }] },
  ];
  return { manifest, urls, data };
}

function mockFixture({ mutate, headers = {} } = {}) {
  const { manifest, urls, data } = fixture();
  if (mutate) mutate(data);
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    const index = urls.indexOf(url);
    assert.notEqual(index, -1, 'unexpected or unscoped provider URL: ' + url);
    return new Response(JSON.stringify(data[index]), {
      status: 200, headers: { 'content-type': 'application/json',
        ...(headers[index] || {}) },
    });
  };
  return { manifest, urls, calls, fetchImpl };
}

test('reads exact GitHub tag/main/test/scorecard/job evidence with GET only', async () => {
  const mock = mockFixture();
  const result = await readPublicationAuthority({
    manifest: mock.manifest, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: mock.fetchImpl,
  });
  assert.deepEqual(result, { repository: REPO, tag: TAG, commit: COMMIT,
    testRunId: 111, scorecardRunId: 222 });
  assert.deepEqual(mock.calls.map((x) => x.url), mock.urls);
  for (const { init } of mock.calls) {
    assert.equal(init.method, 'GET');
    assert.equal(init.redirect, 'error');
    assert.equal(init.cache, 'no-store');
    assert.equal(init.headers.Authorization, 'Bearer ' + TOKEN);
    assert.equal(init.headers['X-GitHub-Api-Version'], '2022-11-28');
  }
});

test('rejects tampered tag/main/revision or mismatched manifest before publication', async () => {
  for (const mutate of [
    (d) => { d[0].object.type = 'commit'; },
    (d) => { d[0].object.sha = 'c'.repeat(40); },
    (d) => { d[1].object.sha = 'c'.repeat(40); },
    (d) => { d[2].object.sha = 'c'.repeat(40); },
    (d) => { d[2].ref = 'refs/heads/old'; },
  ]) {
    const mock = mockFixture({ mutate });
    await assert.rejects(readPublicationAuthority({
      manifest: mock.manifest, publisherCommit: COMMIT,
      token: TOKEN, fetchImpl: mock.fetchImpl,
    }), Error);
  }
  const mock = mockFixture();
  await assert.rejects(readPublicationAuthority({
    manifest: mock.manifest, publisherCommit: 'c'.repeat(40),
    token: TOKEN, fetchImpl: mock.fetchImpl,
  }), Error);
});

test('rejects PR evidence, failed push, duplicate success or stale release candidate attempt', async () => {
  for (const mutate of [
    (d) => { d[3].workflow_runs[0].event = 'pull_request'; },
    (d) => { d[3].workflow_runs[0].conclusion = 'failure'; },
    (d) => { d[4].workflow_runs[0].head_sha = 'c'.repeat(40); },
    (d) => { d[5].jobs[0].run_attempt = 1; },
    (d) => { d[5].jobs[0].run_id = 999; },
    (d) => { d[5].jobs[0].conclusion = 'skipped'; },
    (d) => { d[5].jobs.push({ ...d[5].jobs[0] }); },
    (d) => { d[3].workflow_runs.push({ ...d[3].workflow_runs[0] }); },
  ]) {
    const mock = mockFixture({ mutate });
    await assert.rejects(readPublicationAuthority({
      manifest: mock.manifest, publisherCommit: COMMIT,
      token: TOKEN, fetchImpl: mock.fetchImpl,
    }), Error);
  }
});

test('does not trust partial workflow or job pages and oversized response counts', async () => {
  for (const mutate of [
    (d) => { d[3].total_count = 101; },
    (d) => { d[4].total_count = 101; },
    (d) => { d[5].total_count = 101; },
    (d) => { delete d[3].total_count; },
    (d) => { d[4].total_count = 0; },
    (d) => { d[5].total_count = 2; },
  ]) {
    const mock = mockFixture({ mutate });
    await assert.rejects(readPublicationAuthority({
      manifest: mock.manifest, publisherCommit: COMMIT,
      token: TOKEN, fetchImpl: mock.fetchImpl,
    }), Error);
  }
  const mock = mockFixture({ headers: {
    3: { link: '<https://api.github.com/next>; rel="next"' },
  } });
  await assert.rejects(readPublicationAuthority({
    manifest: mock.manifest, publisherCommit: COMMIT,
    token: TOKEN, fetchImpl: mock.fetchImpl,
  }), Error);
});

test('rejects invalid authorization input and network failures without retries', async () => {
  const mock = mockFixture();
  for (const args of [
    { token: '' }, { token: 'x\nunsafe' },
    { publisherCommit: 'main' },
    { manifest: { ...mock.manifest, tag: '../unsafe' } },
  ]) {
    await assert.rejects(readPublicationAuthority({
      manifest: mock.manifest, publisherCommit: COMMIT,
      token: TOKEN, fetchImpl: mock.fetchImpl, ...args,
    }), Error);
  }
  assert.equal(mock.calls.length, 0);
  let count = 0;
  await assert.rejects(readPublicationAuthority({
    manifest: mock.manifest, publisherCommit: COMMIT, token: TOKEN,
    fetchImpl: async () => { count++; throw new Error('connection lost'); },
  }), /connection lost/);
  assert.equal(count, 1);
});
