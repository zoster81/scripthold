'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const test = require('node:test');

const repositoryRoot = path.resolve(__dirname, '..');
const preparer = path.join(repositoryRoot, 'scripts', 'prepare-mcpb-assets.js');
const catalogPath = path.join(repositoryRoot, 'internal', 'toolcatalog', 'catalog.json');
const targets = [
  ['windows', 'amd64', 'win32', 'scripthold_windows_amd64.exe', 'scripthold_windows_amd64.mcpb', 'scripthold.exe'],
  ['windows', 'arm64', 'win32', 'scripthold_windows_arm64.exe', 'scripthold_windows_arm64.mcpb', 'scripthold.exe'],
  ['linux', 'amd64', 'linux', 'scripthold_linux_amd64', 'scripthold_linux_amd64.mcpb', 'scripthold'],
  ['linux', 'arm64', 'linux', 'scripthold_linux_arm64', 'scripthold_linux_arm64.mcpb', 'scripthold'],
  ['darwin', 'amd64', 'darwin', 'scripthold_darwin_amd64', 'scripthold_darwin_amd64.mcpb', 'scripthold'],
  ['darwin', 'arm64', 'darwin', 'scripthold_darwin_arm64', 'scripthold_darwin_arm64.mcpb', 'scripthold'],
];

function createWorkspace(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-mcpb-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function sha256(data) {
  return crypto.createHash('sha256').update(data).digest('hex');
}

function seedReleaseAssets(directory) {
  const lines = [];
  for (const [osName, arch, , source] of targets) {
    const bytes = Buffer.from(`binary:${osName}:${arch}\n`, 'utf8');
    fs.writeFileSync(path.join(directory, source), bytes);
    lines.push(`${sha256(bytes)}  ${source}`);
  }
  fs.writeFileSync(path.join(directory, 'checksums.txt'), `${lines.join('\n')}\n`, 'utf8');
}

test('prepares six verified native MCPB staging directories', (t) => {
  const directory = createWorkspace(t);
  const assets = path.join(directory, 'assets');
  const output = path.join(directory, 'mcpb');
  fs.mkdirSync(assets);
  seedReleaseAssets(assets);

  const result = spawnSync(process.execPath, [
    preparer,
    'v2.1.0',
    path.join(assets, 'checksums.txt'),
    assets,
    output,
    repositoryRoot,
  ], { cwd: repositoryRoot, encoding: 'utf8' });
  assert.equal(result.status, 0, result.stderr);

  const index = JSON.parse(fs.readFileSync(path.join(output, 'mcpb-assets.json'), 'utf8'));
  assert.equal(index.version, '2.1.0');
  assert.equal(index.packages.length, 6);
  const catalog = JSON.parse(fs.readFileSync(catalogPath, 'utf8'));

  targets.forEach(([osName, arch, platform, source, bundle, binary], position) => {
    const item = index.packages[position];
    assert.deepEqual(
      { os: item.os, arch: item.arch, sourceFile: item.sourceFile, bundleFile: item.bundleFile },
      { os: osName, arch, sourceFile: source, bundleFile: bundle },
    );
    const stage = path.join(output, item.stageDirectory);
    const manifest = JSON.parse(fs.readFileSync(path.join(stage, 'manifest.json'), 'utf8'));
    assert.equal(manifest.manifest_version, '0.3');
    assert.equal(manifest.name, 'scripthold');
    assert.equal(manifest.version, '2.1.0');
    assert.equal(manifest.server.type, 'binary');
    assert.equal(manifest.server.entry_point, `server/${binary}`);
    assert.equal(manifest.server.mcp_config.command, `\${__dirname}/server/${binary}`);
    assert.deepEqual(manifest.server.mcp_config.args, ['${user_config.allowed_directories}']);
    assert.deepEqual(manifest.compatibility.platforms, [platform]);
    assert.equal(manifest.user_config.allowed_directories.type, 'directory');
    assert.equal(manifest.user_config.allowed_directories.multiple, true);
    assert.equal(manifest.user_config.allowed_directories.required, true);
    assert.deepEqual(manifest.tools, catalog.tools.map(({ name, description }) => ({ name, description })));
    assert.deepEqual(
      fs.readFileSync(path.join(stage, 'server', binary)),
      fs.readFileSync(path.join(assets, source)),
    );
    assert.equal(fs.existsSync(path.join(stage, 'LICENSE')), true);
  });
});

test('rejects a release binary whose bytes do not match checksums.txt', (t) => {
  const directory = createWorkspace(t);
  const assets = path.join(directory, 'assets');
  const output = path.join(directory, 'mcpb');
  fs.mkdirSync(assets);
  seedReleaseAssets(assets);
  fs.appendFileSync(path.join(assets, targets[0][3]), 'tampered', 'utf8');

  const result = spawnSync(process.execPath, [preparer, 'v2.1.0', path.join(assets, 'checksums.txt'), assets, output, repositoryRoot], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /checksum mismatch/);
  assert.equal(fs.existsSync(output), false);
});

test('rejects a missing release binary before creating output', (t) => {
  const directory = createWorkspace(t);
  const assets = path.join(directory, 'assets');
  const output = path.join(directory, 'mcpb');
  fs.mkdirSync(assets);
  seedReleaseAssets(assets);
  fs.unlinkSync(path.join(assets, targets[5][3]));

  const result = spawnSync(process.execPath, [preparer, '2.1.0', path.join(assets, 'checksums.txt'), assets, output, repositoryRoot], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /missing release binary/);
  assert.equal(fs.existsSync(output), false);
});