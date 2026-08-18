'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {
  buildGoTestArgs,
  parseArguments,
  selectTargets,
  validateManifest,
} = require('./run-fuzz.js');

function validManifest() {
  return {
    schemaVersion: 1,
    profiles: {
      smoke: { fuzzTime: '10000x' },
      qualification: { fuzzTime: '100000x' },
    },
    targets: [
      {
        id: 'path-normalization',
        package: './internal/security',
        target: 'FuzzNormalizePath',
        riskCategories: ['path-security'],
        profiles: ['smoke', 'qualification'],
      },
      {
        id: 'utf32-validation',
        package: './internal/encoding',
        target: 'FuzzDetectUTF32Validation',
        riskCategories: ['encoding-detection'],
        profiles: ['qualification'],
      },
    ],
  };
}

test('validateManifest accepts bounded risk-based profiles', () => {
  assert.doesNotThrow(() => validateManifest(validManifest()));
});

test('validateManifest rejects duplicate targets and unsafe executable fields', () => {
  const duplicate = validManifest();
  duplicate.targets.push({ ...duplicate.targets[0], id: 'duplicate-path' });
  assert.throws(() => validateManifest(duplicate), /duplicate fuzz entrypoint/);

  const unsafePackage = validManifest();
  unsafePackage.targets[0].package = '../outside';
  assert.throws(() => validateManifest(unsafePackage), /invalid package/);

  const unsafeTarget = validManifest();
  unsafeTarget.targets[0].target = 'FuzzNormalizePath$|.*';
  assert.throws(() => validateManifest(unsafeTarget), /invalid fuzz target/);

  const unsafeTime = validManifest();
  unsafeTime.profiles.smoke.fuzzTime = 'forever';
  assert.throws(() => validateManifest(unsafeTime), /invalid fuzzTime/);
});

test('selectTargets returns only the requested profile in manifest order', () => {
  const manifest = validManifest();
  validateManifest(manifest);
  assert.deepEqual(
    selectTargets(manifest, 'smoke').map((entry) => entry.id),
    ['path-normalization'],
  );
  assert.deepEqual(
    selectTargets(manifest, 'qualification').map((entry) => entry.id),
    ['path-normalization', 'utf32-validation'],
  );
  assert.throws(() => selectTargets(manifest, 'unknown'), /unknown fuzz profile/);
});

test('buildGoTestArgs produces shell-free exact-target invocation', () => {
  const target = validManifest().targets[0];
  assert.deepEqual(buildGoTestArgs(target, '250x'), [
    'test',
    './internal/security',
    '-run',
    '^$',
    '-fuzz',
    '^FuzzNormalizePath$',
    '-fuzztime=250x',
  ]);
});

test('parseArguments defaults to smoke and accepts bounded overrides', () => {
  assert.deepEqual(parseArguments([]), { profile: 'smoke', fuzzTime: null, listOnly: false });
  assert.deepEqual(parseArguments(['--profile', 'qualification', '--fuzztime', '250x', '--list']), {
    profile: 'qualification',
    fuzzTime: '250x',
    listOnly: true,
  });
  assert.throws(() => parseArguments(['--profile']), /requires a value/);
  assert.throws(() => parseArguments(['--unknown']), /unknown argument/);
});
