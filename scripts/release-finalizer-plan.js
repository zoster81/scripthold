'use strict';

const { validatePublicationAuthoritySnapshot } = require('./release-candidate-provenance');
const { planDraftAssetResume, verifyDraftAssetSet, verifyPublishedAssetSet } =
  require('./release-asset-contract');

const MAX_ASSETS = 22;

function requireCondition(value, message) {
  if (!value) throw new Error(message);
}

// One strict 100-item GitHub REST page can contain the entire 22-file
// allowlist. Reject any pagination instead of accidentally omitting assets.
// The future API adapter must authenticate the repository, release ID, status,
// request parameters and raw Link header; this function performs no I/O.
function normalizeReleaseAssetPage(releaseId, page) {
  requireCondition(Number.isSafeInteger(releaseId) && releaseId > 0,
    'invalid expected release id');
  requireCondition(page && typeof page === 'object' && !Array.isArray(page) &&
    page.status === 200 && page.request && typeof page.request === 'object' &&
    page.request.releaseId === releaseId && page.request.page === 1 &&
    page.request.perPage === 100, 'release asset listing is not the exact first page');
  requireCondition(page.link === null, 'release asset pagination must be complete');
  requireCondition(Array.isArray(page.assets) && page.assets.length <= MAX_ASSETS,
    'release asset page exceeds the exact inventory bound');
  return page.assets.map((asset) => {
    requireCondition(asset && typeof asset === 'object' && !Array.isArray(asset),
      'invalid release asset in page');
    // Project only the fields the existing draft/publication gates verify.
    return { id: asset.id, name: asset.name, size: asset.size,
      digest: asset.digest, state: asset.state };
  });
}

function validateUncertainty(uncertain, manifest) {
  if (uncertain === null) return;
  requireCondition(uncertain && typeof uncertain === 'object' &&
    !Array.isArray(uncertain), 'invalid uncertain operation');
  if (uncertain.operation === 'create' || uncertain.operation === 'publish') {
    requireCondition(Object.keys(uncertain).length === 1,
      'unexpected uncertain operation fields');
    return;
  }
  requireCondition(uncertain.operation === 'upload' &&
    Object.keys(uncertain).length === 2 && typeof uncertain.name === 'string' &&
    manifest.assets.some((asset) => asset.name === uncertain.name),
  'invalid uncertain upload name');
}

// Pure next-step proposal only. No output is permission to create, upload or
// publish. A future GitHub finalizer must serialize same-tag writers, fetch
// authentic source/asset evidence again, and fail on any changed snapshot.
function planReleaseFinalizerStep(authority, observation, uncertain = null) {
  validatePublicationAuthoritySnapshot(authority);
  const manifest = authority.manifest;
  validateUncertainty(uncertain, manifest);
  requireCondition(observation && typeof observation === 'object' &&
    !Array.isArray(observation), 'invalid release observation');

  if (observation.kind === 'absent') {
    requireCondition(observation.status === 404, 'ambiguous release absence');
    if (uncertain !== null) {
      return { step: 'halt', reason: uncertain.operation + '-ambiguous' };
    }
    // HTTP 404 alone may also mean permission failure. Inspect an authenticated
    // release listing and exact tag under the workflow lock before creating.
    return { step: 'inspect-absence', tag: manifest.tag };
  }

  requireCondition(observation.kind === 'present' && observation.release &&
    typeof observation.release === 'object' && !Array.isArray(observation.release),
  'missing or invalid release observation');
  const release = observation.release;
  const assets = normalizeReleaseAssetPage(release.id, observation.assetPage);

  if (release.draft === false && release.immutable === true) {
    const finished = verifyPublishedAssetSet(manifest, release, assets);
    return { step: 'complete', releaseId: finished.releaseId, count: finished.count };
  }

  // A public-but-mutable release must never be repaired or treated as valid.
  requireCondition(release.draft === true && release.immutable === false,
    'release is neither an unpublished draft nor an immutable publication');
  const plan = planDraftAssetResume(manifest, release, assets);

  if (uncertain?.operation === 'publish') {
    return { step: 'halt', reason: 'publish-ambiguous' };
  }
  if (uncertain?.operation === 'upload' &&
    !plan.present.some((file) => file.name === uncertain.name)) {
    return { step: 'halt', reason: 'upload-ambiguous' };
  }

  if (plan.missing.length > 0) {
    const name = plan.missing[0];
    const asset = manifest.assets.find((entry) => entry.name === name);
    return { step: 'upload-candidate', releaseId: plan.releaseId,
      name, size: asset.size, sha256: asset.sha256, remaining: plan.missing.length };
  }
  verifyDraftAssetSet(manifest, release, assets);
  return { step: 'publish-candidate', releaseId: plan.releaseId, count: MAX_ASSETS };
}

module.exports = { normalizeReleaseAssetPage, planReleaseFinalizerStep };
