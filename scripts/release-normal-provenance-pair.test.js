'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const test = require('node:test');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { checkNormalProvenancePair } = require('./release-normal-provenance-pair');

const TAG = 'v3.4.0';
const COMMIT = 'a'.repeat(40);
const sha = bytes => createHash('sha256').update(bytes).digest('hex');

function fixture() {
  const names = expectedReleaseAssetNames(TAG).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
  const subjects = names.slice(0, 12);
  const bytes = new Map(subjects.map(name => [name, Buffer.from('fixture-' + name)]));
  const checksums = Buffer.from(subjects.map(name =>
    sha(bytes.get(name)) + '  ' + name).join('\n') + '\n');
  bytes.set('checksums.txt', checksums);
  const statement = {
    _type: 'https://in-toto.io/Statement/v1',
    subject: subjects.map(name => ({ name, digest: { sha256: sha(bytes.get(name)) } })),
    predicateType: 'https://slsa.dev/provenance/v1',
    predicate: {
      buildDefinition: {
        externalParameters: { workflow: {
          repository: 'https://github.com/zoster81/scripthold',
          path: '.github/workflows/release.yml',
          ref: 'refs/tags/' + TAG,
        } },
        resolvedDependencies: [{
          uri: 'git+https://github.com/zoster81/scripthold@refs/tags/' + TAG,
          digest: { gitCommit: COMMIT },
        }],
      },
      runDetails: { builder: { id:
        'https://github.com/zoster81/scripthold/.github/workflows/release.yml@refs/tags/' + TAG,
      } },
    },
  };
  const envelope = {
    payloadType: 'application/vnd.in-toto+json',
    payload: Buffer.from(JSON.stringify(statement)).toString('base64'),
    signatures: [{ sig: 'AAAA' }],
  };
  const bundle = {
    mediaType: 'application/vnd.dev.sigstore.bundle.v0.3+json',
    verificationMaterial: {
      certificate: { rawBytes: 'AAAA' },
      tlogEntries: [{}],
    },
    dsseEnvelope: envelope,
  };
  const bundleBytes = Buffer.from(JSON.stringify(bundle));
  const envelopeBytes = Buffer.from(JSON.stringify(envelope) + '\n');
  bytes.set('scripthold-' + TAG + '-provenance.sigstore.json', bundleBytes);
  bytes.set('scripthold-' + TAG + '-provenance.intoto.jsonl', envelopeBytes);
  const manifest = {
    repository: 'zoster81/scripthold', tag: TAG, commit: COMMIT, stage: 'normal',
    assets: names.map(name => ({
      name, size: bytes.get(name).length, sha256: sha(bytes.get(name)),
    })),
  };
  return { manifest, checksums, bundleBytes, envelopeBytes, bundle, envelope, statement };
}

function verify(f) {
  return checkNormalProvenancePair(f.manifest, f.checksums, f.bundleBytes, f.envelopeBytes);
}

test('checks both unchanged original sidecars against the exact 15-file inventory', () => {
  const f = fixture();
  assert.equal(verify(f), undefined);
  assert.equal(f.manifest.assets.length, 15);
  assert.equal(f.statement.subject.length, 12);
});

test('rejects changed, missing and oversized original sidecar bytes', () => {
  const f = fixture();
  for (const sidecar of ['bundleBytes', 'envelopeBytes']) {
    assert.throws(() => verify({ ...f, [sidecar]: undefined }));
    assert.throws(() => verify({ ...f, [sidecar]: Buffer.alloc(4 * 1024 * 1024 + 1) }),
      /bounded.*provenance|provenance.*bytes/);
    assert.throws(() => verify({ ...f, [sidecar]: Buffer.concat([
      f[sidecar], Buffer.from('!')]) }));
  }
  assert.throws(() => verify({ ...f, envelopeBytes: Buffer.from(f.envelopeBytes.toString().trimEnd()) }));
  assert.throws(() => verify({ ...f, bundleBytes: Buffer.from('invalid-json') }));
});

test('rejects forged inventory, unchecked checksum subjects and wrong source identity', () => {
  const f = fixture();
  assert.throws(() => verify({ ...f, manifest: { ...f.manifest,
    assets: f.manifest.assets.slice(0, 14) } }));
  assert.throws(() => verify({ ...f, manifest: { ...f.manifest,
    commit: 'b'.repeat(40) } }), /source commit|source mismatch/);
  assert.throws(() => verify({ ...f, manifest: { ...f.manifest,
    tag: 'v3.5.0' } }));
  const corrupted = Buffer.from(f.checksums);
  corrupted[0] = corrupted[0] === 97 ? 98 : 97;
  assert.throws(() => verify({ ...f, checksums: corrupted }));
  const changed = structuredClone(f.manifest);
  changed.assets.find(x => x.name.endsWith('.sigstore.json')).sha256 = 'b'.repeat(64);
  assert.throws(() => verify({ ...f, manifest: changed }));
});

test('rejects changed signed payload, builder path and in-toto envelope', () => {
  const f = fixture();
  for (const mutate of [
    s => { s.subject[0].digest.sha256 = 'b'.repeat(64); },
    s => { s.subject[0].name = 'unexpected'; },
    s => { s.subject.push(s.subject[0]); },
    s => { s.predicate.runDetails.builder.id = 'https://example.org/other'; },
    s => { s.predicate.buildDefinition.externalParameters.workflow.path = '.github/workflows/other.yml'; },
    s => { s.predicate.buildDefinition.resolvedDependencies[0].digest.gitCommit = 'b'.repeat(40); },
  ]) {
    const statement = structuredClone(f.statement);
    mutate(statement);
    const bundle = structuredClone(f.bundle);
    bundle.dsseEnvelope.payload = Buffer.from(JSON.stringify(statement)).toString('base64');
    const bytes = Buffer.from(JSON.stringify(bundle));
    const manifest = structuredClone(f.manifest);
    const asset = manifest.assets.find(x => x.name.endsWith('.sigstore.json'));
    asset.size = bytes.length;
    asset.sha256 = sha(bytes);
    assert.throws(() => verify({ ...f, bundleBytes: bytes, manifest }));
  }
  const reordered = Buffer.from(JSON.stringify({
    signatures: f.envelope.signatures, payload: f.envelope.payload,
    payloadType: f.envelope.payloadType,
  }) + '\n');
  const manifest = structuredClone(f.manifest);
  const asset = manifest.assets.find(x => x.name.endsWith('.intoto.jsonl'));
  asset.size = reordered.length;
  asset.sha256 = sha(reordered);
  assert.throws(() => verify({ ...f, manifest, envelopeBytes: reordered }),
    /unchanged|envelope|canonical/);
});
