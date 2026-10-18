'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const test = require('node:test');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { verifyCanonicalNormalChecksums } = require('./release-normal-rehearsal');

const tag = 'v3.3.0';
const subjects = expectedReleaseAssetNames(tag).slice(0, 12);
const sha = value => createHash('sha256').update(value).digest('hex');
const lines = names => Buffer.from(names.map(name => sha(name) + '  ' + name).join('\n') + '\n');

test('accepts the exact twelve canonical original binary/archive checksums', () => {
  const result = verifyCanonicalNormalChecksums(tag, lines(subjects));
  assert.deepEqual([...result.keys()], subjects);
});

test('rejects malformed tag, missing or duplicate checksum subjects', () => {
  assert.throws(() => verifyCanonicalNormalChecksums('../v3.3.0', lines(subjects)),
    /invalid release tag/);
  assert.throws(() => verifyCanonicalNormalChecksums(tag, lines(subjects.slice(1))),
    /checksum/);
  assert.throws(() => verifyCanonicalNormalChecksums(tag, lines([...subjects, subjects[0]])),
    /duplicate checksum/);
});

test('rejects foreign names and invalid checksums without modifying files', () => {
  const changed = [...subjects];
  changed[0] = 'foreign.zip';
  assert.throws(() => verifyCanonicalNormalChecksums(tag, lines(changed)), /checksum/);
  assert.throws(() => verifyCanonicalNormalChecksums(tag, Buffer.from('invalid\n')),
    /checksum/);
  assert.throws(() => verifyCanonicalNormalChecksums(tag, Buffer.alloc(16 * 1024 + 1)),
    /bounded original checksum/);
  assert.throws(() => verifyCanonicalNormalChecksums(tag, null),
    /bounded original checksum/);
});
