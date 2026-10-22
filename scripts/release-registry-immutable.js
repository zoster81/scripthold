'use strict';

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { makeGitHubGet, readJsonResponse, readReleaseObservation } =
  require('./release-finalizer-read');

const REPOSITORY = 'zoster81/scripthold';
const API = 'https://api.github.com/repos/' + REPOSITORY + '/releases';

function requireCondition(ok, reason) {
  if (!ok) throw new Error(reason);
}

// Registry is a separate workflow_call OR manually retryable entrypoint.
// Neither path is allowed to publish OIDC metadata for an incomplete or mutable
// GitHub Release. This reader cannot create, upload, edit or publish assets.
async function verifyRegistryImmutableRelease({
  tag, commit, token, fetchImpl = globalThis.fetch,
}) {
  const names = expectedReleaseAssetNames(tag);
  requireCondition(typeof commit === 'string' && /^[0-9a-f]{40}$/.test(commit),
    'invalid exact Registry source commit');
  const get = makeGitHubGet(token, fetchImpl);
  const url = API + '/tags/' + encodeURIComponent(tag);
  const initial = await readJsonResponse(await get(url), url);
  requireCondition(initial && typeof initial === 'object' &&
    !Array.isArray(initial) && Number.isSafeInteger(initial.id) &&
    initial.id > 0 && initial.tag_name === tag &&
    initial.target_commitish === commit &&
    initial.draft === false && initial.immutable === true,
  'Registry requires one published immutable release for the exact commit');

  const observed = await readReleaseObservation({
    releaseId: initial.id, tag, token, fetchImpl,
  });
  requireCondition(observed.kind === 'present' &&
    observed.release.id === initial.id && observed.release.tag_name === tag &&
    observed.release.draft === false && observed.release.immutable === true &&
    observed.release.target_commitish === commit,
  'Registry release identity or immutable policy changed');
  const files = observed.assetPage.assets;
  requireCondition(files.length === names.length,
    'Registry requires the complete 22-file remote asset inventory');
  const expected = new Set(names);
  const seenNames = new Set();
  const seenIds = new Set();
  for (const file of files) {
    requireCondition(expected.has(file.name) && !seenNames.has(file.name) &&
      Number.isSafeInteger(file.id) && file.id > 0 && !seenIds.has(file.id) &&
      Number.isSafeInteger(file.size) && file.size > 0 &&
      file.size <= 2 * 1024 * 1024 * 1024 &&
      typeof file.digest === 'string' &&
      /^sha256:[0-9a-f]{64}$/.test(file.digest) &&
      file.state === 'uploaded',
    'Registry release asset is missing, duplicated or unverified');
    seenNames.add(file.name);
    seenIds.add(file.id);
  }
  requireCondition(seenNames.size === expected.size,
    'Registry release asset allowlist is incomplete');
  return { releaseId: initial.id, count: files.length };
}

if (require.main === module) {
  (async () => {
    requireCondition(process.env.GITHUB_REPOSITORY === REPOSITORY,
      'Registry issuer repository mismatch');
    const result = await verifyRegistryImmutableRelease({
      tag: process.env.VERSION_TAG,
      commit: process.env.RELEASE_SOURCE_SHA,
      token: process.env.GH_TOKEN,
    });
    console.log('Verified published immutable GitHub Release and ' +
      result.count + ' original assets before Registry OIDC');
  })().catch(error => {
    console.error(error.message);
    process.exitCode = 1;
  });
}

module.exports = { verifyRegistryImmutableRelease };
