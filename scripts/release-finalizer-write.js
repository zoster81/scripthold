'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { inspectFinalizerPreflight } = require('./release-finalizer-preflight');
const { readPublicationAuthority } = require('./release-finalizer-authority');
const { discoverReleaseByTag, makeGitHubGet } = require('./release-finalizer-read');
const { validateStagedManifest } = require('./release-asset-contract');
const { planDraftAssetResume } = require('./release-asset-contract');
const { decodeGoReleaserNotes } = require('./release-normal-source');
const { verifyDraftSourceInventory } = require('./release-finalizer-source-inventory');

const UPLOAD_ORIGIN = 'https://uploads.github.com';
const MAX_RESPONSE = 256 * 1024;

function requireCondition(ok, reason) {
  if (!ok) throw new Error(reason);
}

function stableFileState(file, initial, name) {
  const current = fs.lstatSync(name);
  return file.isFile() && current.isFile() && !current.isSymbolicLink() &&
    file.nlink === 1 && current.nlink === 1 &&
    file.dev === initial.dev && file.ino === initial.ino &&
    current.dev === initial.dev && current.ino === initial.ino &&
    file.size === initial.size && current.size === initial.size &&
    file.mtimeMs === initial.mtimeMs && current.mtimeMs === initial.mtimeMs;
}

async function readUploadResult(response, url) {
  requireCondition(response?.status === 201 && response.redirected !== true &&
    (!response.url || response.url === url),
  'GitHub draft asset upload response is not an authenticated creation');
  requireCondition(/^application\/(?:json|vnd\.github\+json)(?:\s*;|$)/i
    .test(response.headers?.get('content-type') ?? ''),
  'GitHub draft asset upload returned unexpected content type');
  const length = response.headers.get('content-length');
  requireCondition(length === null || /^\d+$/.test(length) &&
    Number(length) <= MAX_RESPONSE, 'oversized GitHub draft upload response');
  requireCondition(response.body &&
    typeof response.body[Symbol.asyncIterator] === 'function',
  'missing streamed GitHub draft upload response');
  let total = 0;
  const chunks = [];
  for await (const chunk of response.body) {
    requireCondition(chunk instanceof Uint8Array, 'invalid GitHub response chunk');
    total += chunk.byteLength;
    requireCondition(total > 0 && total <= MAX_RESPONSE,
      'oversized GitHub draft upload response');
    chunks.push(Buffer.from(chunk));
  }
  let result;
  try {
    result = JSON.parse(new TextDecoder('utf-8', { fatal: true })
      .decode(Buffer.concat(chunks, total)));
  } catch {
    throw new Error('invalid GitHub draft upload JSON response');
  }
  return result;
}

function verifyDraftMetadata(release, manifest, releaseNotes) {
  requireCondition(release.name === manifest.tag &&
    release.body === releaseNotes &&
    release.prerelease === /^v\d+\.\d+\.\d+-/.test(manifest.tag),
  'GitHub draft title, original release notes or prerelease metadata differs');
}

function selectedDraft(preflight, manifest, releaseNotes) {
  requireCondition(preflight.discovery.kind === 'candidate',
    'GitHub draft absence is not sufficient publication authority');
  const observation = preflight.discovery.observation;
  requireCondition(observation.kind === 'present' &&
    observation.release.draft === true &&
    observation.release.immutable === false,
  'only an existing unpublished mutable draft can receive originals');
  const release = observation.release;
  verifyDraftMetadata(release, manifest, releaseNotes);
  const plan = planDraftAssetResume(manifest, release,
    observation.assetPage.assets);
  return { release, plan, authority: preflight.authority };
}

function equalObservation(before, after) {
  requireCondition(JSON.stringify(before.release) === JSON.stringify(after.release) &&
    JSON.stringify(before.plan) === JSON.stringify(after.plan) &&
    JSON.stringify(before.authority) === JSON.stringify(after.authority),
  'remote release or publication authority changed before upload');
}

async function trustedSource(name, asset, directories) {
  requireCondition(directories && typeof directories === 'object' &&
    typeof directories.normalDir === 'string' &&
    typeof directories.mcpbDir === 'string', 'missing original stage directories');
  const group = name.endsWith('.mcpb') || name === 'mcpb-checksums.txt'
    ? directories.mcpbDir : directories.normalDir;
  requireCondition(path.isAbsolute(group), 'original stage must be absolute');
  const root = path.resolve(group);
  const dir = fs.lstatSync(root);
  requireCondition(dir.isDirectory() && !dir.isSymbolicLink() &&
    (process.platform !== 'linux' ||
      (dir.uid === process.getuid() && (dir.mode & 0o077) === 0)),
  'original stage must be a private real directory');
  const pathname = path.join(root, name);
  const original = fs.lstatSync(pathname);
  requireCondition(original.isFile() && !original.isSymbolicLink() &&
    original.nlink === 1 && original.size === asset.size &&
    (process.platform !== 'linux' || original.uid === process.getuid()),
  'unsafe original release asset file');
  const handle = await fs.promises.open(pathname,
    fs.constants.O_RDONLY | (fs.constants.O_NOFOLLOW || 0));
  try {
    requireCondition(stableFileState(await handle.stat(), original, pathname),
      'original source changed before streaming');
    const hash = createHash('sha256');
    let total = 0;
    for await (const bytes of handle.createReadStream({ autoClose: false, start: 0 })) {
      total += bytes.byteLength;
      requireCondition(total <= asset.size, 'oversized source asset');
      hash.update(bytes);
    }
    requireCondition(total === asset.size &&
      hash.digest('hex') === asset.sha256 &&
      stableFileState(await handle.stat(), original, pathname),
    'original release source has changed bytes');
    return { handle, pathname, original };
  } catch (error) {
    await handle.close();
    throw error;
  }
}

// Exactly one non-retryable upload to a pre-existing authenticated draft.
// A lost/ambiguous POST response is a hard stop, not permission to resend.
// The tagged workflow must hold the per-tag GitHub Actions concurrency lock.
// This function cannot independently prove a lock or create/publish releases.
async function uploadOneDraftAsset({
  manifest, publisherCommit, token, normalDir, mcpbDir, releaseNotesMetadata,
  fetchImpl = globalThis.fetch,
}) {
  validateStagedManifest(manifest);
  const releaseNotes = decodeGoReleaserNotes(releaseNotesMetadata);
  requireCondition(typeof token === 'string' && token.length > 0 &&
    token.length <= 4096 && /^[\x21-\x7e]+$/.test(token),
  'invalid GitHub writer token');
  requireCondition(typeof fetchImpl === 'function', 'invalid GitHub fetch adapter');
  const event = { eventName: 'push', repository: 'zoster81/scripthold',
    ref: 'refs/tags/' + manifest?.tag };
  const input = { event, manifest, publisherCommit, token, fetchImpl };
  const first = selectedDraft(await inspectFinalizerPreflight(input), manifest, releaseNotes);
  if (first.plan.missing.length === 0) {
    return { kind: 'draft-complete', releaseId: first.release.id, count: 22 };
  }
  const name = first.plan.missing[0];
  const asset = manifest.assets.find(item => item.name === name);
  const source = await trustedSource(name, asset, { normalDir, mcpbDir });
  try {
    // Revalidate the exact trusted evidence after digesting the original and
    // immediately before sending one mutation to GitHub.
    const second = selectedDraft(await inspectFinalizerPreflight(input), manifest, releaseNotes);
    equalObservation(first, second);
    const url = UPLOAD_ORIGIN + '/repos/zoster81/scripthold/releases/' +
      first.release.id + '/assets?name=' + encodeURIComponent(name);
    const response = await fetchImpl(url, {
      method: 'POST',
      headers: {
        Authorization: 'Bearer ' + token,
        Accept: 'application/vnd.github+json',
        'X-GitHub-Api-Version': '2022-11-28',
        'Content-Type': 'application/octet-stream',
        'Content-Length': String(asset.size),
        'User-Agent': 'scripthold-release-draft-single-writer',
      },
      body: source.handle.createReadStream({ autoClose: false, start: 0 }),
      duplex: 'half', redirect: 'error', cache: 'no-store',
      signal: AbortSignal.timeout(60000),
    });
    const uploaded = await readUploadResult(response, url);
    requireCondition(Number.isSafeInteger(uploaded.id) && uploaded.id > 0 &&
      uploaded.name === name && uploaded.size === asset.size &&
      uploaded.state === 'uploaded' &&
      uploaded.digest === 'sha256:' + asset.sha256,
    'uploaded asset response differs from the original manifest');
    requireCondition(stableFileState(await source.handle.stat(),
      source.original, source.pathname),
    'original source changed during upload');
    const final = selectedDraft(await inspectFinalizerPreflight(input), manifest, releaseNotes);
    requireCondition(final.release.id === first.release.id &&
      final.authority.commit === first.authority.commit &&
      final.plan.present.some(item => item.name === name && item.id === uploaded.id),
    'uploaded asset lacks independently verified draft inventory');
    return { kind: 'uploaded', releaseId: first.release.id,
      name, assetId: uploaded.id, remaining: final.plan.missing.length };
  } finally {
    await source.handle.close();
  }
}

// Creation is deliberately separate from upload/resume: an authenticated
// complete listing plus an exact tagged 404 is still a non-atomic observation.
// A 422, timeout or missing confirmation after POST must halt without retry.
function sameAuthority(a, b) {
  requireCondition(JSON.stringify(a) === JSON.stringify(b),
    'release authority changed while checking draft absence');
}

async function observeDraftAbsence(input) {
  const { manifest, publisherCommit, token, fetchImpl } = input;
  const authority = await readPublicationAuthority({
    manifest, publisherCommit, token, fetchImpl,
  });
  const list = await discoverReleaseByTag({
    tag: manifest.tag, token, fetchImpl,
  });
  requireCondition(list.kind === 'not-found-unproven',
    'draft creation requires a complete authenticated release listing without a tag match');
  const url = 'https://api.github.com/repos/zoster81/scripthold/releases/tags/' +
    encodeURIComponent(manifest.tag);
  const response = await makeGitHubGet(token, fetchImpl)(url);
  requireCondition(response?.status === 404 && response.redirected !== true &&
    (!response.url || response.url === url),
  'draft tag lookup was not an exact GitHub 404');
  const next = await readPublicationAuthority({
    manifest, publisherCommit, token, fetchImpl,
  });
  sameAuthority(authority, next);
  return next;
}

// An authenticated writer must be the sole job holding the per-tag workflow
// concurrency lock. The finalizer cannot infer that exclusivity from a token.
// Only create ONE draft from two separately inspected absence observations;
// never publish here or interpret an uncertain POST as permission to retry.
async function createDraftRelease({
  manifest, publisherCommit, token, releaseNotesMetadata,
  fetchImpl = globalThis.fetch,
}) {
  validateStagedManifest(manifest);
  requireCondition(typeof token === 'string' && token.length > 0 &&
    token.length <= 4096 && /^[\x21-\x7e]+$/.test(token),
  'invalid GitHub writer token');
  requireCondition(typeof fetchImpl === 'function',
    'invalid GitHub fetch adapter');
  // Only the canonical output of the trusted tagged GoReleaser producer is
  // accepted. The caller must authenticate that producing job/run/attempt.
  const releaseNotes = decodeGoReleaserNotes(releaseNotesMetadata);
  const input = { manifest, publisherCommit, token, fetchImpl };
  const first = await observeDraftAbsence(input);
  const second = await observeDraftAbsence(input);
  sameAuthority(first, second);
  const url = 'https://api.github.com/repos/zoster81/scripthold/releases';
  const response = await fetchImpl(url, {
    method: 'POST',
    headers: {
      Authorization: 'Bearer ' + token,
      Accept: 'application/vnd.github+json',
      'X-GitHub-Api-Version': '2022-11-28',
      'Content-Type': 'application/json',
      'User-Agent': 'scripthold-release-draft-single-writer',
    },
    body: JSON.stringify({
      tag_name: manifest.tag,
      target_commitish: manifest.commit,
      name: manifest.tag,
      body: releaseNotes,
      draft: true,
      prerelease: /^v\d+\.\d+\.\d+-/.test(manifest.tag),
      generate_release_notes: false,
    }),
    redirect: 'error', cache: 'no-store',
    signal: AbortSignal.timeout(15000),
  });
  const created = await readUploadResult(response, url);
  requireCondition(Number.isSafeInteger(created.id) && created.id > 0 &&
    created.tag_name === manifest.tag && created.draft === true &&
    created.immutable === false && created.name === manifest.tag &&
    created.body === releaseNotes &&
    created.prerelease === /^v\d+\.\d+\.\d+-/.test(manifest.tag),
  'GitHub draft creation response differs from the producer metadata');
  const observed = await discoverReleaseByTag({
    tag: manifest.tag, token, fetchImpl,
  });
  requireCondition(observed.kind === 'candidate' &&
    observed.releaseId === created.id &&
    observed.observation.release.draft === true &&
    observed.observation.release.immutable === false &&
    observed.observation.assetPage.assets.length === 0,
  'created draft is not independently visible and empty');
  verifyDraftMetadata(observed.observation.release, manifest, releaseNotes);
  const after = await readPublicationAuthority(input);
  sameAuthority(first, after);
  return { kind: 'created', releaseId: created.id, tag: manifest.tag };
}

// A bounded, strictly sequential recovery runner for ONE already-authenticated
// draft. Every iteration obtains fresh authority and independently checks the
// remote asset inventory; any ambiguous upload is terminal for this invocation.
async function uploadAllDraftAssets(input) {
  validateStagedManifest(input?.manifest);
  let remaining = input.manifest.assets.length;
  for (let step = 0; step <= input.manifest.assets.length; step++) {
    const state = await uploadOneDraftAsset(input);
    if (state.kind === 'draft-complete') {
      requireCondition(state.count === 22 && remaining >= 0,
        'invalid complete draft verification');
      return state;
    }
    requireCondition(state.kind === 'uploaded' &&
      Number.isSafeInteger(state.remaining) && state.remaining >= 0 &&
      state.remaining < remaining,
    'draft upload failed to make canonical inventory progress');
    remaining = state.remaining;
  }
  throw new Error('draft upload exceeded maximum original asset count');
}

// One bounded coordinator for a future *authorized* single-writer job.
// It never publishes. A missing draft still requires createDraftRelease's two
// complete independent absence checks; ambiguous POST failures are terminal.
async function prepareDraftAssets(input) {
  validateStagedManifest(input?.manifest);
  decodeGoReleaserNotes(input?.releaseNotesMetadata);
  requireCondition(typeof input.token === 'string' && input.token.length > 0 &&
    input.token.length <= 4096 && /^[\x21-\x7e]+$/.test(input.token),
  'invalid GitHub writer token');
  const fetchImpl = input.fetchImpl ?? globalThis.fetch;
  requireCondition(typeof fetchImpl === 'function', 'invalid GitHub fetch adapter');
  verifyDraftSourceInventory(input.manifest, input.normalDir, input.mcpbDir);
  const discovery = await discoverReleaseByTag({
    tag: input.manifest.tag, token: input.token, fetchImpl,
  });
  if (discovery.kind === 'not-found-unproven') {
    await createDraftRelease({ ...input, fetchImpl });
  } else {
    requireCondition(discovery.kind === 'candidate',
      'unable to authenticate GitHub draft candidate');
  }
  return uploadAllDraftAssets({ ...input, fetchImpl });
}

module.exports = { uploadOneDraftAsset, uploadAllDraftAssets, createDraftRelease,
  prepareDraftAssets };
