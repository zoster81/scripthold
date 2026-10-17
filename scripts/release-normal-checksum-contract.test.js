'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const test = require('node:test');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyNormalChecksumClosure } = require('./release-normal-checksum-contract');

const TAG = 'v3.4.0';
const SHA = 'a'.repeat(40);
function digest(bytes) {
  return crypto.createHash('sha256').update(bytes).digest('hex');
}
function fixture() {
  const names = expectedReleaseAssetNames(TAG).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
  assert.equal(names.length, 15);
  const entries = names.filter(name => name !== 'checksums.txt' &&
    !name.endsWith('.sigstore.json') && !name.endsWith('.intoto.jsonl'));
  assert.equal(entries.length, 12);
  const binaries = new Map(entries.map(name => [name,
    Buffer.from('source:' + name + '\n', 'utf8')]));
  const checksumsBytes = Buffer.from(entries.map(name =>
    digest(binaries.get(name)) + '  ' + name).join('\n') + '\n', 'utf8');
  const manifest = {
    repository: 'zoster81/scripthold',
    tag: TAG, commit: SHA, stage: 'normal',
    assets: names.map(name => {
      const bytes = name === 'checksums.txt' ? checksumsBytes :
        binaries.get(name) || Buffer.from('signed-evidence:' + name);
      return { name, size: bytes.length, sha256: digest(bytes) };
    }),
  };
  return { manifest, checksumsBytes, entries, binaries };
}

test('accepts exactly the 12 canonical signed-subject checksum identities', () => {
  const f = fixture();
  const verified = verifyNormalChecksumClosure(f.manifest, f.checksumsBytes);
  assert.deepEqual(verified.map(x => x.name), f.entries);
  assert.equal(verified.length, 12);
  for (const entry of verified) {
    assert.equal(entry.sha256, digest(f.binaries.get(entry.name)));
    assert.equal(entry.size, f.binaries.get(entry.name).length);
  }
  f.manifest.assets[0].sha256 = 'f'.repeat(64);
  assert.notEqual(verified[0].sha256, f.manifest.assets[0].sha256);
});

test('rejects changed checksum bytes even when the checksum lines are well formed', () => {
  const f = fixture();
  const replaced = Buffer.from(f.checksumsBytes);
  replaced[0] = replaced[0] === 97 ? 98 : 97;
  assert.throws(() => verifyNormalChecksumClosure(f.manifest, replaced),
    /checksum file|digest|mismatch/i);
});

test('refuses missing, extra, duplicated or corrupted signed-subject names', () => {
  const f = fixture();
  const lines = f.checksumsBytes.toString('utf8').trimEnd().split('\n');
  const cases = [
    lines.slice(1),
    [...lines, lines[0]],
    [...lines.slice(0, 1), lines[0], ...lines.slice(2)],
    [...lines.slice(0, 1), lines[1].replace('scripthold_', '../scripthold_'), ...lines.slice(2)],
    [...lines.slice(0, 1), lines[1].replace(/ [^ ]+$/, ' rogue.bin'), ...lines.slice(2)],
    [...lines.slice(0, 1), 'f'.repeat(64) + lines[1].slice(64), ...lines.slice(2)],
  ];
  for (const altered of cases) {
    const payload = Buffer.from(altered.join('\n') + '\n');
    // An already authenticated stage inventory still pins original checksum
    // file bytes, so no changed content can authorize its own digest.
    assert.throws(() => verifyNormalChecksumClosure(f.manifest, payload));
    const cloned = structuredClone(f.manifest);
    const entry = cloned.assets.find(x => x.name === 'checksums.txt');
    entry.size = payload.length; entry.sha256 = digest(payload);
    assert.throws(() => verifyNormalChecksumClosure(cloned, payload));
  }
});

test('fails closed on oversized, non-byte, malformed or noncanonical provenance input', () => {
  const f = fixture();
  for (const bytes of [
    null, '', 'not-bytes', Buffer.alloc(0), Buffer.alloc(16385),
    Buffer.from([0xef, 0xbb, 0xbf, ...f.checksumsBytes]),
  ]) {
    assert.throws(() => verifyNormalChecksumClosure(f.manifest, bytes));
  }
  for (const mutate of [
    m => { m.stage = 'mcpb'; },
    m => { m.repository = 'attacker/scripthold'; },
    m => { m.assets.pop(); },
    m => { m.assets.push(m.assets[0]); },
    m => { m.assets[0].sha256 = 'f'.repeat(64); },
  ]) {
    const m = structuredClone(f.manifest);
    mutate(m);
    assert.throws(() => verifyNormalChecksumClosure(m, f.checksumsBytes));
  }
});
