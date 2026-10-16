'use strict';

const { resolveFinalizerActionContext } = require('./release-finalizer-entrypoint');
const { makeGitHubGet, readJsonResponse } = require('./release-finalizer-read');

const API = 'https://api.github.com/repos/zoster81/scripthold';

function requireCondition(value, message) {
  if (!value) throw new Error(message);
}

function validSha(sha) {
  return typeof sha === 'string' && /^[0-9a-f]{40}$/.test(sha);
}

// Source-only preflight: it can run before any 22-file manifest exists.
// Re-read the annotated tag after main to detect replacement during these
// separate authenticated GET requests; this is not an atomic GitHub snapshot
// or a permission to create or publish a release.
async function inspectFinalizerSource({ env, fetchImpl = globalThis.fetch }) {
  const selected = resolveFinalizerActionContext(env);
  requireCondition(validSha(env.GITHUB_SHA), 'invalid GitHub checkout SHA');
  const get = makeGitHubGet(env.GITHUB_TOKEN, fetchImpl);
  const commit = env.GITHUB_SHA;

  const refUrl = API + '/git/ref/tags/' + selected.tag;
  const tagRef = await readJsonResponse(await get(refUrl), refUrl);
  requireCondition(tagRef && tagRef.ref === 'refs/tags/' + selected.tag &&
    tagRef.object && tagRef.object.type === 'tag' &&
    validSha(tagRef.object.sha), 'source ref is not an annotated release tag');
  const tagObjectSha = tagRef.object.sha;

  const objectUrl = API + '/git/tags/' + tagObjectSha;
  const object = await readJsonResponse(await get(objectUrl), objectUrl);
  requireCondition(object && object.sha === tagObjectSha &&
    object.tag === selected.tag && object.object &&
    object.object.type === 'commit' && object.object.sha === commit,
  'annotated tag does not resolve to the checkout commit');

  const mainUrl = API + '/git/ref/heads/main';
  const main = await readJsonResponse(await get(mainUrl), mainUrl);
  requireCondition(main && main.ref === 'refs/heads/main' && main.object &&
    main.object.type === 'commit' && main.object.sha === commit,
  'checkout and current main do not identify the same commit');

  const refreshed = await readJsonResponse(await get(refUrl), refUrl);
  requireCondition(refreshed && refreshed.ref === tagRef.ref &&
    refreshed.object && refreshed.object.type === 'tag' &&
    refreshed.object.sha === tagObjectSha,
  'GitHub annotated tag changed during source verification');

  return { ...selected, commit, tagObjectSha };
}

if (require.main === module) {
  inspectFinalizerSource({ env: process.env })
    .then((source) => {
      process.stdout.write('Verified read-only source for ' + source.tag + '\n');
    })
    .catch(() => {
      process.stderr.write('Refusing unverified GitHub release source.\n');
      process.exitCode = 1;
    });
}

module.exports = { inspectFinalizerSource, resolveSourceActionContext:
  resolveFinalizerActionContext };
