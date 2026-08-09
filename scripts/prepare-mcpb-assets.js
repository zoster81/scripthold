#!/usr/bin/env node
'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const targets = [
  { os: 'windows', arch: 'amd64', platform: 'win32', sourceFile: 'scripthold_windows_amd64.exe', bundleFile: 'scripthold_windows_amd64.mcpb', binaryFile: 'scripthold.exe' },
  { os: 'windows', arch: 'arm64', platform: 'win32', sourceFile: 'scripthold_windows_arm64.exe', bundleFile: 'scripthold_windows_arm64.mcpb', binaryFile: 'scripthold.exe' },
  { os: 'linux', arch: 'amd64', platform: 'linux', sourceFile: 'scripthold_linux_amd64', bundleFile: 'scripthold_linux_amd64.mcpb', binaryFile: 'scripthold' },
  { os: 'linux', arch: 'arm64', platform: 'linux', sourceFile: 'scripthold_linux_arm64', bundleFile: 'scripthold_linux_arm64.mcpb', binaryFile: 'scripthold' },
  { os: 'darwin', arch: 'amd64', platform: 'darwin', sourceFile: 'scripthold_darwin_amd64', bundleFile: 'scripthold_darwin_amd64.mcpb', binaryFile: 'scripthold' },
  { os: 'darwin', arch: 'arm64', platform: 'darwin', sourceFile: 'scripthold_darwin_arm64', bundleFile: 'scripthold_darwin_arm64.mcpb', binaryFile: 'scripthold' },
];

function fail(message) {
  throw new Error(message);
}

function parseVersion(raw) {
  const version = String(raw || '').replace(/^v/, '');
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
    fail(`invalid release version "${raw || ''}"`);
  }
  return version;
}

function parseChecksums(filename) {
  const checksums = new Map();
  for (const rawLine of fs.readFileSync(filename, 'utf8').split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line) continue;
    const match = line.match(/^([0-9a-fA-F]{64})\s+\*?(.+)$/);
    if (!match) fail(`invalid checksums line: ${rawLine}`);
    const name = path.basename(match[2].trim().replace(/\\/g, '/'));
    if (checksums.has(name)) fail(`duplicate checksum entry for ${name}`);
    checksums.set(name, match[1].toLowerCase());
  }
  return checksums;
}

function sha256File(filename) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const stream = fs.createReadStream(filename);
    stream.on('error', reject);
    stream.on('data', (chunk) => hash.update(chunk));
    stream.on('end', () => resolve(hash.digest('hex')));
  });
}

function requireRegularFile(filename, label) {
  let info;
  try {
    info = fs.lstatSync(filename);
  } catch (error) {
    if (error && error.code === 'ENOENT') fail(`missing ${label}: ${path.basename(filename)}`);
    throw error;
  }
  if (!info.isFile()) fail(`${label} must be a regular file: ${path.basename(filename)}`);
}

async function main() {
  const version = parseVersion(process.argv[2]);
  const checksumsPath = path.resolve(process.cwd(), process.argv[3] || 'checksums.txt');
  const assetsDirectory = path.resolve(process.cwd(), process.argv[4] || '.');
  const outputDirectory = path.resolve(process.cwd(), process.argv[5] || 'mcpb');
  const releaseSourceRoot = path.resolve(process.cwd(), process.argv[6] || path.resolve(__dirname, '..'));
  const catalogPath = path.join(releaseSourceRoot, 'internal', 'toolcatalog', 'catalog.json');
  const licensePath = path.join(releaseSourceRoot, 'LICENSE');

  requireRegularFile(checksumsPath, 'checksums file');
  requireRegularFile(catalogPath, 'tool catalog');
  requireRegularFile(licensePath, 'license file');
  if (fs.existsSync(outputDirectory)) fail(`output already exists: ${outputDirectory}`);

  const checksums = parseChecksums(checksumsPath);
  const expectedSources = new Set(targets.map((target) => target.sourceFile));
  const rawBinaryPattern = /^scripthold_(?:windows|linux|darwin)_(?:amd64|arm64)(?:\.exe)?$/;
  const unexpected = [...checksums.keys()].filter((name) => rawBinaryPattern.test(name) && !expectedSources.has(name));
  if (unexpected.length > 0) fail(`unexpected release binaries: ${unexpected.join(', ')}`);

  const verified = [];
  for (const target of targets) {
    const expected = checksums.get(target.sourceFile);
    if (!expected) fail(`missing release checksum for ${target.sourceFile}`);
    const sourcePath = path.join(assetsDirectory, target.sourceFile);
    requireRegularFile(sourcePath, 'release binary');
    const actual = await sha256File(sourcePath);
    if (actual !== expected) fail(`checksum mismatch for ${target.sourceFile}`);
    verified.push({ ...target, sourcePath, sha256: actual });
  }

  const catalog = JSON.parse(fs.readFileSync(catalogPath, 'utf8'));
  if (!Array.isArray(catalog.tools) || catalog.tools.length === 0) fail('tool catalog must contain at least one tool');
  const names = new Set();
  const tools = catalog.tools.map((tool) => {
    if (typeof tool.name !== 'string' || tool.name.trim() === '') fail('tool catalog contains an invalid name');
    if (typeof tool.description !== 'string' || tool.description.trim() === '') fail(`tool catalog description is missing for ${tool.name}`);
    if (names.has(tool.name)) fail(`tool catalog contains duplicate name ${tool.name}`);
    names.add(tool.name);
    return { name: tool.name, description: tool.description };
  });

  fs.mkdirSync(path.dirname(outputDirectory), { recursive: true });
  const temporaryDirectory = `${outputDirectory}.${process.pid}.tmp`;
  if (fs.existsSync(temporaryDirectory)) fs.rmSync(temporaryDirectory, { recursive: true, force: true });
  fs.mkdirSync(temporaryDirectory);
  try {
    const packages = [];
    for (const target of verified) {
      const stageDirectory = target.bundleFile.replace(/\.mcpb$/, '');
      const stagePath = path.join(temporaryDirectory, stageDirectory);
      const serverDirectory = path.join(stagePath, 'server');
      fs.mkdirSync(serverDirectory, { recursive: true });
      fs.copyFileSync(target.sourcePath, path.join(serverDirectory, target.binaryFile));
      fs.copyFileSync(licensePath, path.join(stagePath, 'LICENSE'));

      const manifest = {
        manifest_version: '0.3',
        name: 'scripthold',
        display_name: 'Scripthold',
        version,
        description: 'Secure encoding-aware MCP server for explicitly authorized local workspaces',
        long_description: 'Scripthold provides encoding-aware filesystem workflows, durable verified mutations, persistent backup operations, and optional durable task execution inside explicitly authorized local directories.',
        author: {
          name: 'Scripthold maintainers',
          url: 'https://github.com/zoster81/scripthold',
        },
        server: {
          type: 'binary',
          entry_point: `server/${target.binaryFile}`,
          mcp_config: {
            command: `\${__dirname}/server/${target.binaryFile}`,
            args: ['${user_config.allowed_directories}'],
            env: {},
          },
        },
        tools,
        keywords: ['mcp', 'filesystem', 'encoding', 'workspace'],
        license: 'GPL-3.0',
        compatibility: {
          platforms: [target.platform],
        },
        user_config: {
          allowed_directories: {
            type: 'directory',
            title: 'Allowed directories',
            description: 'Select one or more local directories that Scripthold may access.',
            multiple: true,
            required: true,
          },
        },
      };
      fs.writeFileSync(path.join(stagePath, 'manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
      packages.push({
        os: target.os,
        arch: target.arch,
        sourceFile: target.sourceFile,
        sourceSha256: target.sha256,
        bundleFile: target.bundleFile,
        stageDirectory,
      });
    }
    fs.writeFileSync(
      path.join(temporaryDirectory, 'mcpb-assets.json'),
      `${JSON.stringify({ version, packages }, null, 2)}\n`,
      'utf8',
    );
    fs.renameSync(temporaryDirectory, outputDirectory);
  } catch (error) {
    fs.rmSync(temporaryDirectory, { recursive: true, force: true });
    throw error;
  }

  console.log(`prepared ${targets.length} MCPB staging directories for v${version}`);
}

main().catch((error) => {
  console.error(`error: ${error.message}`);
  process.exit(1);
});