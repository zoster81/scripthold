'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const fuzzTimePattern = /^[1-9][0-9]*(?:x|ms|s|m)$/;
const packagePattern = /^(?:\.|\.\/[A-Za-z0-9._-]+(?:\/[A-Za-z0-9._-]+)*)$/;
const fuzzTargetPattern = /^Fuzz[A-Za-z0-9_]+$/;
const idPattern = /^[a-z0-9][a-z0-9.-]*$/;

function validateManifest(manifest) {
  if (!manifest || typeof manifest !== 'object' || Array.isArray(manifest)) {
    throw new Error('fuzz manifest must be an object');
  }
  if (manifest.schemaVersion !== 1) {
    throw new Error(`unsupported fuzz manifest schemaVersion: ${manifest.schemaVersion}`);
  }
  if (!manifest.profiles || typeof manifest.profiles !== 'object' || Array.isArray(manifest.profiles)) {
    throw new Error('fuzz manifest profiles must be an object');
  }
  const profileNames = Object.keys(manifest.profiles);
  if (profileNames.length === 0) {
    throw new Error('fuzz manifest must define at least one profile');
  }
  for (const profile of profileNames) {
    const config = manifest.profiles[profile];
    if (!config || typeof config !== 'object' || Array.isArray(config)) {
      throw new Error(`fuzz profile ${profile} must be an object`);
    }
    if (typeof config.fuzzTime !== 'string' || !fuzzTimePattern.test(config.fuzzTime)) {
      throw new Error(`fuzz profile ${profile} has invalid fuzzTime`);
    }
  }
  if (!Array.isArray(manifest.targets) || manifest.targets.length === 0) {
    throw new Error('fuzz manifest targets must be a non-empty array');
  }

  const ids = new Set();
  const entrypoints = new Set();
  for (const entry of manifest.targets) {
    if (!entry || typeof entry !== 'object' || Array.isArray(entry)) {
      throw new Error('fuzz target entry must be an object');
    }
    if (typeof entry.id !== 'string' || !idPattern.test(entry.id)) {
      throw new Error(`invalid fuzz target id: ${entry.id}`);
    }
    if (ids.has(entry.id)) {
      throw new Error(`duplicate fuzz target id: ${entry.id}`);
    }
    ids.add(entry.id);
    if (typeof entry.package !== 'string' || !packagePattern.test(entry.package) || entry.package.includes('..')) {
      throw new Error(`fuzz target ${entry.id} has invalid package`);
    }
    if (typeof entry.target !== 'string' || !fuzzTargetPattern.test(entry.target)) {
      throw new Error(`fuzz target ${entry.id} has invalid fuzz target`);
    }
    const entrypoint = `${entry.package}\0${entry.target}`;
    if (entrypoints.has(entrypoint)) {
      throw new Error(`duplicate fuzz entrypoint: ${entry.package}/${entry.target}`);
    }
    entrypoints.add(entrypoint);
    if (!Array.isArray(entry.riskCategories) || entry.riskCategories.length === 0 || entry.riskCategories.some((value) => typeof value !== 'string' || !idPattern.test(value))) {
      throw new Error(`fuzz target ${entry.id} has invalid riskCategories`);
    }
    if (!Array.isArray(entry.profiles) || entry.profiles.length === 0) {
      throw new Error(`fuzz target ${entry.id} has no profiles`);
    }
    const seenProfiles = new Set();
    for (const profile of entry.profiles) {
      if (typeof profile !== 'string' || !Object.hasOwn(manifest.profiles, profile)) {
        throw new Error(`fuzz target ${entry.id} references unknown profile ${profile}`);
      }
      if (seenProfiles.has(profile)) {
        throw new Error(`fuzz target ${entry.id} has duplicate profile ${profile}`);
      }
      seenProfiles.add(profile);
    }
  }
}

function selectTargets(manifest, profile) {
  if (!Object.hasOwn(manifest.profiles, profile)) {
    throw new Error(`unknown fuzz profile: ${profile}`);
  }
  return manifest.targets.filter((entry) => entry.profiles.includes(profile));
}

function buildGoTestArgs(target, fuzzTime) {
  if (!fuzzTimePattern.test(fuzzTime)) {
    throw new Error(`invalid fuzzTime: ${fuzzTime}`);
  }
  return [
    'test',
    target.package,
    '-run',
    '^$',
    '-fuzz',
    `^${target.target}$`,
    `-fuzztime=${fuzzTime}`,
  ];
}

function parseArguments(args) {
  const result = { profile: 'smoke', fuzzTime: null, listOnly: false };
  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index];
    if (arg === '--profile' || arg === '--fuzztime') {
      if (index + 1 >= args.length) {
        throw new Error(`${arg} requires a value`);
      }
      const value = args[index + 1];
      index += 1;
      if (arg === '--profile') {
        result.profile = value;
      } else {
        if (!fuzzTimePattern.test(value)) {
          throw new Error(`invalid fuzzTime: ${value}`);
        }
        result.fuzzTime = value;
      }
      continue;
    }
    if (arg.startsWith('--profile=')) {
      result.profile = arg.slice('--profile='.length);
      continue;
    }
    if (arg.startsWith('--fuzztime=')) {
      const value = arg.slice('--fuzztime='.length);
      if (!fuzzTimePattern.test(value)) {
        throw new Error(`invalid fuzzTime: ${value}`);
      }
      result.fuzzTime = value;
      continue;
    }
    if (arg === '--list') {
      result.listOnly = true;
      continue;
    }
    throw new Error(`unknown argument: ${arg}`);
  }
  return result;
}

function loadManifest(repositoryRoot) {
  const manifestPath = path.join(repositoryRoot, 'testarchitecture', 'fuzz-manifest.json');
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  validateManifest(manifest);
  return manifest;
}

function run(argv = process.argv.slice(2)) {
  const repositoryRoot = path.resolve(__dirname, '..');
  const options = parseArguments(argv);
  const manifest = loadManifest(repositoryRoot);
  const targets = selectTargets(manifest, options.profile);
  if (targets.length === 0) {
    throw new Error(`fuzz profile ${options.profile} selects no targets`);
  }
  const fuzzTime = options.fuzzTime || manifest.profiles[options.profile].fuzzTime;

  if (options.listOnly) {
    for (const target of targets) {
      process.stdout.write(`${target.id}\t${target.package}\t${target.target}\t${fuzzTime}\n`);
    }
    return 0;
  }

  targets.forEach((target, index) => {
    process.stdout.write(`[fuzz ${index + 1}/${targets.length}] ${target.id}: ${target.package} ${target.target} (${fuzzTime})\n`);
    const result = spawnSync('go', buildGoTestArgs(target, fuzzTime), {
      cwd: repositoryRoot,
      env: process.env,
      stdio: 'inherit',
      shell: false,
      windowsHide: false,
    });
    if (result.error) {
      throw result.error;
    }
    if (result.status !== 0) {
      process.exitCode = result.status === null ? 1 : result.status;
      throw new Error(`fuzz target ${target.id} failed`);
    }
  });
  return 0;
}

if (require.main === module) {
  try {
    run();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = process.exitCode || 1;
  }
}

module.exports = {
  buildGoTestArgs,
  loadManifest,
  parseArguments,
  run,
  selectTargets,
  validateManifest,
};
