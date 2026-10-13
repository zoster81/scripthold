'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const { parseChecksums, projectBundle, verificationArguments, publish } = require('./release-provenance');

const TAG = 'v3.4.0';
const SHA = 'a'.repeat(40);
const NAME = 'scripthold_linux_amd64';
const DATA = Buffer.from('fixture for release provenance\n');
const DIGEST = crypto.createHash('sha256').update(DATA).digest('hex');

function fixtures() {
  const checksums = new Map([[NAME, DIGEST]]);
  const statement = {
    _type: 'https://in-toto.io/Statement/v1',
    subject: [{ name: NAME, digest: { sha256: DIGEST } }],
    predicateType: 'https://slsa.dev/provenance/v1',
    predicate: {
      buildDefinition: {
        externalParameters: {
          workflow: {
            repository: 'https://github.com/zoster81/scripthold',
            path: '.github/workflows/release.yml',
            ref: 'refs/tags/' + TAG,
          },
        },
        resolvedDependencies: [{
          uri: 'git+https://github.com/zoster81/scripthold@refs/tags/' + TAG,
          digest: { gitCommit: SHA },
        }],
      },
      runDetails: {
        builder: {
          id: 'https://github.com/zoster81/scripthold/.github/workflows/release.yml@refs/tags/' + TAG,
        },
      },
    },
  };
  const bundle = {
    mediaType: 'application/vnd.dev.sigstore.bundle.v0.3+json',
    verificationMaterial: { certificate: { rawBytes: 'AAAA' }, tlogEntries: [{}] },
    dsseEnvelope: {
      payloadType: 'application/vnd.in-toto+json',
      payload: Buffer.from(JSON.stringify(statement)).toString('base64'),
      signatures: [{ sig: 'AAAA' }],
    },
  };
  return { bundle, statement, checksums };
}

function changeStatement(fixture, mutate) {
  const statement = JSON.parse(Buffer.from(fixture.bundle.dsseEnvelope.payload, 'base64').toString('utf8'));
  mutate(statement);
  fixture.bundle.dsseEnvelope.payload = Buffer.from(JSON.stringify(statement)).toString('base64');
}

test('projects the unchanged signed DSSE envelope into a single in-toto JSON line', () => {
  const f = fixtures();
  const jsonl = projectBundle(f.bundle, f.checksums, TAG, SHA);
  assert.equal(jsonl.split('\n').length, 2);
  assert.deepEqual(JSON.parse(jsonl), f.bundle.dsseEnvelope);
});

test('GitHub CLI uses exact certificate identity and source binding', () => {
  const args = verificationArguments('artifact', 'bundle.json', 'zoster81/scripthold', TAG, SHA);
  assert.deepEqual(args, [
    'attestation', 'verify', 'artifact', '--bundle', 'bundle.json',
    '--repo', 'zoster81/scripthold',
    '--cert-identity', 'https://github.com/zoster81/scripthold/.github/workflows/release.yml@refs/tags/' + TAG,
    '--source-ref', 'refs/tags/' + TAG,
    '--source-digest', SHA,
    '--deny-self-hosted-runners',
  ]);
});

test('checksum parser rejects missing, malformed, duplicate, unsafe and oversized entries', () => {
  assert.deepEqual(parseChecksums(DIGEST + '  ' + NAME + '\n'), new Map([[NAME, DIGEST]]));
  for (const input of [
    '', 'invalid', DIGEST + '  ../bad', DIGEST + '  ' + NAME + '\n' + DIGEST + '  ' + NAME,
    DIGEST + '  nested/bad',
  ]) assert.throws(() => parseChecksums(input));
});

test('fails closed for incorrect statement, signature, source identity and provenance', () => {
  const bad = [
    (f) => { f.bundle.mediaType = 'unexpected'; },
    (f) => { f.bundle.verificationMaterial.certificate.rawBytes = ''; },
    (f) => { f.bundle.verificationMaterial.tlogEntries = []; },
    (f) => { f.bundle.dsseEnvelope.signatures = []; },
    (f) => { f.bundle.dsseEnvelope.payload = 'not%base64'; },
    (f) => changeStatement(f, (s) => { s.subject[0].digest.sha256 = 'b'.repeat(64); }),
    (f) => changeStatement(f, (s) => { s.subject.push(s.subject[0]); }),
    (f) => changeStatement(f, (s) => { s.predicateType = 'https://example.com/other'; }),
    (f) => changeStatement(f, (s) => { s.predicate.buildDefinition.resolvedDependencies[0].digest.gitCommit = 'b'.repeat(40); }),
    (f) => changeStatement(f, (s) => { s.predicate.buildDefinition.externalParameters.workflow.ref = 'refs/heads/main'; }),
    (f) => changeStatement(f, (s) => { s.predicate.runDetails.builder.id = 'https://example.com/other'; }),
  ];
  for (const mutate of bad) {
    const f = fixtures();
    mutate(f);
    assert.throws(() => projectBundle(f.bundle, f.checksums, TAG, SHA));
  }
});

test('publishes only after exact local checksum and external verifier success', async () => {
  // Create and remove only a unique test-owned directory within the current checkout.
  const cwd = fs.mkdtempSync(path.join(process.cwd(), '.release-provenance-test-'));
  try {
    fs.mkdirSync(path.join(cwd, 'dist'));
    const assetPath = path.join(cwd, 'dist', NAME);
    const bundlePath = path.join(cwd, 'bundle.json');
    const checksumsPath = path.join(cwd, 'checksums.txt');
    const artifactsPath = path.join(cwd, 'artifacts.json');
    const outputPath = path.join(cwd, 'attestation.intoto.jsonl');
    fs.writeFileSync(assetPath, DATA);
    fs.writeFileSync(bundlePath, JSON.stringify(fixtures().bundle));
    fs.writeFileSync(checksumsPath, DIGEST + '  ' + NAME + '\n');
    fs.writeFileSync(artifactsPath, JSON.stringify([{ name: NAME, path: 'dist/' + NAME }]));
    const opts = { cwd, bundlePath, checksumsPath, artifactsPath, outputPath, tag: TAG, sha: SHA };
    let verifications = 0;
    await assert.rejects(
      publish({ ...opts, verifier: async () => { throw new Error('invalid certificate'); } }),
      /invalid certificate/,
    );
    assert.equal(fs.existsSync(outputPath), false);
    await publish({ ...opts, verifier: async (filename, bundle, repo, tag, sha) => {
      verifications++;
      assert.equal(filename, assetPath);
      assert.equal(bundle, bundlePath);
      assert.equal(repo, 'zoster81/scripthold');
      assert.equal(tag, TAG);
      assert.equal(sha, SHA);
    } });
    assert.equal(verifications, 1);
    assert.deepEqual(JSON.parse(fs.readFileSync(outputPath, 'utf8')), fixtures().bundle.dsseEnvelope);
    await assert.rejects(publish({ ...opts, verifier: async () => {} }), /EEXIST/);

    fs.rmSync(outputPath);
    fs.writeFileSync(assetPath, Buffer.from('modified'));
    await assert.rejects(publish({ ...opts, verifier: async () => {} }), /digest mismatch/);
    assert.equal(fs.existsSync(outputPath), false);
  } finally {
    fs.rmSync(cwd, { recursive: true, force: true });
  }
});

test('rejects malformed GoReleaser inventory and outside-dist paths', async () => {
  const cwd = fs.mkdtempSync(path.join(process.cwd(), '.release-provenance-test-'));
  try {
    fs.mkdirSync(path.join(cwd, 'dist'));
    fs.writeFileSync(path.join(cwd, 'dist', NAME), DATA);
    const f = fixtures();
    const bundlePath = path.join(cwd, 'bundle.json');
    const checksumsPath = path.join(cwd, 'checksums.txt');
    const artifactsPath = path.join(cwd, 'artifacts.json');
    const outputPath = path.join(cwd, 'attestation.intoto.jsonl');
    fs.writeFileSync(bundlePath, JSON.stringify(f.bundle));
    fs.writeFileSync(checksumsPath, DIGEST + '  ' + NAME + '\n');
    const opts = { cwd, bundlePath, checksumsPath, artifactsPath, outputPath, tag: TAG,
      sha: SHA, verifier: async () => {} };
    for (const entries of [
      [],
      [{ name: NAME, path: '../escape' }],
      [{ name: NAME, path: 'dist/' + NAME }, { name: NAME, path: 'dist/' + NAME }],
    ]) {
      fs.writeFileSync(artifactsPath, JSON.stringify(entries));
      await assert.rejects(publish(opts));
      assert.equal(fs.existsSync(outputPath), false);
    }
  } finally {
    fs.rmSync(cwd, { recursive: true, force: true });
  }
});
