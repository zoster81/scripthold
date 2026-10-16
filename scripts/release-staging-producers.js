'use strict';

const {
  composeStagedGroupManifests,
  validateStagedGroupManifest,
  planRawStagingBatch,
  planRawNormalStagingBatch,
  verifyRawNormalStagingBatchPayloads,
} = require('./release-asset-contract');

const PRODUCER_KEYS = ['manifest', 'repositoryId', 'runId', 'producerAttempt', 'uploads'];
const MANIFEST_KEYS = ['repository', 'tag', 'commit', 'stage', 'assets'];
const ASSET_KEYS = ['name', 'size', 'sha256'];
const UPLOAD_KEYS = ['fileName', 'uploadId', 'uploadDigest', 'remote'];

function exactRecord(value, keys, label) {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      Object.keys(value).length !== keys.length ||
      !keys.every((key) => Object.hasOwn(value, key))) {
    throw new Error('unexpected ' + label + ' schema');
  }
}

// Bind independent producer job outputs to authenticated GitHub REST artifact
// observations without trusting an upload name, local path, or latest run lookup.
// Caller must fetch observations from fixed GitHub endpoints and independently
// prove the chosen workflow run/job/attempt. This pure gate cannot do either.
function stagingEntries(producer, stage) {
  exactRecord(producer, PRODUCER_KEYS, 'producer');
  exactRecord(producer.manifest, MANIFEST_KEYS, 'producer manifest');
  if (producer.manifest.stage !== stage) {
    throw new Error('staging producer has the wrong group');
  }
  const files = validateStagedGroupManifest(producer.manifest);
  for (const asset of producer.manifest.assets) {
    exactRecord(asset, ASSET_KEYS, 'producer asset');
  }
  if (!Array.isArray(producer.uploads) || producer.uploads.length !== files.size) {
    throw new Error('incomplete producer upload inventory');
  }
  return producer.uploads.map((upload) => {
    exactRecord(upload, UPLOAD_KEYS, 'producer upload');
    return {
      expected: {
        repository: producer.manifest.repository,
        tag: producer.manifest.tag,
        commit: producer.manifest.commit,
        repositoryId: producer.repositoryId,
        runId: producer.runId,
        producerAttempt: producer.producerAttempt,
        stage,
        fileName: upload.fileName,
        uploadId: upload.uploadId,
        uploadDigest: upload.uploadDigest,
      },
      remote: upload.remote,
    };
  });
}

// Produces only an immutable-name/ID download plan and canonical 22-file
// manifest. No release draft or asset mutation is permitted by this result;
// verify all actual downloaded bytes after the action, then refresh authority
// immediately before any future GitHub write.
function assembleRawReleaseStaging(normal, mcpb) {
  const manifest = composeStagedGroupManifests(normal?.manifest, mcpb?.manifest);
  const entries = [
    ...stagingEntries(normal, 'normal'),
    ...stagingEntries(mcpb, 'mcpb'),
  ];
  return { manifest, batch: planRawStagingBatch(manifest, entries) };
}

// Only the verified 15-file normal inventory is handed to the GitHub-only
// MCPB producer. Keep this distinct from the complete publication manifest.
function assembleRawNormalHandoff(normal) {
  const entries = stagingEntries(normal, 'normal');
  const batch = planRawNormalStagingBatch(normal.manifest, entries);
  return {
    manifest: {
      repository: normal.manifest.repository,
      tag: normal.manifest.tag,
      commit: normal.manifest.commit,
      stage: 'normal',
      assets: batch.files.map(({ fileName, size, sha256 }) => ({
        name: fileName, size, sha256,
      })),
    },
    batch,
  };
}

// Caller must open stable, no-follow immutable sources from an isolated private
// destination. This gate checks every physical filename and original byte stream
// before any received normal asset can be used by the future MCPB producer.
async function verifyRawNormalHandoffPayloads(normal, observed, openSource) {
  const entries = stagingEntries(normal, 'normal');
  return verifyRawNormalStagingBatchPayloads(normal.manifest, entries, observed, openSource);
}

module.exports = { assembleRawReleaseStaging, assembleRawNormalHandoff,
  verifyRawNormalHandoffPayloads };
