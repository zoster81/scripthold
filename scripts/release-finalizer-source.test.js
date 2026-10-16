'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { inspectFinalizerSource, resolveSourceActionContext } =
  require('./release-finalizer-source');

const TAG = 'v3.4.0';
const SHA = 'a'.repeat(40);
const OBJECT_SHA = 'b'.repeat(40);
const TOKEN = 'test_token_injected_not_real';
const REPO = 'zoster81/scripthold';
const API = 'https://api.github.com/repos/zoster81/scripthold';

const pushEnv = () => ({
  GITHUB_EVENT_NAME: 'push',
  GITHUB_REF: 'refs/tags/' + TAG,
  GITHUB_REPOSITORY: REPO,
  RELEASE_TAG_INPUT: '',
  GITHUB_SHA: SHA,
  GITHUB_TOKEN: TOKEN,
});
const manualEnv = () => ({
  ...pushEnv(), GITHUB_EVENT_NAME: 'workflow_dispatch',
  GITHUB_REF: 'refs/heads/main', RELEASE_TAG_INPUT: TAG,
});

function provider({ tagType = 'tag', tagSha = OBJECT_SHA, targetSha = SHA,
  mainSha = SHA, refSwap = false, status = 200, wrongResponse = null } = {}) {
  const requests = [];
  let tagRefs = 0;
  const urls = [
    API + '/git/ref/tags/' + TAG,
    API + '/git/tags/' + OBJECT_SHA,
    API + '/git/ref/heads/main',
    API + '/git/ref/tags/' + TAG,
  ];
  const fetchImpl = async (url, init) => {
    requests.push({ url, init });
    assert.equal(url, urls[requests.length - 1], 'untrusted GitHub API URL');
    let value;
    if (requests.length === 1 || requests.length === 4) {
      tagRefs++;
      value = { ref: 'refs/tags/' + TAG, object: {
        type: tagType,
        sha: refSwap && tagRefs === 2 ? 'c'.repeat(40) : tagSha,
      } };
    } else if (requests.length === 2) {
      value = { sha: OBJECT_SHA, tag: TAG, object: { type: 'commit', sha: targetSha } };
    } else {
      value = { ref: 'refs/heads/main', object: { type: 'commit', sha: mainSha } };
    }
    return wrongResponse ?? new Response(JSON.stringify(value), {
      status, headers: { 'content-type': 'application/json' },
    });
  };
  return { requests, fetchImpl };
}

test('validates tag push and manual-main checkout from real read-only GitHub objects', async () => {
  for (const env of [pushEnv(), manualEnv()]) {
    const f = provider();
    const result = await inspectFinalizerSource({ env, fetchImpl: f.fetchImpl });
    assert.deepEqual(result, {
      repository: REPO, tag: TAG, commit: SHA,
      tagObjectSha: OBJECT_SHA, group: 'scripthold-immutable-release-' + TAG,
    });
    assert.equal(f.requests.length, 4);
    for (const { init } of f.requests) {
      assert.equal(init.method, 'GET');
      assert.equal(init.redirect, 'error');
      assert.equal(init.cache, 'no-store');
      assert.equal(init.headers.Authorization, 'Bearer ' + TOKEN);
      assert.equal(init.headers['X-GitHub-Api-Version'], '2022-11-28');
    }
  }
});

test('rejects untrusted event, checkout SHA, token and tag input before any network', async () => {
  const mutations = [
    { GITHUB_EVENT_NAME: 'pull_request' },
    { GITHUB_REF: 'refs/heads/feature' },
    { GITHUB_REPOSITORY: 'untrusted/other' },
    { GITHUB_SHA: 'main' },
    { GITHUB_SHA: SHA.toUpperCase() },
    { GITHUB_TOKEN: '' },
    { GITHUB_TOKEN: 'x\nHeader: unsafe' },
    { RELEASE_TAG_INPUT: TAG },
  ];
  for (const change of mutations) {
    const f = provider();
    await assert.rejects(inspectFinalizerSource({
      env: { ...pushEnv(), ...change }, fetchImpl: f.fetchImpl,
    }), Error);
    assert.equal(f.requests.length, 0);
  }
});

test('rejects unannotated and replaced tags even if they point to the same commit', async () => {
  for (const opts of [
    { tagType: 'commit' },
    { tagSha: 'c'.repeat(40) },
    { refSwap: true },
  ]) {
    const f = provider(opts);
    await assert.rejects(inspectFinalizerSource({
      env: pushEnv(), fetchImpl: f.fetchImpl,
    }), Error);
  }
});

test('refuses a tag pointing to a different commit or main moving independently', async () => {
  for (const opts of [{ targetSha: 'c'.repeat(40) },
    { mainSha: 'c'.repeat(40) }]) {
    const f = provider(opts);
    await assert.rejects(inspectFinalizerSource({
      env: manualEnv(), fetchImpl: f.fetchImpl,
    }), Error);
  }
  const staleCheckout = provider();
  await assert.rejects(inspectFinalizerSource({
    env: { ...pushEnv(), GITHUB_SHA: 'c'.repeat(40) },
    fetchImpl: staleCheckout.fetchImpl,
  }), /checkout commit/);
  assert.equal(staleCheckout.requests.length, 2);
});

test('rejects inconsistent Git tag object and missing annotations', async () => {
  for (const mutate of [
    (value) => { value.sha = 'd'.repeat(40); },
    (value) => { value.tag = 'v3.3.0'; },
    (value) => { value.object.type = 'tag'; },
    (value) => { value.object.sha = 'not-sha'; },
  ]) {
    let n = 0;
    const f = provider();
    const fetchImpl = async (url, init) => {
      const response = await f.fetchImpl(url, init);
      n++;
      if (n !== 2) return response;
      const value = await response.json();
      mutate(value);
      return new Response(JSON.stringify(value),
        { headers: { 'content-type': 'application/json' } });
    };
    await assert.rejects(inspectFinalizerSource({
      env: pushEnv(), fetchImpl,
    }), Error);
  }
});

test('fails closed on unauthorized HTTP, redirects, malformed JSON and network failures', async () => {
  for (const response of [
    new Response('{}', { status: 403, headers: { 'content-type': 'application/json' } }),
    new Response('{}', { status: 429, headers: { 'content-type': 'application/json' } }),
    new Response('bad JSON', { headers: { 'content-type': 'application/json' } }),
    new Response('<html>error</html>', { headers: { 'content-type': 'text/html' } }),
    new Response('{}', { status: 302, headers: { location: 'https://other.test' } }),
  ]) {
    const f = provider({ wrongResponse: response });
    await assert.rejects(inspectFinalizerSource({
      env: pushEnv(), fetchImpl: f.fetchImpl,
    }), Error);
    assert.equal(f.requests.length, 1);
  }
  let count = 0;
  await assert.rejects(inspectFinalizerSource({
    env: pushEnv(),
    fetchImpl: async () => { count++; throw new Error('lost connection'); },
  }), /lost connection/);
  assert.equal(count, 1);
});

test('source-only action context retains manual and tag group identity', () => {
  assert.deepEqual(resolveSourceActionContext(pushEnv()),
    resolveSourceActionContext(manualEnv()));
  assert.deepEqual(resolveSourceActionContext(pushEnv()), {
    repository: REPO, tag: TAG, group: 'scripthold-immutable-release-' + TAG,
  });
});
