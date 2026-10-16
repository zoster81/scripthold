'use strict';

const { expectedReleaseAssetNames } = require('./release-asset-contract');
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
  return {
    id: release.id, tag_name: release.tag_name,
    draft: release.draft, immutable: release.immutable,
  };
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
  requireCondition(first.id === last.id && first.tag_name === last.tag_name &&
    first.draft === last.draft && first.immutable === last.immutable,
  'GitHub release changed during asset inventory read');

  return { kind: 'present', release: last,
    assetPage: { status: 200, request: page.request, link: page.link, assets } };
}


const RELEASES_PER_PAGE = 30;
const MAX_RELEASE_PAGES = 20;

// A non-executable pagination check. GitHub may include prev/first/last
// alongside next; every supplied link must stay on the exact repository API.
function verifyReleaseListLinks(link, currentPage, entries) {
  if (link === null) return;
  requireCondition(typeof link === 'string' && link.length > 0 && link.length <= 4096,
    'invalid GitHub release pagination header');
  const relations = new Set();
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
  }
  requireCondition(!relations.has('next') || entries === RELEASES_PER_PAGE,
    'release list unexpectedly truncated');
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
  for (let page = 1; page <= MAX_RELEASE_PAGES; page++) {
    const url = API + '?per_page=' + RELEASES_PER_PAGE + '&page=' + page;
    const result = await get(url);
    const entries = await readJsonResponse(result, url);
    requireCondition(Array.isArray(entries) && entries.length <= RELEASES_PER_PAGE,
      'invalid or oversized GitHub release list page');
    verifyReleaseListLinks(result.headers.get('link'), page, entries.length);
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

module.exports = { readReleaseObservation, discoverReleaseByTag };
