// Run-local read-only authentication of the same 22 original assets.
// Used independently by verifier and protected finalizer; no writer calls.
'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { bindRawUploadJobOutputs, readJobVerifiedRawCompleteStaging } =
  require('./release-staging-rest');
const { verifyRawCompleteHandoffPayloads } =
  require('./release-staging-producers');
const { readPublicationAuthority } =
  require('./release-finalizer-authority');
const { decodeGoReleaserNotes } =
  require('./release-normal-source');
const { verifyDraftSourceInventory } =
  require('./release-finalizer-source-inventory');
const { requireHostedTaggedVerifierContext,
  requireHostedTaggedWriterContext } =
  require('./release-finalizer-preflight');

async function main() {
  const producer = prefix => bindRawUploadJobOutputs({
    manifest: JSON.parse(process.env['SOURCE_' + prefix + '_MANIFEST']),
    repositoryId: Number(process.env.GITHUB_REPOSITORY_ID),
    runId: Number(process.env.GITHUB_RUN_ID),
    producerAttempt: Number(process.env['SOURCE_' + prefix + '_ATTEMPT']),
    jobOutputs: {
      ids: process.env['SOURCE_' + prefix + '_IDS'],
      digests: process.env['SOURCE_' + prefix + '_DIGESTS'],
      attempt: process.env['SOURCE_' + prefix + '_ATTEMPT'],
    },
  });
  const normal = producer('NORMAL');
  const mcpb = producer('MCPB');
  const trusted = await readJobVerifiedRawCompleteStaging({
    normal, mcpb,
    sourcePolicy: {
      workflowId: Number(process.env.SOURCE_WORKFLOW_ID),
      workflowPath: '.github/workflows/release.yml',
      event: 'push',
      normalJobName: 'release',
      mcpbJobName: 'publish-mcpb / publish',
    },
    token: process.env.GH_TOKEN,
  });
  const releaseNotes = decodeGoReleaserNotes(
    process.env.SOURCE_RELEASE_NOTES_METADATA);
  if (releaseNotes.length < 2) {
    throw new Error('unavailable exact original GoReleaser release notes');
  }
  if (trusted.manifest.commit !== process.env.GITHUB_SHA ||
      trusted.manifest.tag !== process.env.GITHUB_REF_NAME ||
      trusted.manifest.repository !== process.env.GITHUB_REPOSITORY) {
    throw new Error('finalizer manifest disagrees with trusted tag checkout');
  }
  // Both jobs validate their fixed GitHub identity independently.
  // This script is a reader, never a publication capability.
  const contextCheck = process.env.GITHUB_JOB === 'verify-complete'
    ? requireHostedTaggedVerifierContext
    : requireHostedTaggedWriterContext;
  contextCheck(process.env, trusted.manifest, process.env.GITHUB_SHA);
  const root = path.resolve(process.env.RUNNER_TEMP, 'complete-raw');
  const normalDir = path.resolve(process.env.RUNNER_TEMP, 'complete-normal');
  const mcpbDir = path.resolve(process.env.RUNNER_TEMP, 'complete-mcpb');
  for (const dir of [root, normalDir, mcpbDir]) {
    const info = fs.lstatSync(dir);
    if (!info.isDirectory() || info.isSymbolicLink() ||
        info.uid !== process.getuid() || (info.mode & 0o077) !== 0) {
      throw new Error('untrusted complete staging directory');
    }
  }
  const names = fs.readdirSync(root);
  if (names.length !== 22) {
    throw new Error('complete original download must have exactly 22 entries');
  }
  const observed = names.map(name => {
    const stat = fs.lstatSync(path.join(root, name));
    if (stat.uid !== process.getuid()) {
      throw new Error('untrusted downloaded original owner');
    }
    return { name, size: stat.size, nlink: stat.nlink,
      type: stat.isFile() && !stat.isSymbolicLink() ? 'file' : 'unsafe' };
  });
  await verifyRawCompleteHandoffPayloads(
    trusted.normal, trusted.mcpb, observed,
    ({ artifactName }) => fs.createReadStream(path.join(root, artifactName), {
      flags: fs.constants.O_RDONLY | (fs.constants.O_NOFOLLOW || 0),
    }));
  for (const file of trusted.batch.files) {
    const target = file.fileName.endsWith('.mcpb') ||
      file.fileName === 'mcpb-checksums.txt' ? mcpbDir : normalDir;
    fs.renameSync(path.join(root, file.artifactName),
      path.join(target, file.fileName));
  }
  if (fs.readdirSync(root).length !== 0 ||
      fs.readdirSync(normalDir).length !== 15 ||
      fs.readdirSync(mcpbDir).length !== 7) {
    throw new Error('complete original stage lost or gained an asset');
  }
  // Check the exact future draft-writer input contract without granting
  // a writer token or invoking any GitHub release mutation.
  verifyDraftSourceInventory(trusted.manifest, normalDir, mcpbDir);
  const authority = await readPublicationAuthority({
    manifest: trusted.manifest,
    publisherCommit: process.env.GITHUB_SHA,
    token: process.env.GH_TOKEN,
  });
  if (authority.commit !== trusted.manifest.commit) {
    throw new Error('release authority does not match twenty-two originals');
  }
  // The successful MCPB dependency verified all 12 original signatures.
  // This finalizer verifies the same originals by run-bound SHA-256.
  console.log('Verified 22 original assets, producer jobs and release authority; not publishing');
}
main().catch(error => {
  console.error(error.message);
  process.exitCode = 1;
});
