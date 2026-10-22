'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyRegistryImmutableRelease } = require('./release-registry-immutable');

const tag = 'v3.4.0';
const commit = 'a'.repeat(40);
const token = 'test_readonly_token';
const prefix = 'https://api.github.com/repos/zoster81/scripthold/releases';
const id = 123;
const release = () => ({
  id, tag_name: tag, target_commitish: commit,
  draft: false, immutable: true,
});
const assets = () => expectedReleaseAssetNames(tag).map((name, i) => ({
  id: i + 1, name, size: i + 1, state: 'uploaded',
  digest: 'sha256:' + (i + 1).toString(16).padStart(64, '0'),
}));
const response = (data, headers = {}) =>
  new Response(JSON.stringify(data), {
    status: 200, headers: { 'content-type': 'application/json', ...headers },
  });
function stub({ initial = release(), second = release(),
  final = release(), files = assets(), link } = {}) {
  const calls = [];
  let observed = 0;
  const fetchImpl = async (url, init) => {
    calls.push({ url, init });
    if (url === prefix + '/tags/' + tag) return response(initial);
    if (url === prefix + '/' + id) {
      return response(++observed === 1 ? second : final);
    }
    if (url === prefix + '/' + id + '/assets?per_page=100&page=1') {
      return response(files, link ? { link } : {});
    }
    throw new Error('unexpected GitHub endpoint: ' + url);
  };
  return { fetchImpl, calls };
}

test('Registry reads exact immutable release and its complete 22-original inventory', async () => {
  const fake = stub();
  assert.deepEqual(await verifyRegistryImmutableRelease({
    tag, commit, token, fetchImpl: fake.fetchImpl,
  }), { releaseId: id, count: 22 });
  assert.deepEqual(fake.calls.map(x => x.url), [
    prefix + '/tags/' + tag,
    prefix + '/' + id,
    prefix + '/' + id + '/assets?per_page=100&page=1',
    prefix + '/' + id,
  ]);
  for (const { init } of fake.calls) {
    assert.equal(init.method, 'GET');
    assert.equal(init.redirect, 'error');
    assert.equal(init.headers.Authorization, 'Bearer ' + token);
  }
});

test('Registry rejects incomplete, mutable, stale and ambiguous release evidence', async () => {
  const malformed = [
    { initial: { ...release(), immutable: false } },
    { initial: { ...release(), draft: true } },
    { initial: { ...release(), target_commitish: 'b'.repeat(40) } },
    { initial: { ...release(), tag_name: 'v0.0.0' } },
    { second: { ...release(), immutable: false } },
    { second: { ...release(), target_commitish: 'b'.repeat(40) } },
    { final: { ...release(), draft: true } },
    { final: { ...release(), target_commitish: 'b'.repeat(40) } },
    { files: assets().slice(1) },
    { files: [...assets().slice(1), assets()[1]] },
    { files: [...assets(), { ...assets()[0], name: 'unexpected.bin' }] },
    { files: assets().map((a, i) => i ? a : { ...a, digest: 'sha256:nope' }) },
    { files: assets().map((a, i) => i ? a : { ...a, state: 'starter' }) },
    { files: assets().map((a, i) => i ? a : { ...a, size: 0 }) },
    { files: assets().map((a, i) => i ? a : { ...a, id: 0 }) },
    { link: '<https://api.github.com/next>; rel="next"' },
  ];
  for (const data of malformed) {
    const fake = stub(data);
    await assert.rejects(verifyRegistryImmutableRelease({
      tag, commit, token, fetchImpl: fake.fetchImpl,
    }), Error);
  }
  for (const changed of [
    { tag: 'v3.4.0/../../' },
    { commit: 'not-a-commit' },
    { token: 'bad\nheader' },
  ]) {
    const fake = stub();
    await assert.rejects(verifyRegistryImmutableRelease({
      tag, commit, token, fetchImpl: fake.fetchImpl, ...changed,
    }), Error);
    assert.equal(fake.calls.length, 0);
  }
});

test('Registry OIDC is gated by a read-only exact immutable release check', () => {
  const fs = require('node:fs');
  const path = require('node:path');
  const yaml = fs.readFileSync(path.join(__dirname, '..',
    '.github', 'workflows', 'publish-registry.yml'), 'utf8');
  const guard = yaml.indexOf(
    '      - name: Verify complete immutable GitHub Release before Registry publication');
  const download = yaml.indexOf('      - name: Download and verify MCPB release assets');
  const publisher = yaml.indexOf('      - name: Publish to MCP Registry');
  assert.ok(guard > 0 && guard < download && download < publisher,
    'manual and reusable Registry paths require the same immutable gate');
  const slice = yaml.slice(guard, download);
  assert.ok(slice.includes('GH_TOKEN: ${{ github.token }}'));
  assert.ok(slice.includes('git -C release-source rev-parse HEAD'));
  assert.ok(slice.includes('node publication-source/scripts/release-registry-immutable.js'));
  assert.ok(!yaml.includes('contents: write'));
});

test('Registry authenticates the release before executing tag-owned scripts', () => {
  const fs = require('node:fs');
  const path = require('node:path');
  const yaml = fs.readFileSync(path.join(__dirname, '..',
    '.github', 'workflows', 'publish-registry.yml'), 'utf8');
  const trusted = yaml.indexOf('path: publication-source');
  const source = yaml.indexOf('path: release-source');
  const guard = yaml.indexOf('node publication-source/scripts/release-registry-immutable.js');
  const version = yaml.indexOf('node scripts/verify-release-version.js');
  const generate = yaml.indexOf('node release-source/scripts/generate-server-json.js');
  const publish = yaml.indexOf('      - name: Publish to MCP Registry');
  assert.ok(trusted >= 0 && source > trusted && guard > source &&
    version > guard && generate > version && publish > generate,
    'current workflow policy must authenticate before any tag-owned executable');
  assert.ok(!yaml.includes('node release-source/scripts/release-registry-immutable.js'));
  assert.ok(!yaml.includes('node publication-source/scripts/generate-server-json.js'));
  assert.equal((yaml.match(/persist-credentials: false/g) || []).length, 2);
});
