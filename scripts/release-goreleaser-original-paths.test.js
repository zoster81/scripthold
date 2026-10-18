'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const test = require('node:test');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { planOriginalGoReleaserPaths } = require('./release-goreleaser-original-paths');

const tag = 'v3.3.0';
const names = expectedReleaseAssetNames(tag).slice(0, 12);
const sha = data => createHash('sha256').update(data).digest('hex');
const checksums = Buffer.from(names.map(name => sha(name) + '  ' + name).join('\n') + '\n');

function originalEntry(name) {
  if (name === 'checksums.txt') {
    return { name, type: 'Checksum', path: 'dist/checksums.txt' };
  }
  const [, os, arch] = /^scripthold_(darwin|linux|windows)_(amd64|arm64)/.exec(name);
  const archive = /\.(tar\.gz|zip)$/.test(name);
  const extension = os === 'windows' ? '.exe' : '';
  return {
    name,
    type: archive ? 'Archive' : 'Binary',
    goos: os,
    goarch: arch,
    target: os + '_' + arch + '_' + (arch === 'amd64' ? 'v1' : 'v8.0'),
    path: archive ? 'dist/' + name :
      'dist/scripthold_' + os + '_' + arch + '_' +
        (arch === 'amd64' ? 'v1' : 'v8.0') + '/scripthold' + extension,
  };
}

function fixture() {
  const selected = [...names, 'checksums.txt'].map(originalEntry);
  const internal = [
    ...['darwin', 'linux', 'windows'].flatMap(os =>
      ['amd64', 'arm64'].map(arch => ({
        name: os === 'windows' ? 'scripthold.exe' : 'scripthold',
        type: 'Binary', goos: os, goarch: arch,
        path: originalEntry('scripthold_' + os + '_' + arch).path,
      }))),
    { name: 'metadata.json', type: 'Metadata', path: 'dist/metadata.json' },
  ];
  return [...internal.slice(0, 3), ...selected, ...internal.slice(3)];
}

function plan(entries = fixture(), checksumBytes = checksums, version = tag) {
  return planOriginalGoReleaserPaths(version, checksumBytes, entries);
}

test('selects exactly thirteen physical public outputs from twenty real GoReleaser-style entries', () => {
  const result = plan();
  assert.deepEqual(result.map(x => x.name), [...names, 'checksums.txt']);
  assert.equal(result.length, 13);
  assert.deepEqual(result.filter(x => x.type === 'Archive').length, 6);
  assert.deepEqual(result.filter(x => x.type === 'Binary').length, 6);
  assert.equal(result.at(-1).sha256, sha(checksums));
  assert.equal(result.at(-1).size, checksums.length);
  assert.equal(result.find(x => x.name === 'scripthold_linux_arm64').sourceParent,
    'dist/scripthold_linux_arm64_v8.0');
  assert.equal(result.find(x => x.name === 'scripthold_windows_amd64.zip').sourceLeaf,
    'scripthold_windows_amd64.zip');
  assert.equal(result[0].sha256, sha(names[0]));
  assert.equal(result[0].size, undefined, 'GoReleaser metadata does not attest source size');
  assert.ok(result.every(x => !x.sourceParent.startsWith('/') && !x.sourceParent.includes('\\')));
});

test('rejects missing, duplicate or case-aliased public objects even alongside internal entries', () => {
  const entries = fixture();
  assert.throws(() => plan(entries.filter(x => x.name !== 'checksums.txt')), /missing|exactly thirteen/);
  assert.throws(() => plan([...entries, { ...originalEntry(names[0]) }]), /duplicate/);
  assert.throws(() => plan([...entries, { ...originalEntry(names[0]), name: names[0].toUpperCase() }]),
    /alias|duplicate/);
  assert.throws(() => plan([...entries, ...Array(50).fill({ name: 'metadata.json' })]),
    /bounded|metadata/);
});

test('rejects forged GoReleaser paths, types and swapped platform identities', () => {
  for (const change of [
    x => { x.path = '../' + x.path; },
    x => { x.path = 'dist/../' + x.path.slice(5); },
    x => { x.path = x.path.replaceAll('/', '\\'); },
    x => { x.path = '/private/runner/dist/scripthold'; },
    x => { x.path = 'dist/scripthold_linux_amd64_v1/scripthold'; },
    x => { x.type = 'Archive'; },
    x => { x.goarch = 'arm64'; },
    x => { x.goos = 'windows'; },
    x => { x.target = 'windows_arm64_v8.0'; },
  ]) {
    const items = fixture();
    const target = items.find(x => x.name === 'scripthold_darwin_amd64');
    change(target);
    assert.throws(() => plan(items), /path|type|platform|target/);
  }
  const archive = fixture();
  archive.find(x => x.name === 'scripthold_linux_arm64.tar.gz').type = 'Binary';
  assert.throws(() => plan(archive), /type/);
});

test('rejects noncanonical checksum contracts, malicious metadata and unsupported tags', () => {
  assert.throws(() => plan(fixture(), checksums, '../v3.3.0'), /invalid release tag/);
  assert.throws(() => plan(fixture(), Buffer.from('broken\n')), /checksum/);
  assert.throws(() => plan(null), /metadata/);
  assert.throws(() => plan({}), /metadata/);
  assert.throws(() => plan([{ name: 'checksums.txt' }]), /missing|metadata|type|path/);
  const items = fixture();
  items.find(x => x.name === 'checksums.txt').path = 'dist/sub/checksums.txt';
  assert.throws(() => plan(items), /path/);
});

test('return values do not borrow the mutable GoReleaser metadata', () => {
  const items = fixture();
  const result = plan(items);
  items.find(x => x.name === names[0]).path = 'tampered';
  assert.notEqual(result[0].sourceParent, 'tampered');
});
