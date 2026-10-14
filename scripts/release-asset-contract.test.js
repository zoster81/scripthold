'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames, validateStagedManifest, verifyDraftAssetSet } =
  require('./release-asset-contract');

const TAG = 'v3.4.0';
const COMMIT = '1'.repeat(40);

function fixture() {
  const assets = expectedReleaseAssetNames(TAG).map((name, index) => ({
    name,
    size: index + 100,
    sha256: (index + 1).toString(16).padStart(64, '0'),
  }));
  return { repository: 'zoster81/scripthold', tag: TAG, commit: COMMIT, assets };
}

function remote(manifest) {
  return manifest.assets.map((asset, index) => ({
    id: index + 1,
    name: asset.name,
    size: asset.size,
    digest: 'sha256:' + asset.sha256,
    state: 'uploaded',
  }));
}

function release() {
  return { id: 42, tag_name: TAG, draft: true, immutable: false };
}

test('enumerates exactly 22 uniquely named versioned assets, including real attestation companions', () => {
  const names = expectedReleaseAssetNames(TAG);
  assert.equal(names.length, 22);
  assert.equal(new Set(names).size, 22);
  assert.ok(names.includes('checksums.txt'));
  assert.ok(names.includes('mcpb-checksums.txt'));
  assert.ok(names.includes('scripthold-v3.4.0-provenance.sigstore.json'));
  assert.ok(names.includes('scripthold-v3.4.0-provenance.intoto.jsonl'));
  assert.equal(names.filter((n) => n.endsWith('.mcpb')).length, 6);
});

test('rejects invalid version tags', () => {
  for (const tag of ['', '../v3.4.0', 'v3.4.0/', 'v3.4.0\n', 'latest', 'v3.4.0+meta']) {
    assert.throws(() => expectedReleaseAssetNames(tag), /release tag/);
  }
});

test('accepts a fully specified, exact-size offline staging manifest', () => {
  assert.equal(validateStagedManifest(fixture()).size, 22);
});

test('rejects malformed manifest authority', () => {
  for (const patch of [
    { repository: 'someone/else' },
    { tag: 'v3.4.0/evil' },
    { commit: 'g'.repeat(40) },
    { commit: 'A'.repeat(40) },
  ]) {
    assert.throws(() => validateStagedManifest({ ...fixture(), ...patch }));
  }
});

test('rejects missing, unexpected and duplicate staged names', () => {
  const original = fixture();
  const missing = { ...original, assets: original.assets.slice(0, -1) };
  assert.throws(() => validateStagedManifest(missing), /22 assets/);
  const unexpected = fixture();
  unexpected.assets[0].name = 'evil.tar.gz';
  assert.throws(() => validateStagedManifest(unexpected), /unexpected/);
  const duplicate = fixture();
  duplicate.assets[1].name = duplicate.assets[0].name;
  assert.throws(() => validateStagedManifest(duplicate), /duplicate/);
});

test('rejects malformed staged digests, size or filename types', () => {
  for (const patch of [
    { sha256: '0'.repeat(63) },
    { sha256: 'F'.repeat(64) },
    { size: 0 },
    { size: -1 },
    { size: 1.5 },
    { size: Number.MAX_SAFE_INTEGER + 1 },
    { name: '../archive' },
  ]) {
    const manifest = fixture();
    Object.assign(manifest.assets[0], patch);
    assert.throws(() => validateStagedManifest(manifest));
  }
});

test('rejects non-array and oversized staging metadata', () => {
  assert.throws(() => validateStagedManifest({ ...fixture(), assets: null }));
  assert.throws(() => validateStagedManifest({ ...fixture(), assets: new Array(9999) }));
});

test('verifies 22 uploaded GitHub assets by their exact per-file digest and size', () => {
  const manifest = fixture();
  assert.deepEqual(verifyDraftAssetSet(manifest, release(), remote(manifest)), {
    releaseId: 42, tag: TAG, commit: COMMIT, count: 22,
  });
});

test('rejects non-draft, wrong-tag, invalid ID and immutable release metadata', () => {
  const manifest = fixture();
  for (const patch of [
    { draft: false },
    { immutable: true },
    { id: 0 },
    { id: '42' },
    { tag_name: 'v3.3.0' },
  ]) {
    assert.throws(() => verifyDraftAssetSet(manifest, { ...release(), ...patch }, remote(manifest)));
  }
});

test('rejects missing, unexpected, duplicate and corrupt remote objects', () => {
  const original = fixture();
  const good = remote(original);
  const wrong = [
    good.slice(1),
    good.map((a, i) => i === 0 ? { ...a, name: 'surprise.zip' } : a),
    good.map((a, i) => i === 1 ? { ...a, name: good[0].name } : a),
    good.map((a, i) => i === 0 ? { ...a, digest: 'sha256:' + 'f'.repeat(64) } : a),
    good.map((a, i) => i === 0 ? { ...a, size: a.size + 1 } : a),
    good.map((a, i) => i === 0 ? { ...a, state: 'starter' } : a),
    good.map((a, i) => i === 0 ? { ...a, id: '1' } : a),
  ];
  for (const assets of wrong) {
    assert.throws(() => verifyDraftAssetSet(original, release(), assets));
  }
});

test('rejects malformed remote input and duplicate remote asset ids', () => {
  const manifest = fixture();
  assert.throws(() => verifyDraftAssetSet(manifest, release(), null));
  const assets = remote(manifest);
  assets[1].id = assets[0].id;
  assert.throws(() => verifyDraftAssetSet(manifest, release(), assets), /duplicate/);
});
