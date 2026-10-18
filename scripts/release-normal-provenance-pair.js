'use strict';

const { createHash } = require('node:crypto');
const { verifyNormalChecksumClosure } = require('./release-normal-checksum-contract');
const { parseChecksums, projectBundle } = require('./release-provenance');

const MAX_SIDECAR_BYTES = 4 * 1024 * 1024;

function originalSidecar(normal, suffix, bytes) {
  if (!Buffer.isBuffer(bytes) || bytes.length === 0 ||
      bytes.length > MAX_SIDECAR_BYTES) {
    throw new Error('invalid bounded provenance sidecar bytes');
  }
  const name = 'scripthold-' + normal.tag + '-provenance.' + suffix;
  const asset = normal.assets.find(file => file.name === name);
  if (!asset || asset.size !== bytes.length ||
      asset.sha256 !== createHash('sha256').update(bytes).digest('hex')) {
    throw new Error('original provenance sidecar digest or length mismatch');
  }
}

// Checks that the *original* Sigstore bundle and in-toto JSONL are byte-for-byte
// consistent with a complete 15-file normal inventory and twelve subject
// checksums. This is structural consistency, NOT signature, certificate, OIDC,
// GitHub producer, run/attempt or artifact authentication. A trusted caller must
// perform the independent gh attestation verify step before using the evidence.
function checkNormalProvenancePair(normal, checksumBytes, bundleBytes, envelopeBytes) {
  verifyNormalChecksumClosure(normal, checksumBytes);
  originalSidecar(normal, 'sigstore.json', bundleBytes);
  originalSidecar(normal, 'intoto.jsonl', envelopeBytes);

  const bundle = JSON.parse(bundleBytes.toString('utf8'));
  const checksums = parseChecksums(checksumBytes.toString('utf8'));
  const originalEnvelope = Buffer.from(projectBundle(
    bundle, checksums, normal.tag, normal.commit, normal.repository));
  if (!originalEnvelope.equals(envelopeBytes)) {
    throw new Error('in-toto envelope is not the unchanged signed bundle envelope');
  }
}

module.exports = { checkNormalProvenancePair };
