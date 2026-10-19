'use strict';

const { makeGitHubGet, readJsonResponse } = require('./release-finalizer-read');
const { getCompleteList } = require('./release-finalizer-authority');

const API = 'https://api.github.com/repos/zoster81/scripthold/actions/runs/';
const WORKFLOW_ID = 386406506;
const REPOSITORY = 'zoster81/scripthold';
const WORKFLOW_PATH = '.github/workflows/raw-artifact-transfer-probe.yml';

function ensure(ok, label) {
  if (!ok) throw new Error('raw probe source evidence: ' + label);
}
function integer(text) {
  ensure(typeof text === 'string' && /^[1-9][0-9]*$/.test(text),
    'noncanonical numeric identifier');
  const n = Number(text);
  ensure(Number.isSafeInteger(n) && String(n) === text, 'unsafe numeric identifier');
  return n;
}
function sourceRun(run, identity) {
  ensure(run && run.id === identity.runId &&
    run.workflow_id === WORKFLOW_ID && run.path === WORKFLOW_PATH &&
    run.event === identity.event && run.head_sha === identity.sha &&
    run.run_attempt === identity.attempt &&
    run.repository?.id === identity.repositoryId &&
    run.repository?.full_name === REPOSITORY &&
    run.head_repository?.id === identity.repositoryId &&
    run.head_repository?.full_name === REPOSITORY &&
    (run.status === 'in_progress' ||
      (run.status === 'completed' && run.conclusion === 'success')),
  'unexpected GitHub workflow run identity');
}

function sourceInputs(env) {
  ensure(env && typeof env === 'object', 'missing original job outputs');
  const identity = {
    runId: integer(env.GITHUB_RUN_ID), attempt: integer(env.GITHUB_RUN_ATTEMPT),
    repositoryId: integer(env.GITHUB_REPOSITORY_ID),
    sha: env.PROBE_SOURCE_SHA, event: env.PROBE_SOURCE_EVENT,
  };
  ensure(/^[0-9a-f]{40}$/.test(identity.sha ?? '') &&
    ['pull_request', 'workflow_dispatch'].includes(identity.event),
  'invalid source commit or event');
  const expected = new Map();
  const stageAttempts = new Map();
  const ids = new Set();
  for (const [group, count] of [['normal', 15], ['package', 7]]) {
    const upper = group.toUpperCase();
    const attempt = integer(env[upper + '_ATTEMPT']);
    ensure(attempt <= identity.attempt, 'producer attempt newer than workflow');
    stageAttempts.set(group, attempt);
    const rawIds = env[upper + '_IDS'];
    const rawDigests = env[upper + '_DIGESTS'];
    ensure(typeof rawIds === 'string' && rawIds.length <= count * 17 &&
      typeof rawDigests === 'string' && rawDigests.length <= count * 65,
    'oversized producer step outputs');
    const allIds = rawIds.split(',');
    const allDigests = rawDigests.split(',');
    ensure(allIds.length === count && allDigests.length === count,
      'incomplete fixed upload step outputs');
    for (let i = 1; i <= count; i++) {
      const id = integer(allIds[i - 1]);
      const digest = allDigests[i - 1];
      ensure(!ids.has(id) && /^[0-9a-f]{64}$/.test(digest),
        'duplicate upload ID or invalid SHA-256');
      ids.add(id);
      const suffix = group === 'normal' && i === 12 ? 'zip' : 'dat';
      const name = `probe-${identity.runId}-${attempt}-${group}-${String(i).padStart(2, '0')}.${suffix}`;
      expected.set(name, { id, digest: 'sha256:' + digest });
    }
  }
  ensure(expected.size === 22 && ids.size === 22, 'incomplete original upload outputs');
  return { identity, stageAttempts, expected };
}

// The workflow's producer outputs are bound directly through needs.<job>.
// GitHub's artifact API has no uploader-job field: success of the exact job
// and matching exact-ID artifact metadata do NOT prove per-job upload origin.
async function verifyRawProbeSourceEvidence(env, token, fetchImpl = globalThis.fetch) {
  const { identity, stageAttempts, expected } = sourceInputs(env);
  const get = makeGitHubGet(token, fetchImpl);
  const runUrl = API + identity.runId;
  sourceRun(await readJsonResponse(await get(runUrl), runUrl), identity);

  const jobsByAttempt = new Map();
  for (const attempt of new Set(stageAttempts.values())) {
    const url = runUrl + '/attempts/' + attempt + '/jobs?per_page=100&page=1';
    jobsByAttempt.set(attempt, (await getCompleteList(get, url, 'jobs')).jobs);
  }
  for (const [group, attempt] of stageAttempts) {
    const jobs = jobsByAttempt.get(attempt);
    const candidates = jobs.filter(job => job?.name === group);
    ensure(candidates.length === 1 &&
      Number.isSafeInteger(candidates[0].id) && candidates[0].id > 0 &&
      candidates[0].run_id === identity.runId &&
      candidates[0].run_attempt === attempt &&
      candidates[0].head_sha === identity.sha &&
      candidates[0].status === 'completed' &&
      candidates[0].conclusion === 'success',
    'producer job is not an exact successful run attempt');
  }

  const artifactUrl = runUrl + '/artifacts?per_page=100&page=1';
  const actual = (await getCompleteList(get, artifactUrl, 'artifacts')).artifacts;
  ensure(actual.length === expected.size, 'wrong number of raw artifacts');
  const remaining = new Map(expected);
  for (const artifact of actual) {
    const item = remaining.get(artifact?.name);
    const source = artifact?.workflow_run;
    ensure(item && item.id === artifact.id &&
      artifact.digest === item.digest && artifact.expired === false &&
      Number.isSafeInteger(artifact.size_in_bytes) && artifact.size_in_bytes > 0 &&
      source?.id === identity.runId && source?.head_sha === identity.sha &&
      source?.repository_id === identity.repositoryId &&
      source?.head_repository_id === identity.repositoryId,
    'artifact ID, digest, source run or original filename mismatch');
    remaining.delete(artifact.name);
  }
  ensure(remaining.size === 0, 'incomplete verified original artifact set');

  // Detect reruns or changed source identity during multiple provider reads.
  sourceRun(await readJsonResponse(await get(runUrl), runUrl), identity);
  return { verified: expected.size, producerJobs: stageAttempts.size,
    runId: identity.runId };
}

module.exports = { verifyRawProbeSourceEvidence };

if (require.main === module) {
  verifyRawProbeSourceEvidence(process.env, process.env.GH_TOKEN)
    .then(({ verified, producerJobs }) =>
      console.log(`VERIFIED ${verified}/22 artifact IDs and ${producerJobs} exact successful producer jobs`))
    .catch(error => {
      console.error(error.message); process.exitCode = 1;
    });
}
