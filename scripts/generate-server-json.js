#!/usr/bin/env node
'use strict';

const fs = require('fs');
const path = require('path');

const forkRepository = 'https://github.com/zoster81/scripthold';
const forkRegistryName = 'io.github.zoster81/scripthold';
const zeroSha256 = '0'.repeat(64);

function fail(message) {
  console.error(`error: ${message}`);
  process.exit(1);
}

const versionArg = process.argv[2] || '';
const version = versionArg.replace(/^v/, '');
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  fail(`invalid release version "${versionArg}"`);
}

const root = path.resolve(__dirname, '..');
const templatePath = path.join(root, 'server.template.json');
const releaseSourceRoot = path.resolve(process.cwd(), process.argv[5] || root);
const catalogPath = path.join(releaseSourceRoot, 'internal', 'toolcatalog', 'catalog.json');
const checksumsPath = path.resolve(process.cwd(), process.argv[3] || 'checksums.txt');
const outputPath = path.resolve(process.cwd(), process.argv[4] || 'server.json');

const manifest = JSON.parse(fs.readFileSync(templatePath, 'utf8'));
if (manifest.name !== forkRegistryName) {
  fail(`registry template name must be ${forkRegistryName}`);
}
if (manifest.repository?.url !== forkRepository || manifest.homepage !== forkRepository) {
  fail(`registry template repository metadata must target ${forkRepository}`);
}
if (!Array.isArray(manifest.packages) || manifest.packages.length === 0) {
  fail('registry template must contain at least one package');
}
if (!Array.isArray(manifest.tools) || manifest.tools.length !== 0) {
  fail('registry template tools must be empty; tool metadata comes from the authoritative catalog');
}

const catalog = JSON.parse(fs.readFileSync(catalogPath, 'utf8'));
if (!Array.isArray(catalog.tools) || catalog.tools.length === 0) {
  fail('tool catalog must contain at least one tool');
}
const toolNames = new Set();
for (const tool of catalog.tools) {
  if (typeof tool.name !== 'string' || tool.name.trim() === '') {
    fail('tool catalog contains an invalid name');
  }
  if (typeof tool.description !== 'string' || tool.description.trim() === '') {
    fail(`tool catalog description is missing for ${tool.name}`);
  }
  if (toolNames.has(tool.name)) {
    fail(`tool catalog contains duplicate name ${tool.name}`);
  }
  toolNames.add(tool.name);
}
manifest.tools = catalog.tools.map(({ name, description }) => ({ name, description }));

const checksums = new Map();
for (const rawLine of fs.readFileSync(checksumsPath, 'utf8').split(/\r?\n/)) {
  const line = rawLine.trim();
  if (!line) continue;
  const match = line.match(/^([0-9a-fA-F]{64})\s+\*?(.+)$/);
  if (!match) {
    fail(`invalid checksums line: ${rawLine}`);
  }
  const filename = path.basename(match[2].trim().replace(/\\/g, '/'));
  if (checksums.has(filename)) {
    fail(`duplicate checksum entry for ${filename}`);
  }
  checksums.set(filename, match[1].toLowerCase());
}

manifest.version = version;
const seenPackages = new Set();
for (const pkg of manifest.packages) {
  let filename;
  try {
    filename = path.posix.basename(new URL(pkg.identifier).pathname);
  } catch {
    fail(`invalid package identifier in template: ${pkg.identifier}`);
  }
  if (seenPackages.has(filename)) {
    fail(`duplicate package entry for ${filename}`);
  }
  seenPackages.add(filename);

  const checksum = checksums.get(filename);
  if (!checksum || checksum === zeroSha256) {
    fail(`missing release checksum for ${filename}`);
  }
  pkg.identifier = `${forkRepository}/releases/download/v${version}/${filename}`;
  pkg.fileSha256 = checksum;
}

const mcpbPattern = /^scripthold_(?:windows|linux|darwin)_(?:amd64|arm64)\.mcpb$/;
const unexpected = [...checksums.keys()].filter((filename) => mcpbPattern.test(filename) && !seenPackages.has(filename));
if (unexpected.length > 0) {
  fail(`release contains unrepresented MCPB packages: ${unexpected.join(', ')}`);
}

const tempPath = `${outputPath}.${process.pid}.tmp`;
try {
  fs.writeFileSync(tempPath, `${JSON.stringify(manifest, null, 2)}\n`, 'utf8');
  fs.renameSync(tempPath, outputPath);
} finally {
  if (fs.existsSync(tempPath)) fs.unlinkSync(tempPath);
}

console.log(`generated ${outputPath} for ${forkRegistryName} v${version}`);
