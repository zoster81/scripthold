'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyGenuineNormalRawTransfer } = require('./genuine-normal-raw-transfer');

const TAG = 'v3.3.0';
const sha = bytes => createHash('sha256').update(bytes).digest('hex');

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(process.cwd(), '.real-normal-transfer-test-'));
  fs.chmodSync(dir, 0o700);
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const names = expectedReleaseAssetNames(TAG).slice(0, 13);
  const subjects = names.slice(0, 12);
  const bytes = subjects.map(name => Buffer.from('genuine-build-fixture:' + name));
  bytes.push(Buffer.from(subjects.map((name, i) => sha(bytes[i]) + '  ' + name).join('\n') + '\n'));
  const ids = names.map((_, i) => String(101 + i));
  const digests = bytes.map(sha);
  const runId = '12345', attempt = '2';
  for (let i = 0; i < 13; i++) {
    fs.writeFileSync(path.join(dir, `genuine-${runId}-${attempt}-normal-${String(i + 1).padStart(2, '0')}`),
      bytes[i], { mode: 0o600 });
  }
  return { dir, tag: TAG, runId, attempt, ids, digests, bytes };
}

test('checks every raw-transferred original build byte and canonical twelve-subject closure', async t => {
  const f = fixture(t);
  assert.deepEqual(await verifyGenuineNormalRawTransfer(f), { verified: 13, subjects: 12 });
});

test('rejects changed payload, swapped outputs and changed checksums', async t => {
  const f = fixture(t);
  const file = path.join(f.dir, 'genuine-12345-2-normal-06');
  fs.writeFileSync(file, Buffer.from('tampered'));
  await assert.rejects(verifyGenuineNormalRawTransfer(f), /digest|size/);
  fs.writeFileSync(file, f.bytes[5]);
  await assert.rejects(verifyGenuineNormalRawTransfer({ ...f,
    digests: [...f.digests].reverse() }), /digest/);
  const checksums = path.join(f.dir, 'genuine-12345-2-normal-13');
  const forged = Buffer.from(f.bytes[12]);
  forged[0] = forged[0] === 97 ? 98 : 97;
  fs.writeFileSync(checksums, forged);
  await assert.rejects(verifyGenuineNormalRawTransfer({ ...f,
    digests: [...f.digests.slice(0, 12), sha(forged)] }), /checksum|digest/);
});

test('rejects malformed and duplicate IDs, digests, attempts and unexpected paths', async t => {
  const f = fixture(t);
  for (const bad of [
    { ids: f.ids.slice(1) }, { ids: [f.ids[0], ...f.ids.slice(0, 12)] },
    { ids: ['0', ...f.ids.slice(1)] }, { digests: f.digests.slice(1) },
    { digests: ['A'.repeat(64), ...f.digests.slice(1)] },
    { attempt: '00' }, { runId: '../12345' }, { tag: '../v3.3.0' },
  ]) await assert.rejects(verifyGenuineNormalRawTransfer({ ...f, ...bad }));
  fs.writeFileSync(path.join(f.dir, 'unexpected'), 'surprise');
  await assert.rejects(verifyGenuineNormalRawTransfer(f), /inventory/);
});

test('rejects a symlink as transferred original without following it', async t => {
  const f = fixture(t);
  const target = path.join(f.dir, 'genuine-12345-2-normal-03');
  fs.rmSync(target);
  try { fs.symlinkSync(path.join(f.dir, 'genuine-12345-2-normal-02'), target); }
  catch (error) { if (error.code === 'EPERM') { t.skip('symlinks not permitted'); return; } throw error; }
  await assert.rejects(verifyGenuineNormalRawTransfer(f), /regular|linked/);
});

test('hosted proof pins a read-only historical build and exactly 13 raw transfers', () => {
  const yaml = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows',
    'ubuntu26-genuine-normal-transfer-proof.yml'), 'utf8');
  assert.match(yaml, /^permissions:\r?\n  contents: read\r?$/m);
  assert.match(yaml, /ref: v3\.3\.0/);
  assert.match(yaml, /aac3a46bf71135f73594341f1b97dcf08c552832/);
  assert.match(yaml, /5d1cde029753e4b75fbe3236febaecb905a82820/);
  assert.match(yaml, /release --clean --skip=publish,announce/);
  assert.equal((yaml.match(/uses: actions\/upload-artifact@/g) || []).length, 13);
  assert.equal((yaml.match(/archive: false/g) || []).length, 13);
  assert.match(yaml, /artifact-ids: \$\{\{ needs\.producer\.outputs\.ids \}\}/);
  assert.match(yaml, /skip-decompress: true/);
  assert.match(yaml, /digest-mismatch: error/);
  assert.match(yaml, /needs: producer/);
  assert.match(yaml, /node scripts\/genuine-normal-raw-transfer\.js/);
  assert.doesNotMatch(yaml,
    /contents: write|id-token: write|attestations: write|packages: write|gh release (?:upload|create|edit)|secrets\./);
});
