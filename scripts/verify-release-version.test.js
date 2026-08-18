const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const { verifyReleaseVersion } = require('./verify-release-version');

function createChangelogFixture(t, content) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-release-version-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  fs.writeFileSync(path.join(root, 'CHANGELOG.md'), content, 'utf8');
  return root;
}

test('accepts a semantic tag with a matching dated changelog heading', (t) => {
  const root = createChangelogFixture(t, '# Changelog\n\n## 2.0.0 - 2026-07-27\n');
  assert.deepEqual(verifyReleaseVersion('v2.0.0', root), {
    version: '2.0.0',
    changelogVersion: '2.0.0',
  });
});

test('rejects a tag without a matching changelog release', (t) => {
  const root = createChangelogFixture(t, '# Changelog\n\n## 1.8.0 - 2026-07-25\n');
  assert.throws(
    () => verifyReleaseVersion('v2.0.0', root),
    /does not contain a dated release heading for 2\.0\.0/,
  );
});

test('rejects an undated changelog release heading', (t) => {
  const root = createChangelogFixture(t, '# Changelog\n\n## 2.0.0\n');
  assert.throws(
    () => verifyReleaseVersion('v2.0.0', root),
    /does not contain a dated release heading/,
  );
});

test('rejects malformed release tags', (t) => {
  const root = createChangelogFixture(t, '# Changelog\n\n## 2.0.0 - 2026-07-27\n');
  assert.throws(() => verifyReleaseVersion('release-2.0.0', root), /semantic release tag/);
});
