'use strict';

const {
  expectedReleaseAssetNames,
  validateStagedGroupManifest,
  rawStagingArtifactName,
} = require('./release-asset-contract');
const { assembleRawNormalHandoff, assembleRawReleaseStaging } =
  require('./release-staging-producers');
const { makeGitHubGet, readJsonResponse } = require('./release-finalizer-read');

const API = 'https://api.github.com/repos/zoster81/scripthold/actions/artifacts/';
const REPOSITORY = 'zoster81/scripthold';
const PRODUCER_KEYS = ['manifest', 'repositoryId', 'runId', 'producerAttempt', 'uploads'];
const MANIFEST_KEYS = ['repository', 'tag', 'commit', 'stage', 'assets'];
const ASSET_KEYS = ['name', 'size', 'sha256'];
const UPLOAD_KEYS = ['fileName', 'uploadId', 'uploadDigest'];
const SHA256 = /^[0-9a-f]{64}$/;

function ensure(ok, message) {
  if (!ok) throw new Error(message);
}

function exactRecord(value, keys, label) {
  ensure(value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).length === keys.length &&
    keys.every((key) => Object.hasOwn(value, key)),
  'unexpected ' + label + ' schema');
}

function validId(value) {
  return Number.isSafeInteger(value) && value > 0;
}

// Reject incomplete, mixed and path-bearing producer records *before* any
// provider request. No latest-run, name search or producer-local path is used.
function stageUploadPlan(producer, stage, seenIds) {
  exactRecord(producer, PRODUCER_KEYS, 'staging producer');
  exactRecord(producer.manifest, MANIFEST_KEYS, 'staging manifest');
  ensure(producer.manifest.stage === stage &&
    producer.manifest.repository === REPOSITORY, 'unexpected producer stage');
  const assets = validateStagedGroupManifest(producer.manifest);
  for (const asset of producer.manifest.assets) {
    exactRecord(asset, ASSET_KEYS, 'staged asset');
  }
  ensure(validId(producer.repositoryId) && validId(producer.runId) &&
    validId(producer.producerAttempt), 'invalid producer workflow identity');
  ensure(Array.isArray(producer.uploads) && producer.uploads.length === assets.size,
    'incomplete staged producer uploads');

  const files = new Map();
  for (const upload of producer.uploads) {
    exactRecord(upload, UPLOAD_KEYS, 'producer upload');
    ensure(typeof upload.fileName === 'string' && assets.has(upload.fileName) &&
      !files.has(upload.fileName), 'missing or duplicate staged upload');
    ensure(validId(upload.uploadId) && !seenIds.has(upload.uploadId),
      'invalid or duplicate raw artifact ID');
    ensure(typeof upload.uploadDigest === 'string' && SHA256.test(upload.uploadDigest) &&
      upload.uploadDigest === assets.get(upload.fileName).sha256,
    'staged upload digest mismatches original file');
    rawStagingArtifactName(stage, producer.runId, producer.producerAttempt,
      upload.fileName);
    seenIds.add(upload.uploadId);
    files.set(upload.fileName, {
      fileName: upload.fileName, uploadId: upload.uploadId,
      uploadDigest: upload.uploadDigest,
    });
  }
  const manifest = {
    repository: producer.manifest.repository, tag: producer.manifest.tag,
    commit: producer.manifest.commit, stage,
    assets: expectedReleaseAssetNames(producer.manifest.tag)
      .filter((name) => assets.has(name))
      .map((name) => ({ name, ...assets.get(name) })),
  };
  return {
    manifest, repositoryId: producer.repositoryId,
    runId: producer.runId, producerAttempt: producer.producerAttempt,
    uploads: manifest.assets.map((asset) => files.get(asset.name)),
  };
}

// Retain only the exact provider fields that have defined trust meaning.
// The response still MUST be bound to original SHA/size and actual downloaded
// bytes by the shared 15/22-file verifier; HTTPS metadata is not a mutation permit.
function isolateArtifact(record) {
  ensure(record && typeof record === 'object' && !Array.isArray(record) &&
    record.workflow_run && typeof record.workflow_run === 'object' &&
    !Array.isArray(record.workflow_run), 'invalid GitHub Actions artifact response');
  const run = record.workflow_run;
  return {
    id: record.id, name: record.name, expired: record.expired,
    size_in_bytes: record.size_in_bytes, digest: record.digest,
    workflow_run: {
      id: run.id, repository_id: run.repository_id,
      head_repository_id: run.head_repository_id, head_sha: run.head_sha,
    },
  };
}

async function readStage(get, producer) {
  const uploads = [];
  for (const upload of producer.uploads) {
    const url = API + String(upload.uploadId);
    const remote = isolateArtifact(await readJsonResponse(await get(url), url));
    uploads.push({ ...upload, remote });
  }
  return { ...producer, uploads };
}

// Caller must separately prove the trusted workflow/job and producer attempt,
// own the download destination, validate all received bytes, and re-read
// authority immediately before any later release mutation. This reader only
// fetches exact-ID GitHub REST metadata from one immutable repository endpoint.
async function readRawNormalStaging({ normal, token, fetchImpl = globalThis.fetch }) {
  const preflight = stageUploadPlan(normal, 'normal', new Set());
  const get = makeGitHubGet(token, fetchImpl);
  const producer = await readStage(get, preflight);
  return { producer, ...assembleRawNormalHandoff(producer) };
}

async function readRawCompleteStaging({
  normal, mcpb, token, fetchImpl = globalThis.fetch,
}) {
  const seenIds = new Set();
  const first = stageUploadPlan(normal, 'normal', seenIds);
  const second = stageUploadPlan(mcpb, 'mcpb', seenIds);
  ensure(first.manifest.repository === second.manifest.repository &&
    first.manifest.tag === second.manifest.tag &&
    first.manifest.commit === second.manifest.commit &&
    first.runId === second.runId &&
    first.repositoryId === second.repositoryId,
  'staging producers disagree on exact source or workflow identity');
  const get = makeGitHubGet(token, fetchImpl);
  const authenticNormal = await readStage(get, first);
  assembleRawNormalHandoff(authenticNormal);
  const authenticMCPB = await readStage(get, second);
  return { normal: authenticNormal, mcpb: authenticMCPB,
    ...assembleRawReleaseStaging(authenticNormal, authenticMCPB) };
}

module.exports = { readRawNormalStaging, readRawCompleteStaging };
