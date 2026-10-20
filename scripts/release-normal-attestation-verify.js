'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { validateStagedGroupManifest, expectedReleaseAssetNames } =
  require('./release-asset-contract');
const { checkNormalProvenancePair } = require('./release-normal-provenance-pair');
const { verifyWithGitHubCLI } = require('./release-provenance');

const MAX_CHECKSUM_BYTES = 16 * 1024;
const MAX_SIDECAR_BYTES = 4 * 1024 * 1024;

function normalNames(tag) {
  return expectedReleaseAssetNames(tag).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
}

function trustedStageDir(stageDir) {
  if (typeof stageDir !== 'string' || !path.isAbsolute(stageDir)) {
    throw new Error('normal attestation stage requires an absolute directory');
  }
  const dir = path.resolve(stageDir);
  const meta = fs.lstatSync(dir);
  if (!meta.isDirectory() || meta.isSymbolicLink() ||
      (process.platform === 'linux' &&
       (meta.uid !== process.getuid() || (meta.mode & 0o077) !== 0))) {
    throw new Error('normal attestation stage must be a private real directory');
  }
  return dir;
}

async function measuredFile(dir, name, expected) {
  const full = path.join(dir, name);
  const before = fs.lstatSync(full);
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 ||
      (process.platform === 'linux' && before.uid !== process.getuid())) {
    throw new Error('normal original must be an owner-controlled regular file');
  }
  // The fd, not a second pathname lookup, supplies the measured bytes.
  const nofollow = fs.constants.O_NOFOLLOW || 0;
  const file = await fs.promises.open(full, fs.constants.O_RDONLY | nofollow);
  try {
    const pinned = await file.stat();
    if (!pinned.isFile() || pinned.nlink !== 1 ||
        pinned.dev !== before.dev || pinned.ino !== before.ino ||
        pinned.size !== expected.size) {
      throw new Error('original file changed before measurement');
    }
    const hash = createHash('sha256');
    let length = 0;
    for await (const chunk of file.createReadStream({ autoClose: false })) {
      length += chunk.byteLength;
      if (length > expected.size) throw new Error('original exceeds expected size');
      hash.update(chunk);
    }
    if (length !== expected.size || hash.digest('hex') !== expected.sha256) {
      throw new Error('original file digest or size mismatch');
    }
    const after = await file.stat();
    const current = fs.lstatSync(full);
    if (after.dev !== pinned.dev || after.ino !== pinned.ino ||
        after.size !== pinned.size || after.mtimeMs !== pinned.mtimeMs ||
        current.dev !== pinned.dev || current.ino !== pinned.ino ||
        !current.isFile() || current.nlink !== 1) {
      throw new Error('original changed during measurement');
    }
  } finally {
    await file.close();
  }
}

// Inspect the exact original-name 15-file private download stage *after*
// unarchived cross-job transfer. This detects structural and byte mismatches,
// NOT valid signatures or a trustworthy GitHub producer job.
async function inspectNormalStage(manifest, stageDir) {
  if (manifest?.stage !== 'normal') {
    throw new Error('expected normal original stage');
  }
  const expected = validateStagedGroupManifest(manifest);
  const tag = manifest.tag;
  const commit = manifest.commit;
  const names = normalNames(tag);
  const dir = trustedStageDir(stageDir);
  // Read at most one name beyond the exact original inventory. Reading the
  // entire directory would make this security preflight unbounded in memory.
  const observed = [];
  const listing = fs.opendirSync(dir, { bufferSize: names.length + 1 });
  try {
    for (let i = 0; i <= names.length; i++) {
      const entry = listing.readSync();
      if (entry === null) break;
      observed.push(entry.name);
    }
  } finally {
    listing.closeSync();
  }
  observed.sort();
  const required = [...names].sort();
  if (observed.length !== 15 ||
      observed.some((name, i) => name !== required[i])) {
    throw new Error('normal attestation stage requires exactly 15 original names');
  }
  for (const name of names) await measuredFile(dir, name, expected.get(name));

  const small = (name, maximum) => {
    if (expected.get(name).size > maximum) {
      throw new Error('provenance or checksum source is oversized');
    }
    const full = path.join(dir, name);
    const nofollow = fs.constants.O_NOFOLLOW || 0;
    const fd = fs.openSync(full, fs.constants.O_RDONLY | nofollow);
    try {
      const stat = fs.fstatSync(fd);
      if (!stat.isFile() || stat.nlink !== 1 ||
          stat.size !== expected.get(name).size) {
        throw new Error('original sidecar changed after measurement');
      }
      // Read at most the declared bounded size plus one byte so a writer
      // extending the file cannot force an unbounded allocation.
      const buffer = Buffer.allocUnsafe(stat.size + 1);
      let length = 0;
      while (length < buffer.length) {
        const count = fs.readSync(fd, buffer, length, buffer.length - length, length);
        if (count === 0) break;
        length += count;
      }
      const after = fs.fstatSync(fd);
      const current = fs.lstatSync(full);
      if (!current.isFile() || current.nlink !== 1 ||
          current.dev !== stat.dev || current.ino !== stat.ino ||
          after.dev !== stat.dev || after.ino !== stat.ino ||
          after.size !== stat.size || after.mtimeMs !== stat.mtimeMs ||
          length !== stat.size ||
          createHash('sha256').update(buffer.subarray(0, length)).digest('hex') !==
            expected.get(name).sha256) {
        throw new Error('original sidecar changed after measurement');
      }
      return buffer.subarray(0, length);
    } finally {
      fs.closeSync(fd);
    }
  };
  const checksumBytes = small('checksums.txt', MAX_CHECKSUM_BYTES);
  const bundleName = 'scripthold-' + tag + '-provenance.sigstore.json';
  const intotoName = 'scripthold-' + tag + '-provenance.intoto.jsonl';
  const bundleBytes = small(bundleName, MAX_SIDECAR_BYTES);
  const envelopeBytes = small(intotoName, MAX_SIDECAR_BYTES);
  const detached = {
    repository: 'zoster81/scripthold', tag, commit, stage: 'normal',
    assets: names.map(name => ({ name, ...expected.get(name) })),
  };
  checkNormalProvenancePair(detached, checksumBytes, bundleBytes, envelopeBytes);
  return {
    subjects: names.slice(0, 12),
    bundlePath: path.join(dir, bundleName),
    size: names.length,
  };
}

// Only the genuine GitHub CLI Sigstore verifier is allowed here, with the
// existing release identity, tag, source digest and hosted-runner restriction.
// No caller-supplied verification callback can bypass signature checks.
// This does not prove which exact GitHub job uploaded the files, and a
// concurrent same-owner writer can still race with path-based gh verification.
async function verifyStagedNormalAttestations(manifest, stageDir) {
  if (process.platform !== 'linux') {
    throw new Error('genuine normal staged attestations require native Linux');
  }
  const original = validateStagedGroupManifest(manifest);
  const names = normalNames(manifest.tag);
  const pinned = {
    repository: 'zoster81/scripthold', tag: manifest.tag, commit: manifest.commit,
    stage: 'normal', assets: names.map(name => ({ name, ...original.get(name) })),
  };
  const before = await inspectNormalStage(pinned, stageDir);
  for (const name of before.subjects) {
    verifyWithGitHubCLI(path.join(path.dirname(before.bundlePath), name),
      before.bundlePath, pinned.repository, pinned.tag, pinned.commit);
  }
  const after = await inspectNormalStage(pinned, stageDir);
  if (before.bundlePath !== after.bundlePath ||
      before.subjects.join('\n') !== after.subjects.join('\n')) {
    throw new Error('normal attestation subjects changed during verification');
  }
  return { subjectsVerified: 12 };
}

module.exports = { inspectNormalStage, verifyStagedNormalAttestations };
