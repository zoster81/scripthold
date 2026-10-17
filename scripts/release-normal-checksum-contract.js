'use strict';

const { createHash } = require('node:crypto');
const {
  expectedReleaseAssetNames,
  validateStagedGroupManifest,
} = require('./release-asset-contract');
const { parseChecksums } = require('./release-provenance');

// Future-only, source-neutral checksum closure. This is NOT provenance
// authentication: the caller must independently authenticate the normal
// producer, exact GitHub run and artifact IDs, the checksum blob and every
// original byte. It never opens files or authorizes publication.
function verifyNormalChecksumClosure(normal, checksumBytes) {
  if (!normal || normal.stage !== 'normal') {
    throw new Error('expected authenticated normal staging group');
  }
  const assets = validateStagedGroupManifest(normal);
  if (!Buffer.isBuffer(checksumBytes) ||
      checksumBytes.length === 0 || checksumBytes.length > 16 * 1024) {
    throw new Error('invalid bounded original checksum bytes');
  }
  const reference = assets.get('checksums.txt');
  if (!reference ||
      reference.size !== checksumBytes.length ||
      reference.sha256 !== createHash('sha256').update(checksumBytes).digest('hex')) {
    throw new Error('original checksum file digest or length mismatch');
  }
  const lines = parseChecksums(checksumBytes.toString('utf8'));
  const names = expectedReleaseAssetNames(normal.tag).filter(name =>
    name !== 'checksums.txt' &&
    name !== 'mcpb-checksums.txt' &&
    !name.endsWith('.mcpb') &&
    !name.endsWith('.sigstore.json') &&
    !name.endsWith('.intoto.jsonl'));
  if (names.length !== 12 || lines.size !== 12) {
    throw new Error('normal checksum list must have exactly 12 original subjects');
  }
  const verified = [];
  for (const name of names) {
    const expected = assets.get(name);
    if (!expected || lines.get(name) !== expected.sha256) {
      throw new Error('normal checksum subject name or digest mismatch');
    }
    verified.push({ name, size: expected.size, sha256: expected.sha256 });
  }
  return verified;
}

module.exports = { verifyNormalChecksumClosure };
