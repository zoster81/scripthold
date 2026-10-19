'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');
const { verifyGenuineNormalSourceEvidence } = require('./genuine-normal-source-evidence');

const SHA = 'a'.repeat(40);
const RUN_URL = 'https://api.github.com/repos/zoster81/scripthold/actions/runs/9001';
function fixture() {
  const env = {
    GITHUB_RUN_ID: '9001', GITHUB_RUN_ATTEMPT: '2',
    GITHUB_REPOSITORY_ID: '1309982126',
    GENUINE_SOURCE_SHA: SHA, GENUINE_SOURCE_EVENT: 'pull_request',
    PROBE_SOURCE_ATTEMPT: '1',
    PROBE_IDS: Array.from({ length: 13 }, (_, i) => 1001 + i).join(','),
    PROBE_DIGESTS: Array(13).fill('a'.repeat(64)).join(','),
  };
  const repo = { id: 1309982126, full_name: 'zoster81/scripthold' };
  const run = { id: 9001, workflow_id: 389631986,
    path: '.github/workflows/ubuntu26-genuine-normal-transfer-proof.yml',
    event: 'pull_request', head_sha: SHA, run_attempt: 2,
    status: 'in_progress', repository: repo, head_repository: repo };
  const jobs = { total_count: 1, jobs: [{
    id: 321, name: 'producer', run_id: 9001, run_attempt: 1,
    head_sha: SHA, status: 'completed', conclusion: 'success',
  }] };
  const artifacts = Array.from({ length: 13 }, (_, i) => ({
    id: 1001 + i, name: `genuine-9001-1-normal-${String(i + 1).padStart(2, '0')}`,
    digest: 'sha256:' + 'a'.repeat(64), size_in_bytes: 10, expired: false,
    workflow_run: { id: 9001, head_sha: SHA, repository_id: 1309982126,
      head_repository_id: 1309982126 },
  }));
  const payloads = {
    [RUN_URL]: run,
    [RUN_URL + '/attempts/1/jobs?per_page=100&page=1']: jobs,
    [RUN_URL + '/artifacts?per_page=100&page=1']:
      { total_count: 13, artifacts },
  };
  let runReads = 0;
  const fetchImpl = async (url, options) => {
    assert.equal(options.method, 'GET');
    assert.equal(options.redirect, 'error');
    assert.ok(Object.hasOwn(payloads, url), url);
    let payload = payloads[url];
    if (url === RUN_URL && ++runReads === 2) payload = { ...run };
    const bytes = Buffer.from(JSON.stringify(payload));
    return { status: 200, redirected: false, url,
      headers: { get: key => ({
        'content-type': 'application/json',
        'content-length': String(bytes.length), link: null,
      })[key] ?? null },
      body: (async function* () { yield bytes; })(),
    };
  };
  return { env, run, jobs, artifacts, payloads, fetchImpl };
}

test('13 genuine action outputs map to exact successful source job and immutable artifact IDs', async () => {
  const f = fixture();
  assert.deepEqual(await verifyGenuineNormalSourceEvidence(f.env, 'test-token', f.fetchImpl),
    { verified: 13, producerJobs: 1, runId: 9001 });
});

test('rejects malformed source, workflow, job, artifacts, output IDs and digest substitution', async () => {
  for (const mutate of [
    f => { f.run.workflow_id++; },
    f => { f.run.path = '.github/workflows/release.yml'; },
    f => { f.run.event = 'push'; },
    f => { f.run.repository.id++; },
    f => { f.run.head_sha = 'b'.repeat(40); },
    f => { f.jobs.jobs[0].conclusion = 'failure'; },
    f => { f.jobs.jobs[0].run_attempt = 2; },
    f => { f.artifacts[0].digest = 'sha256:' + 'b'.repeat(64); },
    f => { f.artifacts[0].workflow_run.repository_id++; },
    f => { f.artifacts[0].name = 'wrong'; },
    f => { f.artifacts.pop(); f.payloads[RUN_URL + '/artifacts?per_page=100&page=1'].total_count = 12; },
    f => { f.env.PROBE_IDS = f.env.PROBE_IDS.replace('1001', '9999'); },
    f => { f.env.PROBE_DIGESTS = f.env.PROBE_DIGESTS.replace('a'.repeat(64), 'b'.repeat(64)); },
    f => { f.env.PROBE_SOURCE_ATTEMPT = '3'; },
    f => { f.env.GITHUB_RUN_ATTEMPT = '0'; },
  ]) {
    const f = fixture(); mutate(f);
    await assert.rejects(verifyGenuineNormalSourceEvidence(f.env, 'test-token', f.fetchImpl));
  }
});

test('rejects an intervening workflow rerun observed after artifact reads', async () => {
  const f = fixture();
  const originalFetch = f.fetchImpl;
  let count = 0;
  const changedFetch = async (url, options) => {
    const response = await originalFetch(url, options);
    if (url !== RUN_URL || ++count !== 2) return response;
    const altered = { ...f.run, run_attempt: 3 };
    const bytes = Buffer.from(JSON.stringify(altered));
    return { ...response,
      headers: { get: key => ({
        'content-type': 'application/json',
        'content-length': String(bytes.length), link: null,
      })[key] ?? null },
      body: (async function* () { yield bytes; })(),
    };
  };
  await assert.rejects(verifyGenuineNormalSourceEvidence(f.env, 'test-token', changedFetch),
    /run|attempt|identity|rerun/i);
});


test('hosted source proof remains read-only and binds exact needs.producer outputs', () => {
  const fs = require('node:fs');
  const path = require('node:path');
  const yaml = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows',
    'ubuntu26-genuine-normal-transfer-proof.yml'), 'utf8');
  assert.match(yaml, /actions: read/);
  assert.match(yaml, /needs: producer/);
  assert.match(yaml, /GENUINE_SOURCE_SHA: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
  assert.match(yaml, /PROBE_SOURCE_ATTEMPT: \$\{\{ needs\.producer\.outputs\.attempt \}\}/);
  assert.match(yaml, /PROBE_IDS: \$\{\{ needs\.producer\.outputs\.ids \}\}/);
  assert.match(yaml, /PROBE_DIGESTS: \$\{\{ needs\.producer\.outputs\.digests \}\}/);
  assert.match(yaml, /node scripts\/genuine-normal-source-evidence\.js/);
  assert.doesNotMatch(yaml, /contents: write|id-token: write|packages: write|attestations: write/);
});
