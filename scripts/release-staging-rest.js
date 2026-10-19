'use strict';

const {
  expectedReleaseAssetNames,
  validateStagedGroupManifest,
  rawStagingArtifactName,
} = require('./release-asset-contract');
const { assembleRawNormalHandoff, assembleRawReleaseStaging } =
  require('./release-staging-producers');
const { makeGitHubGet, readJsonResponse } = require('./release-finalizer-read');
const { getCompleteList } = require('./release-finalizer-authority');

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

// Bind comma-separated output vectors from the reviewed producer job's fixed
// upload-artifact steps to the canonical stage allowlist. The caller must obtain
// these values from trusted job step outputs, not a producer-supplied manifest.
// This adapter validates *claims*, not their GitHub origin or transferred bytes.
function bindRawUploadJobOutputs({
  manifest, repositoryId, runId, producerAttempt, jobOutputs,
}) {
  exactRecord(manifest, MANIFEST_KEYS, 'staging manifest');
  ensure(validId(repositoryId) && validId(runId) && validId(producerAttempt),
    'invalid trusted producer job identity');
  exactRecord(jobOutputs, ['ids', 'digests', 'attempt'],
    'trusted upload step outputs');
  ensure(typeof jobOutputs.attempt === 'string' &&
    jobOutputs.attempt === String(producerAttempt),
  'upload step attempt differs from trusted producer attempt');
  const files = validateStagedGroupManifest(manifest);
  const names = expectedReleaseAssetNames(manifest.tag)
    .filter(name => files.has(name));

  // Limit decoding before splitting to prevent unbounded untrusted output.
  ensure(typeof jobOutputs.ids === 'string' &&
    jobOutputs.ids.length <= 17 * names.length - 1 &&
    typeof jobOutputs.digests === 'string' &&
    jobOutputs.digests.length <= 65 * names.length,
  'oversized upload step output vector');
  const ids = jobOutputs.ids.split(',');
  const digests = jobOutputs.digests.split(',');
  ensure(ids.length === names.length && digests.length === names.length,
    'incomplete upload step output vector');
  const seen = new Set();
  const uploads = names.map((fileName, index) => {
    const text = ids[index];
    ensure(/^[1-9][0-9]*$/.test(text),
      'noncanonical numeric upload step ID');
    const uploadId = Number(text);
    ensure(validId(uploadId) && String(uploadId) === text && !seen.has(uploadId),
      'invalid, unsafe or duplicate upload step ID');
    const uploadDigest = digests[index];
    ensure(SHA256.test(uploadDigest) &&
      uploadDigest === files.get(fileName).sha256,
    'upload step digest differs from original file');
    seen.add(uploadId);
    return { fileName, uploadId, uploadDigest };
  });
  return stageUploadPlan({
    manifest, repositoryId, runId, producerAttempt, uploads,
  }, manifest.stage, new Set());
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


// A stage claim cannot establish which job ran. This optional, future-only
// layer checks an independently supplied, reviewed workflow/job policy
// against exact run and attempt-scoped GitHub REST job data. Artifact REST
// metadata does not contain an uploader job ID, so this is NOT proof that
// the successful job created each artifact (nor publication authority).
const SOURCE_KEYS = [
  'workflowId', 'workflowPath', 'event', 'normalJobName', 'mcpbJobName',
];
const JOBS_API = 'https://api.github.com/repos/zoster81/scripthold/actions/runs/';

function validateSourcePolicy(sourcePolicy) {
  exactRecord(sourcePolicy, SOURCE_KEYS, 'trusted workflow policy');
  ensure(validId(sourcePolicy.workflowId) &&
    typeof sourcePolicy.workflowPath === 'string' &&
    /^\.github\/workflows\/[A-Za-z0-9_.-]+\.ya?ml$/
      .test(sourcePolicy.workflowPath) &&
    ['push', 'workflow_dispatch'].includes(sourcePolicy.event),
  'invalid trusted workflow identity');
  for (const name of [sourcePolicy.normalJobName, sourcePolicy.mcpbJobName]) {
    ensure(typeof name === 'string' &&
      /^[A-Za-z0-9][A-Za-z0-9 ()_.-]{0,99}$/.test(name),
    'invalid trusted producer job name');
  }
  ensure(sourcePolicy.normalJobName !== sourcePolicy.mcpbJobName,
    'ambiguous trusted producer job names');
}

function verifySourceRun(run, plan, policy) {
  ensure(run && typeof run === 'object' && !Array.isArray(run) &&
    run.id === plan.runId && run.workflow_id === policy.workflowId &&
    run.path === policy.workflowPath && run.event === policy.event &&
    run.head_sha === plan.manifest.commit &&
    validId(run.run_attempt) && run.run_attempt >= plan.producerAttempt &&
    run.repository?.id === plan.repositoryId &&
    run.repository?.full_name === REPOSITORY &&
    run.head_repository?.id === plan.repositoryId &&
    run.head_repository?.full_name === REPOSITORY &&
    (run.status === 'in_progress' ||
      (run.status === 'completed' && run.conclusion === 'success')),
  'source workflow run or attempt is not trustworthy');
}

// This check is read-only; all producer inputs are prevalidated before GET.
// Attempt-scoped job URLs avoid the default latest-attempt ambiguity on reruns.
// The run is reread after the job pages to detect an intervening rerun.
async function verifySourceJobs(plans, sourcePolicy, token, fetchImpl) {
  validateSourcePolicy(sourcePolicy);
  const policy = { ...sourcePolicy };
  const first = plans[0];
  for (const plan of plans) {
    ensure(plan.runId === first.runId &&
      plan.repositoryId === first.repositoryId &&
      plan.manifest.tag === first.manifest.tag &&
      plan.manifest.commit === first.manifest.commit,
    'staging jobs have mixed workflow source identities');
  }
  const get = makeGitHubGet(token, fetchImpl);
  const runUrl = JOBS_API + first.runId;
  const run = await readJsonResponse(await get(runUrl), runUrl);
  for (const plan of plans) verifySourceRun(run, plan, policy);
  const jobs = [];
  for (const plan of plans) {
    const url = runUrl + '/attempts/' + plan.producerAttempt +
      '/jobs?per_page=100&page=1';
    const page = await getCompleteList(get, url, 'jobs');
    const name = plan.manifest.stage === 'normal'
      ? policy.normalJobName : policy.mcpbJobName;
    const matches = page.jobs.filter(job => job && job.name === name);
    ensure(matches.length === 1 && validId(matches[0].id) &&
      matches[0].run_id === plan.runId &&
      matches[0].run_attempt === plan.producerAttempt &&
      matches[0].head_sha === plan.manifest.commit &&
      matches[0].status === 'completed' &&
      matches[0].conclusion === 'success',
    'producer job is not a successful exact-attempt job');
    jobs.push({
      stage: plan.manifest.stage, jobId: matches[0].id,
      runAttempt: plan.producerAttempt,
    });
  }
  const last = await readJsonResponse(await get(runUrl), runUrl);
  for (const plan of plans) verifySourceRun(last, plan, policy);
  ensure(last.run_attempt === run.run_attempt,
    'source workflow was rerun during evidence read');
  return { jobs, runAttempt: last.run_attempt, policy };
}

// Artifact metadata is read in a separate sequence after the producer jobs.
// Re-observe the trusted run so a rerun during those reads is not silently
// combined with the earlier successful job evidence. This is a bounded
// observation, not an atomic GitHub snapshot or publication authority.
async function recheckSourceAfterArtifacts(plan, source, token, fetchImpl) {
  const get = makeGitHubGet(token, fetchImpl);
  const url = JOBS_API + plan.runId;
  const run = await readJsonResponse(await get(url), url);
  verifySourceRun(run, plan, source.policy);
  ensure(run.run_attempt === source.runAttempt,
    'source workflow was rerun after artifact metadata reads');
}

async function readJobVerifiedRawNormalStaging({
  normal, sourcePolicy, token, fetchImpl = globalThis.fetch,
}) {
  const plan = stageUploadPlan(normal, 'normal', new Set());
  const source = await verifySourceJobs([plan], sourcePolicy, token, fetchImpl);
  const result = await readRawNormalStaging({ normal: plan, token, fetchImpl });
  await recheckSourceAfterArtifacts(plan, source, token, fetchImpl);
  return { ...result, jobs: source.jobs };
}

async function readJobVerifiedRawCompleteStaging({
  normal, mcpb, sourcePolicy, token, fetchImpl = globalThis.fetch,
}) {
  const seen = new Set();
  const first = stageUploadPlan(normal, 'normal', seen);
  const second = stageUploadPlan(mcpb, 'mcpb', seen);
  const source = await verifySourceJobs([first, second], sourcePolicy, token, fetchImpl);
  const result = await readRawCompleteStaging({ normal: first, mcpb: second,
    token, fetchImpl });
  await recheckSourceAfterArtifacts(first, source, token, fetchImpl);
  return { ...result, jobs: source.jobs };
}

module.exports = { readRawNormalStaging, readRawCompleteStaging,
  readJobVerifiedRawNormalStaging, readJobVerifiedRawCompleteStaging,
  bindRawUploadJobOutputs };
