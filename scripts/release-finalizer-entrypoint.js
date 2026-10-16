'use strict';

const { resolveFinalizerEvent } = require('./release-finalizer-preflight');

function requireCondition(condition, message) {
  if (!condition) throw new Error(message);
}

// Validate only the small subset of GitHub Actions context that selects the
// native workflow concurrency group. This script never reads credentials,
// contacts GitHub, creates drafts, or modifies a release.
function resolveFinalizerActionContext(env) {
  requireCondition(env && typeof env === 'object' && !Array.isArray(env),
    'invalid GitHub Actions environment');
  requireCondition(typeof env.RELEASE_TAG_INPUT === 'string',
    'missing explicit release input context');
  requireCondition(env.GITHUB_EVENT_NAME !== 'push' ||
    env.RELEASE_TAG_INPUT === '', 'tag push cannot override its release tag');
  return resolveFinalizerEvent({
    eventName: env.GITHUB_EVENT_NAME,
    ref: env.GITHUB_REF,
    repository: env.GITHUB_REPOSITORY,
    inputs: env.GITHUB_EVENT_NAME === 'workflow_dispatch' ?
      { tag: env.RELEASE_TAG_INPUT } : undefined,
  });
}

if (require.main === module) {
  try {
    const context = resolveFinalizerActionContext(process.env);
    process.stdout.write('Validated read-only finalizer group: ' + context.group + '\n');
  } catch {
    process.stderr.write('Refusing untrusted release preflight event.\n');
    process.exitCode = 1;
  }
}

module.exports = { resolveFinalizerActionContext };
