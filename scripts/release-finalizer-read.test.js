'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { readReleaseObservation, discoverReleaseByTag,
  readImmutableReleasePolicy, readProtectedFinalizerEnvironmentPolicy,
  readVerifiedImmutablePublication } =
  require('./release-finalizer-read');
const { planReleaseFinalizerStep } = require('./release-finalizer-plan');

const TAG = 'v3.4.0';
const TOKEN = 'test_private_token_without_network';
const RELEASE_ID = 123;
const BASE = 'https://api.github.com/repos/zoster81/scripthold/releases';
const RELEASE_URL = BASE + '/' + RELEASE_ID;
const ASSETS_URL = RELEASE_URL + '/assets?per_page=100&page=1';

function fixture() {
  const release = { id: RELEASE_ID, tag_name: TAG, draft: true, immutable: false };
  const assets = expectedReleaseAssetNames(TAG).map((name, i) => ({
    id: i + 10, name, size: i + 100, state: 'uploaded',
    digest: 'sha256:' + (i + 1).toString(16).padStart(64, '0'),
  }));
  return { release, assets };
}

function response(value, status = 200, headers = {}) {
  return new Response(value === null ? null : JSON.stringify(value), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', ...headers },
  });
}

function client({ release, assets, errors = {}, mutate = null } = {}) {
  const calls = [];
  let releaseReads = 0;
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (url === RELEASE_URL) {
      releaseReads++;
      return errors['release' + releaseReads] ||
        response(releaseReads === 2 && mutate ? mutate(release) : release);
    }
    assert.equal(url, ASSETS_URL, 'only expected release assets may be fetched');
    return errors.assets || response(assets, 200, errors.assetsHeaders);
  };
  return { fetchImpl, calls };
}

test('authenticates only exact read-only GitHub endpoints and returns a bound draft inventory', async () => {
  const { release, assets } = fixture();
  const fake = client({ release, assets });
  const observation = await readReleaseObservation({
    releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  });
  assert.equal(observation.kind, 'present');
  assert.deepEqual(observation.release, release);
  assert.deepEqual(observation.assetPage.assets, assets);
  assert.deepEqual(fake.calls.map((call) => call.url), [RELEASE_URL, ASSETS_URL, RELEASE_URL]);
  for (const { init } of fake.calls) {
    assert.equal(init.method, 'GET');
    assert.equal(init.redirect, 'error');
    assert.equal(init.cache, 'no-store');
    assert.equal(init.headers.Authorization, 'Bearer ' + TOKEN);
    assert.equal(init.headers.Accept, 'application/vnd.github+json');
    assert.equal(init.headers['X-GitHub-Api-Version'], '2022-11-28');
  }
  assert.notEqual(observation.release, release);
  assert.notEqual(observation.assetPage.assets[0], assets[0]);
});

test('recognizes authenticated 404 only as an absence observation, never permission to create', async () => {
  const { release, assets } = fixture();
  const fake = client({ release, assets, errors: { release1: response({ message: 'Not Found' }, 404) } });
  assert.deepEqual(await readReleaseObservation({
    releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), { kind: 'absent', status: 404 });
  assert.equal(fake.calls.length, 1);
});

test('rejects invalid identity, token and transport before issuing any request', async () => {
  const { release, assets } = fixture();
  const fake = client({ release, assets });
  for (const patch of [
    { releaseId: 0 }, { releaseId: '123' }, { releaseId: 2 ** 53 },
    { tag: '../escape' }, { tag: 'v3.3.0/../../' },
    { token: '' }, { token: 'bad\r\nHeader: injected' },
    { fetchImpl: null },
  ]) {
    await assert.rejects(readReleaseObservation({
      releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl, ...patch,
    }), Error);
  }
  assert.equal(fake.calls.length, 0);
});

test('fails closed on authentication errors, redirects, rate limits and malformed GitHub JSON', async () => {
  const { release, assets } = fixture();
  for (const bad of [
    response({ message: 'Unauthorized' }, 401),
    response({ message: 'Forbidden' }, 403),
    response({ message: 'Busy' }, 500),
    response({ message: 'Limited' }, 429),
    response(null, 302, { Location: 'https://example.org/redirect' }),
    new Response('oops', { status: 200, headers: { 'content-type': 'text/plain' } }),
    new Response('{', { status: 200, headers: { 'content-type': 'application/json' } }),
    new Response(new Uint8Array([0xff, 0xfe]), {
      status: 200, headers: { 'content-type': 'application/json' },
    }),
  ]) {
    const fake = client({ release, assets, errors: { release1: bad } });
    await assert.rejects(readReleaseObservation({
      releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
});

test('detects release identity/state changing across its asset read', async () => {
  const { release, assets } = fixture();
  for (const mutate of [
    (r) => ({ ...r, draft: false }),
    (r) => ({ ...r, immutable: true }),
    (r) => ({ ...r, tag_name: 'v3.3.0' }),
    (r) => ({ ...r, id: RELEASE_ID + 1 }),
    (r) => ({ ...r, name: 'Tampered title' }),
    (r) => ({ ...r, body: 'Tampered release notes' }),
    (r) => ({ ...r, prerelease: true }),
  ]) {
    const fake = client({ release, assets, mutate });
    await assert.rejects(readReleaseObservation({
      releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
});

test('rejects asset pagination and unbounded or inconsistent release responses', async () => {
  const { release, assets } = fixture();
  for (const patch of [
    { assetsHeaders: { Link: '<https://api.github.com/next>; rel="next"' } },
    { assets: assets.concat({ ...assets[0], id: 999 }) },
    { assets: null },
  ]) {
    const fake = client({ release,
      assets: Object.hasOwn(patch, 'assets') ? patch.assets : assets,
      errors: { assetsHeaders: patch.assetsHeaders },
    });
    await assert.rejects(readReleaseObservation({
      releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
  for (const mismatch of [
    { ...release, id: RELEASE_ID + 1 },
    { ...release, tag_name: 'v1.0.0' },
    { ...release, draft: 'true' },
    { ...release, immutable: undefined },
  ]) {
    const fake = client({ release: mismatch, assets });
    await assert.rejects(readReleaseObservation({
      releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
});

test('reader and planner agree on a real API-shaped complete draft and a partial draft', async () => {
  const { release, assets } = fixture();
  const manifest = {
    repository: 'zoster81/scripthold', tag: TAG, commit: 'a'.repeat(40),
    assets: assets.map((asset) => ({
      name: asset.name, size: asset.size, sha256: asset.digest.slice(7),
    })),
  };
  const run = (id, path) => ({
    id, run_number: id, run_attempt: 1, head_branch: 'main',
    head_sha: manifest.commit, event: 'push', conclusion: 'success', path,
    head_repository: { full_name: manifest.repository },
  });
  const authority = {
    repository: manifest.repository, tag: TAG, commit: manifest.commit,
    publisherCommit: manifest.commit, manifest,
    tagRef: { ref: 'refs/tags/' + TAG, object: { type: 'tag', sha: 'b'.repeat(40) } },
    tagObject: { tag: TAG, sha: 'b'.repeat(40),
      object: { type: 'commit', sha: manifest.commit } },
    mainRef: { ref: 'refs/heads/main',
      object: { type: 'commit', sha: manifest.commit } },
    testRuns: { workflow_runs: [run(11, '.github/workflows/test.yml')] },
    scorecardRuns: { workflow_runs: [run(12, '.github/workflows/scorecard.yml')] },
    testJobs: { jobs: [{ name: 'Release candidate', status: 'completed',
      conclusion: 'success', run_id: 11, run_attempt: 1, head_sha: manifest.commit }] },
  };
  for (const [subset, expectedStep] of [
    [assets.slice(0, 4), 'upload-candidate'],
    [assets, 'publish-candidate'],
  ]) {
    const fake = client({ release, assets: subset });
    const observation = await readReleaseObservation({
      releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    });
    assert.equal(planReleaseFinalizerStep(authority, observation).step, expectedStep);
  }
});


function releaseListFetch({ pages, release = fixture().release, assets = fixture().assets,
  pageHeaders = {}, responseErrors = {}, mutate = null }) {
  const calls = [];
  let releaseReads = 0;
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (Object.hasOwn(responseErrors, url)) return responseErrors[url];
    if (url === RELEASE_URL) {
      releaseReads++;
      return response(releaseReads === 2 && mutate ? mutate(release) : release);
    }
    if (url === ASSETS_URL) return response(assets);
    const page = Number(new URL(url).searchParams.get('page'));
    assert.equal(url, BASE + '?per_page=30&page=' + page);
    return response(pages[page - 1] || [], 200, pageHeaders[page] || {});
  };
  return { calls, fetchImpl };
}

test('discovers a unique draft in a complete, bounded authenticated release list', async () => {
  const { release, assets } = fixture();
  const releases = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v3.3.' + i, draft: false, immutable: false,
  }));
  const fake = releaseListFetch({
    pages: [releases, [
      { id: 44, tag_name: 'v3.3.30', draft: false, immutable: false },
      release,
    ]], release, assets,
  });
  const result = await discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  });
  assert.deepEqual(result, {
    kind: 'candidate', releaseId: RELEASE_ID,
    observation: { kind: 'present', release,
      assetPage: { status: 200, request: { releaseId: RELEASE_ID, page: 1, perPage: 100 },
        link: null, assets } },
  });
  assert.deepEqual(fake.calls.map((c) => c.url), [
    BASE + '?per_page=30&page=1', BASE + '?per_page=30&page=2',
    RELEASE_URL, ASSETS_URL, RELEASE_URL,
  ]);
  for (const { init } of fake.calls) {
    assert.equal(init.method, 'GET');
    assert.equal(init.redirect, 'error');
    assert.equal(init.cache, 'no-store');
  }
});

test('does not convert an apparently absent release into creation permission', async () => {
  const fake = releaseListFetch({ pages: [[
    { id: 999, tag_name: 'v3.3.0', draft: false, immutable: false },
  ]] });
  const result = await discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  });
  assert.deepEqual(result, { kind: 'not-found-unproven', tag: TAG });
  assert.equal(fake.calls.length, 1);
});

test('rejects duplicate release tags, duplicate IDs and malformed list entries', async () => {
  const { release } = fixture();
  for (const page of [
    [release, { ...release, id: 124 }],
    [release, { ...release, tag_name: 'v3.4.1' }],
    [{ ...release, id: '123' }],
    [{ ...release, draft: 'false' }],
    [{ ...release, immutable: undefined }],
    [null],
    [{}],
  ]) {
    const fake = releaseListFetch({ pages: [page] });
    await assert.rejects(discoverReleaseByTag({
      tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
});

test('rejects incomplete, forged or inconsistent paginated GitHub lists', async () => {
  const prefix = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.1.' + i, draft: false, immutable: false,
  }));
  for (const headers of [
    { link: '<https://evil.example/releases?per_page=30&page=2>; rel="next"' },
    { link: '<https://api.github.com/repos/zoster81/scripthold/releases?per_page=30&page=3>; rel="next"' },
    { link: '<https://api.github.com/repos/zoster81/scripthold/releases?per_page=30&page=2>; rel="next", <https://api.github.com/repos/zoster81/scripthold/releases?per_page=30&page=2>; rel="next"' },
    { link: 'malformed' },
  ]) {
    const fake = releaseListFetch({ pages: [prefix, []], pageHeaders: { 1: headers } });
    await assert.rejects(discoverReleaseByTag({
      tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
  const fake = releaseListFetch({ pages: [prefix.slice(0, 2)],
    pageHeaders: { 1: { link:
      '<https://api.github.com/repos/zoster81/scripthold/releases?per_page=30&page=2>; rel="next"' } } });
  await assert.rejects(discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), Error);
});

test('rejects premature release-list completion against an advertised last page', async () => {
  const { release } = fixture();
  const first = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.4.' + i, draft: false, immutable: false,
  }));
  first[0] = release;
  const next = BASE + '?per_page=30&page=2';
  const last = BASE + '?per_page=30&page=3';
  const fake = releaseListFetch({
    pages: [first, [{ id: 5000, tag_name: 'v2.5.0', draft: false, immutable: false }]],
    pageHeaders: { 1: { Link:
      '<' + next + '>; rel="next", <' + last + '>; rel="last"' } },
  });
  await assert.rejects(discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /pagination|incomplete|last page/);
  assert.equal(fake.calls.some((call) => call.url === RELEASE_URL), false);
});

test('rejects a next page inconsistent with the advertised last page', async () => {
  const first = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.0.' + i, draft: false, immutable: false,
  }));
  const next = BASE + '?page=2&per_page=30';
  const current = BASE + '?page=1&per_page=30';
  const fake = releaseListFetch({
    pages: [first, [fixture().release]],
    pageHeaders: { 1: { Link:
      '<' + next + '>; rel="next", <' + current + '>; rel="last"' } },
  });
  await assert.rejects(discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /pagination|last page/);
  assert.equal(fake.calls.length, 1);
});

test('rejects a changed advertised last page across authenticated reads', async () => {
  const first = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.0.' + i, draft: false, immutable: false,
  }));
  const second = Array.from({ length: 30 }, (_, i) => ({
    id: i + 2000, tag_name: 'v2.1.' + i, draft: false, immutable: false,
  }));
  const page2 = BASE + '?page=2&per_page=30';
  const page3 = BASE + '?page=3&per_page=30';
  const page4 = BASE + '?page=4&per_page=30';
  const fake = releaseListFetch({
    pages: [first, second],
    pageHeaders: {
      1: { Link: '<' + page2 + '>; rel="next", <' + page3 + '>; rel="last"' },
      2: { Link: '<' + page3 + '>; rel="next", <' + page4 + '>; rel="last"' },
    },
  });
  await assert.rejects(discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /inconsistent.*last page/);
  assert.equal(fake.calls.length, 2);
});

test('accepts an exactly full advertised final page after checking the empty tail', async () => {
  const { release } = fixture();
  const first = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.0.' + i, draft: false, immutable: false,
  }));
  first[0] = release;
  const second = Array.from({ length: 30 }, (_, i) => ({
    id: i + 2000, tag_name: 'v2.1.' + i, draft: false, immutable: false,
  }));
  const next = BASE + '?page=2&per_page=30';
  const prior = BASE + '?page=1&per_page=30';
  const fake = releaseListFetch({
    pages: [first, second, []],
    pageHeaders: {
      1: { Link: '<' + next + '>; rel="next", <' + next + '>; rel="last"' },
      2: { Link: '<' + prior + '>; rel="prev", <' + prior + '>; rel="first"' },
    },
  });
  const result = await discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  });
  assert.equal(result.kind, 'candidate');
  assert.equal(result.releaseId, RELEASE_ID);
  assert.deepEqual(fake.calls.slice(0, 3).map((call) => call.url), [
    BASE + '?per_page=30&page=1', BASE + '?per_page=30&page=2',
    BASE + '?per_page=30&page=3',
  ]);
});

test('accepts actual GitHub-style next/last and prev/first pagination relations', async () => {
  const first = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.0.' + i, draft: false, immutable: false,
  }));
  const next = BASE + '?page=2&per_page=30';
  const prior = BASE + '?page=1&per_page=30';
  const fake = releaseListFetch({ pages: [first, [fixture().release]],
    pageHeaders: {
      1: { Link: '<' + next + '>; rel="next", <' + next + '>; rel="last"' },
      2: { Link: '<' + prior + '>; rel="prev", <' + prior + '>; rel="first"' },
    },
  });
  const result = await discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  });
  assert.equal(result.kind, 'candidate');
  assert.equal(result.releaseId, RELEASE_ID);
});

test('rejects an ID repeated on later pages even when its tag differs', async () => {
  const first = Array.from({ length: 30 }, (_, i) => ({
    id: i + 1000, tag_name: 'v2.0.' + i, draft: false, immutable: false,
  }));
  const fake = releaseListFetch({ pages: [first, [{
    id: first[0].id, tag_name: TAG, draft: true, immutable: false,
  }]] });
  await assert.rejects(discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /duplicate or malformed/);
});

test('rejects unauthorized, wrong-origin, wrong state and mutated candidate rechecks', async () => {
  const { release } = fixture();
  for (const bad of [response({ message: 'Forbidden' }, 403),
    response({ message: 'Not Found' }, 404),
    response({ message: 'limited' }, 429),
    new Response('bad JSON', { status: 200, headers: { 'content-type': 'application/json' } })]) {
    const fake = releaseListFetch({ pages: [[release]],
      responseErrors: { [BASE + '?per_page=30&page=1']: bad } });
    await assert.rejects(discoverReleaseByTag({
      tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
  for (const mutate of [
    (r) => ({ ...r, tag_name: 'v3.3.0' }),
    (r) => ({ ...r, draft: false }),
    (r) => ({ ...r, id: 9999 }),
  ]) {
    const fake = releaseListFetch({ pages: [[release]], mutate });
    await assert.rejects(discoverReleaseByTag({
      tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
    }), Error);
  }
});

test('never trusts truncated searches after a bounded release page limit', async () => {
  const pages = Array.from({ length: 21 }, (_, n) =>
    Array.from({ length: 30 }, (_, i) => ({
      id: n * 30 + i + 20000, tag_name: 'v5.' + n + '.' + i,
      draft: false, immutable: false,
    })));
  const fake = releaseListFetch({ pages });
  await assert.rejects(discoverReleaseByTag({
    tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), /page limit|bounded/);
  assert.ok(fake.calls.length <= 20);
});

test('reads GitHub immutable-release enforcement with a separate admin token and no writes', async () => {
  const calls = [];
  const fetchImpl = async (url, options) => {
    calls.push({ url, options });
    return response({ enabled: true, enforced_by_owner: false });
  };
  assert.deepEqual(await readImmutableReleasePolicy({
    adminToken: 'test_admin_token_not_live', fetchImpl,
  }), { enabled: true, enforcedByOwner: false });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url,
    'https://api.github.com/repos/zoster81/scripthold/immutable-releases');
  assert.equal(calls[0].options.method, 'GET');
  assert.equal(calls[0].options.headers.Authorization,
    'Bearer test_admin_token_not_live');
  assert.equal(calls[0].options.redirect, 'error');
  assert.equal(calls[0].options.cache, 'no-store');
});

test('immutable-release policy refuses disabled, ambiguous, inaccessible and malformed evidence', async () => {
  for (const bad of [
    response({ enabled: false, enforced_by_owner: false }),
    response({ enabled: true }),
    response({ enabled: 'true', enforced_by_owner: false }),
    response({ enabled: true, enforced_by_owner: 'false' }),
    response(null, 403),
    response(null, 404),
    response({ message: 'Unauthorized' }, 401),
    new Response('not JSON', { status: 200, headers: { 'content-type': 'application/json' } }),
  ]) {
    let calls = 0;
    await assert.rejects(readImmutableReleasePolicy({
      adminToken: 'test_admin_token_not_live',
      fetchImpl: async () => { calls++; return bad; },
    }), Error);
    assert.equal(calls, 1);
  }
  let calls = 0;
  for (const adminToken of ['', 'header\r\ninj', undefined]) {
    await assert.rejects(readImmutableReleasePolicy({
      adminToken, fetchImpl: async () => { calls++; return response({ enabled: true }); },
    }), Error);
  }
  assert.equal(calls, 0);
});

test('protected finalizer policy checks exact GitHub reviewer, tag-only refs and no admin bypass', async () => {
  const envUrl = 'https://api.github.com/repos/zoster81/scripthold/environments/scripthold-immutable-publish';
  const refsUrl = envUrl + '/deployment-branch-policies';
  const environment = () => ({
    name: 'scripthold-immutable-publish', can_admins_bypass: false,
    deployment_branch_policy: { protected_branches: false, custom_branch_policies: true },
    protection_rules: [
      { type: 'required_reviewers', prevent_self_review: false,
        reviewers: [{ type: 'User', reviewer: { id: 7560683, login: 'zoster81' } }] },
      { type: 'branch_policy' },
    ],
  });
  const policies = () => ({
    total_count: 1, branch_policies: [{ name: 'v*', type: 'tag' }],
  });
  const run = async (env, refs, badHeaders = {}) => {
    const calls = [];
    const fetchImpl = async (url, init) => {
      calls.push({ url, init });
      assert.equal(init.method, 'GET');
      assert.equal(init.headers.Authorization, 'Bearer admin_only_test_token');
      assert.equal(init.redirect, 'error');
      assert.equal(init.cache, 'no-store');
      if (url === envUrl) return response(env, 200, badHeaders);
      assert.equal(url, refsUrl);
      return response(refs);
    };
    return { calls, check: () => readProtectedFinalizerEnvironmentPolicy({
      adminToken: 'admin_only_test_token', fetchImpl,
    }) };
  };
  const valid = await run(environment(), policies());
  assert.deepEqual(await valid.check(), { protected: true, environment: 'scripthold-immutable-publish' });
  assert.deepEqual(valid.calls.map(x => x.url), [envUrl, refsUrl]);

  const cases = [
    [{ ...environment(), can_admins_bypass: true }, policies()],
    [{ ...environment(), can_admins_bypass: undefined }, policies()],
    [{ ...environment(), name: 'unprotected' }, policies()],
    [{ ...environment(), deployment_branch_policy: {
      protected_branches: true, custom_branch_policies: false } }, policies()],
    [{ ...environment(), protection_rules: [{ type: 'branch_policy' }] }, policies()],
    [{ ...environment(), protection_rules: environment().protection_rules.concat(
      environment().protection_rules[0]) }, policies()],
    [{ ...environment(), protection_rules: [
      { ...environment().protection_rules[0], reviewers: [] },
      { type: 'branch_policy' }] }, policies()],
    [{ ...environment(), protection_rules: [
      { ...environment().protection_rules[0], reviewers: [
        { type: 'User', reviewer: { login: 'another-maintainer', id: 1 } }] },
      { type: 'branch_policy' }] }, policies()],
    [environment(), { total_count: 0, branch_policies: [] }],
    [environment(), { total_count: 1, branch_policies: [{ name: 'main', type: 'branch' }] }],
    [environment(), { total_count: 2, branch_policies: [
      { name: 'v*', type: 'tag' }, { name: '*', type: 'branch' }] }],
  ];
  for (const [env, refs] of cases) {
    const fake = await run(env, refs);
    await assert.rejects(fake.check(), /protected|environment|tag policy/i);
    assert.ok(fake.calls.length <= 2);
    assert.equal(fake.calls.filter(x => x.init.method !== 'GET').length, 0);
  }
  const linked = await run(environment(), policies(), {
    link: '<https://api.github.com/next>; rel="next"',
  });
  await assert.rejects(linked.check(), /protected|pagination|environment/i);
  for (const [bad, status] of [[{ message: 'Forbidden' }, 403],
    [{ message: 'Not Found' }, 404]]) {
    const fetchImpl = async () => response(bad, status);
    await assert.rejects(readProtectedFinalizerEnvironmentPolicy({
      adminToken: 'admin_only_test_token', fetchImpl,
    }), Error);
  }
});

test('independently reads an exact 22-asset immutable published release', async () => {
  const { release, assets } = fixture();
  const manifest = { repository: 'zoster81/scripthold', tag: TAG,
    commit: 'a'.repeat(40),
    assets: assets.map(({ name, size, digest }) => ({
      name, size, sha256: digest.slice('sha256:'.length),
    })),
  };
  const published = { ...release, draft: false, immutable: true };
  const fake = client({ release: published, assets });
  assert.deepEqual(await readVerifiedImmutablePublication({
    manifest, releaseId: RELEASE_ID, token: TOKEN, fetchImpl: fake.fetchImpl,
  }), { releaseId: RELEASE_ID, count: 22 });
  assert.deepEqual(fake.calls.map(c => c.url),
    [RELEASE_URL, ASSETS_URL, RELEASE_URL]);

  for (const [remote, original] of [
    [{ ...published, immutable: false }, assets],
    [{ ...published, draft: true }, assets],
    [published, assets.slice(1)],
    [published, assets.map((asset, i) =>
      i === 3 ? { ...asset, digest: 'sha256:' + '0'.repeat(64) } : asset)],
    [published, assets.map((asset, i) =>
      i === 3 ? { ...asset, state: 'starter' } : asset)],
  ]) {
    const bad = client({ release: remote, assets: original });
    await assert.rejects(readVerifiedImmutablePublication({
      manifest, releaseId: RELEASE_ID, token: TOKEN, fetchImpl: bad.fetchImpl,
    }), Error);
  }
});

test('bounds response bytes and propagates network/stream failures without retrying', async () => {
  const { release, assets } = fixture();
  const requests = [];
  const tooLarge = new Response(' '.repeat(600000), {
    headers: { 'content-type': 'application/json' },
  });
  const fetchImpl = async (url) => { requests.push(url); return tooLarge; };
  await assert.rejects(readReleaseObservation({
    releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl,
  }), Error);
  assert.equal(requests.length, 1);

  let count = 0;
  await assert.rejects(readReleaseObservation({
    releaseId: RELEASE_ID, tag: TAG, token: TOKEN,
    fetchImpl: async () => { count++; throw new Error('connection lost'); },
  }), /connection lost/);
  assert.equal(count, 1);

  const fake = client({ release, assets });
  await assert.doesNotReject(readReleaseObservation({
    releaseId: RELEASE_ID, tag: TAG, token: TOKEN, fetchImpl: fake.fetchImpl,
  }));
});
