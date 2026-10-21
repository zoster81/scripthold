'use strict';

const { expectedReleaseAssetNames, validateStagedManifest } =
  require('./release-asset-contract');
const { readPublicationAuthority } = require('./release-finalizer-authority');
const { discoverReleaseByTag } = require('./release-finalizer-read');

const REPOSITORY = 'zoster81/scripthold';
const GROUP_PREFIX = 'scripthold-immutable-release-';

function requireCondition(condition, message) {
  if (!condition) throw new Error(message);
}

// Both trigger types must resolve to the exact same GitHub Actions group.
// This helper is NOT a lock: only the future workflow-level concurrency
// configuration can serialize independent GitHub Actions runs.
function resolveFinalizerEvent(event) {
  requireCondition(event && typeof event === 'object' &&
    !Array.isArray(event) && event.repository === REPOSITORY,
  'unexpected finalizer event repository');
  let tag;
  if (event.eventName === 'push') {
    requireCondition(typeof event.ref === 'string' &&
      event.ref.startsWith('refs/tags/'),
    'finalizer push must target a tag');
    tag = event.ref.slice('refs/tags/'.length);
  } else if (event.eventName === 'workflow_dispatch') {
    requireCondition(event.ref === 'refs/heads/main' &&
      event.inputs && typeof event.inputs === 'object' &&
      !Array.isArray(event.inputs), 'manual finalizer must run from main');
    tag = event.inputs.tag;
  } else {
    throw new Error('unsupported finalizer event type');
  }
  requireCondition(typeof tag === 'string' && tag.length <= 96,
    'invalid finalizer tag length');
  expectedReleaseAssetNames(tag);
  return { repository: REPOSITORY, tag, group: GROUP_PREFIX + tag };
}

// Shared defense in depth for both the current read-only tagged job and the
// future protected writer. These supplied fields cannot prove environment
// approval or authenticate GitHub's runner; the workflow enforces those gates.
function requireTaggedWriterContext(context, manifest, publisherCommit) {
  const keys = ['eventName', 'repository', 'ref', 'workflowRef',
    'sha', 'job', 'runId', 'runAttempt'];
  const validShape = context && typeof context === 'object' &&
    !Array.isArray(context) && Object.keys(context).length === keys.length &&
    keys.every(key => Object.hasOwn(context, key));
  const valid = validShape && manifest && publisherCommit === manifest.commit &&
    context.eventName === 'push' &&
    context.repository === REPOSITORY &&
    context.ref === 'refs/tags/' + manifest.tag &&
    context.workflowRef ===
      REPOSITORY + '/.github/workflows/release.yml@refs/tags/' + manifest.tag &&
    context.sha === manifest.commit && context.job === 'finalize' &&
    typeof context.runId === 'string' &&
    /^[1-9][0-9]*$/.test(context.runId) &&
    Number.isSafeInteger(Number(context.runId)) &&
    typeof context.runAttempt === 'string' &&
    /^[1-9][0-9]*$/.test(context.runAttempt) &&
    Number.isSafeInteger(Number(context.runAttempt));
  requireCondition(valid, 'invalid tagged release writer context');
}

function requireHostedTaggedFinalizerContext(env, manifest, publisherCommit) {
  requireCondition(env && typeof env === 'object' && !Array.isArray(env),
    'invalid hosted tagged release context');
  requireTaggedWriterContext({
    eventName: env.GITHUB_EVENT_NAME,
    repository: env.GITHUB_REPOSITORY,
    ref: env.GITHUB_REF,
    workflowRef: env.GITHUB_WORKFLOW_REF,
    sha: env.GITHUB_SHA,
    job: env.GITHUB_JOB,
    runId: env.GITHUB_RUN_ID,
    runAttempt: env.GITHUB_RUN_ATTEMPT,
  }, manifest, publisherCommit);
}

// Read-only cross-check before a finalizer may consider a next-step proposal.
// It deliberately produces NO mutation permit. The actual writer must enforce
// GitHub Actions single-writer concurrency, then run a fresh check again
// immediately before each remote mutation.
async function inspectFinalizerPreflight({
  event, manifest, publisherCommit, token, fetchImpl = globalThis.fetch,
}) {
  const selected = resolveFinalizerEvent(event);
  validateStagedManifest(manifest);
  requireCondition(manifest.repository === selected.repository &&
    manifest.tag === selected.tag, 'event tag disagrees with staging manifest');
  const inputs = { manifest, publisherCommit, token, fetchImpl };
  const before = await readPublicationAuthority(inputs);
  const discovery = await discoverReleaseByTag({
    tag: selected.tag, token, fetchImpl,
  });
  requireCondition(discovery.kind === 'candidate',
    'GitHub release absence is unproven; stop without creating a release');
  const after = await readPublicationAuthority(inputs);
  requireCondition(
    Object.keys(before).length === Object.keys(after).length &&
    Object.keys(before).every((key) => before[key] === after[key]),
  'GitHub publication authority changed during release discovery');
  return { ...selected, authority: after, discovery };
}

module.exports = { resolveFinalizerEvent, inspectFinalizerPreflight,
  requireTaggedWriterContext, requireHostedTaggedFinalizerContext };
