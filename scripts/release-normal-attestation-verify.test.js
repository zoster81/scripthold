'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { inspectNormalStage, verifyStagedNormalAttestations } =
  require('./release-normal-attestation-verify');

const TAG = 'v3.4.0';
const COMMIT = 'a'.repeat(40);
const digest = bytes => createHash('sha256').update(bytes).digest('hex');

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(process.cwd(), '.normal-attest-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.chmodSync(dir, 0o700);
  const names = expectedReleaseAssetNames(TAG).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
  const subjects = names.slice(0, 12);
  const original = new Map(subjects.map(name => [name, Buffer.from('original-' + name)]));
  original.set('checksums.txt', Buffer.from(subjects.map(name =>
    digest(original.get(name)) + '  ' + name).join('\n') + '\n'));
  const statement = {
    _type: 'https://in-toto.io/Statement/v1',
    subject: subjects.map(name => ({ name, digest: { sha256: digest(original.get(name)) } })),
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
  original.set('scripthold-' + TAG + '-provenance.sigstore.json', Buffer.from(JSON.stringify({
    mediaType: 'application/vnd.dev.sigstore.bundle.v0.3+json',
    verificationMaterial: { certificate: { rawBytes: 'AAAA' }, tlogEntries: [{}] },
    dsseEnvelope: envelope,
  })));
  original.set('scripthold-' + TAG + '-provenance.intoto.jsonl',
    Buffer.from(JSON.stringify(envelope) + '\n'));
  const assets = names.map(name => ({
    name, size: original.get(name).length, sha256: digest(original.get(name)),
  }));
  for (const [name, bytes] of original) {
    fs.writeFileSync(path.join(dir, name), bytes, { mode: 0o600 });
  }
  return { dir, manifest: {
    repository: 'zoster81/scripthold', tag: TAG, commit: COMMIT, stage: 'normal', assets,
  }, subjects, original };
}

test('measures exactly 15 original stage bytes and the twelve signed subjects', async t => {
  const f = fixture(t);
  const snapshot = await inspectNormalStage(f.manifest, f.dir);
  assert.deepEqual(snapshot.subjects, f.subjects);
  assert.equal(snapshot.bundlePath, path.join(f.dir,
    'scripthold-' + TAG + '-provenance.sigstore.json'));
  assert.equal(snapshot.size, 15);
  f.manifest.assets[0].sha256 = 'b'.repeat(64);
  await assert.rejects(inspectNormalStage(f.manifest, f.dir), /digest|mismatch/);
});

test('enumerates at most sixteen entries for a fifteen-file private stage', async t => {
  const f = fixture(t);
  const unrestricted = fs.readdirSync;
  fs.readdirSync = function (target, ...options) {
    if (target === f.dir) {
      throw new Error('unbounded directory listing is forbidden');
    }
    return unrestricted.call(this, target, ...options);
  };
  try {
    assert.equal((await inspectNormalStage(f.manifest, f.dir)).size, 15);
    fs.writeFileSync(path.join(f.dir, 'surplus-file'), Buffer.from('extra'));
    await assert.rejects(inspectNormalStage(f.manifest, f.dir),
      /exactly 15 original names/);
  } finally {
    fs.readdirSync = unrestricted;
  }
});

test('bounded original sidecars never use an unbounded descriptor read', async t => {
  const f = fixture(t);
  const unboundedRead = fs.readFileSync;
  let directReads = 0;
  fs.readFileSync = function (source, ...options) {
    if (typeof source === 'number') {
      directReads++;
      throw new Error('unbounded original descriptor read forbidden');
    }
    return unboundedRead.call(this, source, ...options);
  };
  try {
    const checked = await inspectNormalStage(f.manifest, f.dir);
    assert.equal(checked.size, 15);
    assert.equal(directReads, 0);
  } finally {
    fs.readFileSync = unboundedRead;
  }
});

test('normal attestation preflight rejects missing, extra, replaced or linked original bytes', async t => {
  const f = fixture(t);
  const leaf = f.subjects[0];
  fs.writeFileSync(path.join(f.dir, leaf), Buffer.from('modified'));
  await assert.rejects(inspectNormalStage(f.manifest, f.dir), /mismatch|size|changed/);
  fs.writeFileSync(path.join(f.dir, leaf), f.original.get(leaf));
  fs.writeFileSync(path.join(f.dir, 'foreign'), Buffer.from('surprise'));
  await assert.rejects(inspectNormalStage(f.manifest, f.dir), /exactly|unexpected/);
  fs.rmSync(path.join(f.dir, 'foreign'));
  fs.rmSync(path.join(f.dir, leaf));
  if (process.platform === 'win32') {
    await assert.rejects(inspectNormalStage(f.manifest, f.dir), /exactly|missing/);
  } else {
    fs.symlinkSync(path.join(f.dir, f.subjects[1]), path.join(f.dir, leaf));
    await assert.rejects(inspectNormalStage(f.manifest, f.dir), /regular|linked/);
  }
});

test('structural DSSE and source identity cannot substitute for genuine crypto', async t => {
  const f = fixture(t);
  const bundleName = 'scripthold-' + TAG + '-provenance.sigstore.json';
  const bundle = JSON.parse(f.original.get(bundleName));
  bundle.dsseEnvelope.signatures[0].sig = 'BBBB';
  const changed = Buffer.from(JSON.stringify(bundle));
  fs.writeFileSync(path.join(f.dir, bundleName), changed);
  f.manifest.assets.find(a => a.name === bundleName).sha256 = digest(changed);
  f.manifest.assets.find(a => a.name === bundleName).size = changed.length;
  const envelopeName = 'scripthold-' + TAG + '-provenance.intoto.jsonl';
  const envelope = Buffer.from(JSON.stringify(bundle.dsseEnvelope) + '\n');
  fs.writeFileSync(path.join(f.dir, envelopeName), envelope);
  f.manifest.assets.find(a => a.name === envelopeName).sha256 = digest(envelope);
  f.manifest.assets.find(a => a.name === envelopeName).size = envelope.length;
  const snapshot = await inspectNormalStage(f.manifest, f.dir);
  assert.equal(snapshot.subjects.length, 12,
    'structural verification does not claim this modified signature is valid');
  if (process.platform !== 'linux') {
    await assert.rejects(verifyStagedNormalAttestations(f.manifest, f.dir),
      /native Linux/);
  }
});
