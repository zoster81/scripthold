'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { expectedReleaseAssetNames, validateStagedManifest, verifyDraftAssetSet,
  verifyStagedAssetStream, stagedArtifactName, verifyRunStagingArtifact,
  verifyStagingArchiveStream, validateStagingArchiveEntries,
  verifyRawStagingArtifact, verifyRawStagingStream, rawStagingArtifactName } =
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


test('checks the actual downloaded Actions archive byte stream against the verified producer', async () => {
  const { Readable } = require('node:stream');
  const { createHash } = require('node:crypto');
  const { expected, remote } = actionsStageFixture();
  const parts = [Buffer.from('ZIP-test-header'), Buffer.from('verified compressed data')];
  const bytes = Buffer.concat(parts);
  const hash = createHash('sha256').update(bytes).digest('hex');
  const producer = { ...expected, uploadDigest: hash };
  const github = { ...remote, digest: 'sha256:' + hash, size_in_bytes: bytes.length };
  assert.deepEqual(await verifyStagingArchiveStream(producer, github, Readable.from(parts)), {
    artifactId: expected.uploadId, name: remote.name,
    archiveDigest: github.digest, size: bytes.length,
  });
});

test('rejects tampered, truncated, oversized and malformed Actions archive streams', async () => {
  const { Readable } = require('node:stream');
  const { createHash } = require('node:crypto');
  const { expected, remote } = actionsStageFixture();
  const bytes = Buffer.from('synthetic ZIP bytes, not a release asset');
  const hash = createHash('sha256').update(bytes).digest('hex');
  const producer = { ...expected, uploadDigest: hash };
  const github = { ...remote, digest: 'sha256:' + hash, size_in_bytes: bytes.length };
  for (const parts of [
    [Buffer.from('synthetic ZIP byteS, not a release asset')],
    [bytes.subarray(0, bytes.length - 1)],
    [bytes, Buffer.from('x')],
    [],
  ]) {
    await assert.rejects(() => verifyStagingArchiveStream(producer, github, Readable.from(parts)));
  }
  await assert.rejects(() => verifyStagingArchiveStream(
    producer, { ...github, id: github.id + 1 }, Readable.from([bytes])));
  await assert.rejects(() => verifyStagingArchiveStream(producer, github, Readable.from(['not bytes'])));
  async function* broken() { yield bytes; throw new Error('stream interrupted'); }
  await assert.rejects(() => verifyStagingArchiveStream(producer, github, Readable.from(broken())),
    /stream interrupted/);
});

function rawStageFixture(fileName = 'scripthold_linux_amd64', stage = 'normal') {
  const { createHash } = require('node:crypto');
  const manifest = fixture();
  const bytes = Buffer.from('synthetic staging bytes, not a release package');
  const asset = manifest.assets.find((item) => item.name === fileName);
  if (!asset) throw new Error('invalid synthetic test name');
  asset.size = bytes.length;
  asset.sha256 = createHash('sha256').update(bytes).digest('hex');
  const expected = {
    repository: manifest.repository, repositoryId: 1234, tag: manifest.tag,
    commit: manifest.commit, runId: 9001, producerAttempt: 2, stage,
    fileName, uploadId: 7654, uploadDigest: asset.sha256,
  };
  const github = {
    id: expected.uploadId,
    name: rawStagingArtifactName(stage, expected.runId, expected.producerAttempt, fileName),
    size_in_bytes: asset.size,
    digest: 'sha256:' + asset.sha256, expired: false,
    workflow_run: {
      id: expected.runId, repository_id: expected.repositoryId,
      head_repository_id: expected.repositoryId, head_sha: expected.commit,
    },
  };
  return { manifest, expected, github, bytes };
}

test('authenticates a raw Actions artifact by immutable ID, manifest bytes and producer identity', async () => {
  const { Readable } = require('node:stream');
  for (const [name, stage] of [
    ['scripthold_linux_amd64', 'normal'],
    ['checksums.txt', 'normal'],
    ['scripthold_linux_amd64.mcpb', 'mcpb'],
    ['mcpb-checksums.txt', 'mcpb'],
  ]) {
    const { manifest, expected, github, bytes } = rawStageFixture(name, stage);
    const match = {
      artifactId: expected.uploadId, artifactName: github.name,
      fileName: name, size: bytes.length, sha256: expected.uploadDigest,
    };
    assert.deepEqual(verifyRawStagingArtifact(manifest, expected, github), match);
    assert.deepEqual(await verifyRawStagingStream(
      manifest, expected, github, Readable.from([bytes.subarray(0, 7), bytes.subarray(7)])), match);
  }
});

test('raw artifact naming prevents same-run rerun collisions for all 22 files', () => {
  const names = expectedReleaseAssetNames(TAG);
  const first = new Set();
  const second = new Set();
  for (const name of names) {
    const stage = name.endsWith('.mcpb') || name === 'mcpb-checksums.txt'
      ? 'mcpb' : 'normal';
    const a = rawStagingArtifactName(stage, 9001, 1, name);
    const b = rawStagingArtifactName(stage, 9001, 2, name);
    assert.notEqual(a, b);
    assert.ok(a.endsWith('-' + name));
    assert.ok(a.length <= 240);
    first.add(a);
    second.add(b);
  }
  assert.equal(first.size, 22);
  assert.equal(second.size, 22);
  assert.equal(new Set([...first, ...second]).size, 44);
  assert.notEqual(rawStagingArtifactName('normal', 9001, 1, 'checksums.txt'),
    rawStagingArtifactName('normal', 9002, 1, 'checksums.txt'));
});

test('raw artifact name builder rejects unsafe, malformed and oversized producer identity', () => {
  for (const [stage, run, attempt, file] of [
    ['bad', 9001, 1, 'checksums.txt'], ['normal', 0, 1, 'checksums.txt'],
    ['normal', '9001', 1, 'checksums.txt'], ['normal', 9001, 0, 'checksums.txt'],
    ['normal', 9001, 1.5, 'checksums.txt'], ['normal', 9001, 1, '../checksums.txt'],
    ['normal', 9001, 1, 'nested/checksums.txt'], ['normal', 9001, 1, 'other'],
    ['mcpb', 9001, 1, 'checksums.txt'],
    ['normal', 9001, 1, 'scripthold-v3.4.0-' + 'a'.repeat(280) + '-provenance.intoto.jsonl'],
  ]) {
    assert.throws(() => rawStagingArtifactName(stage, run, attempt, file));
  }
});

test('raw artifact metadata rejects name reuse by another producer attempt', () => {
  const { manifest, expected, github } = rawStageFixture();
  assert.throws(() => verifyRawStagingArtifact(manifest,
    { ...expected, producerAttempt: 3 }, github), /identity/);
  assert.throws(() => verifyRawStagingArtifact(manifest, expected,
    { ...github, name: expected.fileName }), /identity/);
  assert.throws(() => verifyRawStagingArtifact(manifest, expected,
    { ...github, name: rawStagingArtifactName(expected.stage, 9001, 3, expected.fileName) }),
  /identity/);
});

test('raw artifact metadata rejects impersonation, wrong manifest and stage confusion', () => {
  const { manifest, expected, github } = rawStageFixture();
  for (const patch of [
    { repository: 'attacker/fork' }, { repositoryId: '1234' }, { repositoryId: 0 },
    { tag: 'v3.4.1' }, { commit: 'a'.repeat(40) },
    { runId: 9002 }, { producerAttempt: 0 }, { producerAttempt: '2' },
    { stage: 'mcpb' }, { stage: 'unknown' }, { fileName: '../escape' },
    { fileName: 'unknown.bin' }, { fileName: 'scripthold_linux_amd64.mcpb' },
    { uploadId: 0 }, { uploadId: '7654' },
    { uploadDigest: 'f'.repeat(64) }, { uploadDigest: 'F'.repeat(64) },
  ]) {
    assert.throws(() => verifyRawStagingArtifact(manifest, { ...expected, ...patch }, github));
  }
  for (const patch of [
    { id: 7655 }, { name: 'checksums.txt' }, { name: '../escape' },
    { digest: 'sha256:' + '0'.repeat(64) }, { digest: expected.uploadDigest },
    { size_in_bytes: github.size_in_bytes + 1 }, { size_in_bytes: '43' },
    { expired: true }, { expired: undefined },
    { workflow_run: null },
    { workflow_run: { ...github.workflow_run, head_sha: 'a'.repeat(40) } },
    { workflow_run: { ...github.workflow_run, id: 9002 } },
    { workflow_run: { ...github.workflow_run, repository_id: 99 } },
  ]) {
    assert.throws(() => verifyRawStagingArtifact(manifest, expected, { ...github, ...patch }));
  }
  assert.throws(() => verifyRawStagingArtifact(manifest, expected, null));
  assert.throws(() => verifyRawStagingArtifact({ ...manifest, commit: 'a'.repeat(40) }, expected, github));
  assert.throws(() => verifyRawStagingArtifact(
    { ...manifest, assets: manifest.assets.slice(1) }, expected, github));
});

test('raw artifact stream rejects corruption, truncation, extension and source failures', async () => {
  const { Readable } = require('node:stream');
  const { manifest, expected, github, bytes } = rawStageFixture();
  for (const chunks of [
    [], [bytes.subarray(0, -1)], [Buffer.from('modified, original bytes never trusted')],
    [bytes, Buffer.from('x')],
  ]) {
    await assert.rejects(() =>
      verifyRawStagingStream(manifest, expected, github, Readable.from(chunks)));
  }
  await assert.rejects(() =>
    verifyRawStagingStream(manifest, expected, github, Readable.from(['not binary'])));
  async function* interrupted() { yield bytes; throw new Error('stream interrupted'); }
  await assert.rejects(() =>
    verifyRawStagingStream(manifest, expected, github, Readable.from(interrupted())),
    /stream interrupted/);
});

function stagedEntryFixture(stage = 'normal') {
  const manifest = fixture();
  const names = manifest.assets.filter(a => stage === 'normal'
    ? !a.name.endsWith('.mcpb') && a.name !== 'mcpb-checksums.txt'
    : a.name.endsWith('.mcpb') || a.name === 'mcpb-checksums.txt');
  return { manifest, entries: names.map(a => ({
    name: a.name, type: 'file', size: a.size, compressedSize: Math.max(1, a.size - 8),
  })) };
}

test('checks a flat exact archive inventory before any extraction', () => {
  for (const stage of ['normal', 'mcpb']) {
    const { manifest, entries } = stagedEntryFixture(stage);
    const names = validateStagingArchiveEntries(manifest, stage, entries);
    assert.equal(names.size, stage === 'normal' ? 15 : 7);
    assert.ok(names.has(stage === 'normal' ? 'checksums.txt' : 'mcpb-checksums.txt'));
  }
});

test('refuses unexpected paths, symlinks, duplicates and inconsistent archive metadata', () => {
  const { manifest, entries } = stagedEntryFixture();
  const changes = [
    (a) => { a[0].name = '../outside'; },
    (a) => { a[0].name = '/absolute'; },
    (a) => { a[0].name = 'dir\\\\escape'; },
    (a) => { a[1].name = a[0].name; },
    (a) => { a[0].type = 'symlink'; },
    (a) => { a[0].type = 'directory'; },
    (a) => { a[0].size++; },
    (a) => { a[0].compressedSize = -1; },
    (a) => { a[0].compressedSize = 2 ** 43; },
    (a) => { a[0].unixMode = 0o120777; },
    (a) => { a[0].unixMode = 0o040755; },
    (a) => { a[0].unixMode = 0o100644 + 2 ** 32; },
    (a) => { a[0].unixMode = 0o100644 + 0.5; },
    (a) => { a[0].unixMode = 0o104755; },
  ];
  for (const change of changes) {
    const copy = entries.map(e => ({ ...e }));
    change(copy);
    assert.throws(() => validateStagingArchiveEntries(manifest, 'normal', copy));
  }
  assert.throws(() => validateStagingArchiveEntries(manifest, 'normal', entries.slice(1)));
  assert.throws(() => validateStagingArchiveEntries(manifest, 'mcpb', entries));
  assert.throws(() => validateStagingArchiveEntries(manifest, 'latest', entries));
  assert.throws(() => validateStagingArchiveEntries(manifest, 'normal', null));
});

test('rejects malformed remote input and duplicate remote asset ids', () => {
  const manifest = fixture();
  assert.throws(() => verifyDraftAssetSet(manifest, release(), null));
  const assets = remote(manifest);
  assets[1].id = assets[0].id;
  assert.throws(() => verifyDraftAssetSet(manifest, release(), assets), /duplicate/);
});
