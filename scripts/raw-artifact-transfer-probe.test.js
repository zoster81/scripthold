'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
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

function assertSafeTransferWorkflow(yaml) {
  assert.match(yaml, /^on:\r?\n  pull_request:\r?\n/m);
  assert.match(yaml, /^permissions:\r?\n  contents: read\r?$/m);
  assert.match(yaml, /^  consumer:\r?\n    needs: \[normal, package\]\r?$/m);
  assert.doesNotMatch(yaml, /^\s*contents: write\s*$/m);
  assert.doesNotMatch(yaml, /^\s*(?:pull_request_target|release|push):\s*$/m);

  const producerSteps = [...yaml.matchAll(
    /^      - name: Upload synthetic (normal|package) (\d{2})\r?\n([\s\S]*?)(?=^      - name:|^  (?:consumer|normal|package):)/gm,
  )];
  assert.equal(producerSteps.length, 22);
  assert.equal((yaml.match(/^        uses: actions\/upload-artifact@/gm) || []).length, 22);
  for (const [index, [, group, number, body]] of producerSteps.entries()) {
    const expectedGroup = index < 15 ? 'normal' : 'package';
    const expectedNumber = String(index < 15 ? index + 1 : index - 14).padStart(2, '0');
    assert.equal(group, expectedGroup);
    assert.equal(number, expectedNumber);
    assert.match(body, /^        uses: actions\/upload-artifact@[0-9a-f]{40}\s+# v7\.0\.2\r?$/m);
    assert.match(body, /^          archive: false\r?$/m);
    assert.match(body, /^          overwrite: false\r?$/m);
    assert.match(body, /^          if-no-files-found: error\r?$/m);
    const extension = group === 'normal' && number === '12' ? 'zip' : 'dat';
    const pathLine = '          path: fixtures/probe-${{ github.run_id }}-${{ github.run_attempt }}-' +
      group + '-' + number + '.' + extension;
    assert.ok(body.split(/\r?\n/).includes(pathLine), 'wrong producer filename');
    assert.doesNotMatch(body, /^          name:/m);
  }
  const downloads = [...yaml.matchAll(
    /^        uses: actions\/download-artifact@([0-9a-f]{40}) # v8\r?$/gm,
  )];
  assert.equal(downloads.length, 2);
  assert.match(yaml, /^          artifact-ids: \$\{\{ needs\.normal\.outputs\.ids \}\}$/m);
  assert.match(yaml, /^          path: \$\{\{ runner\.temp \}\}\/raw-normal-probe$/m);
  assert.match(yaml, /^          artifact-ids: \$\{\{ needs\.normal\.outputs\.ids \}\},\$\{\{ needs\.package\.outputs\.ids \}\}\r?$/m);
  for (const [key, value] of [
    ['merge-multiple', 'true'],
    ['skip-decompress', 'true'],
    ['digest-mismatch', 'error'],
  ]) {
    const exact = new RegExp('^          ' + key + ': ' + value + '\\r?$', 'gm');
    assert.equal([...yaml.matchAll(exact)].length, 2,
      'both strict raw-download steps must keep ' + key);
  }
}

function assertPinnedUbuntuProbe(yaml) {
  assert.match(yaml, /^  workflow_dispatch:\r?\npermissions:\r?$/m);
  const runsOn = [...yaml.matchAll(/^    runs-on: (.+)\r?$/gm)].map(([, label]) =>
    label.replace(/\r$/, ''));
  assert.deepEqual(runsOn, Array(3).fill('ubuntu-26.04'));
  assert.doesNotMatch(yaml, /ubuntu-24\.04|ubuntu-latest|self-hosted/);
}

test('synthetic raw artifact probe pins all three jobs to Ubuntu 26.04', () => {
  const yaml = fs.readFileSync(workflow, 'utf8');
  assertPinnedUbuntuProbe(yaml);
  for (const [needle, changed] of [
    ['runs-on: ubuntu-26.04', 'runs-on: ubuntu-24.04'],
    ['runs-on: ubuntu-26.04', 'runs-on: ubuntu-latest'],
    ['runs-on: ubuntu-26.04', 'runs-on: self-hosted'],
    ['runs-on: ubuntu-26.04', 'runs-on: ${{ inputs.ubuntu_image }}'],
    ['  workflow_dispatch:\npermissions:',
      '  workflow_dispatch:\n    inputs:\n      ubuntu_image:\n        type: string\npermissions:'],
  ]) {
    assert.ok(yaml.includes(needle), 'missing expected workflow contract');
    assert.throws(() => assertPinnedUbuntuProbe(yaml.replace(needle, changed)),
      undefined, 'unsafe runner image selection accepted: ' + changed);
  }
});

test('actionlint only allows the officially available Ubuntu 26.04 runner label', () => {
  const config = fs.readFileSync(path.join(__dirname, '..', '.github', 'actionlint.yaml'), 'utf8');
  const active = config.split(/\r?\n/).filter(line =>
    line.trim() && !line.trim().startsWith('#')).join('\n');
  assert.equal(active, 'self-hosted-runner:\n  labels:\n    - ubuntu-26.04');
});

test('isolated raw-transfer workflow retains strict publication and artifact policy', () => {
  const yaml = fs.readFileSync(workflow, 'utf8');
  assertSafeTransferWorkflow(yaml);
  for (const [original, unsafe] of [
    ['archive: false', 'archive: true'],
    ['overwrite: false', 'overwrite: true'],
    ['if-no-files-found: error', 'if-no-files-found: warn'],
    ['skip-decompress: true', 'skip-decompress: false'],
    ['digest-mismatch: error', 'digest-mismatch: warn'],
    ['contents: read', 'contents: write'],
    ['merge-multiple: true', 'merge-multiple: false'],
    ['needs: [normal, package]', 'needs: [normal]'],
  ]) {
    assert.ok(yaml.includes(original), 'missing expected input ' + original);
    assert.throws(() => assertSafeTransferWorkflow(yaml.replace(original, unsafe)),
      undefined, 'unsafe workflow change was not detected: ' + unsafe);
  }
});

function assertNativeNormalStagingProbe(yaml) {
  const consumer = yaml.slice(yaml.indexOf('\n  consumer:\n'));
  const verified = consumer.indexOf("console.log('22/22 exact synthetic raw transfers verified')");
  const isolated = consumer.indexOf('      - name: Download exactly 15 normal IDs for native staging');
  const checkout = consumer.indexOf('      - name: Checkout isolated Go test source');
  const go = consumer.indexOf('      - name: Verify native private handoff of the 15 transferred originals');
  assert.ok(verified >= 0 && isolated > verified && checkout > isolated && go > checkout,
    'staging may consume raw downloads only after complete 22-byte validation');
  assert.match(consumer, /^        uses: actions\/checkout@[0-9a-f]{40} # v7\.0\.1$/m);
  assert.match(consumer, /^        uses: actions\/setup-go@[0-9a-f]{40} # v7\.0\.0$/m);
  const separate = consumer.slice(isolated, checkout);
  assert.match(separate, /^          artifact-ids: \$\{\{ needs\.normal\.outputs\.ids \}\}$/m);
  assert.match(separate, /^          digest-mismatch: error$/m);
  assert.match(separate, /^          path: \$\{\{ runner\.temp \}\}\/raw-normal-probe$/m);
  const goStep = consumer.slice(go);
  assert.match(goStep, /^          RAW_NORMAL_PROBE: '1'$/m);
  assert.match(goStep, /^          NORMAL_ATTEMPT: \$\{\{ needs\.normal\.outputs\.attempt \}\}$/m);
  assert.match(consumer, /^          go test \.\/internal\/releasearchive -run '\^TestCITransferredNormalHandoff\$' -count=1 -v$/m);
}

test('Linux native 15-original staging runs after authentic cross-job byte proof', () => {
  const yaml = fs.readFileSync(workflow, 'utf8');
  assertNativeNormalStagingProbe(yaml);
  for (const [good, bad] of [
    ["      - name: Verify native private handoff of the 15 transferred originals",
      "      - name: Skip native private handoff"],
    ["          RAW_NORMAL_PROBE: '1'", "          RAW_NORMAL_PROBE: '0'"],
    ["          path: ${{ runner.temp }}/raw-normal-probe",
      "          path: ${{ runner.temp }}/raw-probe"],
    ["          NORMAL_ATTEMPT: ${{ needs.normal.outputs.attempt }}",
      "          NORMAL_ATTEMPT: 1"],
  ]) {
    assert.ok(yaml.includes(good));
    const index = yaml.lastIndexOf(good);
    const mutated = yaml.slice(0, index) + bad + yaml.slice(index + good.length);
    assert.throws(() => assertNativeNormalStagingProbe(mutated));
  }
});

test('producer ZIP fixture and both producer attempts match consumer expectations', () => {
  const yaml = fs.readFileSync(workflow, 'utf8');
  const encoded = /printf '%s' '([A-Za-z0-9+/=]+)' \| base64 --decode/.exec(yaml);
  assert.ok(encoded, 'synthetic ZIP generator must be present');
  assert.deepEqual(Buffer.from(encoded[1], 'base64'), zipBytes);
  assert.equal((yaml.match(/attempt: \$\{\{ steps\.collect\.outputs\.attempt \}\}/g) || []).length, 2);
  assert.equal((yaml.match(/^\s+D\d+: \$\{\{ steps\.a\d+\.outputs\.artifact-digest \}\}$/gm) || []).length, 22);
  assert.equal((yaml.match(/actual="\$\(sha256sum "\$file"\)"/g) || []).length, 2);
});

test('native job outputs bind ordered SHA-256 vectors and successful direct dependencies', () => {
  const yaml = fs.readFileSync(workflow, 'utf8');
  for (const group of ['normal', 'package']) {
    const start = yaml.indexOf('  ' + group + ':');
    const end = yaml.indexOf(group === 'normal' ? '  package:' : '  consumer:', start);
    assert.ok(start >= 0 && end > start);
    const producer = yaml.slice(start, end);
    assert.ok(producer.includes('      digests: ${{ steps.collect.outputs.digests }}'));
    assert.ok(producer.includes('echo "digests=${digests[*]}"'));
  }
  for (const group of ['normal', 'package']) {
    assert.ok(yaml.includes('          ' + group.toUpperCase() +
      '_DIGESTS: ${{ needs.' + group + '.outputs.digests }}'));
    assert.ok(yaml.includes('          ' + group.toUpperCase() +
      '_RESULT: ${{ needs.' + group + '.result }}'));
  }
  assert.ok(verifier.includes("createHash('sha256')"));
  assert.ok(verifier.includes('artifact digest mismatch'));
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
  const digest = (name) => crypto.createHash('sha256')
    .update(fs.readFileSync(path.join(root, name))).digest('hex');
  return { parent, root, names, env: {
    RUNNER_TEMP: parent, GITHUB_RUN_ID: '9001', GITHUB_RUN_ATTEMPT: '9',
    NORMAL_ATTEMPT: normalAttempt, PACKAGE_ATTEMPT: packageAttempt,
    NORMAL_RESULT: 'success', PACKAGE_RESULT: 'success',
    NORMAL_IDS: Array.from({ length: 15 }, (_, i) => String(i + 1)).join(','),
    PACKAGE_IDS: Array.from({ length: 7 }, (_, i) => String(i + 16)).join(','),
    NORMAL_DIGESTS: names.slice(0, 15).map(digest).join(','),
    PACKAGE_DIGESTS: names.slice(15).map(digest).join(','),
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
  assert.throws(() => runVerifier(fixture), /artifact digest mismatch/);
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

test('rejects missing, malformed, stale or reordered digest and ID evidence', (t) => {
  const fixture = createFixture(t, '1', '3');
  const normal = fixture.env.NORMAL_DIGESTS.split(',');
  const pkg = fixture.env.PACKAGE_DIGESTS.split(',');
  const swapped = [...normal];
  [swapped[0], swapped[1]] = [swapped[1], swapped[0]];
  const tests = [
    [{ NORMAL_DIGESTS: '' }, /invalid producer digests/],
    [{ PACKAGE_DIGESTS: pkg.slice(0, -1).join(',') }, /invalid producer digests/],
    [{ NORMAL_DIGESTS: normal.concat(normal[0]).join(',') }, /invalid producer digests/],
    [{ NORMAL_DIGESTS: normal.join(',').toUpperCase() }, /invalid producer digests/],
    [{ NORMAL_DIGESTS: '0'.repeat(64) + ',' + normal.slice(1).join(',') }, /artifact digest mismatch/],
    [{ NORMAL_DIGESTS: swapped.join(',') }, /artifact digest mismatch/],
    [{ PACKAGE_DIGESTS: pkg.concat('0'.repeat(64)).join(',') }, /invalid producer digests/],
    [{ NORMAL_IDS: fixture.env.NORMAL_IDS.split(',').slice(1).join(','),
       PACKAGE_IDS: '1,' + fixture.env.PACKAGE_IDS }, /invalid IDs/],
    [{ NORMAL_RESULT: 'skipped' }, /untrusted producer result/],
    [{ PACKAGE_RESULT: 'failure' }, /untrusted producer result/],
    [{ NORMAL_ATTEMPT: '10' }, /invalid producer attempt/],
    [{ GITHUB_RUN_ATTEMPT: '0' }, /invalid run attempt/],
  ];
  for (const [overrides, message] of tests) {
    assert.throws(() => runVerifier(fixture, overrides), message,
      'missing fail-closed check: ' + JSON.stringify(overrides));
  }
  assert.deepEqual(runVerifier(fixture), ['22/22 exact synthetic raw transfers verified']);
});

test('rejects digest reuse when original bytes differ', (t) => {
  const fixture = createFixture(t);
  const original = fixture.env.NORMAL_DIGESTS.split(',');
  original[1] = original[0];
  assert.throws(() => runVerifier(fixture, { NORMAL_DIGESTS: original.join(',') }),
    /artifact digest mismatch/);
});

test('rejects substituted bytes even when forged producer digest matches them', (t) => {
  const fixture = createFixture(t);
  for (const index of [0, 11, 15]) {
    const file = path.join(fixture.root, fixture.names[index]);
    const original = fs.readFileSync(file);
    const altered = Buffer.from('forged synthetic payload');
    fs.writeFileSync(file, altered);
    const group = index < 15 ? 'NORMAL_DIGESTS' : 'PACKAGE_DIGESTS';
    const digestIndex = index < 15 ? index : index - 15;
    const digests = fixture.env[group].split(',');
    digests[digestIndex] = crypto.createHash('sha256').update(altered).digest('hex');
    assert.throws(() => runVerifier(fixture, { [group]: digests.join(',') }),
      /changed original bytes/);
    fs.writeFileSync(file, original);
  }
});

test('refuses corruption of an original valid ZIP, not only its filename', (t) => {
  const fixture = createFixture(t);
  const file = path.join(fixture.root, fixture.names[11]);
  fs.writeFileSync(file, Buffer.alloc(zipBytes.length));
  assert.throws(() => runVerifier(fixture), /artifact digest mismatch/);
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
