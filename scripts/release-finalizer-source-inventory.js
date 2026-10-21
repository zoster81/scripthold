'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { validateStagedManifest } = require('./release-asset-contract');

function requireCondition(ok, reason) {
  if (!ok) throw new Error(reason);
}

// Read-only boundary shared by the finalizer's authenticated source staging
// and the future draft writer. The previous stage verifies all original bytes;
// uploads independently rehash each source with an open descriptor.
function verifyDraftSourceInventory(manifest, normalDir, mcpbDir) {
  validateStagedManifest(manifest);
  const groups = [
    { root: normalDir, assets: manifest.assets.filter(asset =>
      !asset.name.endsWith('.mcpb') && asset.name !== 'mcpb-checksums.txt') },
    { root: mcpbDir, assets: manifest.assets.filter(asset =>
      asset.name.endsWith('.mcpb') || asset.name === 'mcpb-checksums.txt') },
  ];
  requireCondition(groups[0].assets.length === 15 && groups[1].assets.length === 7,
    'expected fifteen normal and seven MCPB original sources');
  for (const { root, assets } of groups) {
    requireCondition(typeof root === 'string' && path.isAbsolute(root),
      'original stage directory must be absolute');
    const full = path.resolve(root);
    const directory = fs.lstatSync(full);
    requireCondition(directory.isDirectory() && !directory.isSymbolicLink() &&
      (process.platform !== 'linux' ||
        (directory.uid === process.getuid() && (directory.mode & 0o077) === 0)),
    'original source stage directory is unsafe');
    const actual = fs.readdirSync(full);
    const expected = new Set(assets.map(asset => asset.name));
    requireCondition(actual.length === expected.size &&
      actual.every(name => expected.has(name)),
    'incomplete or unexpected original source inventory');
    for (const asset of assets) {
      const file = fs.lstatSync(path.join(full, asset.name));
      requireCondition(file.isFile() && !file.isSymbolicLink() &&
        file.nlink === 1 && file.size === asset.size &&
        (process.platform !== 'linux' || file.uid === process.getuid()),
      'unsafe original source before draft preparation');
    }
  }
}

module.exports = { verifyDraftSourceInventory };
