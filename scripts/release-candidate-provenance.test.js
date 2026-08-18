'use strict';

const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const test = require('node:test');

const {
  selectExactSuccessfulPushRun,
  verifySuccessfulReleaseCandidateJob,
} = require('./release-candidate-provenance');

function run(overrides = {}) {
  return {
    id: 11,
    run_number: 7,
    head_sha: 'abc123',
    head_branch: 'main',
    event: 'push',
    conclusion: 'success',
    path: '.github/workflows/test.yml',
    head_repository: { full_name: 'zoster81/scripthold' },
    ...overrides,
  };
}

test('selects the latest exact-SHA successful main push run for the fork', () => {
  const payload = {
    workflow_runs: [
      run({ id: 10, run_number: 6 }),
      run({ id: 12, run_number: 8 }),
      run({ id: 13, run_number: 9, head_sha: 'other' }),
      run({ id: 14, run_number: 10, head_branch: 'feature' }),
      run({ id: 15, run_number: 11, event: 'pull_request' }),
      run({ id: 16, run_number: 12, conclusion: 'failure' }),
      run({ id: 17, run_number: 13, path: '.github/workflows/codeql.yml' }),
      run({ id: 18, run_number: 14, head_repository: { full_name: 'other/repo' } }),
    ],
  };
  assert.equal(
    selectExactSuccessfulPushRun(payload, { sha: 'abc123', repository: 'zoster81/scripthold' }),
    12,
  );
});

test('rejects missing exact-SHA push provenance', () => {
  assert.throws(
    () => selectExactSuccessfulPushRun({ workflow_runs: [run({ head_sha: 'other' })] }, {
      sha: 'abc123',
      repository: 'zoster81/scripthold',
    }),
    /no successful exact-commit Test Suite push run/,
  );
});

test('verifies exactly one successful Release candidate job', () => {
  assert.doesNotThrow(() => verifySuccessfulReleaseCandidateJob({
    jobs: [
      { name: 'Analyze', conclusion: 'success' },
      { name: 'Release candidate', conclusion: 'success' },
    ],
  }));

  for (const jobs of [
    [],
    [{ name: 'Release candidate', conclusion: 'failure' }],
    [
      { name: 'Release candidate', conclusion: 'success' },
      { name: 'Release candidate', conclusion: 'failure' },
    ],
    [
      { name: 'Release candidate', conclusion: 'success' },
      { name: 'Release candidate', conclusion: 'success' },
    ],
  ]) {
    assert.throws(
      () => verifySuccessfulReleaseCandidateJob({ jobs }),
      /exactly one successful Release candidate job/,
    );
  }
});

test('rejects malformed GitHub API payloads', () => {
  assert.throws(
    () => selectExactSuccessfulPushRun({}, { sha: 'abc123', repository: 'z' }),
    /workflow_runs must be an array/,
  );
  assert.throws(() => verifySuccessfulReleaseCandidateJob({}), /jobs must be an array/);
});

test('CLI enforces provenance policy over stdin and argv', () => {
  const script = path.join(__dirname, 'release-candidate-provenance.js');
  const selection = spawnSync(
    process.execPath,
    [script, 'select-run', 'abc123', 'zoster81/scripthold'],
    { input: JSON.stringify({ workflow_runs: [run()] }), encoding: 'utf8' },
  );
  assert.equal(selection.status, 0, selection.stderr);
  assert.equal(selection.stdout, '11');

  const verification = spawnSync(
    process.execPath,
    [script, 'verify-jobs'],
    { input: JSON.stringify({ jobs: [{ name: 'Release candidate', conclusion: 'success' }] }), encoding: 'utf8' },
  );
  assert.equal(verification.status, 0, verification.stderr);
  assert.equal(verification.stdout, '');
});
