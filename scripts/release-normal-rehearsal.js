#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const { expectedReleaseAssetNames } = require('./release-asset-contract');
const { parseChecksums, validateArtifacts } = require('./release-provenance');

// Verify the complete canonical set, without trusting a producer's own names.
// This reusable pure check also permits negative tests without disk mutation.
function verifyCanonicalNormalChecksums(tag, checksumBytes) {
  const expected = expectedReleaseAssetNames(tag).slice(0, 12);
  if (expected.length !== 12) throw new Error('expected twelve normal subjects');
  if (!Buffer.isBuffer(checksumBytes) ||
      checksumBytes.length === 0 || checksumBytes.length > 16 * 1024) {
    throw new Error('invalid bounded original checksum bytes');
  }
  const checksums = parseChecksums(checksumBytes.toString('utf8'));
  if (checksums.size !== 12 ||
      expected.some(name => !checksums.has(name))) {
    throw new Error('normal checksum subjects must be exactly twelve canonical names');
  }
  return checksums;
}

// Read-only check of genuine GoReleaser outputs. It does not sign,
// stage, publish, or authenticate a GitHub producer or release.
async function verifyNormalBuildOutput(tag, cwd) {
  const expected = expectedReleaseAssetNames(tag).slice(0, 12);
  const dist = path.resolve(cwd, 'dist');
  const rootStat = fs.lstatSync(dist);
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) {
    throw new Error('invalid GoReleaser output directory');
  }

  const checksumsPath = path.join(dist, 'checksums.txt');
  const checksumStat = fs.lstatSync(checksumsPath);
  if (!checksumStat.isFile() || checksumStat.nlink !== 1 ||
      checksumStat.size === 0 || checksumStat.size > 16 * 1024) {
    throw new Error('invalid bounded original checksum file');
  }
  const checksumBytes = fs.readFileSync(checksumsPath);
  const checksums = verifyCanonicalNormalChecksums(tag, checksumBytes);

  const artifactsPath = path.join(dist, 'artifacts.json');
  const metaStat = fs.lstatSync(artifactsPath);
  if (!metaStat.isFile() || metaStat.nlink !== 1 ||
      metaStat.size === 0 || metaStat.size > 2 * 1024 * 1024) {
    throw new Error('invalid bounded GoReleaser artifact metadata');
  }
  const artifacts = JSON.parse(fs.readFileSync(artifactsPath, 'utf8'));
  const files = await validateArtifacts(checksums, artifacts, cwd);
  if (files.size !== 12) throw new Error('incomplete original GoReleaser assets');
  return { tag, count: 13, names: [...expected, 'checksums.txt'] };
}

if (require.main === module) {
  if (process.argv.length !== 4) {
    console.error('usage: node scripts/release-normal-rehearsal.js TAG SOURCE_DIRECTORY');
    process.exitCode = 1;
  } else {
    verifyNormalBuildOutput(process.argv[2], process.argv[3]).then(result => {
      console.log('verified ' + result.count + ' non-published normal files for ' + result.tag);
    }).catch(error => {
      console.error('normal rehearsal: ' + error.message);
      process.exitCode = 1;
    });
  }
}

module.exports = { verifyNormalBuildOutput, verifyCanonicalNormalChecksums };
