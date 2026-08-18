'use strict';

function requireArray(payload, field) {
  if (!payload || typeof payload !== 'object' || !Array.isArray(payload[field])) {
    throw new Error(`${field} must be an array`);
  }
  return payload[field];
}

function selectExactSuccessfulPushRun(payload, { sha, repository }) {
  if (typeof sha !== 'string' || sha.length === 0) {
    throw new Error('sha must be a non-empty string');
  }
  if (typeof repository !== 'string' || repository.length === 0) {
    throw new Error('repository must be a non-empty string');
  }

  const matches = requireArray(payload, 'workflow_runs').filter((run) => {
    return run &&
      run.head_sha === sha &&
      run.head_branch === 'main' &&
      run.event === 'push' &&
      run.conclusion === 'success' &&
      run.path === '.github/workflows/test.yml' &&
      run.head_repository &&
      run.head_repository.full_name === repository;
  });

  if (matches.length === 0) {
    throw new Error('no successful exact-commit Test Suite push run found');
  }
  matches.sort((a, b) => (Number(a.run_number) || 0) - (Number(b.run_number) || 0));
  const selected = matches[matches.length - 1];
  if (!selected.id) {
    throw new Error('selected workflow run has no id');
  }
  return selected.id;
}

function verifySuccessfulReleaseCandidateJob(payload) {
  const matches = requireArray(payload, 'jobs').filter((job) => job && job.name === 'Release candidate');
  if (matches.length !== 1 || matches[0].conclusion !== 'success') {
    throw new Error('expected exactly one successful Release candidate job');
  }
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
  verifySuccessfulReleaseCandidateJob,
};
