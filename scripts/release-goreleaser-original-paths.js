'use strict';

const { createHash } = require('node:crypto');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyCanonicalNormalChecksums } = require('./release-normal-rehearsal');

const MAX_GORELEASER_ENTRIES = 64;

// Exact local layouts observed with pinned GoReleaser v2.18.1, not transferable
// source authority. A future GoReleaser layout change must be reviewed rather
// than silently repairing, flattening, or normalizing a producer-supplied path.
function expectedProducerEntry(name) {
  if (name === 'checksums.txt') {
    return { type: 'Checksum', path: 'dist/checksums.txt',
      sourceParent: 'dist', sourceLeaf: 'checksums.txt' };
  }
  const match = /^scripthold_(darwin|linux|windows)_(amd64|arm64)(.*)$/.exec(name);
  if (!match) throw new Error('unexpected canonical normal output');
  const [, os, arch, extension] = match;
  const binary = extension === '' || (os === 'windows' && extension === '.exe');
  const archive = (os === 'windows' && extension === '.zip') ||
    (os !== 'windows' && extension === '.tar.gz');
  if (!binary && !archive) throw new Error('unsupported normal output extension');
  const sourceParent = binary
    ? 'dist/scripthold_' + os + '_' + arch + '_' + (arch === 'amd64' ? 'v1' : 'v8.0')
    : 'dist';
  const sourceLeaf = binary ? 'scripthold' + (os === 'windows' ? '.exe' : '') : name;
  return {
    type: binary ? 'Binary' : 'Archive',
    goos: os, goarch: arch,
    target: os + '_' + arch + '_' + (arch === 'amd64' ? 'v1' : 'v8.0'),
    sourceParent, sourceLeaf,
    path: sourceParent + '/' + sourceLeaf,
  };
}

// Select exactly the 13 original public GoReleaser files from the producer's
// artifacts.json (which also contains intermediate binaries and metadata).
// This pure mapping does NOT open/authorize source paths, attest producer
// identity, authenticate checksums, establish file sizes, or sign anything.
// Its local source paths must never be sent to another job as authority.
function planOriginalGoReleaserPaths(tag, checksumBytes, artifactMetadata) {
  const checksums = verifyCanonicalNormalChecksums(tag, checksumBytes);
  if (!Array.isArray(artifactMetadata) ||
      artifactMetadata.length < 13 ||
      artifactMetadata.length > MAX_GORELEASER_ENTRIES) {
    throw new Error('invalid bounded GoReleaser artifact metadata inventory');
  }
  const names = expectedReleaseAssetNames(tag).slice(0, 13);
  const namesByFold = new Map(names.map(name => [name.toLowerCase(), name]));
  const selected = new Map();

  for (const artifact of artifactMetadata) {
    if (!artifact || typeof artifact !== 'object' || Array.isArray(artifact) ||
        typeof artifact.name !== 'string' || artifact.name.length > 240) {
      throw new Error('invalid GoReleaser artifact metadata entry');
    }
    const canonical = namesByFold.get(artifact.name.toLowerCase());
    if (!canonical) continue; // Ignore internal build binaries and metadata.
    if (canonical !== artifact.name) {
      throw new Error('case-aliased public GoReleaser output');
    }
    if (selected.has(canonical)) throw new Error('duplicate public GoReleaser output');
    const expected = expectedProducerEntry(canonical);
    if (artifact.type !== expected.type) {
      throw new Error('public GoReleaser output type mismatch');
    }
    if (expected.goos && (artifact.goos !== expected.goos || artifact.goarch !== expected.goarch)) {
      throw new Error('public GoReleaser output platform mismatch');
    }
    if (expected.target && artifact.target !== expected.target) {
      throw new Error('public GoReleaser output target mismatch');
    }
    if (artifact.path !== expected.path) {
      throw new Error('public GoReleaser output source path mismatch');
    }
    const original = {
      name: canonical,
      type: expected.type,
      sourceParent: expected.sourceParent,
      sourceLeaf: expected.sourceLeaf,
    };
    if (canonical === 'checksums.txt') {
      original.sha256 = createHash('sha256').update(checksumBytes).digest('hex');
      original.size = checksumBytes.length;
    } else {
      original.sha256 = checksums.get(canonical);
    }
    selected.set(canonical, original);
  }

  if (selected.size !== names.length) {
    throw new Error('missing original GoReleaser outputs; expected exactly thirteen');
  }
  return names.map(name => selected.get(name));
}

module.exports = { planOriginalGoReleaserPaths };
