'use strict';

const { makeGitHubGet, readJsonResponse } = require('./release-finalizer-read');
const { validateStagedManifest } = require('./release-asset-contract');
const {
  selectExactSuccessfulPushRun,
  validatePublicationAuthoritySnapshot,
} = require('./release-candidate-provenance');

const API = 'https://api.github.com/repos/zoster81/scripthold';
const MAX_ITEMS = 100;

function ensure(condition, reason) {
  if (!condition) throw new Error(reason);
}

async function getCompleteList(get, url, field) {
  const response = await get(url);
  const data = await readJsonResponse(response, url);
  ensure(response.headers.get('link') === null,
    'incomplete GitHub publication authority pagination');
  ensure(data && typeof data === 'object' && !Array.isArray(data) &&
    Number.isSafeInteger(data.total_count) &&
    data.total_count >= 0 && data.total_count <= MAX_ITEMS &&
    Array.isArray(data[field]) && data[field].length === data.total_count,
  'incomplete or oversized GitHub publication authority list');
  return { [field]: data[field] };
}

// Read-only evidence from the canonical repository. This verifies one
// observation, NOT permission to mutate a release. The caller must check out
// the same source revision, serialize by tag and fetch again before writes.
async function readPublicationAuthority({
  manifest, publisherCommit, token, fetchImpl = globalThis.fetch,
}) {
  validateStagedManifest(manifest);
  ensure(typeof publisherCommit === 'string' &&
    /^[0-9a-f]{40}$/.test(publisherCommit) &&
    publisherCommit === manifest.commit, 'publisher checkout differs from release source');
  const get = makeGitHubGet(token, fetchImpl);
  const tag = manifest.tag;
  const commit = manifest.commit;

  const tagRefUrl = API + '/git/ref/tags/' + tag;
  const tagRef = await readJsonResponse(await get(tagRefUrl), tagRefUrl);
  ensure(tagRef && tagRef.ref === 'refs/tags/' + tag && tagRef.object &&
    tagRef.object.type === 'tag' &&
    typeof tagRef.object.sha === 'string' &&
    /^[0-9a-f]{40}$/.test(tagRef.object.sha),
  'source reference is not an exact annotated Git tag');
  const tagUrl = API + '/git/tags/' + tagRef.object.sha;
  const tagObject = await readJsonResponse(await get(tagUrl), tagUrl);

  const mainUrl = API + '/git/ref/heads/main';
  const mainRef = await readJsonResponse(await get(mainUrl), mainUrl);

  const suffix = '?event=push&branch=main&head_sha=' +
    commit + '&per_page=100&page=1';
  const testUrl = API + '/actions/workflows/test.yml/runs' + suffix;
  const scorecardUrl = API + '/actions/workflows/scorecard.yml/runs' + suffix;
  const testRuns = await getCompleteList(get, testUrl, 'workflow_runs');
  const scorecardRuns = await getCompleteList(get, scorecardUrl, 'workflow_runs');
  const testRunId = selectExactSuccessfulPushRun(testRuns, {
    sha: commit, repository: manifest.repository,
  });
  const testRun = testRuns.workflow_runs.find((run) => run && run.id === testRunId);
  ensure(testRun && Number.isSafeInteger(testRun.run_attempt) &&
    testRun.run_attempt > 0, 'invalid selected workflow run attempt');
  const jobsUrl = API + '/actions/runs/' + testRunId +
    '/attempts/' + testRun.run_attempt + '/jobs?per_page=100&page=1';
  const testJobs = await getCompleteList(get, jobsUrl, 'jobs');

  return validatePublicationAuthoritySnapshot({
    repository: manifest.repository, tag, commit, publisherCommit, manifest,
    tagRef, tagObject, mainRef, testRuns, scorecardRuns, testJobs,
  });
}

module.exports = { readPublicationAuthority };
