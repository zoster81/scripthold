'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { Readable } = require('node:stream');
const test = require('node:test');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { measureNormalSourcePayloads,
  encodeGoReleaserNotes, decodeGoReleaserNotes } = require('./release-normal-source');
test('preserves exact GoReleaser 2.18.1 generated dist notes without touching the 22 asset manifest', () => {
  const body = Buffer.from('## Changelog\n* Fix: preserve notes exactly\n\n', 'utf8');
  const metadata = encodeGoReleaserNotes(body);
  assert.equal(metadata.v, 1);
  assert.equal(metadata.size, body.length);
  assert.equal(metadata.sha256, hash(body));
  assert.deepEqual(Buffer.from(decodeGoReleaserNotes(metadata), 'utf8'), body);
  assert.equal(decodeGoReleaserNotes(JSON.stringify(metadata)), body.toString('utf8'));
  assert.equal(fixture().names.length, 15, 'notes are not another normal release artifact');
});

test('rejects forged, noncanonical, malformed or excessive GoReleaser notes', () => {
  const valid = encodeGoReleaserNotes(Buffer.from('## Changelog\n', 'utf8'));
  const corrupted = [
    { ...valid, sha256: '0'.repeat(64) }, { ...valid, size: 1 },
    { ...valid, base64: valid.base64 + '\n' },
    { ...valid, base64: 'AA==' }, { ...valid, v: 2 },
    { ...valid, injected: 'other notes' },
  ];
  for (const data of corrupted) {
    assert.throws(() => decodeGoReleaserNotes(data));
  }
  for (const data of [Buffer.alloc(0), Buffer.from('no newline'),
    Buffer.from('bad\r\n'), Buffer.from([0xff, 0x0a]),
    Buffer.from([0, 10]), Buffer.from([0xef, 0xbb, 0xbf, 0x23, 0x0a]),
    Buffer.alloc(65537, 10)]) {
    assert.throws(() => encodeGoReleaserNotes(data));
  }
  assert.throws(() => decodeGoReleaserNotes('{}'));
  assert.throws(() => decodeGoReleaserNotes(Buffer.from('invalid')));
});


const TAG = 'v3.4.0';
const COMMIT = 'a'.repeat(40);
const hash = bytes => createHash('sha256').update(bytes).digest('hex');

function fixture() {
  const names = expectedReleaseAssetNames(TAG).filter(name =>
    !name.endsWith('.mcpb') && name !== 'mcpb-checksums.txt');
  assert.equal(names.length, 15);
  const subjects = names.filter(name =>
    name !== 'checksums.txt' && !name.endsWith('.sigstore.json') &&
    !name.endsWith('.intoto.jsonl'));
  assert.equal(subjects.length, 12);
  const bytes = new Map(names.filter(name => name !== 'checksums.txt')
    .map(name => [name, Buffer.from('original:' + name + '\n')]));
  bytes.set('checksums.txt', Buffer.from(subjects.map(name =>
    hash(bytes.get(name)) + '  ' + name).join('\n') + '\n'));
  const observed = names.map(name => ({
    name, type: 'file', nlink: 1, size: bytes.get(name).length,
  }));
  const open = async ({ name }) => Readable.from([bytes.get(name)]);
  return { names, bytes, observed, open };
}

test('measures exactly 15 originals and closes 12 checksum subjects without file I/O', async () => {
  const f = fixture();
  const result = await measureNormalSourcePayloads(TAG, COMMIT, f.observed, f.open);
  assert.equal(result.repository, 'zoster81/scripthold');
  assert.equal(result.tag, TAG);
  assert.equal(result.commit, COMMIT);
  assert.equal(result.stage, 'normal');
  assert.deepEqual(result.assets.map(a => a.name), f.names);
  for (const asset of result.assets) {
    assert.equal(asset.size, f.bytes.get(asset.name).length);
    assert.equal(asset.sha256, hash(f.bytes.get(asset.name)));
  }
  // Never claim signature/DSSE authentication from measuring source bytes.
  assert.deepEqual(Object.keys(result).sort(),
    ['assets', 'commit', 'repository', 'stage', 'tag']);
});

test('rejects missing, duplicate, foreign, path, symlink and hardlink entries before opening', async () => {
  const f = fixture();
  const invalid = [
    f.observed.slice(1),
    [...f.observed, f.observed[0]],
    f.observed.map((o, i) => i === 0 ? { ...o, name: 'foreign' } : o),
    f.observed.map((o, i) => i === 0 ? { ...o, name: '../' + o.name } : o),
    f.observed.map((o, i) => i === 0 ? { ...o, type: 'symlink' } : o),
    f.observed.map((o, i) => i === 0 ? { ...o, nlink: 2 } : o),
    f.observed.map((o, i) => i === 0 ? { ...o, size: 0 } : o),
    f.observed.map((o, i) => i === 0 ? { ...o, localPath: '/tmp/forged' } : o),
  ];
  for (const entries of invalid) {
    let opens = 0;
    await assert.rejects(measureNormalSourcePayloads(TAG, COMMIT, entries, async () => {
      opens++;
      return Readable.from([]);
    }));
    assert.equal(opens, 0, 'metadata preflight must complete before any source read');
  }
});

test('rejects tampered/truncated/oversized streams and a forged checksum list', async () => {
  const f = fixture();
  const binary = f.names.find(name => name === 'scripthold_linux_amd64');
  for (const bad of [Buffer.from('bad'), Buffer.concat([f.bytes.get(binary), Buffer.from('extra')])]) {
    const open = async ({ name }) =>
      Readable.from([name === binary ? bad : f.bytes.get(name)]);
    await assert.rejects(measureNormalSourcePayloads(TAG, COMMIT, f.observed, open));
  }
  const forgedChecksum = Buffer.from(f.bytes.get('checksums.txt'));
  forgedChecksum[0] = forgedChecksum[0] === 97 ? 98 : 97;
  const observed = f.observed.map(x => x.name === 'checksums.txt' ?
    { ...x, size: forgedChecksum.length } : x);
  await assert.rejects(measureNormalSourcePayloads(TAG, COMMIT, observed,
    async ({ name }) => Readable.from([name === 'checksums.txt' ?
      forgedChecksum : f.bytes.get(name)])), /checksum|subject|digest/i);
  await assert.rejects(measureNormalSourcePayloads(TAG, COMMIT, f.observed,
    async ({ name }) => name === binary ? 'not a byte stream' :
      Readable.from([f.bytes.get(name)])));
});

test('rejects oversize checksum/attestation metadata and wrong source identity', async () => {
  const f = fixture();
  for (const name of ['checksums.txt',
    'scripthold-v3.4.0-provenance.sigstore.json',
    'scripthold-v3.4.0-provenance.intoto.jsonl']) {
    const size = name === 'checksums.txt' ? 16385 : 4 * 1024 * 1024 + 1;
    const changed = f.observed.map(x => x.name === name ? { ...x, size } : x);
    await assert.rejects(measureNormalSourcePayloads(TAG, COMMIT, changed, f.open));
  }
  for (const [tag, commit] of [['../v3.4.0', COMMIT], [TAG, 'B'.repeat(40)],
    [TAG, ''], [null, COMMIT]]) {
    let opens = 0;
    await assert.rejects(measureNormalSourcePayloads(tag, commit, f.observed,
      async () => { opens++; return Readable.from([]); }));
    assert.equal(opens, 0);
  }
});

test('rejects a source that never advances while allowing intermittent empty chunks', async () => {
  const f = fixture();
  const original = f.names[0];
  const noProgress = async function* () {
    for (let i = 0; i < 101; i++) yield Buffer.alloc(0);
    throw new Error('unbounded source consumed');
  };
  await assert.rejects(
    measureNormalSourcePayloads(TAG, COMMIT, f.observed,
      async ({ name }) => name === original ? noProgress() : Readable.from([f.bytes.get(name)])),
    /original asset stream made no progress/,
  );

  const intermittent = async function* (bytes) {
    for (let i = 0; i < 99; i++) yield Buffer.alloc(0);
    yield bytes.subarray(0, 1);
    for (let i = 0; i < 99; i++) yield Buffer.alloc(0);
    yield bytes.subarray(1);
  };
  const measured = await measureNormalSourcePayloads(TAG, COMMIT, f.observed,
    async ({ name }) => intermittent(f.bytes.get(name)));
  assert.equal(measured.assets.length, 15);
  assert.equal(measured.assets[0].sha256, hash(f.bytes.get(original)));
});

test('pins all sizes before await and does not let source opener rewrite the plan', async () => {
  const f = fixture();
  const original = f.observed[1].size;
  let altered = false;
  const result = await measureNormalSourcePayloads(TAG, COMMIT, f.observed,
    async ({ name, size }) => {
      if (!altered) {
        altered = true;
        f.observed[1].size = 1;
      }
      assert.equal(size, f.bytes.get(name).length);
      return Readable.from([f.bytes.get(name)]);
    });
  assert.equal(result.assets[1].size, original);
});
