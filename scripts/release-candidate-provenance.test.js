'use strict';

const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const test = require('node:test');
const { expectedReleaseAssetNames } = require('./release-asset-contract');

const {
  selectExactSuccessfulPushRun,
  selectExactSuccessfulScorecardRun,
  verifySuccessfulReleaseCandidateJob,
  validatePublicationAuthoritySnapshot,
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

test('selects only exact-SHA successful default-branch Scorecard push evidence', () => {
  const scorecardRun = (overrides = {}) => run({
    path: '.github/workflows/scorecard.yml',
    ...overrides,
  });
  const payload = {
    workflow_runs: [
      scorecardRun({ id: 21, run_number: 4 }),
      scorecardRun({ id: 22, run_number: 5 }),
      scorecardRun({ id: 23, run_number: 6, head_branch: 'v3.2.1' }),
      scorecardRun({ id: 24, run_number: 7, event: 'workflow_dispatch' }),
      scorecardRun({ id: 25, run_number: 8, conclusion: 'failure' }),
      scorecardRun({ id: 26, run_number: 9, path: '.github/workflows/test.yml' }),
    ],
  };
  assert.equal(
    selectExactSuccessfulScorecardRun(payload, { sha: 'abc123', repository: 'zoster81/scripthold' }),
    22,
  );
  assert.throws(
    () => selectExactSuccessfulScorecardRun({ workflow_runs: [scorecardRun({ head_sha: 'other' })] }, {
      sha: 'abc123',
      repository: 'zoster81/scripthold',
    }),
    /no successful exact-commit OpenSSF Scorecard push run/,
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

const AUTH_SHA = 'a'.repeat(40);
const ANNOTATED_TAG_SHA = 'b'.repeat(40);

function publicationEvidence() {
  const tag = 'v3.4.0';
  return {
    repository: 'zoster81/scripthold',
    tag,
    commit: AUTH_SHA,
    publisherCommit: AUTH_SHA,
    manifest: { repository: 'zoster81/scripthold', tag, commit: AUTH_SHA,
      assets: expectedReleaseAssetNames(tag).map((name, i) => ({
        name, size: i + 1, sha256: 'a'.repeat(64),
      })),
    },
    tagRef: {
      ref: 'refs/tags/' + tag,
      object: { type: 'tag', sha: ANNOTATED_TAG_SHA },
    },
    tagObject: {
      sha: ANNOTATED_TAG_SHA, tag,
      object: { type: 'commit', sha: AUTH_SHA },
    },
    mainRef: {
      ref: 'refs/heads/main',
      object: { type: 'commit', sha: AUTH_SHA },
    },
    testRuns: { workflow_runs: [
      run({ id: 124, run_number: 8, run_attempt: 2, head_sha: AUTH_SHA }),
    ] },
    scorecardRuns: { workflow_runs: [
      run({ id: 125, run_number: 3, head_sha: AUTH_SHA,
        path: '.github/workflows/scorecard.yml' }),
    ] },
    testJobs: { jobs: [
      { name: 'Release candidate', status: 'completed', conclusion: 'success',
        run_id: 124, run_attempt: 2, head_sha: AUTH_SHA },
    ] },
  };
}

test('validates exact annotated tag, main push gates and publisher revision snapshot', () => {
  assert.deepEqual(validatePublicationAuthoritySnapshot(publicationEvidence()), {
    repository: 'zoster81/scripthold',
    tag: 'v3.4.0',
    commit: AUTH_SHA,
    testRunId: 124,
    scorecardRunId: 125,
  });
});

test('rejects tampered Git tag, source and mutable publisher identities', () => {
  for (const mutate of [
    (e) => { e.repository = 'other/repository'; },
    (e) => { e.tag = 'refs/tags/v3.4.0'; },
    (e) => { e.tagRef.ref = 'refs/heads/main'; },
    (e) => { e.tagRef.object.type = 'commit'; },
    (e) => { e.tagRef.object.sha = 'c'.repeat(40); },
    (e) => { e.tagObject.tag = 'v3.3.0'; },
    (e) => { e.tagObject.sha = 'c'.repeat(40); },
    (e) => { e.tagObject.object.type = 'tag'; },
    (e) => { e.tagObject.object.sha = 'c'.repeat(40); },
    (e) => { e.mainRef.ref = 'refs/heads/release'; },
    (e) => { e.mainRef.object.type = 'tag'; },
    (e) => { e.mainRef.object.sha = 'c'.repeat(40); },
    (e) => { e.publisherCommit = 'main'; },
    (e) => { e.publisherCommit = 'c'.repeat(40); },
    (e) => { e.commit = 'not-a-sha'; },
    (e) => { e.manifest.commit = 'c'.repeat(40); },
    (e) => { e.manifest.assets[0].name = '../outside'; },
    (e) => { e.manifest.repository = 'wrong/repo'; },
  ]) {
    const evidence = publicationEvidence();
    mutate(evidence);
    assert.throws(() => validatePublicationAuthoritySnapshot(evidence),
      Error, 'untrusted tag or publication code was accepted');
  }
});

test('rejects wrong CI and Scorecard run or unbound candidate job', () => {
  for (const mutate of [
    (e) => { e.testRuns.workflow_runs[0].head_sha = 'c'.repeat(40); },
    (e) => { e.testRuns.workflow_runs[0].event = 'pull_request'; },
    (e) => { e.testRuns.workflow_runs[0].head_branch = 'feature'; },
    (e) => { e.testRuns.workflow_runs[0].conclusion = 'failure'; },
    (e) => { e.testRuns.workflow_runs[0].path = '.github/workflows/codeql.yml'; },
    (e) => { e.testRuns.workflow_runs[0].run_attempt = 3; },
    (e) => { e.testRuns.workflow_runs[0].id = '124'; },
    (e) => { e.scorecardRuns.workflow_runs[0].event = 'workflow_dispatch'; },
    (e) => { e.scorecardRuns.workflow_runs[0].head_sha = 'c'.repeat(40); },
    (e) => { e.scorecardRuns.workflow_runs[0].path = '.github/workflows/test.yml'; },
    (e) => { e.testJobs.jobs[0].run_id = 999; },
    (e) => { e.testJobs.jobs[0].run_attempt = 1; },
    (e) => { e.testJobs.jobs[0].status = 'in_progress'; },
    (e) => { e.testJobs.jobs[0].head_sha = 'c'.repeat(40); },
    (e) => { e.scorecardRuns.workflow_runs[0].id = '125'; },
    (e) => { e.testRuns.workflow_runs.push({ ...e.testRuns.workflow_runs[0] }); },
    (e) => { e.testJobs.jobs[0].conclusion = 'skipped'; },
    (e) => { e.testJobs.jobs.push({ ...e.testJobs.jobs[0] }); },
    (e) => { e.testJobs.jobs = []; },
  ]) {
    const evidence = publicationEvidence();
    mutate(evidence);
    assert.throws(() => validatePublicationAuthoritySnapshot(evidence),
      Error, 'wrong-run evidence or missing gate was accepted');
  }
});

test('rejects malformed, missing and oversized authority snapshots', () => {
  for (const input of [
    null, [], {}, { ...publicationEvidence(), tagRef: null },
    { ...publicationEvidence(), tagObject: null },
    { ...publicationEvidence(), mainRef: null },
    { ...publicationEvidence(), manifest: null },
    { ...publicationEvidence(), testRuns: {} },
    { ...publicationEvidence(), scorecardRuns: { workflow_runs: new Array(1001) } },
    { ...publicationEvidence(), testJobs: { jobs: new Array(1001) } },
  ]) {
    assert.throws(() => validatePublicationAuthoritySnapshot(input), Error);
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

  const scorecardSelection = spawnSync(
    process.execPath,
    [script, 'select-scorecard-run', 'abc123', 'zoster81/scripthold'],
    {
      input: JSON.stringify({ workflow_runs: [run({ path: '.github/workflows/scorecard.yml' })] }),
      encoding: 'utf8',
    },
  );
  assert.equal(scorecardSelection.status, 0, scorecardSelection.stderr);
  assert.equal(scorecardSelection.stdout, '11');

  const verification = spawnSync(
    process.execPath,
    [script, 'verify-jobs'],
    { input: JSON.stringify({ jobs: [{ name: 'Release candidate', conclusion: 'success' }] }), encoding: 'utf8' },
  );
  assert.equal(verification.status, 0, verification.stderr);
  assert.equal(verification.stdout, '');
});
