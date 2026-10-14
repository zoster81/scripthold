'use strict';

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

module.exports = { expectedReleaseAssetNames, validateStagedManifest, verifyDraftAssetSet };
