'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const test = require('node:test');

const repositoryRoot = path.resolve(__dirname, '..');
const generator = path.join(repositoryRoot, 'scripts', 'generate-server-json.js');
const toolCatalog = path.join(repositoryRoot, 'internal', 'toolcatalog', 'catalog.json');
const expectedFiles = [
  'scripthold_windows_amd64.mcpb',
  'scripthold_windows_arm64.mcpb',
  'scripthold_linux_amd64.mcpb',
  'scripthold_linux_arm64.mcpb',
  'scripthold_darwin_amd64.mcpb',
  'scripthold_darwin_arm64.mcpb',
];
const expectedRuntimes = [
  { os: ['windows'], arch: ['amd64'] },
  { os: ['windows'], arch: ['arm64'] },
  { os: ['linux'], arch: ['amd64'] },
  { os: ['linux'], arch: ['arm64'] },
  { os: ['darwin'], arch: ['amd64'] },
  { os: ['darwin'], arch: ['arm64'] },
];

function createWorkspace(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'scripthold-registry-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

function checksumFor(index) {
  return (index + 1).toString(16).repeat(64).slice(0, 64);
}

test('generates a fork-owned manifest from release checksums', (t) => {
  const directory = createWorkspace(t);
  const checksumsPath = path.join(directory, 'checksums.txt');
  const outputPath = path.join(directory, 'server.json');
  const lines = expectedFiles.map((filename, index) => `${checksumFor(index)}  ${filename}`);
  lines.push(`${'f'.repeat(64)}  scripthold_windows_amd64.zip`);
  fs.writeFileSync(checksumsPath, `${lines.join('\n')}\n`, 'utf8');

  const result = spawnSync(process.execPath, [generator, 'v2.3.4', checksumsPath, outputPath, repositoryRoot], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  });
  assert.equal(result.status, 0, result.stderr);

  const manifest = JSON.parse(fs.readFileSync(outputPath, 'utf8'));
  assert.equal(manifest.name, 'io.github.zoster81/scripthold');
  assert.equal(manifest.version, '2.3.4');
  assert.equal(manifest.repository.url, 'https://github.com/zoster81/scripthold');
  assert.equal(manifest.packages.length, expectedFiles.length);
  manifest.packages.forEach((pkg, index) => {
    assert.equal(pkg.registryType, 'mcpb');
    assert.equal(pkg.identifier, `https://github.com/zoster81/scripthold/releases/download/v2.3.4/${expectedFiles[index]}`);
    assert.equal(pkg.fileSha256, checksumFor(index));
    assert.deepEqual(pkg.runtime, expectedRuntimes[index]);
  });

  const catalog = JSON.parse(fs.readFileSync(toolCatalog, 'utf8'));
  assert.deepEqual(
    manifest.tools,
    catalog.tools.map(({ name, description }) => ({ name, description })),
  );
});

test('fails when an MCPB package checksum is missing', (t) => {
  const directory = createWorkspace(t);
  const checksumsPath = path.join(directory, 'checksums.txt');
  const outputPath = path.join(directory, 'server.json');
  fs.writeFileSync(checksumsPath, `${checksumFor(0)}  ${expectedFiles[0]}\n`, 'utf8');

  const result = spawnSync(process.execPath, [generator, '2.3.4', checksumsPath, outputPath], {
    cwd: repositoryRoot,
    encoding: 'utf8',
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /missing release checksum/);
  assert.equal(fs.existsSync(outputPath), false);
});
