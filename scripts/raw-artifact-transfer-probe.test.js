'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');

const workflow = path.join(__dirname, '..', '.github', 'workflows',
  'raw-artifact-transfer-probe.yml');
const zipBytes = Buffer.from('504b0506000000000000000000000000000000000000', 'hex');

function loadVerifier() {
  const yaml = fs.readFileSync(workflow, 'utf8');
  const match = /^          node <<'NODE'\r?\n([\s\S]*?)^          NODE$/m.exec(yaml);
  assert.ok(match, 'expected exactly one inline Node verification script');
  const script = match[1].split(/\r?\n/).map((line) => {
    assert.ok(line.startsWith('          ') || line === '',
      'verification script must retain its YAML block indentation');
    return line.slice(10);
  }).join('\n');
  new vm.Script(script);
  return script;
}

const verifier = loadVerifier();

test('producer ZIP fixture and both producer attempts match consumer expectations', () => {
  const yaml = fs.readFileSync(workflow, 'utf8');
  const encoded = /printf '%s' '([A-Za-z0-9+/=]+)' \| base64 --decode/.exec(yaml);
  assert.ok(encoded, 'synthetic ZIP generator must be present');
  assert.deepEqual(Buffer.from(encoded[1], 'base64'), zipBytes);
  assert.equal((yaml.match(/attempt: \$\{\{ steps\.collect\.outputs\.attempt \}\}/g) || []).length, 2);
  assert.equal((yaml.match(/^\s+D\d+: \$\{\{ steps\.a\d+\.outputs\.artifact-digest \}\}$/gm) || []).length, 22);
  assert.equal((yaml.match(/actual="\$\(sha256sum "\$file"\)"/g) || []).length, 2);
});

function createFixture(t, normalAttempt = '1', packageAttempt = '2') {
  const parent = fs.mkdtempSync(path.join(process.cwd(), '.raw-probe-test-'));
  t.after(() => fs.rmSync(parent, { recursive: true, force: true }));
  const root = path.join(parent, 'raw-probe');
  fs.mkdirSync(root, { mode: 0o700 });
  const names = [];
  for (const [group, count, attempt] of [
    ['normal', 15, normalAttempt], ['package', 7, packageAttempt],
  ]) {
    for (let i = 1; i <= count; i++) {
      const number = String(i).padStart(2, '0');
      const ext = group === 'normal' && i === 12 ? 'zip' : 'dat';
      const name = 'probe-9001-' + attempt + '-' + group + '-' + number + '.' + ext;
      const bytes = group === 'normal' && i === 12
        ? zipBytes : Buffer.from('raw-probe:' + group + ':' + number + '\n');
      fs.writeFileSync(path.join(root, name), bytes);
      names.push(name);
    }
  }
  return { parent, root, names, env: {
    RUNNER_TEMP: parent, GITHUB_RUN_ID: '9001', GITHUB_RUN_ATTEMPT: '9',
    NORMAL_ATTEMPT: normalAttempt, PACKAGE_ATTEMPT: packageAttempt,
    NORMAL_IDS: Array.from({ length: 15 }, (_, i) => String(i + 1)).join(','),
    PACKAGE_IDS: Array.from({ length: 7 }, (_, i) => String(i + 16)).join(','),
  } };
}

function runVerifier(fixture, overrides = {}) {
  // Windows does not expose Unix mode bits reliably. Override ONLY the private
  // directory's mode bits there; actual Linux permission policy is CI-only.
  const checkedFs = {
    ...fs,
    lstatSync(file) {
      const stat = fs.lstatSync(file);
      if (process.platform === 'win32' && file === fixture.root) {
        return {
          mode: stat.mode & ~0o077,
          isDirectory: () => stat.isDirectory(),
          isSymbolicLink: () => stat.isSymbolicLink(),
        };
      }
      return stat;
    },
  };
  const logs = [];
  const sandbox = {
    Buffer,
    process: { env: { ...fixture.env, ...overrides } },
    require: (name) => name === 'node:fs' ? checkedFs : require(name),
    console: { log: (line) => logs.push(line) },
  };
  new vm.Script(verifier).runInNewContext(sandbox, { timeout: 5000 });
  return logs;
}

test('validates all 22 actual fixture files with independent producer attempts', (t) => {
  const fixture = createFixture(t, '1', '3');
  assert.deepEqual(runVerifier(fixture), ['22/22 exact synthetic raw transfers verified']);
  assert.equal(fs.readFileSync(path.join(fixture.root, fixture.names[11])).equals(zipBytes), true);
});

test('rejects missing, extra, corrupt and renamed physical files', (t) => {
  const fixture = createFixture(t);
  const target = path.join(fixture.root, fixture.names[0]);
  fs.writeFileSync(target, 'changed but same-sized');
  assert.throws(() => runVerifier(fixture), /changed original bytes/);
  fs.writeFileSync(target, 'raw-probe:normal:01\n');
  fs.rmSync(target);
  assert.throws(() => runVerifier(fixture), /ENOENT/);
  fs.writeFileSync(target, 'raw-probe:normal:01\n');
  fs.renameSync(target, target + '.renamed');
  assert.throws(() => runVerifier(fixture), /ENOENT/);
  fs.renameSync(target + '.renamed', target);
  fs.writeFileSync(path.join(fixture.root, 'extra.dat'), 'unexpected');
  assert.throws(() => runVerifier(fixture), /unexpected or missing physical files/);
});

test('rejects invalid ID inventories and missing or mixed producer attempts', (t) => {
  const fixture = createFixture(t);
  for (const bad of [
    { NORMAL_IDS: '' },
    { NORMAL_IDS: '1,1' },
    { NORMAL_IDS: '9007199254740992' },
    { PACKAGE_IDS: '16,17,18,19,20,21' },
    { NORMAL_ATTEMPT: '0' },
    { PACKAGE_ATTEMPT: '../2' },
    { NORMAL_ATTEMPT: '2' },
    { PACKAGE_ATTEMPT: '1' },
  ]) {
    assert.throws(() => runVerifier(fixture, bad));
  }
});

test('refuses corruption of an original valid ZIP, not only its filename', (t) => {
  const fixture = createFixture(t);
  const file = path.join(fixture.root, fixture.names[11]);
  fs.writeFileSync(file, Buffer.alloc(zipBytes.length));
  assert.throws(() => runVerifier(fixture), /changed original bytes/);
});

test('rejects a symlink that replaces a downloaded file', (t) => {
  const fixture = createFixture(t);
  const source = path.join(fixture.root, fixture.names[0]);
  const moved = path.join(fixture.parent, 'moved-elsewhere');
  fs.renameSync(source, moved);
  try {
    fs.symlinkSync(moved, source, 'file');
  } catch (error) {
    if (process.platform === 'win32' && ['EPERM', 'EACCES'].includes(error.code)) {
      t.skip('creating symlinks requires Windows privileges');
      return;
    }
    throw error;
  }
  assert.throws(() => runVerifier(fixture), /unsafe file/);
});

test('rejects a nonprivate destination on Unix', (t) => {
  if (process.platform === 'win32') {
    t.skip('Unix mode bits are not available on Windows');
    return;
  }
  const fixture = createFixture(t);
  fs.chmodSync(fixture.root, 0o755);
  assert.throws(() => runVerifier(fixture), /unsafe destination/);
});

test('rejects a hard-linked downloaded file before validating its bytes', (t) => {
  const fixture = createFixture(t);
  const source = path.join(fixture.root, fixture.names[0]);
  const alias = path.join(fixture.parent, 'hardlink');
  fs.linkSync(source, alias);
  assert.throws(() => runVerifier(fixture), /unsafe file/);
});
