'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { createHash } = require('node:crypto');
const { Readable } = require('node:stream');
const { expectedReleaseAssetNames, rawStagingArtifactName,
  validateStagedManifest, planRawStagingBatch } = require('./release-asset-contract');
const { assembleRawReleaseStaging, assembleRawNormalHandoff,
  planRawNormalPrivateStage, verifyRawNormalHandoffPayloads } = require('./release-staging-producers');
const { readRawNormalStaging, readRawCompleteStaging,
  readJobVerifiedRawNormalStaging, readJobVerifiedRawCompleteStaging } =
  require('./release-staging-rest');

const TAG = 'v3.4.0';
const COMMIT = 'a'.repeat(40);
const REPOSITORY = 'zoster81/scripthold';
const REPOSITORY_ID = 9201;
const RUN_ID = 53001;

function fixture() {
  const all = expectedReleaseAssetNames(TAG).map((name, index) => ({
    name, size: 200 + index,
    sha256: (index + 1).toString(16).padStart(64, '0'),
  }));
  const makeStage = (stage, producerAttempt) => {
    const assets = all.filter(({ name }) => stage === 'mcpb'
      ? name.endsWith('.mcpb') || name === 'mcpb-checksums.txt'
      : !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
    return {
      manifest: { repository: REPOSITORY, tag: TAG, commit: COMMIT, stage, assets },
      repositoryId: REPOSITORY_ID, runId: RUN_ID, producerAttempt,
      uploads: assets.map((asset, index) => {
        const uploadId = (stage === 'normal' ? 1000 : 2000) + index;
        return {
          fileName: asset.name, uploadId, uploadDigest: asset.sha256,
          remote: {
            id: uploadId, expired: false,
            name: rawStagingArtifactName(stage, RUN_ID, producerAttempt, asset.name),
            size_in_bytes: asset.size, digest: 'sha256:' + asset.sha256,
            workflow_run: {
              id: RUN_ID, repository_id: REPOSITORY_ID,
              head_repository_id: REPOSITORY_ID, head_sha: COMMIT,
            },
          },
        };
      }),
    };
  };
  return { normal: makeStage('normal', 1), mcpb: makeStage('mcpb', 3) };
}

test('assembles the exact 15+7 raw producer evidence into one canonical read-only batch', () => {
  const { normal, mcpb } = fixture();
  const result = assembleRawReleaseStaging(normal, mcpb);
  assert.deepEqual(result.manifest.assets.map(a => a.name), expectedReleaseAssetNames(TAG));
  assert.deepEqual(result.batch.actionInputs, {
    'artifact-ids': result.batch.files.map(f => String(f.artifactId)).join(','),
    'merge-multiple': true, 'skip-decompress': true, 'digest-mismatch': 'error',
  });
  assert.equal(result.batch.files.length, 22);
  assert.equal(new Set(result.batch.files.map(f => f.artifactId)).size, 22);
  assert.deepEqual(result.batch.files.map(f => f.fileName), expectedReleaseAssetNames(TAG));
  assert.equal(result.batch.files.filter(f => f.fileName.endsWith('.mcpb')).length, 6);
  assert.ok(!JSON.stringify(result).includes('workflow_run'));
  assert.ok(!JSON.stringify(result).includes('remote'));
  assert.ok(!JSON.stringify(result).includes('C:\\\\'));
  normal.manifest.assets[0].size = 9;
  normal.uploads[0].remote.digest = 'sha256:' + 'f'.repeat(64);
  assert.equal(result.manifest.assets[0].size, 200, 'output must not alias mutable source');
});

test('raw assembly is independent of producer/upload enumeration order', () => {
  const canonical = fixture();
  const shuffled = fixture();
  shuffled.normal.manifest.assets.reverse();
  shuffled.mcpb.manifest.assets.reverse();
  shuffled.normal.uploads.reverse();
  shuffled.mcpb.uploads.reverse();
  assert.deepEqual(assembleRawReleaseStaging(shuffled.normal, shuffled.mcpb),
    assembleRawReleaseStaging(canonical.normal, canonical.mcpb));
});

test('rejects missing, duplicated, extra or swapped producer records', () => {
  for (const mutate of [
    f => { f.normal.uploads.pop(); },
    f => { f.mcpb.uploads.pop(); },
    f => { f.normal.uploads.push(f.normal.uploads[0]); },
    f => { f.mcpb.uploads[1] = structuredClone(f.mcpb.uploads[0]); },
    f => { f.normal.uploads[0].fileName = f.mcpb.uploads[0].fileName; },
    f => { f.normal.manifest.stage = 'mcpb'; },
    f => { f.mcpb.manifest.stage = 'normal'; },
    f => { f.normal.uploads[0].fileName = '../path'; },
    f => { f.mcpb.uploads[0].fileName = 'SCRIPH~1.MCPB'; },
  ]) {
    const f = fixture();
    mutate(f);
    assert.throws(() => assembleRawReleaseStaging(f.normal, f.mcpb));
  }
});

test('rejects mismatched repository, tag, source commit, producer workflow or repository ID', () => {
  for (const mutate of [
    f => { f.normal.manifest.repository = 'attacker/fork'; },
    f => { f.mcpb.manifest.tag = 'v3.4.1'; },
    f => { f.normal.manifest.commit = 'b'.repeat(40); },
    f => { f.mcpb.runId = RUN_ID + 1; },
    f => { f.normal.repositoryId = REPOSITORY_ID + 1; },
    f => { f.mcpb.producerAttempt = 0; },
    f => { f.normal.runId = '53001'; },
    f => { f.normal.producerAttempt = Number.MAX_SAFE_INTEGER + 1; },
    f => { f.normal.uploads[1].remote.workflow_run.head_sha = 'b'.repeat(40); },
    f => { f.normal.uploads[1].remote.workflow_run.repository_id = REPOSITORY_ID + 1; },
    f => { f.mcpb.uploads[1].remote.workflow_run.id = RUN_ID + 1; },
  ]) {
    const f = fixture();
    mutate(f);
    assert.throws(() => assembleRawReleaseStaging(f.normal, f.mcpb));
  }
});

test('rejects forged IDs, digests, expired uploads, attempt and file-name substitution', () => {
  for (const mutate of [
    f => { f.normal.uploads[0].uploadId = f.mcpb.uploads[0].uploadId; },
    f => { f.mcpb.uploads[0].uploadId = -1; },
    f => { f.normal.uploads[0].uploadDigest = 'f'.repeat(64); },
    f => { f.normal.uploads[0].remote.digest = 'sha256:' + 'f'.repeat(64); },
    f => { f.mcpb.uploads[0].remote.expired = true; },
    f => { f.mcpb.uploads[0].remote.name = rawStagingArtifactName('mcpb', RUN_ID, 2, f.mcpb.uploads[0].fileName); },
    f => { f.normal.uploads[0].remote.size_in_bytes++; },
    f => { f.normal.uploads[0].remote.id++; },
    f => { f.normal.uploads[0].remote.workflow_run.head_repository_id = 9999; },
  ]) {
    const f = fixture();
    mutate(f);
    assert.throws(() => assembleRawReleaseStaging(f.normal, f.mcpb));
  }
});

test('refuses unexpected producer schema fields, especially path or mutable authority', () => {
  for (const mutate of [
    f => { f.normal.uploads[0].path = '/runner/private/file'; },
    f => { f.normal.path = 'C:\\\\temporary\\\\asset'; },
    f => { f.mcpb.manifest.localDir = '/var/tmp'; },
    f => { f.mcpb.manifest.assets[0].absolutePath = '/tmp/file'; },
    f => { f.normal.uploads[0].latest = true; },
    f => { f.normal.manifest.assets[0].url = 'https://attacker.example'; },
  ]) {
    const f = fixture();
    mutate(f);
    assert.throws(() => assembleRawReleaseStaging(f.normal, f.mcpb));
  }
});

test('never uses an invalid producer, missing remote observation or nonarrays as authority', () => {
  const f = fixture();
  for (const pair of [
    [null, f.mcpb], [f.normal, undefined], [[], f.mcpb],
    [{ ...f.normal, uploads: null }, f.mcpb],
    [f.normal, { ...f.mcpb, uploads: {} }],
    [f.normal, { ...f.mcpb, uploads: Array(7).fill(null) }],
  ]) {
    assert.throws(() => assembleRawReleaseStaging(...pair));
  }
  f.normal.uploads[0].remote = null;
  assert.throws(() => assembleRawReleaseStaging(f.normal, f.mcpb));
});

test('normal-only handoff produces exactly 15 authenticated raw IDs for MCPB input', () => {
  const { normal } = fixture();
  const result = assembleRawNormalHandoff(normal);
  const names = expectedReleaseAssetNames(TAG).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
  assert.equal(result.manifest.stage, 'normal');
  assert.deepEqual(result.manifest.assets.map(a => a.name), names);
  assert.deepEqual(result.batch.files.map(f => f.fileName), names);
  assert.equal(result.batch.files.length, 15);
  assert.equal(new Set(result.batch.files.map(f => f.artifactId)).size, 15);
  assert.deepEqual(result.batch.actionInputs, {
    'artifact-ids': result.batch.files.map(f => String(f.artifactId)).join(','),
    'merge-multiple': true, 'skip-decompress': true, 'digest-mismatch': 'error',
  });
  assert.throws(() => validateStagedManifest(result.manifest), /22 assets/,
    'final publication still requires a complete 22-file manifest');
  assert.throws(() => planRawStagingBatch(result.manifest, []), /22 assets/,
    'full raw-download planner must never accept the normal-only handoff');
  assert.ok(!JSON.stringify(result).includes('workflow_run'));
  assert.ok(!JSON.stringify(result).includes('/tmp'));
  normal.manifest.assets[0].size = 1;
  normal.uploads[0].remote.digest = 'sha256:' + 'f'.repeat(64);
  assert.equal(result.manifest.assets[0].size, 200);
});

test('normal handoff projects exact verified raw originals for private Go staging', () => {
  const { normal } = fixture();
  const { batch } = assembleRawNormalHandoff(normal);
  const projected = planRawNormalPrivateStage(normal);
  assert.deepEqual(projected, batch.files.map(file => ({
    downloadName: file.artifactName,
    original: { name: file.fileName, size: file.size, sha256: file.sha256 },
  })));
  assert.equal(projected.length, 15);
  assert.equal(new Set(projected.map(file => file.downloadName)).size, 15);
  assert.ok(!JSON.stringify(projected).includes('artifactId'));
  assert.ok(!JSON.stringify(projected).includes('remote'));
  normal.manifest.assets[0].size = 1;
  normal.uploads[0].remote.digest = 'sha256:' + 'f'.repeat(64);
  assert.equal(projected[0].original.size, batch.files[0].size);
  assert.deepEqual(projected[0].original.sha256, batch.files[0].sha256);
});

test('normal Go staging projection refuses unauthenticated producer uploads', () => {
  for (const mutate of [
    n => { n.uploads[0].uploadDigest = 'f'.repeat(64); },
    n => { n.uploads[0].remote.digest = 'sha256:' + 'f'.repeat(64); },
    n => { n.uploads[0].remote.name = 'wrong-name'; },
    n => { n.uploads[0].uploadId = n.uploads[1].uploadId; },
    n => { n.uploads.pop(); },
  ]) {
    const { normal } = fixture();
    mutate(normal);
    assert.throws(() => planRawNormalPrivateStage(normal));
  }
});

test('normal handoff authenticates all 15 original streams before MCPB packaging', async () => {
  const { normal, observed, bytes } = normalByteFixture();
  const opened = [];
  const result = await verifyRawNormalHandoffPayloads(normal, observed.slice().reverse(), (file) => {
    opened.push(file.fileName);
    return Readable.from([bytes.get(file.fileName)]);
  });
  assert.equal(result.size, 15);
  assert.deepEqual(opened, expectedReleaseAssetNames(TAG).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt'));
});

test('normal handoff rejects incomplete directory before opening a source', async () => {
  const { normal, observed, bytes } = normalByteFixture();
  for (const mutate of [
    x => x.pop(),
    x => { x[1] = x[0]; },
    x => { x[0].type = 'symlink'; },
    x => { x[0].nlink = 2; },
    x => { x[0].name = '../escape'; },
    x => { x[0].size++; },
  ]) {
    const invalid = structuredClone(observed);
    mutate(invalid);
    let opened = 0;
    await assert.rejects(() => verifyRawNormalHandoffPayloads(normal, invalid,
      (file) => { opened++; return Readable.from([bytes.get(file.fileName)]); }));
    assert.equal(opened, 0, 'no streams before complete inventory verification');
  }
});

test('normal handoff rejects an opener that forges its own byte authority', async () => {
  const { normal, observed, bytes } = normalByteFixture();
  const forged = Buffer.from('forged normal payload');
  let opened = 0;
  await assert.rejects(() => verifyRawNormalHandoffPayloads(normal, observed, (file) => {
    opened++;
    if (opened === 1) {
      file.size = forged.length;
      file.sha256 = createHash('sha256').update(forged).digest('hex');
      return Readable.from([forged]);
    }
    return Readable.from([bytes.get(file.fileName)]);
  }), /asset (?:size|digest) mismatch/);
  assert.equal(opened, 1, 'must stop on first forged source');
});

test('normal handoff opener changes cannot corrupt returned trusted inventory', async () => {
  const { normal, observed, bytes } = normalByteFixture();
  let original;
  let firstName;
  const verified = await verifyRawNormalHandoffPayloads(normal, observed, (file) => {
    if (!firstName) {
      firstName = file.fileName;
      original = { size: file.size, sha256: file.sha256 };
      file.size = 1;
      file.sha256 = '0'.repeat(64);
    }
    return Readable.from([bytes.get(file.fileName)]);
  });
  assert.equal(verified.size, 15);
  assert.equal(verified.get(firstName).size, original.size);
  assert.equal(verified.get(firstName).sha256, original.sha256);
});

test('normal handoff refuses corrupt and interrupted streams', async () => {
  const { normal, observed, bytes } = normalByteFixture();
  for (const failure of ['changed', 'truncated', 'interrupted']) {
    let opened = 0;
    await assert.rejects(() => verifyRawNormalHandoffPayloads(normal, observed, (file) => {
      opened++;
      const source = bytes.get(file.fileName);
      if (opened !== 4) return Readable.from([source]);
      if (failure === 'changed') return Readable.from([Buffer.alloc(source.length)]);
      if (failure === 'truncated') return Readable.from([source.subarray(1)]);
      return Readable.from((async function* () {
        yield source;
        throw new Error('interrupted normal-stage transfer');
      })());
    }));
    assert.equal(opened, 4);
  }
});

function normalByteFixture() {
  const { normal } = fixture();
  const bytes = new Map();
  for (const asset of normal.manifest.assets) {
    const content = Buffer.from('normal-stage original:' + asset.name);
    bytes.set(asset.name, content);
    asset.size = content.length;
    asset.sha256 = createHash('sha256').update(content).digest('hex');
    const upload = normal.uploads.find(item => item.fileName === asset.name);
    upload.uploadDigest = asset.sha256;
    upload.remote.size_in_bytes = asset.size;
    upload.remote.digest = 'sha256:' + asset.sha256;
  }
  const observed = assembleRawNormalHandoff(normal).batch.files.map(file => ({
    name: file.artifactName, type: 'file', nlink: 1, size: file.size,
  }));
  return { normal, observed, bytes };
}

test('normal handoff cannot depend on producer enumeration order', () => {
  const { normal } = fixture();
  const shuffled = structuredClone(normal);
  shuffled.manifest.assets.reverse();
  shuffled.uploads.reverse();
  assert.deepEqual(assembleRawNormalHandoff(shuffled), assembleRawNormalHandoff(normal));
});

test('normal handoff rejects incomplete, forged, mixed and path-bearing input', () => {
  for (const mutate of [
    f => { f.normal.uploads.pop(); },
    f => { f.normal.uploads.push(f.normal.uploads[0]); },
    f => { f.normal.uploads[0].uploadId = f.normal.uploads[1].uploadId; },
    f => { f.normal.uploads[0].fileName = '../escape'; },
    f => { f.normal.uploads[0].remote.expired = true; },
    f => { f.normal.uploads[0].remote.size_in_bytes++; },
    f => { f.normal.uploads[0].uploadDigest = 'b'.repeat(64); },
    f => { f.normal.uploads[0].remote.workflow_run.id++; },
    f => { f.normal.uploads[0].remote.name = rawStagingArtifactName(
      'normal', RUN_ID, 2, f.normal.uploads[0].fileName); },
    f => { f.normal.manifest.stage = 'mcpb'; },
    f => { f.normal.manifest.tag = 'v3.5.0'; },
    f => { f.normal.manifest.commit = 'c'.repeat(40); },
    f => { f.normal.manifest.assets[0].filePath = '/tmp/producer'; },
    f => { f.normal.uploads[0].path = 'C:\\\\unsafe'; },
  ]) {
    const f = fixture();
    mutate(f);
    assert.throws(() => assembleRawNormalHandoff(f.normal));
  }
});

function rawRestFixture() {
  const verified = fixture();
  const remoteById = new Map(
    [...verified.normal.uploads, ...verified.mcpb.uploads]
      .map((upload) => [upload.uploadId, upload.remote]));
  const omitRemote = (producer) => ({
    ...producer,
    uploads: producer.uploads.map(({ fileName, uploadId, uploadDigest }) =>
      ({ fileName, uploadId, uploadDigest })),
  });
  return { normal: omitRemote(verified.normal), mcpb: omitRemote(verified.mcpb),
    remoteById };
}

const ARTIFACT_API = 'https://api.github.com/repos/zoster81/scripthold/actions/artifacts/';

function rawRestClient(artifacts) {
  const calls = [];
  return {
    calls,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      assert.match(url, /^https:\/\/api\.github\.com\/repos\/zoster81\/scripthold\/actions\/artifacts\/[1-9][0-9]*$/);
      const id = Number(url.slice(ARTIFACT_API.length));
      return new Response(JSON.stringify(artifacts.get(id)), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    },
  };
}

test('authenticated GitHub GETs bind the exact 15 normal stage artifact IDs', async () => {
  const { normal, remoteById } = rawRestFixture();
  remoteById.get(1000).localPath = '/var/tmp/runner-private';
  remoteById.get(1000).download_url = 'https://attacker.example/resource';
  const client = rawRestClient(remoteById);
  const value = await readRawNormalStaging({
    normal, token: 'test-token', fetchImpl: client.fetchImpl,
  });
  const expected = assembleRawNormalHandoff(fixture().normal);
  assert.deepEqual(value.manifest, expected.manifest);
  assert.deepEqual(value.batch, expected.batch);
  assert.deepEqual(value.producer.uploads.map(u => u.uploadId),
    expected.batch.files.map(f => f.artifactId));
  assert.deepEqual(client.calls.map(c => c.url),
    expected.batch.files.map(f => ARTIFACT_API + f.artifactId));
  assert.equal(client.calls.length, 15);
  for (const { init } of client.calls) {
    assert.equal(init.method, 'GET');
    assert.equal(init.redirect, 'error');
    assert.equal(init.cache, 'no-store');
    assert.equal(init.headers.Authorization, 'Bearer test-token');
  }
  assert.ok(!JSON.stringify(value).includes('test-token'));
  assert.ok(!JSON.stringify(value).includes('/var/tmp'));
  assert.ok(!JSON.stringify(value).includes('attacker.example'));
});

test('authenticated REST observation is usable by the separate complete byte-stream gate', async () => {
  const { normal, observed, bytes } = normalByteFixture();
  const remoteById = new Map(normal.uploads.map(u => [u.uploadId, u.remote]));
  const source = { ...normal, uploads: normal.uploads.map(
    ({ fileName, uploadId, uploadDigest }) => ({ fileName, uploadId, uploadDigest })) };
  const client = rawRestClient(remoteById);
  const received = await readRawNormalStaging({
    normal: source, token: 'test-token', fetchImpl: client.fetchImpl,
  });
  const verified = await verifyRawNormalHandoffPayloads(received.producer, observed,
    (file) => Readable.from([bytes.get(file.fileName)]));
  assert.equal(verified.size, 15);
});

test('authenticated REST adapter assembles an exact 15+7 immutable-ID manifest', async () => {
  const { normal, mcpb, remoteById } = rawRestFixture();
  normal.uploads.reverse();
  mcpb.uploads.reverse();
  normal.manifest.assets.reverse();
  mcpb.manifest.assets.reverse();
  const client = rawRestClient(remoteById);
  const value = await readRawCompleteStaging({
    normal, mcpb, token: 'test-token', fetchImpl: client.fetchImpl,
  });
  assert.deepEqual(value.manifest.assets.map(a => a.name), expectedReleaseAssetNames(TAG));
  assert.equal(value.batch.files.length, 22);
  assert.deepEqual(value.batch.files.map(f => f.artifactId),
    client.calls.map(c => Number(c.url.slice(ARTIFACT_API.length))));
  assert.deepEqual(value.batch.actionInputs['artifact-ids'],
    value.batch.files.map(f => String(f.artifactId)).join(','));
});

test('rejects incomplete producer IDs and mixed provenance before any GitHub REST request', async () => {
  for (const mutate of [
    f => { f.normal.uploads.pop(); },
    f => { f.normal.uploads[1].uploadId = f.normal.uploads[0].uploadId; },
    f => { f.normal.uploads[0].uploadId = 0; },
    f => { f.normal.uploads[0].uploadId = '1000'; },
    f => { f.normal.uploads[0].fileName = '../escape'; },
    f => { f.normal.uploads[0].uploadDigest = 'f'.repeat(64); },
    f => { f.normal.uploads[0].path = '/tmp/outside'; },
    f => { f.normal.repositoryId = 0; },
    f => { f.normal.producerAttempt = 0; },
    f => { f.normal.manifest.stage = 'mcpb'; },
    f => { f.normal.manifest.assets[0].absolutePath = '/var/tmp/copy'; },
    f => { f.mcpb.runId++; },
    f => { f.mcpb.manifest.commit = 'b'.repeat(40); },
    f => { f.mcpb.uploads[0].uploadId = f.normal.uploads[0].uploadId; },
  ]) {
    const f = rawRestFixture();
    mutate(f);
    const client = rawRestClient(f.remoteById);
    await assert.rejects(readRawCompleteStaging({
      normal: f.normal, mcpb: f.mcpb, token: 'test-token',
      fetchImpl: client.fetchImpl,
    }));
    assert.equal(client.calls.length, 0, 'no API read after invalid producer inventory');
  }
});

test('rejects wrong or expired GitHub artifacts even from an exact ID endpoint', async () => {
  for (const mutate of [
    r => { r.id++; },
    r => { r.name = 'name-for-another-upload'; },
    r => { r.expired = true; },
    r => { r.size_in_bytes++; },
    r => { r.digest = 'sha256:' + 'f'.repeat(64); },
    r => { r.workflow_run.id++; },
    r => { r.workflow_run.head_sha = 'b'.repeat(40); },
    r => { r.workflow_run.repository_id++; },
  ]) {
    const f = rawRestFixture();
    const record = structuredClone(f.remoteById.get(1000));
    mutate(record);
    f.remoteById.set(1000, record);
    const client = rawRestClient(f.remoteById);
    await assert.rejects(readRawNormalStaging({
      normal: f.normal, token: 'test-token', fetchImpl: client.fetchImpl,
    }));
  }
});

test('raw GitHub artifact reader rejects unauthorized, redirect and corrupt REST responses', async () => {
  const { normal } = rawRestFixture();
  const responses = [
    new Response('forbidden', { status: 403 }),
    new Response(null, { status: 302,
      headers: { location: 'https://attacker.example/data' } }),
    new Response('not-json', { status: 200,
      headers: { 'content-type': 'text/plain' } }),
    new Response('{', { status: 200,
      headers: { 'content-type': 'application/json' } }),
  ];
  for (const response of responses) {
    let requested = 0;
    await assert.rejects(readRawNormalStaging({
      normal, token: 'test-token',
      fetchImpl: async (_url, init) => {
        requested++;
        assert.equal(init.method, 'GET');
        return response;
      },
    }));
    assert.equal(requested, 1);
  }
});

test('raw REST reader never accepts malformed auth or an untrusted client', async () => {
  const { normal } = rawRestFixture();
  let fetched = false;
  for (const patch of [
    { token: '' }, { token: 'not\nvalid' }, { fetchImpl: null },
  ]) {
    await assert.rejects(readRawNormalStaging({
      normal, token: 'test-token', fetchImpl: async () => { fetched = true; },
      ...patch,
    }));
  }
  assert.equal(fetched, false);
});

const JOBS_API = 'https://api.github.com/repos/zoster81/scripthold/actions/runs/';
const SOURCE_POLICY = Object.freeze({
  workflowId: 17,
  workflowPath: '.github/workflows/immutable-staging.yml',
  event: 'push',
  normalJobName: 'Produce normal assets',
  mcpbJobName: 'Produce MCPB assets',
});

function jobRestFixture() {
  const f = rawRestFixture();
  const run = {
    id: RUN_ID, run_attempt: 3, workflow_id: SOURCE_POLICY.workflowId,
    path: SOURCE_POLICY.workflowPath, event: SOURCE_POLICY.event,
    head_sha: COMMIT, status: 'in_progress',
    repository: { id: REPOSITORY_ID, full_name: REPOSITORY },
    head_repository: { id: REPOSITORY_ID, full_name: REPOSITORY },
  };
  const normalJob = {
    id: 801, name: SOURCE_POLICY.normalJobName, run_id: RUN_ID,
    run_attempt: 1, head_sha: COMMIT,
    status: 'completed', conclusion: 'success',
  };
  const mcpbJob = {
    id: 803, name: SOURCE_POLICY.mcpbJobName, run_id: RUN_ID,
    run_attempt: 3, head_sha: COMMIT,
    status: 'completed', conclusion: 'success',
  };
  const pages = new Map([[1, [normalJob]], [3, [mcpbJob]]]);
  const calls = [];
  const fetchImpl = async (url, init) => {
    calls.push(url);
    assert.equal(init.method, 'GET');
    assert.equal(init.redirect, 'error');
    const attempt = /^https:\/\/api\.github\.com\/repos\/zoster81\/scripthold\/actions\/runs\/53001\/attempts\/([1-9]\d*)\/jobs\?per_page=100&page=1$/.exec(url);
    let payload;
    if (url === JOBS_API + RUN_ID) payload = run;
    else if (attempt) {
      const jobs = pages.get(Number(attempt[1])) ?? [];
      payload = { total_count: jobs.length, jobs };
    } else if (url.startsWith(ARTIFACT_API)) {
      payload = f.remoteById.get(Number(url.slice(ARTIFACT_API.length)));
    } else {
      assert.fail('Unexpected authenticated REST endpoint: ' + url);
    }
    return new Response(JSON.stringify(payload), {
      status: 200, headers: { 'content-type': 'application/json' },
    });
  };
  return { ...f, run, normalJob, mcpbJob, pages, calls, fetchImpl };
}

test('source job proof binds the 15-file handoff to a successful exact attempt', async () => {
  const f = jobRestFixture();
  const result = await readJobVerifiedRawNormalStaging({
    normal: f.normal, sourcePolicy: SOURCE_POLICY, token: 'test-token',
    fetchImpl: f.fetchImpl,
  });
  assert.equal(result.batch.files.length, 15);
  assert.deepEqual(result.jobs, [{ stage: 'normal', jobId: 801, runAttempt: 1 }]);
  assert.deepEqual(f.calls.slice(0, 3), [
    JOBS_API + RUN_ID,
    JOBS_API + RUN_ID + '/attempts/1/jobs?per_page=100&page=1',
    JOBS_API + RUN_ID,
  ]);
  assert.equal(f.calls.length, 18);
});

test('source job proof authenticates two independent producer attempts', async () => {
  const f = jobRestFixture();
  const result = await readJobVerifiedRawCompleteStaging({
    normal: f.normal, mcpb: f.mcpb, sourcePolicy: SOURCE_POLICY,
    token: 'test-token', fetchImpl: f.fetchImpl,
  });
  assert.equal(result.batch.files.length, 22);
  assert.deepEqual(result.jobs, [
    { stage: 'normal', jobId: 801, runAttempt: 1 },
    { stage: 'mcpb', jobId: 803, runAttempt: 3 },
  ]);
  assert.deepEqual(f.calls.slice(0, 4), [
    JOBS_API + RUN_ID,
    JOBS_API + RUN_ID + '/attempts/1/jobs?per_page=100&page=1',
    JOBS_API + RUN_ID + '/attempts/3/jobs?per_page=100&page=1',
    JOBS_API + RUN_ID,
  ]);
  assert.equal(f.calls.length, 26);
});

test('source job proof rejects wrong workflow, job, SHA, attempts and duplication', async () => {
  for (const mutate of [
    f => { f.run.workflow_id++; },
    f => { f.run.path += '/different'; },
    f => { f.run.event = 'pull_request'; },
    f => { f.run.head_sha = 'b'.repeat(40); },
    f => { f.run.repository.id++; },
    f => { f.run.head_repository.full_name = 'attacker/fork'; },
    f => { f.run.run_attempt = 2; },
    f => { f.normalJob.run_attempt = 3; },
    f => { f.normalJob.head_sha = 'b'.repeat(40); },
    f => { f.normalJob.conclusion = 'failure'; },
    f => { f.normalJob.status = 'in_progress'; },
    f => { f.pages.get(1).push(structuredClone(f.normalJob)); },
    f => { f.normalJob.name = 'Untrusted job'; },
  ]) {
    const f = jobRestFixture();
    mutate(f);
    await assert.rejects(readJobVerifiedRawCompleteStaging({
      normal: f.normal, mcpb: f.mcpb, sourcePolicy: SOURCE_POLICY,
      token: 'test-token', fetchImpl: f.fetchImpl,
    }));
    assert.equal(f.calls.some(url => url.startsWith(ARTIFACT_API)), false);
  }
});

test('invalid job policy or producer inventory causes zero GitHub reads', async () => {
  for (const sourcePolicy of [
    null, { ...SOURCE_POLICY, workflowId: '17' },
    { ...SOURCE_POLICY, normalJobName: '../wrong' },
    { ...SOURCE_POLICY, source: 'attacker' },
  ]) {
    const f = jobRestFixture();
    await assert.rejects(readJobVerifiedRawNormalStaging({
      normal: f.normal, sourcePolicy, token: 'test-token', fetchImpl: f.fetchImpl,
    }));
    assert.equal(f.calls.length, 0);
  }
  const f = jobRestFixture();
  f.normal.uploads[0].uploadId = 0;
  await assert.rejects(readJobVerifiedRawNormalStaging({
    normal: f.normal, sourcePolicy: SOURCE_POLICY,
    token: 'test-token', fetchImpl: f.fetchImpl,
  }));
  assert.equal(f.calls.length, 0);
});

test('source job reader rejects missing pages, pagination and changed run attempts', async () => {
  const f = jobRestFixture();
  const original = f.fetchImpl;
  let runReads = 0;
  f.fetchImpl = async (url, init) => {
    const response = await original(url, init);
    if (url === JOBS_API + RUN_ID && ++runReads === 2) {
      return new Response(JSON.stringify({ ...f.run, run_attempt: 4 }), {
        status: 200, headers: { 'content-type': 'application/json' },
      });
    }
    return response;
  };
  await assert.rejects(readJobVerifiedRawNormalStaging({
    normal: f.normal, sourcePolicy: SOURCE_POLICY,
    token: 'test-token', fetchImpl: f.fetchImpl,
  }), /rerun/);
  assert.equal(f.calls.some(url => url.startsWith(ARTIFACT_API)), false);

  for (const corrupt of ['partial', 'paginated', 'oversized', 'foreign']) {
    const g = jobRestFixture();
    const base = g.fetchImpl;
    g.fetchImpl = async (url, init) => {
      const response = await base(url, init);
      if (!url.includes('/attempts/1/jobs?')) return response;
      if (corrupt === 'foreign') {
        return new Response(JSON.stringify({ total_count: 1,
          jobs: [{ ...g.normalJob, run_id: RUN_ID + 1 }] }), {
          status: 200, headers: { 'content-type': 'application/json' },
        });
      }
      if (corrupt === 'partial') {
        return new Response(JSON.stringify({ total_count: 2, jobs: [g.normalJob] }), {
          status: 200, headers: { 'content-type': 'application/json' },
        });
      }
      if (corrupt === 'paginated') {
        return new Response(JSON.stringify({ total_count: 1, jobs: [g.normalJob] }), {
          status: 200,
          headers: { 'content-type': 'application/json',
            link: '<https://api.github.com/repos/zoster81/scripthold/actions/runs/53001/attempts/1/jobs?per_page=100&page=2>; rel="next"' },
        });
      }
      return new Response(JSON.stringify({ total_count: 1, jobs: [g.normalJob] }), {
        status: 200, headers: { 'content-type': 'application/json',
          'content-length': '900000' },
      });
    };
    await assert.rejects(readJobVerifiedRawNormalStaging({
      normal: g.normal, sourcePolicy: SOURCE_POLICY,
      token: 'test-token', fetchImpl: g.fetchImpl,
    }));
    assert.equal(g.calls.some(url => url.startsWith(ARTIFACT_API)), false);
  }
});

test('source job evidence never authorizes a failed completed workflow run', async () => {
  const f = jobRestFixture();
  f.run.status = 'completed';
  f.run.conclusion = 'failure';
  await assert.rejects(readJobVerifiedRawNormalStaging({
    normal: f.normal, sourcePolicy: SOURCE_POLICY,
    token: 'test-token', fetchImpl: f.fetchImpl,
  }));
  assert.equal(f.calls.filter(url => url.startsWith(ARTIFACT_API)).length, 0);
});

test('job verification pins prevalidated producer and policy snapshots across awaits', async () => {
  const f = jobRestFixture();
  const policy = { ...SOURCE_POLICY };
  const original = f.fetchImpl;
  let mutated = false;
  const fetchImpl = (url, init) => {
    if (!mutated && url === JOBS_API + RUN_ID) {
      mutated = true;
      policy.workflowId = 99999;
      f.normal.runId = 9;
      f.normal.uploads[0].uploadId = 99999;
      f.normal.manifest.assets[0].sha256 = 'f'.repeat(64);
    }
    return original(url, init);
  };
  const result = await readJobVerifiedRawNormalStaging({
    normal: f.normal, sourcePolicy: policy, token: 'test-token', fetchImpl,
  });
  assert.equal(mutated, true);
  assert.equal(result.batch.files.length, 15);
  assert.equal(result.batch.files[0].artifactId, 1000);
  assert.ok(!f.calls.some(url => url.includes('99999')));
  assert.deepEqual(result.jobs, [{ stage: 'normal', jobId: 801, runAttempt: 1 }]);
});
