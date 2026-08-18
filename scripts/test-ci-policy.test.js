const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const {
  classifyEvent,
  classifyPaths,
  changedPathsUseOnlyRegularFiles,
  matrixOutputsForTier,
  parseLsTreeZ,
  parseNameStatusZ,
  requiredJobsForTier,
  selectGoPackages,
  verifyJobResults,
} = require('./test-ci-policy.js');

test('docs-only paths use the docs tier', () => {
  assert.equal(classifyPaths(['README.md', 'docs/HTTP_SECURITY.md']).tier, 'docs');
});

test('Go changes plus documentation use the go tier', () => {
  assert.equal(classifyPaths(['internal/security/path.go', 'docs/SAFE_FILESYSTEM_OPERATIONS.md']).tier, 'go');
  assert.equal(classifyPaths(['filetoolsserver/handler/path_test.go']).tier, 'go');
});

test('workflow, script, metadata, fixture, and unknown changes fail closed to full', () => {
  for (const paths of [
    ['.github/workflows/test.yml'],
    ['scripts/test-ci-policy.js'],
    ['testarchitecture/evidence-map.json'],
    ['go.mod'],
    ['Dockerfile'],
    ['internal/sourceintelligence/testdata/provider-contracts.json'],
    ['docs/HTTP_SECURITY.md', 'config/example.json'],
  ]) {
    assert.equal(classifyPaths(paths).tier, 'full', paths.join(', '));
  }
});

test('empty or malformed paths fail closed to full', () => {
  for (const paths of [[], ['../README.md'], ['/README.md'], ['docs\\README.md'], ['docs//README.md'], ['']]) {
    assert.equal(classifyPaths(paths).tier, 'full', JSON.stringify(paths));
  }
});

test('rename parsing evaluates both source and destination paths', () => {
  const goRename = Buffer.from('R100\0internal/security/path.go\0docs/path.md\0');
  assert.deepEqual(parseNameStatusZ(goRename), ['internal/security/path.go', 'docs/path.md']);
  assert.equal(classifyPaths(parseNameStatusZ(goRename)).tier, 'go');

  const scriptRename = Buffer.from('R087\0scripts/old.js\0docs/old.md\0');
  assert.equal(classifyPaths(parseNameStatusZ(scriptRename)).tier, 'full');
});

test('malformed and type-changing name-status data fails closed', () => {
  assert.throws(() => parseNameStatusZ(Buffer.from('R100\0only-one-path\0')));
  assert.throws(() => parseNameStatusZ(Buffer.from('X\0README.md\0')));
  assert.throws(() => parseNameStatusZ(Buffer.from('T\0docs/GUIDE.md\0')));
  assert.throws(() => parseNameStatusZ(Buffer.from('U\0docs/GUIDE.md\0')));
});

test('tree modes reject symlinks and gitlinks from narrow evidence tiers', () => {
  const regularTree = parseLsTreeZ(Buffer.from(
    '100644 blob 0123456789012345678901234567890123456789\tREADME.md\0' +
    '100755 blob 1111111111111111111111111111111111111111\tinternal/tool.go\0',
  ));
  const unsafeTree = parseLsTreeZ(Buffer.from(
    '120000 blob 2222222222222222222222222222222222222222\tdocs/GUIDE.md\0' +
    '160000 commit 3333333333333333333333333333333333333333\tvendor/tool\0',
  ));

  assert.equal(changedPathsUseOnlyRegularFiles(['README.md', 'internal/tool.go'], regularTree, new Map()), true);
  assert.equal(changedPathsUseOnlyRegularFiles(['docs/GUIDE.md'], regularTree, unsafeTree), false);
  assert.equal(changedPathsUseOnlyRegularFiles(['vendor/tool'], unsafeTree, new Map()), false);
  assert.throws(() => parseLsTreeZ(Buffer.from('100644 blob bad\tREADME.md\0')));
});

test('main pushes, manual runs, and unknown events are always full qualification', () => {
  assert.equal(classifyEvent('push', ['README.md']).tier, 'full');
  assert.equal(classifyEvent('workflow_dispatch', ['README.md']).tier, 'full');
  assert.equal(classifyEvent('schedule', ['README.md']).tier, 'full');
  assert.equal(classifyEvent('pull_request', ['README.md']).tier, 'docs');
});

test('required jobs are monotonic across evidence tiers', () => {
  const docs = requiredJobsForTier('docs');
  const go = requiredJobsForTier('go');
  const full = requiredJobsForTier('full');

  for (const job of docs) assert.ok(go.includes(job), `go tier missing docs job ${job}`);
  for (const job of go) assert.ok(full.includes(job), `full tier missing go job ${job}`);
  assert.ok(full.includes('native-smoke'));
  assert.ok(full.includes('cross-build'));
  assert.ok(full.includes('container-smoke'));
  assert.ok(full.includes('release-config'));
});

test('matrix selection preserves full exact-SHA breadth while narrowing ordinary Go evidence', () => {
  const go = matrixOutputsForTier('go');
  assert.deepEqual(go.race, [{os: 'ubuntu-latest', scope: 'targeted'}]);
  assert.deepEqual(go.platform, [
    {os: 'windows-latest', platform: 'windows', scope: 'targeted'},
    {os: 'macos-latest', platform: 'darwin', scope: 'targeted'},
  ]);

  const full = matrixOutputsForTier('full');
  assert.deepEqual(full.race, [
    {os: 'ubuntu-latest', scope: 'full'},
    {os: 'windows-latest', scope: 'full'},
    {os: 'macos-latest', scope: 'full'},
  ]);
  assert.deepEqual(full.platform, [
    {os: 'windows-latest', platform: 'windows', scope: 'full'},
    {os: 'macos-latest', platform: 'darwin', scope: 'full'},
  ]);
});

test('targeted package selection is derived from the evidence map', () => {
  const evidenceMap = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'testarchitecture', 'evidence-map.json'), 'utf8'));
  const race = selectGoPackages(evidenceMap, 'race', 'linux');
  assert.ok(race.includes('./filetoolsserver/handler'));
  assert.ok(race.includes('./internal/backupstore'));
  assert.ok(race.includes('./internal/sourceintelligence'));
  assert.ok(!race.includes('./internal/security'));

  const windowsPlatform = selectGoPackages(evidenceMap, 'platform', 'windows');
  assert.ok(windowsPlatform.includes('./cmd/scripthold'));
  assert.ok(windowsPlatform.includes('./internal/filesystem'));
  assert.ok(!windowsPlatform.includes('./internal/httptransport'));
});

test('aggregate verification rejects failed or missing required evidence', () => {
  const success = Object.fromEntries(requiredJobsForTier('docs').map((job) => [job, 'success']));
  success['linux-regression'] = 'skipped';
  assert.doesNotThrow(() => verifyJobResults('docs', success));

  assert.throws(() => verifyJobResults('docs', {...success, gitleaks: 'failure'}));
  assert.throws(() => verifyJobResults('docs', {...success, policy: 'skipped'}));
  assert.throws(() => verifyJobResults('unknown', success));
});
