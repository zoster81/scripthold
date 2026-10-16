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

// This is a strictly read-only HTTPS client for a previously identified release.
// It cannot discover a missing draft, create a release, upload, publish or lock
// concurrent finalizers. The caller must provide a real authorized GitHub token.
async function readReleaseObservation({ releaseId, tag, token, fetchImpl = globalThis.fetch }) {
  requireCondition(Number.isSafeInteger(releaseId) && releaseId > 0,
    'invalid release id');
  expectedReleaseAssetNames(tag);
  requireCondition(typeof token === 'string' && token.length > 0 &&
    token.length <= 4096 && /^[\x21-\x7e]+$/.test(token),
  'invalid GitHub token');
  requireCondition(typeof fetchImpl === 'function', 'invalid GitHub HTTPS client');

  const releaseUrl = API + '/' + releaseId;
  const assetUrl = releaseUrl + '/assets?per_page=100&page=1';
  const headers = {
    Authorization: 'Bearer ' + token,
    Accept: 'application/vnd.github+json',
    'X-GitHub-Api-Version': API_VERSION,
    'User-Agent': 'scripthold-release-finalizer-reader',
  };
  const get = (url) => fetchImpl(url, {
    method: 'GET', headers, redirect: 'error', cache: 'no-store',
    signal: AbortSignal.timeout(15000),
  });

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

module.exports = { readReleaseObservation };
