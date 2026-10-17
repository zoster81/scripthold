'use strict';

const { createHash } = require('node:crypto');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyNormalChecksumClosure } = require('./release-normal-checksum-contract');

const MAX_ASSET_BYTES = 2 * 1024 * 1024 * 1024;
const MAX_CHECKSUM_BYTES = 16 * 1024;
const MAX_ATTESTATION_BYTES = 4 * 1024 * 1024;
const SOURCE_SHA = /^[0-9a-f]{40}$/;

// Measure a producer-owned, flat, private 15-file stage before any cross-job
// upload. This is inventory/byte consistency, NOT cryptographic Sigstore,
// GitHub job/run authentication, filesystem authorization or upload authority.
// The caller must enumerate the entire directory, retain stable no-follow
// single-owner handles, and upload the SAME verified original bytes.
async function measureNormalSourcePayloads(tag, commit, observed, openSource) {
  const names = expectedReleaseAssetNames(tag).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
  if (!SOURCE_SHA.test(commit) || names.length !== 15) {
    throw new Error('invalid normal producer source identity');
  }
  if (!Array.isArray(observed) || observed.length !== names.length) {
    throw new Error('normal producer requires exactly 15 original files');
  }
  if (typeof openSource !== 'function') {
    throw new Error('missing stable normal producer source opener');
  }
  const expected = new Set(names);
  const captured = new Map();
  for (const item of observed) {
    if (!item || typeof item !== 'object' || Array.isArray(item) ||
        Object.keys(item).length !== 4 ||
        !['name', 'size', 'type', 'nlink'].every(key => Object.hasOwn(item, key)) ||
        !expected.has(item.name) || captured.has(item.name) ||
        item.type !== 'file' || item.nlink !== 1 ||
        !Number.isSafeInteger(item.size) || item.size <= 0 ||
        item.size > MAX_ASSET_BYTES ||
        (item.name === 'checksums.txt' && item.size > MAX_CHECKSUM_BYTES) ||
        ((item.name.endsWith('.sigstore.json') || item.name.endsWith('.intoto.jsonl')) &&
          item.size > MAX_ATTESTATION_BYTES)) {
      throw new Error('invalid original producer file inventory');
    }
    captured.set(item.name, item.size);
  }
  const pinned = names.map(name => {
    if (!captured.has(name)) throw new Error('missing original producer file');
    return { name, size: captured.get(name) };
  });

  const assets = [];
  let checksumBytes;
  for (const file of pinned) {
    // Never let an untrusted asynchronous opener rewrite the expected sizes.
    const source = await openSource({ ...file });
    if (!source || typeof source[Symbol.asyncIterator] !== 'function') {
      throw new Error('original asset requires a byte stream');
    }
    const digest = createHash('sha256');
    const fragments = [];
    let size = 0;
    for await (const chunk of source) {
      if (!(chunk instanceof Uint8Array)) {
        throw new Error('original asset stream contains nonbinary data');
      }
      size += chunk.byteLength;
      if (size > file.size) throw new Error('original asset exceeds declared size');
      digest.update(chunk);
      if (file.name === 'checksums.txt') fragments.push(Buffer.from(chunk));
    }
    if (size !== file.size) throw new Error('original asset size mismatch');
    assets.push({ name: file.name, size, sha256: digest.digest('hex') });
    if (file.name === 'checksums.txt') checksumBytes = Buffer.concat(fragments);
  }
  const manifest = { repository: 'zoster81/scripthold', tag, commit,
    stage: 'normal', assets };
  verifyNormalChecksumClosure(manifest, checksumBytes);
  return manifest;
}

module.exports = { measureNormalSourcePayloads };
