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

function validateManifestAssets(manifest, stage) {
  requireCondition(manifest && typeof manifest === 'object' && !Array.isArray(manifest),
    'invalid staging manifest');
  requireCondition(manifest.repository === REPOSITORY, 'unexpected repository');
  requireCondition(typeof manifest.commit === 'string' && COMMIT_SHA.test(manifest.commit),
    'invalid exact source commit');
  const names = expectedReleaseAssetNames(manifest.tag);
  const expected = new Set(stage === null ? names : names.filter((name) => {
    const mcpb = name.endsWith('.mcpb') || name === 'mcpb-checksums.txt';
    return (stage === 'mcpb') === mcpb;
  }));
  requireCondition(Array.isArray(manifest.assets) && manifest.assets.length === expected.size,
    `expected exactly ${expected.size} assets`);
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

function validateStagedManifest(manifest) {
  return validateManifestAssets(manifest, null);
}

// Producers authenticate only their own stage; this must never relax the
// complete 22-file manifest required by the draft-publication finalizer.
function validateStagedGroupManifest(manifest) {
  requireCondition(manifest && typeof manifest === 'object' && !Array.isArray(manifest) &&
    (manifest.stage === 'normal' || manifest.stage === 'mcpb'),
  'invalid staging group');
  return validateManifestAssets(manifest, manifest.stage);
}

// Preserve source provenance while copying metadata out of the independent
// producer objects. Local filesystem paths are intentionally not retained.
function composeStagedGroupManifests(normal, mcpb) {
  requireCondition(normal?.stage === 'normal' && mcpb?.stage === 'mcpb',
    'expected normal and MCPB staging groups in order');
  const normalFiles = validateStagedGroupManifest(normal);
  const mcpbFiles = validateStagedGroupManifest(mcpb);
  requireCondition(normal.repository === mcpb.repository && normal.tag === mcpb.tag &&
    normal.commit === mcpb.commit, 'producer manifests reference different sources');
  const assets = expectedReleaseAssetNames(normal.tag).map((name) => {
    const asset = normalFiles.get(name) ?? mcpbFiles.get(name);
    requireCondition(asset !== undefined, 'missing composed staging file');
    return { name, size: asset.size, sha256: asset.sha256 };
  });
  const complete = {
    repository: normal.repository, tag: normal.tag, commit: normal.commit, assets,
  };
  validateStagedManifest(complete);
  return complete;
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

async function verifyExactBytes(source, declaredSize, sha256) {
  requireCondition(source && typeof source[Symbol.asyncIterator] === 'function',
    'asset requires a readable byte stream');
  const hash = createHash('sha256');
  let size = 0;
  for await (const chunk of source) {
    requireCondition(chunk instanceof Uint8Array, 'asset stream contains nonbinary data');
    size += chunk.byteLength;
    requireCondition(size <= declaredSize, 'asset exceeds declared size');
    hash.update(chunk);
  }
  requireCondition(size === declaredSize, 'asset size mismatch');
  requireCondition(hash.digest('hex') === sha256, 'asset digest mismatch');
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
  await verifyExactBytes(source, expected.size, expected.sha256);
  return { name: expected.name, size: expected.size, sha256: expected.sha256 };
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

// The pinned upload-artifact v7 action ignores 'name' with archive:false
// and instead uses the actual file basename. Reusing a basename across run
// attempts causes name conflicts, so the producer must stage a copy under
// this deterministic attempt-scoped basename without modifying original bytes.
function rawStagingArtifactName(stage, runId, producerAttempt, fileName) {
  stagedArtifactName(stage, runId, producerAttempt);
  requireCondition(isRecognizedReleaseAssetName(fileName), 'invalid raw staging filename');
  const inMCPB = fileName.endsWith('.mcpb') || fileName === 'mcpb-checksums.txt';
  requireCondition((stage === 'mcpb') === inMCPB, 'raw staging file belongs to a different stage');
  const name = `scripthold-raw-${stage}-${runId}-${producerAttempt}-${fileName}`;
  requireCondition(name.length <= 240, 'raw staging artifact name is too long');
  return name;
}

// Artifact name comes from the verified ZIP stage convention or the manifest's
// exact attempt-scoped file name in v7 unarchived mode. Never select by name alone.
function verifyStagingArtifactMetadata(expected, remote, name, maxBytes) {
  requireCondition(expected && typeof expected === 'object' && !Array.isArray(expected),
    'invalid expected staging identity');
  requireCondition(expected.repository === REPOSITORY, 'unexpected staging repository');
  requireCondition(validPositiveID(expected.repositoryId), 'invalid staging repository id');
  expectedReleaseAssetNames(expected.tag);
  requireCondition(typeof expected.commit === 'string' && COMMIT_SHA.test(expected.commit),
    'invalid staged source commit');
  requireCondition(validPositiveID(expected.uploadId), 'invalid upload artifact id');
  requireCondition(typeof expected.uploadDigest === 'string' && SHA256.test(expected.uploadDigest),
    'invalid producer artifact digest');
  requireCondition(remote && typeof remote === 'object' && !Array.isArray(remote),
    'invalid GitHub Actions artifact metadata');
  requireCondition(remote.id === expected.uploadId && remote.name === name,
    'Actions artifact identity mismatch');
  requireCondition(remote.expired === false, 'Actions artifact expired or not confirmed');
  requireCondition(Number.isSafeInteger(remote.size_in_bytes) &&
    remote.size_in_bytes > 0 && remote.size_in_bytes <= maxBytes,
    'invalid Actions artifact size');
  requireCondition(remote.digest === 'sha256:' + expected.uploadDigest,
    'Actions artifact digest mismatch');
  const run = remote.workflow_run;
  requireCondition(run && typeof run === 'object' && !Array.isArray(run),
    'missing artifact workflow identity');
  requireCondition(run.id === expected.runId && run.repository_id === expected.repositoryId &&
    run.head_repository_id === expected.repositoryId && run.head_sha === expected.commit,
    'Actions artifact workflow provenance mismatch');
  return { artifactId: remote.id, name, archiveDigest: remote.digest, size: remote.size_in_bytes };
}

// The expected ID, SHA and digest must come from trusted workflow/job outputs.
// REST metadata alone does not authenticate the payload's extracted file bytes.
function verifyRunStagingArtifact(expected, remote) {
  requireCondition(expected && typeof expected === 'object' && !Array.isArray(expected),
    'invalid expected staging identity');
  const name = stagedArtifactName(expected.stage, expected.runId, expected.producerAttempt);
  return verifyStagingArtifactMetadata(expected, remote, name, MAX_STAGING_ARCHIVE_BYTES);
}

// A v7 upload with archive:false uses the uploaded file's basename as
// artifact name. Its digest must equal the original release file SHA.
// This does not open/download the file or grant execution/permission authority.
function verifyRawStagingArtifact(manifest, expected, remote) {
  const files = validateStagedManifest(manifest);
  requireCondition(expected && typeof expected === 'object' && !Array.isArray(expected),
    'invalid raw staging identity');
  requireCondition(expected.repository === manifest.repository &&
    expected.commit === manifest.commit && expected.tag === manifest.tag,
    'raw staging manifest provenance mismatch');
  requireCondition(typeof expected.fileName === 'string' && files.has(expected.fileName),
    'unrecognized raw staging filename');
  const inMCPB = expected.fileName.endsWith('.mcpb') || expected.fileName === 'mcpb-checksums.txt';
  requireCondition((expected.stage === 'mcpb') === inMCPB,
    'raw staging file belongs to a different stage');
  const original = files.get(expected.fileName);
  requireCondition(expected.uploadDigest === original.sha256,
    'raw uploaded digest differs from original manifest');
  const artifactName = rawStagingArtifactName(expected.stage, expected.runId,
    expected.producerAttempt, expected.fileName);
  const record = verifyStagingArtifactMetadata(expected, remote, artifactName, MAX_ASSET_BYTES);
  requireCondition(record.size === original.size,
    'raw uploaded length differs from original manifest');
  return { artifactId: record.artifactId, artifactName,
    fileName: expected.fileName, size: original.size, sha256: original.sha256 };
}

// Callers must independently authorize/open a stable source by the proven ID
// in owner-controlled staging. The byte stream itself is checked, not just REST metadata.
async function verifyRawStagingStream(manifest, expected, remote, source) {
  const file = verifyRawStagingArtifact(manifest, expected, remote);
  await verifyExactBytes(source, file.size, file.sha256);
  return file;
}

// Before any transfer, require the exact 22 original release files, each bound
// to an authenticated upload ID/digest. download-artifact@v8 only warns if one
// of several requested artifact IDs is absent, so action success is not enough.
// This is metadata-only: consumers must still verify all 22 downloaded streams.
function verifyRawStagingInventory(manifest, entries) {
  const files = validateStagedManifest(manifest);
  requireCondition(Array.isArray(entries) && entries.length === files.size,
    'raw staging inventory must include exactly 22 uploads');
  const found = new Map();
  const seenIDs = new Set();
  const attempts = new Map();
  let runId;
  let repositoryId;
  for (const item of entries) {
    requireCondition(item && typeof item === 'object' && !Array.isArray(item),
      'invalid raw staging entry');
    const expected = item.expected;
    const artifact = verifyRawStagingArtifact(manifest, expected, item.remote);
    requireCondition(!found.has(artifact.fileName), 'duplicate raw staging file');
    requireCondition(!seenIDs.has(artifact.artifactId), 'duplicate raw staging artifact ID');
    if (runId === undefined) {
      runId = expected.runId;
      repositoryId = expected.repositoryId;
    }
    requireCondition(expected.runId === runId && expected.repositoryId === repositoryId,
      'mixed raw staging workflow identity');
    if (attempts.has(expected.stage)) {
      requireCondition(attempts.get(expected.stage) === expected.producerAttempt,
        'mixed producer attempts in raw staging group');
    } else {
      attempts.set(expected.stage, expected.producerAttempt);
    }
    found.set(artifact.fileName, artifact);
    seenIDs.add(artifact.artifactId);
  }
  const canonical = new Map();
  for (const name of files.keys()) {
    requireCondition(found.has(name), 'missing raw staging file');
    canonical.set(name, found.get(name));
  }
  return canonical;
}

// The pinned download-artifact v8 derives multi-download subdirectories from
// remote artifact names and only warns when some requested IDs are missing.
// Download one authenticated ID per private caller-owned slot instead. No host
// path is transmitted here; consumers must create and secure slot directories,
// verify the sole downloaded filename and check its original bytes.
function planRawStagingDownloads(manifest, entries) {
  const files = verifyRawStagingInventory(manifest, entries);
  // Manifest asset order is untrusted; slot assignments must not change when
  // producer JSON happens to enumerate the same verified files differently.
  return expectedReleaseAssetNames(manifest.tag).map((name, index) => {
    const file = files.get(name);
    return {
      artifactId: file.artifactId,
      artifactIdsInput: String(file.artifactId),
      slot: 'slot-' + String(index + 1).padStart(2, '0'),
      artifactName: file.artifactName,
      fileName: file.fileName,
      size: file.size,
      sha256: file.sha256,
    };
  });
}

// The pinned download-artifact v8 accepts a CSV of IDs in one action step.
// Explicit merge-multiple prevents the action from constructing directory names
// from remote metadata; all IDs must still be rechecked after its warning-only
// missing-ID behavior. The caller supplies a fresh private destination path.
function planRawStagingBatch(manifest, entries) {
  const files = planRawStagingDownloads(manifest, entries).map(({
    artifactId, artifactName, fileName, size, sha256,
  }) => ({ artifactId, artifactName, fileName, size, sha256 }));
  return {
    actionInputs: {
      'artifact-ids': files.map((file) => String(file.artifactId)).join(','),
      'merge-multiple': true,
      'skip-decompress': true,
      'digest-mismatch': 'error',
    },
    files,
  };
}

// The caller must enumerate an owner-controlled directory using lstat/fstat
// with no-follow, stable inode and parent-directory checks. This metadata
// preflight alone does not authorize opening paths or prove the payload bytes.
function verifyRawStagingBatchEntries(manifest, entries, observed) {
  const expected = planRawStagingBatch(manifest, entries).files;
  requireCondition(Array.isArray(observed) && observed.length === expected.length,
    'raw batch must contain exactly 22 downloaded files');
  const byPhysicalName = new Map(expected.map((file) => [file.artifactName, file]));
  const found = new Set();
  for (const item of observed) {
    requireCondition(item && typeof item === 'object' && !Array.isArray(item) &&
      typeof item.name === 'string' && byPhysicalName.has(item.name),
    'unexpected raw batch file name or path');
    requireCondition(!found.has(item.name), 'duplicate raw batch file');
    requireCondition(item.type === 'file' && item.nlink === 1,
      'raw batch entry must be a single-link regular file');
    requireCondition(item.size === byPhysicalName.get(item.name).size,
      'raw batch file size mismatch');
    found.add(item.name);
  }
  const canonical = new Map();
  for (const file of expected) {
    requireCondition(found.has(file.artifactName), 'missing raw batch file');
    canonical.set(file.fileName, file);
  }
  return canonical;
}

// Validate the complete inventory before any stream is requested, then hash
// each authenticated original payload once in manifest order. The caller must
// open a stable no-follow source and retain its immutable bytes for subsequent
// use: hashing a path now cannot prove it stays unchanged for a later upload.
async function verifyRawStagingSources(files, openSource) {
  requireCondition(typeof openSource === 'function', 'invalid raw staging source opener');
  for (const file of files.values()) {
    const source = await openSource(file);
    await verifyExactBytes(source, file.size, file.sha256);
  }
  return files;
}

async function verifyRawStagingPayloads(manifest, entries, openSource) {
  return verifyRawStagingSources(verifyRawStagingInventory(manifest, entries), openSource);
}

// Always finish the flat-directory metadata check before opening even one
// source. A Linux-only caller must still enforce no-follow and stable identity
// while opening each file; an observed entry is not filesystem authorization.
async function verifyRawStagingBatchPayloads(manifest, entries, observed, openSource) {
  return verifyRawStagingSources(
    verifyRawStagingBatchEntries(manifest, entries, observed), openSource);
}

// The exact producer-upload ID, SHA and run are authenticated against the
// GitHub Actions artifact record before examining downloaded archive bytes.
async function verifyStagingArchiveStream(expected, remote, source) {
  const archive = verifyRunStagingArtifact(expected, remote);
  await verifyExactBytes(source, archive.size, expected.uploadDigest);
  return archive;
}

// This is a fail-closed *pre-extraction* inventory check. Its caller must use
// a trustworthy ZIP metadata reader and independently constrain extraction;
// list validation itself neither parses ZIP structures nor extracts files.
function validateStagingArchiveEntries(manifest, stage, entries) {
  const all = validateStagedManifest(manifest);
  requireCondition(stage === 'normal' || stage === 'mcpb', 'invalid staging group');
  const expected = new Map([...all].filter(([name]) => stage === 'normal'
    ? !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt'
    : name.endsWith('.mcpb') || name === 'mcpb-checksums.txt'));
  requireCondition(Array.isArray(entries) && entries.length === expected.size,
    'invalid staged ZIP entry count');
  const seen = new Set();
  let compressedTotal = 0;
  for (const entry of entries) {
    requireCondition(entry && typeof entry === 'object' && !Array.isArray(entry),
      'invalid ZIP entry metadata');
    requireCondition(typeof entry.name === 'string' && expected.has(entry.name),
      'unexpected ZIP entry name or path');
    requireCondition(!seen.has(entry.name), 'duplicate ZIP entry name');
    requireCondition(entry.type === 'file', 'ZIP entry must be an ordinary file');
    requireCondition(entry.size === expected.get(entry.name).size,
      'ZIP entry uncompressed size mismatch');
    requireCondition(Number.isSafeInteger(entry.compressedSize) &&
      entry.compressedSize > 0 && entry.compressedSize <= MAX_STAGING_ARCHIVE_BYTES,
      'invalid ZIP compressed size');
    if (entry.unixMode !== undefined) {
      requireCondition(Number.isSafeInteger(entry.unixMode) &&
        entry.unixMode >= 0 && entry.unixMode <= 0o177777 &&
        (entry.unixMode & 0o170000) === 0o100000 &&
        (entry.unixMode & 0o7000) === 0,
      'ZIP entry Unix mode must be regular file');
    }
    compressedTotal += entry.compressedSize;
    requireCondition(compressedTotal <= MAX_STAGING_ARCHIVE_BYTES,
      'ZIP compressed entries exceed staging size bound');
    seen.add(entry.name);
  }
  requireCondition(seen.size === expected.size, 'incomplete staging inventory');
  return seen;
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
  verifyStagedAssetStream, stagedArtifactName, verifyRunStagingArtifact,
  verifyStagingArchiveStream, validateStagingArchiveEntries,
  verifyRawStagingArtifact, verifyRawStagingStream, rawStagingArtifactName,
  verifyRawStagingInventory, verifyRawStagingPayloads,
  planRawStagingDownloads, planRawStagingBatch, verifyRawStagingBatchEntries,
  verifyRawStagingBatchPayloads, isRecognizedReleaseAssetName, validateStagedGroupManifest,
  composeStagedGroupManifests };
