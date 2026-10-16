'use strict';

function requireArray(payload, field) {
  if (!payload || typeof payload !== 'object' || !Array.isArray(payload[field])) {
    throw new Error(`${field} must be an array`);
  }
  return payload[field];
}

function selectExactSuccessfulWorkflowPushRun(payload, {
  sha,
  repository,
  workflowPath,
  workflowName,
}) {
  if (typeof sha !== 'string' || sha.length === 0) {
    throw new Error('sha must be a non-empty string');
  }
  if (typeof repository !== 'string' || repository.length === 0) {
    throw new Error('repository must be a non-empty string');
  }
  if (typeof workflowPath !== 'string' || workflowPath.length === 0) {
    throw new Error('workflowPath must be a non-empty string');
  }
  if (typeof workflowName !== 'string' || workflowName.length === 0) {
    throw new Error('workflowName must be a non-empty string');
  }

  const matches = requireArray(payload, 'workflow_runs').filter((run) => {
    return run &&
      run.head_sha === sha &&
      run.head_branch === 'main' &&
      run.event === 'push' &&
      run.conclusion === 'success' &&
      run.path === workflowPath &&
      run.head_repository &&
      run.head_repository.full_name === repository;
  });

  if (matches.length === 0) {
    throw new Error(`no successful exact-commit ${workflowName} push run found`);
  }
  matches.sort((a, b) => (Number(a.run_number) || 0) - (Number(b.run_number) || 0));
  const selected = matches[matches.length - 1];
  if (!selected.id) {
    throw new Error('selected workflow run has no id');
  }
  return selected.id;
}

function selectExactSuccessfulPushRun(payload, { sha, repository }) {
  return selectExactSuccessfulWorkflowPushRun(payload, {
    sha,
    repository,
    workflowPath: '.github/workflows/test.yml',
    workflowName: 'Test Suite',
  });
}

function selectExactSuccessfulScorecardRun(payload, { sha, repository }) {
  return selectExactSuccessfulWorkflowPushRun(payload, {
    sha,
    repository,
    workflowPath: '.github/workflows/scorecard.yml',
    workflowName: 'OpenSSF Scorecard',
  });
}

function verifySuccessfulReleaseCandidateJob(payload) {
  const matches = requireArray(payload, 'jobs').filter((job) => job && job.name === 'Release candidate');
  if (matches.length !== 1 || matches[0].conclusion !== 'success') {
    throw new Error('expected exactly one successful Release candidate job');
  }
}

// This is a read-only snapshot validation, not a publication permit. The caller
// must fetch these objects from authenticated repository-scoped GitHub APIs,
// serialize competing finalizers and recheck immediately before each mutation.
function validatePublicationAuthoritySnapshot(evidence) {
  const validSHA = (sha) => typeof sha === 'string' && /^[0-9a-f]{40}$/.test(sha);
  const requireAuthority = (condition, reason) => {
    if (!condition) throw new Error(reason);
  };
  requireAuthority(evidence && typeof evidence === 'object' && !Array.isArray(evidence),
    'invalid publication evidence');
  const { repository, tag, commit, publisherCommit, manifest, tagRef, tagObject, mainRef,
    testRuns, scorecardRuns, testJobs } = evidence;
  requireAuthority(repository === 'zoster81/scripthold', 'unexpected publication repository');
  requireAuthority(typeof tag === 'string' &&
    /^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$/.test(tag),
  'invalid publication tag');
  requireAuthority(validSHA(commit) && publisherCommit === commit,
    'publisher code revision does not match source commit');
  // Bind authority to the declared 22-file manifest; the payload bytes
  // are authenticated separately before publication.
  // Lazy import keeps the existing release-candidate CLI modes independent.
  const { validateStagedManifest } = require('./release-asset-contract');
  validateStagedManifest(manifest);
  requireAuthority(manifest.repository === repository && manifest.tag === tag &&
    manifest.commit === commit, 'staged assets do not match publication identity');

  requireAuthority(tagRef && tagRef.ref === 'refs/tags/' + tag &&
    tagRef.object && tagRef.object.type === 'tag' &&
    validSHA(tagRef.object.sha), 'tag must be an annotated Git tag reference');
  requireAuthority(tagObject && tagObject.sha === tagRef.object.sha &&
    tagObject.tag === tag && tagObject.object &&
    tagObject.object.type === 'commit' && tagObject.object.sha === commit,
  'annotated Git tag does not resolve to exact source commit');
  requireAuthority(mainRef && mainRef.ref === 'refs/heads/main' &&
    mainRef.object && mainRef.object.type === 'commit' &&
    mainRef.object.sha === commit, 'main ref differs from release commit');

  for (const [payload, field] of [
    [testRuns, 'workflow_runs'], [scorecardRuns, 'workflow_runs'], [testJobs, 'jobs'],
  ]) {
    requireAuthority(payload && typeof payload === 'object' &&
      Array.isArray(payload[field]) && payload[field].length <= 1000,
    'unbounded or invalid GitHub publication evidence');
  }
  const testRunId = selectExactSuccessfulPushRun(testRuns, { sha: commit, repository });
  const scorecardRunId = selectExactSuccessfulScorecardRun(scorecardRuns,
    { sha: commit, repository });
  requireAuthority(Number.isSafeInteger(testRunId) && testRunId > 0 &&
    Number.isSafeInteger(scorecardRunId) && scorecardRunId > 0,
  'invalid exact-commit workflow run IDs');
  requireAuthority(scorecardRuns.workflow_runs.filter((run) =>
    run && run.id === scorecardRunId).length === 1,
  'ambiguous Scorecard run identity');
  const selected = testRuns.workflow_runs.filter((run) => run && run.id === testRunId);
  requireAuthority(selected.length === 1 &&
    Number.isSafeInteger(selected[0].run_attempt) && selected[0].run_attempt > 0,
  'ambiguous test run attempt');
  const candidate = testJobs.jobs.filter((job) => job && job.name === 'Release candidate');
  requireAuthority(candidate.length === 1 && candidate[0].status === 'completed' &&
    candidate[0].conclusion === 'success' &&
    candidate[0].head_sha === commit && candidate[0].run_id === testRunId &&
    candidate[0].run_attempt === selected[0].run_attempt,
  'Release candidate job is not bound to successful exact run attempt');
  return { repository, tag, commit, testRunId, scorecardRunId };
}

function readStdin() {
  return new Promise((resolve, reject) => {
    let data = '';
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => { data += chunk; });
    process.stdin.on('end', () => resolve(data));
    process.stdin.on('error', reject);
  });
}

async function main(argv) {
  const mode = argv[2];
  const input = await readStdin();
  let payload;
  try {
    payload = JSON.parse(input);
  } catch (error) {
    throw new Error(`Invalid GitHub API JSON: ${error.message}`);
  }

  switch (mode) {
    case 'select-run': {
      const sha = argv[3];
      const repository = argv[4];
      const runID = selectExactSuccessfulPushRun(payload, { sha, repository });
      process.stdout.write(String(runID));
      return;
    }
    case 'select-scorecard-run': {
      const sha = argv[3];
      const repository = argv[4];
      const runID = selectExactSuccessfulScorecardRun(payload, { sha, repository });
      process.stdout.write(String(runID));
      return;
    }
    case 'verify-jobs':
      verifySuccessfulReleaseCandidateJob(payload);
      return;
    default:
      throw new Error(`Unknown mode ${JSON.stringify(mode)}`);
  }
}

if (require.main === module) {
  main(process.argv).catch((error) => {
    process.stderr.write(`${error.message}\n`);
    process.exitCode = 1;
  });
}

module.exports = {
  selectExactSuccessfulPushRun,
  selectExactSuccessfulScorecardRun,
  verifySuccessfulReleaseCandidateJob,
  validatePublicationAuthoritySnapshot,
};
