'use strict';
const assert = require('node:assert/strict');
const test = require('node:test');
const { verifyRawProbeSourceEvidence } = require('./raw-probe-source-evidence');

const sha = 'a'.repeat(40);
function fixture() {
  const env = {
    GITHUB_RUN_ID: '9001', GITHUB_RUN_ATTEMPT: '2',
    GITHUB_REPOSITORY_ID: '1309982126',
    PROBE_SOURCE_SHA: sha, PROBE_SOURCE_EVENT: 'pull_request',
    NORMAL_ATTEMPT: '1', PACKAGE_ATTEMPT: '2',
    NORMAL_IDS: Array.from({ length: 15 }, (_, i) => 1001 + i).join(','),
    PACKAGE_IDS: Array.from({ length: 7 }, (_, i) => 2001 + i).join(','),
    NORMAL_DIGESTS: Array(15).fill('a'.repeat(64)).join(','),
    PACKAGE_DIGESTS: Array(7).fill('b'.repeat(64)).join(','),
  };
  const repo = { id: 1309982126, full_name: 'zoster81/scripthold' };
  const run = {
    id: 9001, workflow_id: 386406506,
    path: '.github/workflows/raw-artifact-transfer-probe.yml',
    event: 'pull_request', head_sha: sha, run_attempt: 2,
    status: 'in_progress', conclusion: null, repository: repo,
    head_repository: repo,
  };
  const jobs = attempt => ({ total_count: attempt === 1 ? 1 : 2, jobs:
    (attempt === 1 ? ['normal'] : ['package', 'consumer']).map((name, i) => ({
      id: 3000 + attempt * 10 + i, name, run_id: 9001,
      run_attempt: attempt, head_sha: sha, status: name === 'consumer'
        ? 'in_progress' : 'completed', conclusion: name === 'consumer'
          ? null : 'success',
    })) });
  const artifacts = [];
  for (const [group, count, attempt, ids, digest] of [
    ['normal', 15, 1, 1001, 'a'.repeat(64)],
    ['package', 7, 2, 2001, 'b'.repeat(64)],
  ]) for (let i = 1; i <= count; i++) artifacts.push({
    id: ids + i - 1,
    name: `probe-9001-${attempt}-${group}-${String(i).padStart(2, '0')}.${group === 'normal' && i === 12 ? 'zip' : 'dat'}`,
    digest: 'sha256:' + digest, expired: false, size_in_bytes: 26,
    workflow_run: { id: 9001, head_sha: sha,
      repository_id: 1309982126, head_repository_id: 1309982126 },
  });
  const payloads = {
    'https://api.github.com/repos/zoster81/scripthold/actions/runs/9001': run,
    'https://api.github.com/repos/zoster81/scripthold/actions/runs/9001/attempts/1/jobs?per_page=100&page=1': jobs(1),
    'https://api.github.com/repos/zoster81/scripthold/actions/runs/9001/attempts/2/jobs?per_page=100&page=1': jobs(2),
    'https://api.github.com/repos/zoster81/scripthold/actions/runs/9001/artifacts?per_page=100&page=1': { total_count: 22, artifacts },
  };
  const fetchImpl = async (url, options) => {
    assert.equal(options.method, 'GET');
    assert.equal(options.redirect, 'error');
    assert.ok(Object.hasOwn(payloads, url), url);
    const text = JSON.stringify(payloads[url]);
    return { status: 200, redirected: false, url,
      headers: { get: key => ({
        'content-type': 'application/json',
        'content-length': String(Buffer.byteLength(text)),
        link: null,
      })[key] ?? null },
      body: (async function* () { yield Buffer.from(text); })(),
    };
  };
  return { env, run, payloads, artifacts, fetchImpl };
}

test('read-only raw probe binds 22 job step outputs to exact GitHub run/attempt/artifacts', async () => {
  const f = fixture();
  const result = await verifyRawProbeSourceEvidence(f.env, 'fake-token', f.fetchImpl);
  assert.deepEqual(result, { verified: 22, producerJobs: 2, runId: 9001 });
});

test('raw probe rejects altered workflow, retry, uploader job, artifact IDs and digest', async () => {
  for (const mutation of [
    f => { f.run.workflow_id++; },
    f => { f.run.head_sha = 'b'.repeat(40); },
    f => { f.run.run_attempt++; },
    f => { f.payloads['https://api.github.com/repos/zoster81/scripthold/actions/runs/9001/attempts/1/jobs?per_page=100&page=1'].jobs[0].conclusion = 'failure'; },
    f => { f.artifacts[0].workflow_run.repository_id++; },
    f => { f.artifacts[0].digest = 'sha256:' + 'c'.repeat(64); },
    f => { f.artifacts[0].name = '../other'; },
    f => { f.artifacts.pop(); f.payloads['https://api.github.com/repos/zoster81/scripthold/actions/runs/9001/artifacts?per_page=100&page=1'].total_count = 21; },
    f => { f.env.NORMAL_IDS = f.env.NORMAL_IDS.replace('1001', '9999'); },
    f => { f.env.NORMAL_ATTEMPT = '3'; },
    f => { f.env.GITHUB_RUN_ATTEMPT = '1'; },
    f => { f.run.path = '.github/workflows/release.yml'; },
  ]) {
    const f = fixture(); mutation(f);
    await assert.rejects(verifyRawProbeSourceEvidence(f.env, 'fake-token', f.fetchImpl));
  }
});
