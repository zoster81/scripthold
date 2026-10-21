'use strict';

const { expectedReleaseAssetNames, validateStagedManifest,
  verifyPublishedAssetSet } = require('./release-asset-contract');
const { normalizeReleaseAssetPage } = require('./release-finalizer-plan');

const API = 'https://api.github.com/repos/zoster81/scripthold/releases';
const MAX_JSON_BYTES = 512 * 1024;
const API_VERSION = '2022-11-28';

function requireCondition(condition, message) {
  if (!condition) throw new Error(message);
}

function releaseIdentity(release, id, tag) {
  requireCondition(release && typeof release === 'object' && !Array.isArray(release) &&
    release.id === id && release.tag_name === tag &&
    typeof release.draft === 'boolean' && typeof release.immutable === 'boolean',
  'GitHub release identity or state mismatch');
  const result = {
    id: release.id, tag_name: release.tag_name,
    draft: release.draft, immutable: release.immutable,
  };
  // Draft publishing must preserve the original title, notes and prerelease
  // state. Expose them when GitHub supplies them, even if malformed, so the
  // writer can reject an incomplete or conflicting draft without guessing.
  for (const field of ['name', 'body', 'prerelease']) {
    if (Object.hasOwn(release, field)) result[field] = release[field];
  }
  return result;
}

async function readJsonResponse(response, url) {
  requireCondition(response && typeof response === 'object' &&
    response.status === 200 && response.redirected !== true &&
    (typeof response.url !== 'string' || response.url === '' || response.url === url),
  'GitHub response failed status/origin verification');
  requireCondition(typeof response.headers?.get === 'function' &&
    /^application\/(?:json|vnd\.github\+json)(?:\s*;|$)/i
      .test(response.headers.get('content-type') ?? ''),
  'unexpected GitHub JSON content type');
  const contentLength = response.headers.get('content-length');
  if (contentLength !== null) {
    requireCondition(/^\d+$/.test(contentLength) &&
      Number(contentLength) <= MAX_JSON_BYTES, 'GitHub JSON content length is invalid');
  }
  requireCondition(response.body &&
    typeof response.body[Symbol.asyncIterator] === 'function',
  'GitHub JSON response is not a byte stream');
  let size = 0;
  const parts = [];
  for await (const bytes of response.body) {
    requireCondition(bytes instanceof Uint8Array, 'nonbinary GitHub response');
    size += bytes.byteLength;
    requireCondition(size <= MAX_JSON_BYTES, 'GitHub JSON exceeds byte bound');
    parts.push(Buffer.from(bytes));
  }
  requireCondition(size > 0, 'empty GitHub JSON response');
  let value;
  try {
    value = JSON.parse(new TextDecoder('utf-8', { fatal: true })
      .decode(Buffer.concat(parts, size)));
  } catch {
    throw new Error('invalid UTF-8 GitHub JSON payload');
  }
  return value;
}


function makeGitHubGet(token, fetchImpl) {
  requireCondition(typeof token === 'string' && token.length > 0 &&
    token.length <= 4096 && /^[\x21-\x7e]+$/.test(token),
  'invalid GitHub token');
  requireCondition(typeof fetchImpl === 'function', 'invalid GitHub HTTPS client');
  const headers = {
    Authorization: 'Bearer ' + token,
    Accept: 'application/vnd.github+json',
    'X-GitHub-Api-Version': API_VERSION,
    'User-Agent': 'scripthold-release-finalizer-reader',
  };
  return (url) => fetchImpl(url, {
    method: 'GET', headers, redirect: 'error', cache: 'no-store',
    signal: AbortSignal.timeout(15000),
  });
}

// This is a strictly read-only HTTPS client for a previously identified release.
// It cannot discover a missing draft, create a release, upload, publish or lock
// concurrent finalizers. The caller must provide a real authorized GitHub token.
async function readReleaseObservation({ releaseId, tag, token, fetchImpl = globalThis.fetch }) {
  requireCondition(Number.isSafeInteger(releaseId) && releaseId > 0,
    'invalid release id');
  expectedReleaseAssetNames(tag);
  const get = makeGitHubGet(token, fetchImpl);

  const releaseUrl = API + '/' + releaseId;
  const assetUrl = releaseUrl + '/assets?per_page=100&page=1';

  const firstResponse = await get(releaseUrl);
  if (firstResponse?.status === 404) {
    // A 404 alone is only an observation, never evidence that creation is safe.
    requireCondition(firstResponse.redirected !== true &&
      (!firstResponse.url || firstResponse.url === releaseUrl),
    'redirected GitHub release 404');
    return { kind: 'absent', status: 404 };
  }
  const first = releaseIdentity(await readJsonResponse(firstResponse, releaseUrl), releaseId, tag);
  const pageResponse = await get(assetUrl);
  const page = {
    status: pageResponse?.status,
    request: { releaseId, page: 1, perPage: 100 },
    link: pageResponse?.headers?.get('link'),
    assets: await readJsonResponse(pageResponse, assetUrl),
  };
  const assets = normalizeReleaseAssetPage(releaseId, page);

  // Detect a draft becoming public or changing identity during the asset read.
  // This is not an atomic snapshot: a future writer still needs serialization
  // and immediate fresh reads before each mutation.
  const last = releaseIdentity(await readJsonResponse(await get(releaseUrl), releaseUrl),
    releaseId, tag);
  requireCondition(JSON.stringify(first) === JSON.stringify(last),
    'GitHub release identity, state or metadata changed during asset inventory read');

  return { kind: 'present', release: last,
    assetPage: { status: 200, request: page.request, link: page.link, assets } };
}


const RELEASES_PER_PAGE = 30;
const MAX_RELEASE_PAGES = 20;

// A non-executable pagination check. GitHub may include prev/first/last
// alongside next; every supplied link must stay on the exact repository API.
function verifyReleaseListLinks(link, currentPage, entries) {
  if (link === null) return null;
  requireCondition(typeof link === 'string' && link.length > 0 && link.length <= 4096,
    'invalid GitHub release pagination header');
  const relations = new Set();
  let lastPage = null;
  for (const item of link.split(',')) {
    const match = /^<([^<>]+)>;\s*rel="(next|prev|first|last)"$/.exec(item.trim());
    requireCondition(match && !relations.has(match[2]),
      'malformed or duplicate release pagination relation');
    relations.add(match[2]);
    let url;
    try {
      url = new URL(match[1]);
    } catch {
      throw new Error('invalid GitHub release pagination URL');
    }
    const keys = Array.from(url.searchParams.keys());
    const rawPage = url.searchParams.get('page');
    requireCondition(url.origin === 'https://api.github.com' &&
      url.pathname === '/repos/zoster81/scripthold/releases' &&
      !url.hash && !url.username && !url.password &&
      keys.length === 2 && keys.includes('page') && keys.includes('per_page') &&
      url.searchParams.get('per_page') === String(RELEASES_PER_PAGE) &&
      /^[1-9]\d*$/.test(rawPage ?? ''),
    'foreign or malformed release pagination URL');
    const page = Number(rawPage);
    requireCondition(Number.isSafeInteger(page) &&
      (match[2] !== 'next' || page === currentPage + 1) &&
      (match[2] !== 'prev' || page === currentPage - 1) &&
      (match[2] !== 'first' || page === 1) &&
      (match[2] !== 'last' || page >= currentPage),
    'inconsistent release pagination');
    if (match[2] === 'last') lastPage = page;
  }
  requireCondition(!relations.has('next') || (entries === RELEASES_PER_PAGE &&
    (lastPage === null || lastPage > currentPage)),
  'release list pagination contradicts advertised last page');
  return lastPage;
}

// Read-only recovery discovery. It can find one candidate from a completely
// traversed bounded listing, but cannot prove nonexistence or permission to
// create: draft visibility depends on caller authorization, and no scan is an
// atomic cross-request snapshot. A future writer must serialize by tag and
// revalidate through authenticated provider reads before any mutation.
async function discoverReleaseByTag({ tag, token, fetchImpl = globalThis.fetch }) {
  expectedReleaseAssetNames(tag);
  const get = makeGitHubGet(token, fetchImpl);
  const ids = new Set();
  let candidate = null;
  let complete = false;
  let advertisedLastPage = null;
  for (let page = 1; page <= MAX_RELEASE_PAGES; page++) {
    const url = API + '?per_page=' + RELEASES_PER_PAGE + '&page=' + page;
    const result = await get(url);
    const entries = await readJsonResponse(result, url);
    requireCondition(Array.isArray(entries) && entries.length <= RELEASES_PER_PAGE,
      'invalid or oversized GitHub release list page');
    const statedLastPage = verifyReleaseListLinks(
      result.headers.get('link'), page, entries.length);
    if (statedLastPage !== null && statedLastPage !== undefined) {
      requireCondition(statedLastPage <= MAX_RELEASE_PAGES &&
        (advertisedLastPage === null || advertisedLastPage === statedLastPage),
      'inconsistent GitHub release last page');
      advertisedLastPage = statedLastPage;
    }
    if (advertisedLastPage !== null) {
      requireCondition(page <= advertisedLastPage ||
        (page === advertisedLastPage + 1 && entries.length === 0),
      'release list pagination exceeded advertised last page');
    }
    for (const entry of entries) {
      requireCondition(entry && typeof entry === 'object' && !Array.isArray(entry) &&
        Number.isSafeInteger(entry.id) && entry.id > 0 &&
        typeof entry.tag_name === 'string' && entry.tag_name.length > 0 &&
        typeof entry.draft === 'boolean' && typeof entry.immutable === 'boolean' &&
        !ids.has(entry.id), 'duplicate or malformed GitHub release list entry');
      ids.add(entry.id);
      if (entry.tag_name === tag) {
        requireCondition(candidate === null, 'duplicate GitHub release tag');
        candidate = { id: entry.id, draft: entry.draft, immutable: entry.immutable };
      }
    }
    if (entries.length < RELEASES_PER_PAGE) {
      requireCondition(advertisedLastPage === null ||
        page === advertisedLastPage ||
        (page === advertisedLastPage + 1 && entries.length === 0),
      'release list pagination ended before advertised last page');
      complete = true;
      break;
    }
  }
  requireCondition(complete, 'bounded GitHub release page limit exceeded');
  if (candidate === null) {
    return { kind: 'not-found-unproven', tag };
  }
  const observation = await readReleaseObservation({
    releaseId: candidate.id, tag, token, fetchImpl,
  });
  requireCondition(observation.kind === 'present' &&
    observation.release.draft === candidate.draft &&
    observation.release.immutable === candidate.immutable,
  'GitHub release candidate changed during discovery');
  return { kind: 'candidate', releaseId: candidate.id, observation };
}

// A separate administrator-authorized GET is required before any future
// public transition. A 403, absent setting or disabled policy fails closed.
// This is a point-in-time observation, not a lock or publication permit.
// The ordinary workflow GITHUB_TOKEN is not assumed to have admin access.
async function readImmutableReleasePolicy({
  adminToken, fetchImpl = globalThis.fetch,
} = {}) {
  const url = 'https://api.github.com/repos/zoster81/scripthold/immutable-releases';
  const response = await makeGitHubGet(adminToken, fetchImpl)(url);
  const policy = await readJsonResponse(response, url);
  requireCondition(policy && typeof policy === 'object' &&
    !Array.isArray(policy) && policy.enabled === true &&
    typeof policy.enforced_by_owner === 'boolean',
  'GitHub immutable releases are not independently confirmed enabled');
  return { enabled: true, enforcedByOwner: policy.enforced_by_owner };
}

// A future authorized writer must independently confirm its GitHub deployment
// environment still requires the sole maintainer's approval and accepts only
// version-tag deployments. This uses the separate Administration:read credential.
// GitHub's own environment gate, not this observation, grants job approval.
async function readProtectedFinalizerEnvironmentPolicy({
  adminToken, fetchImpl = globalThis.fetch,
} = {}) {
  const base = 'https://api.github.com/repos/zoster81/scripthold/environments/' +
    'scripthold-immutable-publish';
  const get = makeGitHubGet(adminToken, fetchImpl);
  const envResponse = await get(base);
  const env = await readJsonResponse(envResponse, base);
  requireCondition(envResponse.headers.get('link') === null &&
    env && typeof env === 'object' && !Array.isArray(env) &&
    env.name === 'scripthold-immutable-publish' &&
    env.can_admins_bypass === false &&
    env.deployment_branch_policy?.protected_branches === false &&
    env.deployment_branch_policy?.custom_branch_policies === true &&
    Array.isArray(env.protection_rules) && env.protection_rules.length === 2,
  'GitHub finalizer environment is not independently protected');
  const reviewerRules = env.protection_rules.filter(rule =>
    rule && rule.type === 'required_reviewers');
  const branchRules = env.protection_rules.filter(rule =>
    rule && rule.type === 'branch_policy');
  requireCondition(reviewerRules.length === 1 && branchRules.length === 1 &&
    reviewerRules[0].prevent_self_review === false &&
    Array.isArray(reviewerRules[0].reviewers) &&
    reviewerRules[0].reviewers.length === 1 &&
    reviewerRules[0].reviewers[0]?.type === 'User' &&
    reviewerRules[0].reviewers[0].reviewer?.id === 7560683 &&
    reviewerRules[0].reviewers[0].reviewer?.login === 'zoster81',
  'GitHub finalizer environment lacks the exact protected maintainer reviewer');

  const refsUrl = base + '/deployment-branch-policies';
  const refsResponse = await get(refsUrl);
  const refs = await readJsonResponse(refsResponse, refsUrl);
  requireCondition(refsResponse.headers.get('link') === null &&
    refs && typeof refs === 'object' && !Array.isArray(refs) &&
    refs.total_count === 1 && Array.isArray(refs.branch_policies) &&
    refs.branch_policies.length === 1 &&
    refs.branch_policies[0]?.name === 'v*' &&
    refs.branch_policies[0]?.type === 'tag',
  'GitHub finalizer environment has an unsafe or incomplete tag policy');
  return { protected: true, environment: env.name };
}

// Prove that a publication credential is a GitHub App installation token
// exposing only this repository. The REST list does NOT prove its effective
// permissions; the future mint action must request only contents:write.
async function readScopedPublicationInstallation({
  token, fetchImpl = globalThis.fetch,
} = {}) {
  const url = 'https://api.github.com/installation/repositories?per_page=2&page=1';
  const response = await makeGitHubGet(token, fetchImpl)(url);
  const listing = await readJsonResponse(response, url);
  requireCondition(response.headers.get('link') === null &&
    listing && typeof listing === 'object' && !Array.isArray(listing) &&
    listing.total_count === 1 && Array.isArray(listing.repositories) &&
    listing.repositories.length === 1,
  'GitHub App installation token must be scoped to one repository');
  const repo = listing.repositories[0];
  requireCondition(repo && typeof repo === 'object' && !Array.isArray(repo) &&
    Number.isSafeInteger(repo.id) && repo.id > 0 &&
    repo.name === 'scripthold' && repo.full_name === 'zoster81/scripthold' &&
    repo.owner?.login === 'zoster81',
  'GitHub App installation repository scope is not authorized');
  return { repository: repo.full_name };
}

// Read-only final-state proof for a previously identified release. GitHub
// reports immutable:true only after publication; all 22 remote asset IDs,
// names, sizes, digests and states must agree with the trusted original set.
// This cannot establish the authenticity of the caller's input manifest.
async function readVerifiedImmutablePublication({
  manifest, releaseId, token, fetchImpl = globalThis.fetch,
}) {
  validateStagedManifest(manifest);
  const observation = await readReleaseObservation({
    releaseId, tag: manifest.tag, token, fetchImpl,
  });
  requireCondition(observation.kind === 'present',
    'expected an independently observed immutable public release');
  const verified = verifyPublishedAssetSet(manifest,
    observation.release, observation.assetPage.assets);
  return { releaseId: verified.releaseId, count: verified.count };
}

module.exports = { readReleaseObservation, discoverReleaseByTag,
  readImmutableReleasePolicy, readProtectedFinalizerEnvironmentPolicy,
  readScopedPublicationInstallation, readVerifiedImmutablePublication,
  makeGitHubGet, readJsonResponse };
