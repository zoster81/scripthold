#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyCanonicalNormalChecksums } = require('./release-normal-rehearsal');

const MAX_ASSET_BYTES = 2 * 1024 * 1024 * 1024;
const MAX_CHECKSUM_BYTES = 16 * 1024;
const SHA256 = /^[0-9a-f]{64}$/;
const ID = /^[1-9][0-9]*$/;

function canonicalId(value) {
  return typeof value === 'string' && ID.test(value) &&
    Number.isSafeInteger(Number(value)) && String(Number(value)) === value;
}

// Validates the actual raw-download bytes of 13 newly built GoReleaser originals.
// This is NOT Sigstore evidence, a complete 15-file normal stage, GitHub job
// authentication, or publication authority. The caller must bind outputs through
// needs.producer and independently authenticate the hosted source job.
async function verifyGenuineNormalRawTransfer({ tag, runId, attempt, ids, digests, dir }) {
  const names = expectedReleaseAssetNames(tag).slice(0, 13);
  if (names.length !== 13 || !canonicalId(runId) || !canonicalId(attempt) ||
      !Array.isArray(ids) || ids.length !== 13 ||
      ids.some(id => !canonicalId(id)) || new Set(ids).size !== 13 ||
      !Array.isArray(digests) || digests.length !== 13 ||
      digests.some(digest => typeof digest !== 'string' || !SHA256.test(digest))) {
    throw new Error('invalid 13-original producer outputs');
  }
  if (typeof dir !== 'string' || !path.isAbsolute(dir)) {
    throw new Error('raw original stage requires absolute path');
  }
  const root = path.resolve(dir);
  const meta = fs.lstatSync(root);
  if (!meta.isDirectory() || meta.isSymbolicLink() ||
      (process.platform === 'linux' &&
       (meta.uid !== process.getuid() || (meta.mode & 0o077) !== 0))) {
    throw new Error('raw original stage must be a private real directory');
  }
  const expected = names.map((name, i) => ({
    name, original: 'genuine-' + runId + '-' + attempt + '-normal-' +
      String(i + 1).padStart(2, '0'), digest: digests[i],
  }));
  const observed = fs.readdirSync(root).sort();
  if (observed.length !== expected.length ||
      observed.some((name, i) => name !== expected.map(x => x.original).sort()[i])) {
    throw new Error('raw original stage inventory mismatch');
  }
  const measured = new Map();
  let checksumBytes;
  for (const item of expected) {
    const filePath = path.join(root, item.original);
    const before = fs.lstatSync(filePath);
    if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 ||
        before.size <= 0 || before.size >
          (item.name === 'checksums.txt' ? MAX_CHECKSUM_BYTES : MAX_ASSET_BYTES) ||
        (process.platform === 'linux' && before.uid !== process.getuid())) {
      throw new Error('raw original must be an owner-controlled regular file');
    }
    const fh = await fs.promises.open(filePath,
      fs.constants.O_RDONLY | (fs.constants.O_NOFOLLOW || 0));
    try {
      const pinned = await fh.stat();
      if (!pinned.isFile() || pinned.nlink !== 1 ||
          pinned.dev !== before.dev || pinned.ino !== before.ino ||
          pinned.size !== before.size) {
        throw new Error('raw original changed before read');
      }
      const hash = createHash('sha256');
      let size = 0;
      const chunks = [];
      for await (const chunk of fh.createReadStream({ autoClose: false })) {
        size += chunk.length;
        if (size > pinned.size) throw new Error('raw original exceeds declared size');
        hash.update(chunk);
        if (item.name === 'checksums.txt') chunks.push(Buffer.from(chunk));
      }
      const after = await fh.stat();
      const current = fs.lstatSync(filePath);
      if (size !== pinned.size || after.size !== pinned.size ||
          after.mtimeMs !== pinned.mtimeMs || after.dev !== pinned.dev ||
          after.ino !== pinned.ino || current.dev !== pinned.dev ||
          current.ino !== pinned.ino || !current.isFile() || current.nlink !== 1) {
        throw new Error('raw original changed during read');
      }
      const digest = hash.digest('hex');
      if (digest !== item.digest) throw new Error('raw original digest mismatch');
      measured.set(item.name, digest);
      if (item.name === 'checksums.txt') checksumBytes = Buffer.concat(chunks);
    } finally {
      await fh.close();
    }
  }
  const checksums = verifyCanonicalNormalChecksums(tag, checksumBytes);
  for (const [name, digest] of measured) {
    if (name !== 'checksums.txt' && checksums.get(name) !== digest) {
      throw new Error('raw original checksum closure mismatch');
    }
  }
  return { verified: 13, subjects: 12 };
}

if (require.main === module) {
  const env = process.env;
  (async () => {
    if (env.PRODUCER_RESULT !== 'success' ||
        env.GITHUB_RUN_ID !== env.PROBE_RUN_ID ||
        !canonicalId(env.GITHUB_RUN_ATTEMPT) ||
        !canonicalId(env.PROBE_SOURCE_ATTEMPT) ||
        Number(env.PROBE_SOURCE_ATTEMPT) > Number(env.GITHUB_RUN_ATTEMPT)) {
      throw new Error('untrusted producer workflow attempt');
    }
    const result = await verifyGenuineNormalRawTransfer({
      tag: env.PROBE_TAG, runId: env.PROBE_RUN_ID,
      attempt: env.PROBE_SOURCE_ATTEMPT,
      ids: env.PROBE_IDS?.split(','), digests: env.PROBE_DIGESTS?.split(','),
      dir: env.PROBE_DIR,
    });
    console.log('Verified genuine nonpublishing GoReleaser raw transfers: ' +
      result.verified + '/13; checksum subjects: ' + result.subjects);
  })().catch(error => {
    console.error('genuine normal transfer proof: ' + error.message);
    process.exitCode = 1;
  });
}

module.exports = { verifyGenuineNormalRawTransfer };
