'use strict';

const { createHash } = require('node:crypto');

const REPOSITORY = 'zoster81/scripthold';
const MAX_ASSET_BYTES = 2 * 1024 * 1024 * 1024;
const SHA256 = /^[0-9a-f]{64}$/;
const COMMIT_SHA = /^[0-9a-f]{40}$/;

const NORMAL_ASSETS = [
  'scripthold_darwin_amd64',
  'scripthold_darwin_amd64.tar.gz',
  'scripthold_darwin_arm64',
  'scripthold_darwin_arm64.tar.gz',
  'scripthold_linux_amd64',
  'scripthold_linux_amd64.tar.gz',
  'scripthold_linux_arm64',
  'scripthold_linux_arm64.tar.gz',
  'scripthold_windows_amd64.exe',
  'scripthold_windows_amd64.zip',
  'scripthold_windows_arm64.exe',
  'scripthold_windows_arm64.zip',
];

const MCPB_ASSETS = [
  'scripthold_darwin_amd64.mcpb',
  'scripthold_darwin_arm64.mcpb',
  'scripthold_linux_amd64.mcpb',
  'scripthold_linux_arm64.mcpb',
  'scripthold_windows_amd64.mcpb',
  'scripthold_windows_arm64.mcpb',
];

function requireCondition(condition, message) {
  if (!condition) throw new Error(message);
}

function expectedReleaseAssetNames(tag) {
  const match = typeof tag === 'string' &&
    /^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$/.exec(tag);
  requireCondition(match && match[0] === tag, 'invalid release tag');
  return [
    ...NORMAL_ASSETS,
    'checksums.txt',
    `scripthold-${tag}-provenance.sigstore.json`,
    `scripthold-${tag}-provenance.intoto.jsonl`,
    ...MCPB_ASSETS,
    'mcpb-checksums.txt',
  ];
}

function validateStagedManifest(manifest) {
  requireCondition(manifest && typeof manifest === 'object' && !Array.isArray(manifest),
    'invalid staging manifest');
  requireCondition(manifest.repository === REPOSITORY, 'unexpected repository');
  requireCondition(typeof manifest.commit === 'string' && COMMIT_SHA.test(manifest.commit),
    'invalid exact source commit');
  const expected = new Set(expectedReleaseAssetNames(manifest.tag));
  requireCondition(Array.isArray(manifest.assets) && manifest.assets.length === expected.size,
    'expected exactly 22 assets');
  const result = new Map();
  for (const asset of manifest.assets) {
    requireCondition(asset && typeof asset === 'object' && !Array.isArray(asset),
      'invalid staged asset');
    requireCondition(typeof asset.name === 'string' && expected.has(asset.name),
      'unexpected staged asset name');
    requireCondition(!result.has(asset.name), 'duplicate staged asset name');
    requireCondition(Number.isSafeInteger(asset.size) && asset.size > 0 &&
      asset.size <= MAX_ASSET_BYTES, 'invalid staged asset size');
    requireCondition(typeof asset.sha256 === 'string' && SHA256.test(asset.sha256),
      'invalid staged sha256');
    result.set(asset.name, { size: asset.size, sha256: asset.sha256 });
  }
  requireCondition(result.size === expected.size, 'missing staged assets');
  return result;
}

function isRecognizedReleaseAssetName(name) {
  return typeof name === 'string' && (
    NORMAL_ASSETS.includes(name) ||
    MCPB_ASSETS.includes(name) ||
    name === 'checksums.txt' ||
    name === 'mcpb-checksums.txt' ||
    /^scripthold-v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?-provenance\.(?:sigstore\.json|intoto\.jsonl)$/.test(name)
  );
}

// The caller owns file authorization and opens the stream; this check verifies actual
// transferred bytes, not only the claimed checksum metadata in a staging manifest.
async function verifyStagedAssetStream(expected, source) {
  requireCondition(expected && typeof expected === 'object' &&
    isRecognizedReleaseAssetName(expected.name), 'unexpected release asset name');
  requireCondition(Number.isSafeInteger(expected.size) && expected.size > 0 &&
    expected.size <= MAX_ASSET_BYTES, 'invalid release asset size');
  requireCondition(typeof expected.sha256 === 'string' && SHA256.test(expected.sha256),
    'invalid release asset digest');
  requireCondition(source && typeof source[Symbol.asyncIterator] === 'function',
    'release asset requires a readable byte stream');

  const digest = createHash('sha256');
  let size = 0;
  for await (const chunk of source) {
    requireCondition(chunk instanceof Uint8Array, 'release asset stream contains nonbinary data');
    size += chunk.byteLength;
    requireCondition(size <= expected.size, 'release asset exceeds its declared size');
    digest.update(chunk);
  }
  requireCondition(size === expected.size, 'release asset size mismatch');
  requireCondition(digest.digest('hex') === expected.sha256, 'release asset digest mismatch');
  return { name: expected.name, size, sha256: expected.sha256 };
}

const MAX_STAGING_ARCHIVE_BYTES = 4 * 1024 * 1024 * 1024;

function validPositiveID(value) {
  return Number.isSafeInteger(value) && value > 0;
}

function stagedArtifactName(stage, runId, producerAttempt) {
  requireCondition(stage === 'normal' || stage === 'mcpb', 'invalid staging group');
  requireCondition(validPositiveID(runId), 'invalid workflow run id');
  requireCondition(validPositiveID(producerAttempt), 'invalid producer run attempt');
  return `scripthold-${stage}-${runId}-${producerAttempt}`;
}

// The expected ID, SHA and digest must come from trusted workflow/job outputs.
// REST metadata alone does not authenticate the payload's extracted file bytes.
function verifyRunStagingArtifact(expected, remote) {
  requireCondition(expected && typeof expected === 'object' && !Array.isArray(expected),
    'invalid expected staging identity');
  requireCondition(expected.repository === REPOSITORY, 'unexpected staging repository');
  requireCondition(validPositiveID(expected.repositoryId), 'invalid staging repository id');
  expectedReleaseAssetNames(expected.tag);
  requireCondition(typeof expected.commit === 'string' && COMMIT_SHA.test(expected.commit),
    'invalid staged source commit');
  const name = stagedArtifactName(expected.stage, expected.runId, expected.producerAttempt);
  requireCondition(validPositiveID(expected.uploadId), 'invalid upload artifact id');
  requireCondition(typeof expected.uploadDigest === 'string' && SHA256.test(expected.uploadDigest),
    'invalid producer artifact digest');
  requireCondition(remote && typeof remote === 'object' && !Array.isArray(remote),
    'invalid GitHub Actions artifact metadata');
  requireCondition(remote.id === expected.uploadId && remote.name === name,
    'Actions artifact identity mismatch');
  requireCondition(remote.expired === false, 'Actions artifact expired or not confirmed');
  requireCondition(Number.isSafeInteger(remote.size_in_bytes) &&
    remote.size_in_bytes > 0 && remote.size_in_bytes <= MAX_STAGING_ARCHIVE_BYTES,
    'invalid Actions archive size');
  requireCondition(remote.digest === 'sha256:' + expected.uploadDigest,
    'Actions artifact archive digest mismatch');
  const run = remote.workflow_run;
  requireCondition(run && typeof run === 'object' && !Array.isArray(run),
    'missing artifact workflow identity');
  requireCondition(run.id === expected.runId && run.repository_id === expected.repositoryId &&
    run.head_repository_id === expected.repositoryId && run.head_sha === expected.commit,
    'Actions artifact workflow provenance mismatch');
  return { artifactId: remote.id, name, archiveDigest: remote.digest, size: remote.size_in_bytes };
}

function verifyDraftAssetSet(manifest, release, remoteAssets) {
  const expected = validateStagedManifest(manifest);
  requireCondition(release && typeof release === 'object' && !Array.isArray(release),
    'invalid GitHub release');
  requireCondition(Number.isSafeInteger(release.id) && release.id > 0,
    'invalid GitHub release id');
  requireCondition(release.tag_name === manifest.tag, 'release tag mismatch');
  requireCondition(release.draft === true && release.immutable === false,
    'GitHub release is not an unpublished mutable draft');
  requireCondition(Array.isArray(remoteAssets) && remoteAssets.length === expected.size,
    'expected exactly 22 uploaded assets');
  const seenNames = new Set();
  const seenIds = new Set();
  for (const asset of remoteAssets) {
    requireCondition(asset && typeof asset === 'object' && !Array.isArray(asset),
      'invalid remote release asset');
    requireCondition(typeof asset.name === 'string' && expected.has(asset.name),
      'unexpected remote release asset name');
    requireCondition(!seenNames.has(asset.name), 'duplicate remote release asset name');
    requireCondition(Number.isSafeInteger(asset.id) && asset.id > 0 &&
      !seenIds.has(asset.id), 'duplicate or invalid remote release asset id');
    requireCondition(asset.state === 'uploaded', 'release asset was not uploaded');
    const staged = expected.get(asset.name);
    requireCondition(asset.size === staged.size, 'release asset size mismatch');
    requireCondition(asset.digest === 'sha256:' + staged.sha256,
      'release asset digest mismatch');
    seenNames.add(asset.name);
    seenIds.add(asset.id);
  }
  requireCondition(seenNames.size === expected.size, 'missing release assets');
  return { releaseId: release.id, tag: manifest.tag, commit: manifest.commit, count: seenNames.size };
}

module.exports = { expectedReleaseAssetNames, validateStagedManifest, verifyDraftAssetSet,
  verifyStagedAssetStream, stagedArtifactName, verifyRunStagingArtifact };
