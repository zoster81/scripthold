'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const { expectedReleaseAssetNames, rawStagingArtifactName } =
  require('./release-asset-contract');
const { assembleRawReleaseStaging } = require('./release-staging-producers');

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
