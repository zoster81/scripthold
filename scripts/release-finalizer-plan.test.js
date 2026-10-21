'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { uploadOneDraftAsset, uploadAllDraftAssets, createDraftRelease,
  prepareDraftAssets, publishCompleteDraft,
  finalizeCompleteRelease } = require('./release-finalizer-write');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { encodeGoReleaserNotes } = require('./release-normal-source');
const { normalizeReleaseAssetPage, planReleaseFinalizerStep } =
  require('./release-finalizer-plan');

const COMMIT = 'a'.repeat(40);
const TAG_OBJECT = 'b'.repeat(40);
const TAG = 'v3.4.0';
const REPO = 'zoster81/scripthold';
const API = 'https://api.github.com/repos/zoster81/scripthold';
const NOTES = 'Trusted GoReleaser notes for next version\n';
const notesMetadata = () => encodeGoReleaserNotes(Buffer.from(NOTES));
const validWriterContext = () => ({
  eventName: 'push', repository: REPO, ref: 'refs/tags/' + TAG,
  workflowRef: REPO + '/.github/workflows/release.yml@refs/tags/' + TAG,
  sha: COMMIT, job: 'finalize', runId: '12345', runAttempt: '1',
});

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

test('draft asset writer refuses ambiguous or conflicting release states before any upload', async () => {
  const { uploadOneDraftAsset } = require('./release-finalizer-write');
  const { authority, remote } = fixture();
  const calls = [];
  const fake = async (url, options) => {
    calls.push({ url, method: options?.method });
    return new Response('', { status: 403 });
  };
  await assert.rejects(uploadOneDraftAsset({
    manifest: authority.manifest,
    publisherCommit: COMMIT,
    token: 'test_token_not_live',
    normalDir: 'not-an-authorized-source',
    mcpbDir: 'not-an-authorized-source',
    fetchImpl: fake,
  }));
  assert.equal(calls.filter(x => x.method !== 'GET').length, 0);
  assert.equal(remote.length, 22);
});

function simulatedGitHubDraft({ response = 'success', changeBeforePost = false,
  preverifiedRemoteAssets = false, metadataPatch = {}, changeNotesBeforePost = false,
  startAbsent = false, publicationPolicy = 'enabled', publishResponse = 'success',
  environmentPolicy = 'protected' } = {}) {
  const original = Buffer.from('verified original test binary bytes');
  const sha256 = createHash('sha256').update(original).digest('hex');
  const assets = expectedReleaseAssetNames(TAG).map((name, index) => ({
    name, size: index === 0 ? original.length : 1,
    sha256: index === 0 ? sha256 : startAbsent ?
      createHash('sha256').update(Buffer.alloc(1)).digest('hex') : 'f'.repeat(64),
  }));
  const manifest = { repository: REPO, tag: TAG, commit: COMMIT, assets };
  let uploaded = false;
  const uploadedAssets = [];
  let created = !startAbsent;
  let published = false;
  let publishCount = 0;
  let policyReads = 0;
  let environmentReads = 0;
  let createCount = 0;
  let postCount = 0;
  let lookupCount = 0;
  let releaseReads = 0;
  const calls = [];
  const tagObject = 'b'.repeat(40);
  const run = (id, workflowPath) => ({
    id, run_number: id, run_attempt: 1, head_sha: COMMIT,
    head_branch: 'main', event: 'push', conclusion: 'success',
    path: workflowPath, head_repository: { full_name: REPO },
  });
  const fetchImpl = async (url, options) => {
    calls.push({ url, method: options.method });
    if (url === API + '/environments/scripthold-immutable-publish') {
      assert.equal(options.method, 'GET');
      assert.equal(options.headers.Authorization, 'Bearer admin_only_test_token');
      environmentReads++;
      return new Response(JSON.stringify({
        name: 'scripthold-immutable-publish',
        can_admins_bypass: environmentPolicy === 'bypass' ||
          (environmentPolicy === 'flip' && environmentReads > 1),
        deployment_branch_policy: {
          protected_branches: false, custom_branch_policies: true,
        },
        protection_rules: [
          { type: 'required_reviewers', prevent_self_review: false,
            reviewers: [{ type: 'User',
              reviewer: { id: 7560683, login: 'zoster81' } }] },
          { type: 'branch_policy' },
        ],
      }), { status: 200, headers: { 'content-type': 'application/json' } });
    }
    if (url === API + '/environments/scripthold-immutable-publish/deployment-branch-policies') {
      assert.equal(options.method, 'GET');
      assert.equal(options.headers.Authorization, 'Bearer admin_only_test_token');
      return new Response(JSON.stringify({
        total_count: 1, branch_policies: [{
          type: environmentPolicy === 'wrong-ref' ? 'branch' : 'tag',
          name: environmentPolicy === 'wrong-ref' ? 'main' : 'v*',
        }],
      }), { status: 200, headers: { 'content-type': 'application/json' } });
    }
    if (url === API + '/immutable-releases') {
      assert.equal(options.method, 'GET');
      assert.equal(options.headers.Authorization, 'Bearer admin_only_test_token');
      policyReads++;
      return new Response(JSON.stringify({ enabled: publicationPolicy === 'enabled' ||
        (publicationPolicy === 'flip' && policyReads === 1),
        enforced_by_owner: false }), { status: 200,
        headers: { 'content-type': 'application/json' } });
    }
    if (options.method === 'PATCH') {
      assert.equal(url, API + '/releases/123');
      assert.deepEqual(JSON.parse(options.body), { draft: false });
      publishCount++;
      assert.equal(published, false, 'never publish the same release twice');
      if (publishResponse === 'lost') throw new Error('lost GitHub publication response');
      published = true;
      return new Response(JSON.stringify({ id: 123, tag_name: TAG,
        draft: false, immutable: true, name: TAG, body: NOTES,
        prerelease: false }), { status: 200,
        headers: { 'content-type': 'application/json' } });
    }
    if (options.method === 'POST' && url === API + '/releases') {
      createCount++;
      assert.equal(created, false, 'a draft must not be created twice');
      const payload = JSON.parse(options.body);
      assert.equal(payload.draft, true);
      assert.equal(payload.body, NOTES);
      created = true;
      return new Response(JSON.stringify({ id: 123, tag_name: TAG,
        draft: true, immutable: false, name: TAG, body: NOTES,
        prerelease: false }), { status: 201,
        headers: { 'content-type': 'application/json' } });
    }
    if (startAbsent && options.method === 'POST' &&
        url.startsWith('https://uploads.github.com/repos/zoster81/scripthold/releases/123/assets?name=')) {
      postCount++;
      const name = new URL(url).searchParams.get('name');
      const index = assets.findIndex(asset => asset.name === name);
      assert.ok(index >= 0, 'only manifest originals may be uploaded');
      assert.ok(!uploadedAssets.some(asset => asset.name === name), 'no duplicated upload');
      const chunks = [];
      for await (const chunk of options.body) chunks.push(Buffer.from(chunk));
      assert.deepEqual(Buffer.concat(chunks), index === 0 ? original : Buffer.alloc(1));
      const asset = { id: 501 + index, name, size: assets[index].size,
        state: 'uploaded', digest: 'sha256:' + assets[index].sha256 };
      uploadedAssets.push(asset);
      return new Response(JSON.stringify(asset), { status: 201,
        headers: { 'content-type': 'application/json' } });
    }
    if (options.method === 'POST') {
      postCount++;
      assert.match(url, /^https:\/\/uploads\.github\.com\/repos\/zoster81\/scripthold\/releases\/123\/assets\?name=/);
      assert.equal(options.headers['Content-Type'], 'application/octet-stream');
      const received = [];
      for await (const data of options.body) received.push(Buffer.from(data));
      assert.deepEqual(Buffer.concat(received), original);
      if (response === 'lost') throw new Error('ambiguous GitHub upload response');
      if (response === 'conflict') return new Response('conflict', { status: 422 });
      uploaded = true;
      return new Response(JSON.stringify({
        id: 501, name: assets[0].name,
        state: 'uploaded', size: original.length,
        digest: 'sha256:' + sha256,
      }), { status: 201, headers: { 'content-type': 'application/json' } });
    }
    let body;
    if (url === API + '/git/ref/tags/' + TAG) {
      body = { ref: 'refs/tags/' + TAG, object: { type: 'tag', sha: tagObject } };
    } else if (url === API + '/git/tags/' + tagObject) {
      body = { tag: TAG, sha: tagObject, object: { type: 'commit', sha: COMMIT } };
    } else if (url === API + '/git/ref/heads/main') {
      body = { ref: 'refs/heads/main', object: { type: 'commit', sha: COMMIT } };
    } else if (url.includes('/actions/workflows/test.yml/runs?')) {
      body = { total_count: 1, workflow_runs: [run(11, '.github/workflows/test.yml')] };
    } else if (url.includes('/actions/workflows/scorecard.yml/runs?')) {
      body = { total_count: 1, workflow_runs: [run(12, '.github/workflows/scorecard.yml')] };
    } else if (url.includes('/actions/runs/11/attempts/1/jobs?')) {
      body = { total_count: 1, jobs: [{
        name: 'Release candidate', status: 'completed', conclusion: 'success',
        run_id: 11, run_attempt: 1, head_sha: COMMIT,
      }] };
    } else if (url === API + '/releases?per_page=30&page=1') {
      lookupCount++;
      body = created ? [{ id: 123, tag_name: TAG,
        draft: !published && !(changeBeforePost && lookupCount > 1),
        immutable: published }] : [];
    } else if (url === API + '/releases/tags/' + TAG && !created) {
      return new Response(null, { status: 404 });
    } else if (url === API + '/releases/123') {
      releaseReads++;
      body = { id: 123, tag_name: TAG,
        draft: !published && !(changeBeforePost && lookupCount > 1),
        immutable: published, name: TAG,
        body: changeNotesBeforePost && releaseReads > 2 ? 'Tampered notes\n' : NOTES,
        prerelease: false, ...metadataPatch };
    } else if (url === API + '/releases/123/assets?per_page=100&page=1') {
      body = startAbsent ? uploadedAssets : [
        ...(preverifiedRemoteAssets ?
          (preverifiedRemoteAssets === 'complete' ? assets : assets.slice(1))
            .map((a, index) => ({
              id: 600 + index, name: a.name, size: a.size,
              digest: 'sha256:' + a.sha256, state: 'uploaded',
            })) : []),
        ...(uploaded ? [{
          id: 501, name: assets[0].name,
          size: original.length, digest: 'sha256:' + sha256,
          state: 'uploaded',
        }] : []),
      ];
    } else {
      throw new Error('unexpected fake GitHub URL: ' + url);
    }
    return new Response(JSON.stringify(body), {
      status: 200, headers: { 'content-type': 'application/json' },
    });
  };
  return { manifest, original, fetchImpl, calls,
    get postCount() { return postCount; },
    get createCount() { return createCount; },
    get publishCount() { return publishCount; } };
}

test('existing draft rejects missing, forged and foreign GoReleaser notes before any API call', async () => {
  for (const releaseNotesMetadata of [
    undefined, NOTES, { ...notesMetadata(), size: 4 },
    { ...notesMetadata(), sha256: '0'.repeat(64) },
  ]) {
    const fake = simulatedGitHubDraft();
    await assert.rejects(uploadOneDraftAsset({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      releaseNotesMetadata, fetchImpl: fake.fetchImpl,
    }));
    assert.equal(fake.calls.length, 0);
    assert.equal(fake.postCount, 0);
  }
});

test('existing draft refuses foreign title, release body, prerelease and missing metadata', async () => {
  for (const metadataPatch of [
    { name: 'Other release' }, { body: 'Tampered notes\n' },
    { prerelease: true }, { body: null },
    { body: undefined }, { name: undefined }, { prerelease: undefined },
  ]) {
    const fake = simulatedGitHubDraft({ metadataPatch });
    await assert.rejects(uploadOneDraftAsset({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), /metadata|draft|body|note|release/i);
    assert.equal(fake.postCount, 0);
  }
});

test('changed release notes between source reads prevent even one draft upload', async () => {
  const fake = simulatedGitHubDraft({ changeNotesBeforePost: true });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-notes-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    for (const dir of [normalDir, mcpbDir]) fs.mkdirSync(dir, { mode: 0o700 });
    fs.writeFileSync(path.join(normalDir, fake.manifest.assets[0].name),
      fake.original, { mode: 0o600 });
    await assert.rejects(uploadOneDraftAsset({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      normalDir, mcpbDir, releaseNotesMetadata: notesMetadata(),
      fetchImpl: fake.fetchImpl,
    }));
    assert.equal(fake.postCount, 0);
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('uploads at most one independently confirmed original to an existing draft', async () => {
  const fake = simulatedGitHubDraft();
  const root = path.resolve(__dirname, '.test-finalizer-temp-');
  // Use the repository-controlled directory, never the host default tmp.
  const base = fs.mkdtempSync(root);
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    for (const dir of [normalDir, mcpbDir]) fs.mkdirSync(dir, { mode: 0o700 });
    fs.writeFileSync(path.join(normalDir, fake.manifest.assets[0].name),
      fake.original, { mode: 0o600 });
    const answer = await uploadOneDraftAsset({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network', normalDir, mcpbDir,
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    });
    assert.deepEqual(answer, { kind: 'uploaded', releaseId: 123,
      name: fake.manifest.assets[0].name, assetId: 501, remaining: 21 });
    assert.equal(fake.postCount, 1);
    assert.ok(fake.calls.every(call =>
      call.method === 'GET' || call.method === 'POST'));
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('bounded draft runner resumes one missing original then checks all twenty-two', async () => {
  const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: true });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-all-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    fs.writeFileSync(path.join(normalDir, fake.manifest.assets[0].name),
      fake.original, { mode: 0o600 });
    const result = await uploadAllDraftAssets({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network', normalDir, mcpbDir,
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    });
    assert.deepEqual(result, { kind: 'draft-complete', releaseId: 123, count: 22 });
    assert.equal(fake.postCount, 1, 'no retry or superfluous uploads');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('draft preparation resumes the existing authenticated draft without creating another', async () => {
  const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: true });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-prepare-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    for (const asset of fake.manifest.assets) {
      const dest = asset.name.endsWith('.mcpb') ||
        asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
      const bytes = asset.name === fake.manifest.assets[0].name ?
        fake.original : Buffer.alloc(asset.size, 0);
      fs.writeFileSync(path.join(dest, asset.name), bytes, { mode: 0o600 });
    }
    const result = await prepareDraftAssets({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network', normalDir, mcpbDir,
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    });
    assert.deepEqual(result, { kind: 'draft-complete', releaseId: 123, count: 22 });
    assert.equal(fake.postCount, 1, 'only the missing original may be uploaded');
    assert.equal(fake.calls.filter(call => call.url === API + '/releases' &&
      call.method === 'POST').length, 0, 'never create a second draft');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('draft preparation creates one draft and resumes without duplicate writes', async () => {
  const fake = simulatedGitHubDraft({ startAbsent: true });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-create-complete-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    for (const asset of fake.manifest.assets) {
      const dest = asset.name.endsWith('.mcpb') ||
        asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
      fs.writeFileSync(path.join(dest, asset.name),
        asset.name === fake.manifest.assets[0].name ?
          fake.original : Buffer.alloc(asset.size), { mode: 0o600 });
    }
    const input = {
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network', normalDir, mcpbDir,
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    };
    assert.deepEqual(await prepareDraftAssets(input),
      { kind: 'draft-complete', releaseId: 123, count: 22 });
    assert.deepEqual(await prepareDraftAssets(input),
      { kind: 'draft-complete', releaseId: 123, count: 22 });
    assert.equal(fake.createCount, 1, 'only one draft may be created');
    assert.equal(fake.postCount, 22, 'each of the 22 original assets uploaded once');

    // Each subsequent call must reject unsafe or changed disk inputs before
    // making even a read-only GitHub request, despite a complete remote draft.
    const previousRequests = fake.calls.length;
    const originalPath = path.join(normalDir, fake.manifest.assets[0].name);
    const extraPath = path.join(mcpbDir, 'unexpected-original');
    fs.writeFileSync(extraPath, Buffer.from('not an allowed original'));
    await assert.rejects(prepareDraftAssets(input), /inventory/i);
    assert.equal(fake.calls.length, previousRequests);
    fs.unlinkSync(extraPath);

    fs.writeFileSync(originalPath, Buffer.from('wrong size'));
    await assert.rejects(prepareDraftAssets(input), /unsafe original source/i);
    assert.equal(fake.calls.length, previousRequests);
    fs.writeFileSync(originalPath, fake.original);

    const hardlink = path.join(base, 'alternate-hardlink');
    fs.linkSync(originalPath, hardlink);
    await assert.rejects(prepareDraftAssets(input), /unsafe original source/i);
    assert.equal(fake.calls.length, previousRequests);
    fs.unlinkSync(hardlink);

    if (process.platform !== 'win32') {
      const relocated = path.join(base, 'untrusted-original');
      fs.renameSync(originalPath, relocated);
      try {
        fs.symlinkSync(relocated, originalPath);
        await assert.rejects(prepareDraftAssets(input), /unsafe original source/i);
        assert.equal(fake.calls.length, previousRequests);
      } finally {
        fs.unlinkSync(originalPath);
        fs.renameSync(relocated, originalPath);
      }
    }
    assert.deepEqual(await prepareDraftAssets(input),
      { kind: 'draft-complete', releaseId: 123, count: 22 });
    assert.equal(fake.postCount, 22, 'no writes after restoring trusted sources');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('draft preparation refuses incomplete source staging before any GitHub request', async () => {
  const fake = simulatedDraftCreation();
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-incomplete-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    await assert.rejects(prepareDraftAssets({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network', normalDir, mcpbDir,
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), /original|source|stage|inventory/i);
    assert.equal(fake.calls.length, 0, 'no remote reads or writes with incomplete sources');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('publishes only one complete 22-original draft after independent immutable policy checks', async () => {
  const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: 'complete' });
  const input = {
    manifest: fake.manifest, publisherCommit: COMMIT,
    token: 'test_token_never_used_on_network',
    adminToken: 'admin_only_test_token',
    releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
  };
  assert.deepEqual(await publishCompleteDraft(input),
    { kind: 'immutable-publication', releaseId: 123, count: 22 });
  assert.equal(fake.publishCount, 1, 'one controlled draft-to-public transition');
  assert.equal(fake.calls.filter(call => call.url === API + '/immutable-releases').length,
    2, 'immutability policy is verified twice around the mutation');
  assert.equal(fake.calls.filter(call => call.method === 'PATCH').length, 1);
  assert.equal(fake.postCount, 0, 'publication must not create or upload files');
  await assert.rejects(publishCompleteDraft(input), /draft|immutable|publication/i);
  assert.equal(fake.publishCount, 1, 'repeated invocation must not republish');
});

test('incomplete inventory or disabled immutable policy forbids public release', async () => {
  for (const opts of [
    { preverifiedRemoteAssets: true },
    { preverifiedRemoteAssets: 'complete', publicationPolicy: 'disabled' },
    { preverifiedRemoteAssets: 'complete', publicationPolicy: 'flip' },
    { preverifiedRemoteAssets: 'complete', metadataPatch: { body: 'altered notes' } },
  ]) {
    const fake = simulatedGitHubDraft(opts);
    await assert.rejects(publishCompleteDraft({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      adminToken: 'admin_only_test_token',
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), Error);
    assert.equal(fake.publishCount, 0, 'no publishing with insufficient evidence');
  }
  const authorized = simulatedGitHubDraft({ preverifiedRemoteAssets: 'complete' });
  await assert.rejects(publishCompleteDraft({
    manifest: authorized.manifest, publisherCommit: COMMIT,
    token: 'test_token_never_used_on_network',
    releaseNotesMetadata: notesMetadata(), fetchImpl: authorized.fetchImpl,
  }), /token/i);
  assert.equal(authorized.publishCount, 0, 'no publication without admin read proof');
});

test('public release refuses shared admin-read and write credential before network', async () => {
  const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: 'complete' });
  await assert.rejects(publishCompleteDraft({
    manifest: fake.manifest, publisherCommit: COMMIT,
    token: 'same_broad_token', adminToken: 'same_broad_token',
    releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
  }), /separate|distinct/i);
  assert.equal(fake.calls.length, 0,
    'shared administration and release-write authority must fail before GET');
  assert.equal(fake.publishCount, 0);
});

test('single-writer refuses untrusted tagged GitHub Actions context without contacting GitHub', async () => {
  const trusted = validWriterContext();
  for (const invalid of [
    undefined, null, {},
    { ...trusted, eventName: 'workflow_dispatch' },
    { ...trusted, repository: 'other/repo' },
    { ...trusted, ref: 'refs/heads/main' },
    { ...trusted, ref: 'refs/tags/v0.0.0' },
    { ...trusted, workflowRef: REPO + '/.github/workflows/test.yml@refs/tags/' + TAG },
    { ...trusted, workflowRef: REPO + '/.github/workflows/release.yml@refs/heads/main' },
    { ...trusted, sha: 'f'.repeat(40) },
    { ...trusted, job: 'verify' },
    { ...trusted, runId: '0' }, { ...trusted, runAttempt: '0' },
    { ...trusted, unexpected: true },
  ]) {
    const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: 'complete' });
    await assert.rejects(finalizeCompleteRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      adminToken: 'admin_only_test_token', actionContext: invalid,
      normalDir: 'not-an-authorized-source', mcpbDir: 'not-an-authorized-source',
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), /tagged release writer context/i);
    assert.equal(fake.calls.length, 0, 'invalid workflow context has zero API calls');
  }
});

test('single-writer coordinator completes one authenticated draft before one immutable publish', async () => {
  const fake = simulatedGitHubDraft({ startAbsent: true });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-coordinator-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    for (const asset of fake.manifest.assets) {
      const dir = asset.name.endsWith('.mcpb') ||
        asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
      fs.writeFileSync(path.join(dir, asset.name),
        asset.name === fake.manifest.assets[0].name ?
          fake.original : Buffer.alloc(asset.size), { mode: 0o600 });
    }
    assert.deepEqual(await finalizeCompleteRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      adminToken: 'admin_only_test_token', normalDir, mcpbDir,
      actionContext: validWriterContext(),
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), { kind: 'immutable-publication', releaseId: 123, count: 22 });
    assert.equal(fake.createCount, 1);
    assert.equal(fake.postCount, 22);
    assert.equal(fake.publishCount, 1);
    const mutations = fake.calls.filter(call => call.method !== 'GET');
    assert.equal(mutations[0].url, API + '/releases', 'draft is created first');
    assert.equal(mutations.at(-1).method, 'PATCH', 'publication is last');
    assert.equal(fake.calls[0].url,
      API + '/environments/scripthold-immutable-publish',
      'protected environment must be checked before any draft creation');
    assert.equal(fake.calls[2].url, API + '/immutable-releases',
      'immutable policy must also be checked before draft creation');
    assert.equal(fake.calls.filter(call =>
      call.url === API + '/environments/scripthold-immutable-publish').length, 2,
    'environment protection must be rechecked before public publication');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('single-writer halts on unsafe or changed protected GitHub deployment policy', async () => {
  for (const environmentPolicy of ['bypass', 'wrong-ref', 'flip']) {
    const fake = simulatedGitHubDraft({ startAbsent: true, environmentPolicy });
    const base = fs.mkdtempSync(path.resolve(__dirname, '.test-protected-writer-policy-'));
    const normalDir = path.join(base, 'normal');
    const mcpbDir = path.join(base, 'mcpb');
    try {
      fs.mkdirSync(normalDir, { mode: 0o700 });
      fs.mkdirSync(mcpbDir, { mode: 0o700 });
      for (const asset of fake.manifest.assets) {
        const dir = asset.name.endsWith('.mcpb') ||
          asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
        fs.writeFileSync(path.join(dir, asset.name),
          asset.name === fake.manifest.assets[0].name ?
            fake.original : Buffer.alloc(asset.size), { mode: 0o600 });
      }
      await assert.rejects(finalizeCompleteRelease({
        manifest: fake.manifest, publisherCommit: COMMIT,
        token: 'test_token_never_used_on_network',
        adminToken: 'admin_only_test_token', normalDir, mcpbDir,
        actionContext: validWriterContext(),
        releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
      }), /protected|environment|tag policy/i);
      assert.equal(fake.publishCount, 0,
        'unsafe environment or mid-flight approval bypass must forbid public PATCH');
      if (environmentPolicy !== 'flip') {
        assert.equal(fake.createCount + fake.postCount, 0,
          'never create or upload a draft without initial protected GitHub policy');
      }
    } finally {
      fs.rmSync(base, { recursive: true, force: true });
    }
  }
});

test('single-writer coordinator fails before any network or mutation with unsafe inputs', async () => {
  for (const overrides of [
    {},
    { adminToken: 'test_token_never_used_on_network' },
    { adminToken: undefined },
  ]) {
    const fake = simulatedGitHubDraft({ startAbsent: true });
    await assert.rejects(finalizeCompleteRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      adminToken: 'admin_only_test_token',
      releaseNotesMetadata: notesMetadata(),
      normalDir: 'relative-unsafe-source', mcpbDir: 'relative-unsafe-source',
      actionContext: validWriterContext(),
      fetchImpl: fake.fetchImpl, ...overrides,
    }), Error);
    assert.equal(fake.calls.length, 0, 'untrusted inputs must fail before GitHub');
  }
  const disabled = simulatedGitHubDraft({
    startAbsent: true, publicationPolicy: 'disabled',
  });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-disabled-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    for (const asset of disabled.manifest.assets) {
      const dir = asset.name.endsWith('.mcpb') ||
        asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
      fs.writeFileSync(path.join(dir, asset.name),
        asset.name === disabled.manifest.assets[0].name ?
          disabled.original : Buffer.alloc(asset.size), { mode: 0o600 });
    }
    await assert.rejects(finalizeCompleteRelease({
      manifest: disabled.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      adminToken: 'admin_only_test_token', normalDir, mcpbDir,
      actionContext: validWriterContext(),
      releaseNotesMetadata: notesMetadata(), fetchImpl: disabled.fetchImpl,
    }), /immutable|enabled/i);
    assert.equal(disabled.createCount, 0);
    assert.equal(disabled.postCount, 0);
    assert.equal(disabled.publishCount, 0);
    assert.deepEqual(disabled.calls.map(call => call.method), ['GET', 'GET', 'GET'],
      'environment and immutable policy reads must precede any mutation');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('single-writer coordinator stops after ambiguous upload without publication', async () => {
  const fake = simulatedGitHubDraft({
    preverifiedRemoteAssets: true, response: 'lost',
  });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-upload-fault-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    for (const asset of fake.manifest.assets) {
      const dir = asset.name.endsWith('.mcpb') ||
        asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
      fs.writeFileSync(path.join(dir, asset.name),
        asset.name === fake.manifest.assets[0].name ?
          fake.original : Buffer.alloc(asset.size), { mode: 0o600 });
    }
    await assert.rejects(finalizeCompleteRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      adminToken: 'admin_only_test_token', normalDir, mcpbDir,
      actionContext: validWriterContext(),
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), /ambiguous GitHub upload response/);
    assert.equal(fake.createCount, 0);
    assert.equal(fake.postCount, 1, 'failed/uncertain upload is not retried');
    assert.equal(fake.publishCount, 0, 'no publication after ambiguous upload');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('one-shot publisher rejects a different release id before public PATCH', async () => {
  const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: 'complete' });
  await assert.rejects(publishCompleteDraft({
    manifest: fake.manifest, publisherCommit: COMMIT,
    token: 'test_token_never_used_on_network',
    adminToken: 'admin_only_test_token',
    releaseNotesMetadata: notesMetadata(), expectedReleaseId: 124,
    fetchImpl: fake.fetchImpl,
  }), /draft identity changed/);
  assert.equal(fake.publishCount, 0, 'changed draft cannot be published');
});

test('an ambiguous publication response stops without a second PATCH', async () => {
  const fake = simulatedGitHubDraft({ preverifiedRemoteAssets: 'complete',
    publishResponse: 'lost' });
  await assert.rejects(publishCompleteDraft({
    manifest: fake.manifest, publisherCommit: COMMIT,
    token: 'test_token_never_used_on_network',
    adminToken: 'admin_only_test_token',
    releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
  }), /lost GitHub publication response/);
  assert.equal(fake.publishCount, 1, 'a failed/uncertain PATCH is never retried');
});

test('draft preparation never retries an ambiguous draft creation', async () => {
  const fake = simulatedDraftCreation({ mode: 'lost' });
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-create-lost-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    fs.mkdirSync(normalDir, { mode: 0o700 });
    fs.mkdirSync(mcpbDir, { mode: 0o700 });
    for (const asset of fake.manifest.assets) {
      const dest = asset.name.endsWith('.mcpb') ||
        asset.name === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
      fs.writeFileSync(path.join(dest, asset.name), Buffer.alloc(asset.size),
        { mode: 0o600 });
    }
    await assert.rejects(prepareDraftAssets({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network', normalDir, mcpbDir,
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }), /lost creation response/);
    assert.equal(fake.writes, 1, 'an ambiguous draft POST may never be retried');
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

test('single-asset finalizer stops on conflicting, ambiguous or changed draft mutations', async () => {
  const base = fs.mkdtempSync(path.resolve(__dirname, '.test-finalizer-reject-'));
  const normalDir = path.join(base, 'normal');
  const mcpbDir = path.join(base, 'mcpb');
  try {
    for (const dir of [normalDir, mcpbDir]) fs.mkdirSync(dir, { mode: 0o700 });
    for (const kind of ['lost', 'conflict', 'changed']) {
      const fake = simulatedGitHubDraft({
        response: kind, changeBeforePost: kind === 'changed',
      });
      fs.writeFileSync(path.join(normalDir, fake.manifest.assets[0].name),
        fake.original, { mode: 0o600 });
      await assert.rejects(uploadOneDraftAsset({
        manifest: fake.manifest, publisherCommit: COMMIT,
        token: 'test_token_never_used_on_network', normalDir, mcpbDir,
        releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
      }));
      assert.equal(fake.postCount, kind === 'changed' ? 0 : 1,
        'never retry an ambiguous or rejected upload');
    }
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
});

function simulatedDraftCreation({ mode = 'success' } = {}) {
  const { authority } = fixture();
  const manifest = authority.manifest;
  let created = false;
  let writes = 0;
  let listReads = 0;
  const calls = [];
  const run = (id, file) => ({
    id, run_attempt: 1, head_sha: COMMIT, head_branch: 'main',
    event: 'push', conclusion: 'success', path: file,
    head_repository: { full_name: REPO },
  });
  const fetchImpl = async (url, opts) => {
    calls.push({ url, method: opts.method });
    if (opts.method === 'POST') {
      writes++;
      assert.equal(url, API + '/releases');
      const payload = JSON.parse(opts.body);
      assert.equal(payload.tag_name, TAG);
      assert.equal(payload.draft, true);
      assert.equal(payload.generate_release_notes, false);
      assert.equal(payload.body, 'Trusted GoReleaser notes for next version\n');
      if (mode === 'lost') throw new Error('lost creation response');
      if (mode === 'conflict') return new Response('conflict', { status: 422 });
      created = true;
      return new Response(JSON.stringify({
        id: 123, tag_name: TAG, draft: true, immutable: false,
        name: TAG, body: payload.body,
        prerelease: mode === 'forged-prerelease-response' ? true : false,
      }), { status: 201, headers: { 'content-type': 'application/json' } });
    }
    if (url === API + '/releases/tags/' + TAG) {
      return new Response(null, { status: mode === 'bad-status' ? 403 : 404 });
    }
    let result;
    if (url === API + '/git/ref/tags/' + TAG) {
      result = authority.tagRef;
    } else if (url === API + '/git/tags/' + TAG_OBJECT) {
      result = authority.tagObject;
    } else if (url === API + '/git/ref/heads/main') {
      result = authority.mainRef;
    } else if (url.includes('/actions/workflows/test.yml/runs?')) {
      result = { total_count: 1, workflow_runs: [run(11, '.github/workflows/test.yml')] };
    } else if (url.includes('/actions/workflows/scorecard.yml/runs?')) {
      result = { total_count: 1, workflow_runs: [run(12, '.github/workflows/scorecard.yml')] };
    } else if (url.includes('/actions/runs/11/attempts/1/jobs?')) {
      result = { total_count: 1, jobs: [authority.testJobs.jobs[0]] };
    } else if (url === API + '/releases?per_page=30&page=1') {
      listReads++;
      result = created ? [{
        id: 123, tag_name: TAG, draft: true, immutable: false,
      }] : (mode === 'conflicting-list' ? [{
        id: 444, tag_name: TAG, draft: true, immutable: false,
      }] : []);
    } else if (url === API + '/releases/123') {
      result = { id: 123, tag_name: TAG, draft: true, immutable: false,
        name: TAG, body: mode === 'tampered-remote-notes' ? 'Different text\n' : NOTES,
        prerelease: false };
    } else if (url === API + '/releases/444') {
      result = { id: 444, tag_name: TAG, draft: true, immutable: false };
    } else if (url === API + '/releases/123/assets?per_page=100&page=1' ||
               url === API + '/releases/444/assets?per_page=100&page=1') {
      result = [];
    } else {
      throw new Error('unexpected simulated GitHub URL: ' + url);
    }
    return new Response(JSON.stringify(result), {
      status: 200, headers: { 'content-type': 'application/json' },
    });
  };
  return { manifest, fetchImpl, calls,
    get writes() { return writes; }, get listReads() { return listReads; } };
}

test('creates one tagged unpublished draft only after authenticated absence checks', async () => {
  const fake = simulatedDraftCreation();
  const result = await createDraftRelease({
    manifest: fake.manifest, publisherCommit: COMMIT,
    token: 'test_token_never_used_on_network',
    releaseNotesMetadata: encodeGoReleaserNotes(Buffer.from(
      'Trusted GoReleaser notes for next version\n')),
    fetchImpl: fake.fetchImpl,
  });
  assert.deepEqual(result, { kind: 'created', releaseId: 123, tag: TAG });
  assert.equal(fake.writes, 1);
  assert.ok(fake.listReads >= 3);
});

test('draft creation does not accept a changed API body or prerelease response', async () => {
  for (const mode of ['tampered-remote-notes', 'forged-prerelease-response']) {
    const fake = simulatedDraftCreation({ mode });
    await assert.rejects(createDraftRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      releaseNotesMetadata: notesMetadata(), fetchImpl: fake.fetchImpl,
    }));
    assert.equal(fake.writes, 1, 'never reissue an uncertain draft creation');
  }
});

test('draft creation rejects forged or free-form notes before any GitHub API call', async () => {
  const original = encodeGoReleaserNotes(Buffer.from(
    'Trusted GoReleaser notes for next version\n', 'utf8'));
  const candidates = [undefined, 'unattributed text', { ...original, sha256: '0'.repeat(64) },
    { ...original, size: original.size - 1 }, { ...original, other: 'foreign' }];
  for (const releaseNotesMetadata of candidates) {
    const fake = simulatedDraftCreation();
    await assert.rejects(createDraftRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      releaseNotesMetadata, fetchImpl: fake.fetchImpl,
    }));
    assert.equal(fake.writes, 0);
    assert.equal(fake.calls.length, 0, 'reject before reading from or writing to GitHub');
  }
});

test('draft creation halts on inaccessible, conflicting, or uncertain responses', async () => {
  for (const mode of ['bad-status', 'conflicting-list', 'conflict', 'lost']) {
    const fake = simulatedDraftCreation({ mode });
    await assert.rejects(createDraftRelease({
      manifest: fake.manifest, publisherCommit: COMMIT,
      token: 'test_token_never_used_on_network',
      releaseNotesMetadata: encodeGoReleaserNotes(Buffer.from(
        'Trusted GoReleaser notes for next version\n')),
      fetchImpl: fake.fetchImpl,
    }));
    assert.equal(fake.writes, mode === 'bad-status' ||
      mode === 'conflicting-list' ? 0 : 1,
    'invalid initial discovery cannot initiate a draft POST and ambiguous writes never retry');
  }
});

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
