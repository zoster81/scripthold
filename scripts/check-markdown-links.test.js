const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');

const {
  extractLocalTargets,
  validateLocalTarget,
} = require('./check-markdown-links.js');

test('extracts local inline and reference links while ignoring external links and fenced code', () => {
  const markdown = [
    '[guide](docs/GUIDE.md)',
    '[section](docs/GUIDE.md#usage)',
    '[external](https://example.com)',
    '[mail](mailto:test@example.com)',
    '[ref]: <docs/REF.md>',
    '```md',
    '[not-a-link-for-validation](missing.md)',
    '```',
  ].join('\n');

  assert.deepEqual(extractLocalTargets(markdown), [
    'docs/GUIDE.md',
    'docs/GUIDE.md#usage',
    'docs/REF.md',
  ]);
});

test('validates repository-contained local link targets', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-links-'));
  try {
    fs.mkdirSync(path.join(root, 'docs'));
    fs.mkdirSync(path.join(root, 'nested'));
    fs.writeFileSync(path.join(root, 'docs', 'GUIDE.md'), '# Guide\n');
    fs.writeFileSync(path.join(root, 'nested', 'README.md'), '# Nested\n');

    assert.deepEqual(validateLocalTarget(root, path.join(root, 'README.md'), 'docs/GUIDE.md#guide'), {ok: true});
    assert.deepEqual(validateLocalTarget(root, path.join(root, 'nested', 'README.md'), '../docs/GUIDE.md'), {ok: true});
    assert.equal(validateLocalTarget(root, path.join(root, 'README.md'), 'docs/MISSING.md').ok, false);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test('rejects malformed encodings and links that escape the repository root', () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-links-'));
  try {
    fs.writeFileSync(path.join(root, 'README.md'), '# Root\n');
    assert.equal(validateLocalTarget(root, path.join(root, 'README.md'), '../outside.md').ok, false);
    assert.equal(validateLocalTarget(root, path.join(root, 'README.md'), '%E0%A4%A').ok, false);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
  }
});

test('rejects local targets that resolve outside the repository through a symlink or junction', (t) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-links-root-'));
  const outside = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-links-outside-'));
  try {
    fs.mkdirSync(path.join(root, 'docs'));
    fs.writeFileSync(path.join(root, 'README.md'), '# Root\n');
    fs.writeFileSync(path.join(outside, 'TARGET.md'), '# Outside\n');
    try {
      fs.symlinkSync(outside, path.join(root, 'docs', 'external'), process.platform === 'win32' ? 'junction' : 'dir');
    } catch (error) {
      if (error && ['EPERM', 'EACCES', 'ENOTSUP'].includes(error.code)) {
        t.skip(`symlink/junction creation unavailable: ${error.code}`);
        return;
      }
      throw error;
    }
    assert.equal(validateLocalTarget(root, path.join(root, 'README.md'), 'docs/external/TARGET.md').ok, false);
  } finally {
    fs.rmSync(root, {recursive: true, force: true});
    fs.rmSync(outside, {recursive: true, force: true});
  }
});
