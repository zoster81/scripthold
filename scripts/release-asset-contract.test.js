'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames, validateStagedManifest, verifyDraftAssetSet,
  verifyStagedAssetStream, stagedArtifactName, verifyRunStagingArtifact } =
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

test('streams staged bytes and verifies the original digest without buffering whole release files', async () => {
  const { Readable } = require('node:stream');
  const { createHash } = require('node:crypto');
  const chunks = [Buffer.from('verified '), Buffer.from('native executable bytes')];
  const expected = {
    name: 'scripthold_linux_amd64',
    size: chunks.reduce((sum, chunk) => sum + chunk.length, 0),
    sha256: createHash('sha256').update(Buffer.concat(chunks)).digest('hex'),
  };
  const result = await verifyStagedAssetStream(expected, Readable.from(chunks));
  assert.equal(result.name, expected.name);
  assert.equal(result.sha256, expected.sha256);
  assert.equal(result.size, expected.size);
});

test('stream verification rejects wrong bytes, truncated/oversized content and malformed metadata', async () => {
  const { Readable } = require('node:stream');
  const { createHash } = require('node:crypto');
  const bytes = Buffer.from('normal archive bytes');
  const expected = {
    name: 'scripthold_linux_amd64.tar.gz',
    size: bytes.length,
    sha256: createHash('sha256').update(bytes).digest('hex'),
  };
  for (const values of [
    [Buffer.from('normal archive byte!')],
    [bytes.subarray(0, bytes.length - 1)],
    [bytes, Buffer.from('extra')],
    [],
  ]) {
    await assert.rejects(() => verifyStagedAssetStream(expected, Readable.from(values)));
  }
  for (const change of [
    { name: '../escape' }, { name: 'unknown.bin' },
    { size: 0 }, { size: 1.5 }, { size: Number.MAX_SAFE_INTEGER },
    { sha256: 'A'.repeat(64) }, { sha256: '0'.repeat(63) },
  ]) {
    await assert.rejects(() => verifyStagedAssetStream({ ...expected, ...change }, Readable.from([bytes])));
  }
});

test('stream verification propagates source errors and rejects nonbinary chunks', async () => {
  const { Readable } = require('node:stream');
  const { createHash } = require('node:crypto');
  const bytes = Buffer.from('normal binary');
  const expected = {
    name: 'checksums.txt', size: bytes.length,
    sha256: createHash('sha256').update(bytes).digest('hex'),
  };
  await assert.rejects(() => verifyStagedAssetStream(expected, Readable.from(['not a byte buffer'])));
  async function *failing() { yield bytes; throw new Error('source failure'); }
  await assert.rejects(() => verifyStagedAssetStream(expected, Readable.from(failing())), /source failure/);
});

function actionsStageFixture() {
  const uploadDigest = 'e'.repeat(64);
  const expected = {
    repository: 'zoster81/scripthold', repositoryId: 1234,
    tag: TAG, commit: COMMIT, runId: 9001, producerAttempt: 1,
    stage: 'normal', uploadId: 7654, uploadDigest,
  };
  const remote = {
    id: 7654, name: 'scripthold-normal-9001-1', size_in_bytes: 4096,
    digest: 'sha256:' + uploadDigest, expired: false,
    workflow_run: {
      id: 9001, repository_id: 1234, head_repository_id: 1234,
      head_sha: COMMIT,
    },
  };
  return { expected, remote };
}

test('binds an Actions upload to exact source, repository, run, producer attempt and digest', () => {
  const { expected, remote } = actionsStageFixture();
  assert.equal(stagedArtifactName('normal', 9001, 1), remote.name);
  assert.equal(stagedArtifactName('mcpb', 9001, 2), 'scripthold-mcpb-9001-2');
  assert.deepEqual(verifyRunStagingArtifact(expected, remote), {
    artifactId: 7654, name: remote.name, archiveDigest: remote.digest, size: 4096,
  });
});

test('fails closed on malformed or out-of-scope staging authorities', () => {
  for (const change of [
    { repository: 'attacker/another' },
    { repositoryId: 0 }, { repositoryId: '1234' },
    { commit: 'a'.repeat(40) }, { commit: 'A'.repeat(40) },
    { tag: '../v3.4.0' },
    { runId: 0 }, { runId: '9001' },
    { producerAttempt: 0 }, { producerAttempt: -1 },
    { stage: 'latest' }, { uploadId: 0 }, { uploadId: '7654' },
    { uploadDigest: 'G'.repeat(64) },
  ]) {
    const { expected, remote } = actionsStageFixture();
    assert.throws(() => verifyRunStagingArtifact({ ...expected, ...change }, remote));
  }
  for (const args of [['normal', 0, 1], ['mcpb', 1, 0], ['latest', 1, 1]]) {
    assert.throws(() => stagedArtifactName(...args));
  }
});

test('rejects mismatched, expired or malformed authenticated GitHub artifact metadata', () => {
  for (const change of [
    { id: 7655 }, { name: 'scripthold-normal-9001-2' },
    { name: '../scripthold-normal-9001-1' },
    { digest: 'sha256:' + '0'.repeat(64) },
    { digest: 'e'.repeat(64) },
    { expired: true }, { expired: undefined }, { size_in_bytes: 0 },
    { size_in_bytes: 2 ** 43 }, { size_in_bytes: 1.5 },
    { workflow_run: null },
    { workflow_run: { id: 9, repository_id: 1234, head_repository_id: 1234, head_sha: COMMIT } },
    { workflow_run: { id: 9001, repository_id: 999, head_repository_id: 1234, head_sha: COMMIT } },
    { workflow_run: { id: 9001, repository_id: 1234, head_repository_id: 999, head_sha: COMMIT } },
    { workflow_run: { id: 9001, repository_id: 1234, head_repository_id: 1234, head_sha: 'a'.repeat(40) } },
  ]) {
    const { expected, remote } = actionsStageFixture();
    assert.throws(() => verifyRunStagingArtifact(expected, { ...remote, ...change }));
  }
  const { expected } = actionsStageFixture();
  assert.throws(() => verifyRunStagingArtifact(expected, null));
});

test('binds each stage and avoids selecting artifacts by latest run or mutable name', () => {
  const { expected, remote } = actionsStageFixture();
  const mcpbExpected = { ...expected, stage: 'mcpb', producerAttempt: 2 };
  assert.throws(() => verifyRunStagingArtifact(mcpbExpected, remote));
  assert.deepEqual(verifyRunStagingArtifact(mcpbExpected,
    { ...remote, name: 'scripthold-mcpb-9001-2' }).artifactId, 7654);
});

test('rejects malformed remote input and duplicate remote asset ids', () => {
  const manifest = fixture();
  assert.throws(() => verifyDraftAssetSet(manifest, release(), null));
  const assets = remote(manifest);
  assets[1].id = assets[0].id;
  assert.throws(() => verifyDraftAssetSet(manifest, release(), assets), /duplicate/);
});
