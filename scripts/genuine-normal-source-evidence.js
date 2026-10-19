#!/usr/bin/env node
'use strict';

const { makeGitHubGet, readJsonResponse } = require('./release-finalizer-read');
const { getCompleteList } = require('./release-finalizer-authority');

const API = 'https://api.github.com/repos/zoster81/scripthold/actions/runs/';
const REPOSITORY = 'zoster81/scripthold';
const WORKFLOW_ID = 389631986;
const WORKFLOW_PATH = '.github/workflows/ubuntu26-genuine-normal-transfer-proof.yml';
const HEX = /^[0-9a-f]{64}$/;

function ensure(ok, message) {
  if (!ok) throw new Error('genuine normal source: ' + message);
}
function id(text) {
  ensure(typeof text === 'string' && /^[1-9][0-9]*$/.test(text),
    'noncanonical numeric identity');
  const number = Number(text);
  ensure(Number.isSafeInteger(number) && String(number) === text,
    'unsafe numeric identity');
  return number;
}
function sourceInputs(env) {
  ensure(env && typeof env === 'object', 'missing trusted job outputs');
  const runId = id(env.GITHUB_RUN_ID);
  const attempt = id(env.GITHUB_RUN_ATTEMPT);
  const producerAttempt = id(env.PROBE_SOURCE_ATTEMPT);
  const repositoryId = id(env.GITHUB_REPOSITORY_ID);
  const sha = env.GENUINE_SOURCE_SHA;
  const event = env.GENUINE_SOURCE_EVENT;
  ensure(producerAttempt <= attempt && typeof sha === 'string' &&
    /^[0-9a-f]{40}$/.test(sha) &&
    ['pull_request', 'workflow_dispatch'].includes(event),
  'invalid source run identity');
  ensure(typeof env.PROBE_IDS === 'string' &&
    env.PROBE_IDS.length <= 13 * 17 &&
    typeof env.PROBE_DIGESTS === 'string' &&
    env.PROBE_DIGESTS.length <= 13 * 65,
  'oversized upload vectors');
  const ids = env.PROBE_IDS.split(',').map(id);
  const digests = env.PROBE_DIGESTS.split(',');
  ensure(ids.length === 13 && new Set(ids).size === 13 &&
    digests.length === 13 && digests.every(d => HEX.test(d)),
  'incomplete or duplicate raw upload records');
  const expected = new Map();
  for (let i = 1; i <= 13; i++) {
    expected.set('genuine-' + runId + '-' + producerAttempt + '-normal-' +
      String(i).padStart(2, '0'),
    { id: ids[i - 1], digest: 'sha256:' + digests[i - 1] });
  }
  return { runId, attempt, producerAttempt, repositoryId, sha, event, expected };
}
function verifyRun(run, source) {
  ensure(run && run.id === source.runId &&
    run.workflow_id === WORKFLOW_ID && run.path === WORKFLOW_PATH &&
    run.event === source.event && run.head_sha === source.sha &&
    run.run_attempt === source.attempt &&
    run.repository?.id === source.repositoryId &&
    run.repository?.full_name === REPOSITORY &&
    run.head_repository?.id === source.repositoryId &&
    run.head_repository?.full_name === REPOSITORY &&
    (run.status === 'in_progress' ||
      (run.status === 'completed' && run.conclusion === 'success')),
  'unexpected GitHub source workflow run or attempt');
}

// The workflow binds upload IDs/digests through needs.producer; GitHub REST
// independently corroborates run, exact attempt, job success and 13 artifact
// records. GitHub does NOT expose artifact uploader-job identity. This is a
// bounded observation, not a provider atomic snapshot or publication right.
async function verifyGenuineNormalSourceEvidence(env, token,
  fetchImpl = globalThis.fetch) {
  const source = sourceInputs(env);
  const get = makeGitHubGet(token, fetchImpl);
  const runUrl = API + source.runId;
  verifyRun(await readJsonResponse(await get(runUrl), runUrl), source);

  const jobsUrl = runUrl + '/attempts/' + source.producerAttempt +
    '/jobs?per_page=100&page=1';
  const jobs = (await getCompleteList(get, jobsUrl, 'jobs')).jobs;
  const producer = jobs.filter(job => job?.name === 'producer');
  ensure(producer.length === 1 &&
    Number.isSafeInteger(producer[0].id) && producer[0].id > 0 &&
    producer[0].run_id === source.runId &&
    producer[0].run_attempt === source.producerAttempt &&
    producer[0].head_sha === source.sha &&
    producer[0].status === 'completed' &&
    producer[0].conclusion === 'success',
  'producer is not the exact successful job');

  const artifactUrl = runUrl + '/artifacts?per_page=100&page=1';
  const artifacts = (await getCompleteList(get, artifactUrl, 'artifacts')).artifacts;
  ensure(artifacts.length === source.expected.size,
    'wrong number of genuine raw artifacts');
  const remaining = new Map(source.expected);
  for (const artifact of artifacts) {
    const expected = remaining.get(artifact?.name);
    const run = artifact?.workflow_run;
    ensure(expected && artifact.id === expected.id &&
      artifact.digest === expected.digest && artifact.expired === false &&
      Number.isSafeInteger(artifact.size_in_bytes) &&
      artifact.size_in_bytes > 0 && artifact.size_in_bytes <= 2 * 1024 * 1024 * 1024 &&
      run?.id === source.runId && run?.head_sha === source.sha &&
      run?.repository_id === source.repositoryId &&
      run?.head_repository_id === source.repositoryId,
    'wrong genuine artifact ID, digest, name or source run');
    remaining.delete(artifact.name);
  }
  ensure(remaining.size === 0, 'missing original artifacts');
  verifyRun(await readJsonResponse(await get(runUrl), runUrl), source);
  return { verified: 13, producerJobs: 1, runId: source.runId };
}

module.exports = { verifyGenuineNormalSourceEvidence };

if (require.main === module) {
  verifyGenuineNormalSourceEvidence(process.env, process.env.GH_TOKEN)
    .then(({ verified, producerJobs }) =>
      console.log('Verified ' + verified + ' genuine original REST records and ' +
        producerJobs + ' successful exact producer job'))
    .catch(error => { console.error(error.message); process.exitCode = 1; });
}
