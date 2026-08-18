#!/usr/bin/env node

const fs = require('node:fs');
const {spawnSync} = require('node:child_process');

const TIERS = new Set(['docs', 'go', 'full']);
const BASE_REQUIRED_JOBS = ['classify', 'policy', 'gitleaks'];
const GO_REQUIRED_JOBS = [
  ...BASE_REQUIRED_JOBS,
  'verify-modules',
  'linux-regression',
  'platform-tests',
  'race',
  'static-analysis',
  'fuzz',
];
const FULL_REQUIRED_JOBS = [
  ...GO_REQUIRED_JOBS,
  'native-smoke',
  'cross-build',
  'container-smoke',
  'release-config',
];

function isValidRepositoryPath(value) {
  if (typeof value !== 'string' || value.length === 0 || value.includes('\0') || value.includes('\\')) return false;
  if (value.startsWith('/') || value.includes('//') || value.includes('\uFFFD') || /[\r\n]/.test(value)) return false;
  const segments = value.split('/');
  return segments.every((segment) => segment.length > 0 && segment !== '.' && segment !== '..');
}

function isSensitivePolicyPath(value) {
  return value === '.github' || value.startsWith('.github/') ||
    value === 'scripts' || value.startsWith('scripts/') ||
    value === 'testarchitecture' || value.startsWith('testarchitecture/');
}

function isDocumentationPath(value) {
  if (!value.toLowerCase().endsWith('.md')) return false;
  return !value.includes('/') || value.startsWith('docs/');
}

function classifyPaths(paths) {
  if (!Array.isArray(paths) || paths.length === 0) {
    return {tier: 'full', reason: 'empty change set'};
  }

  let sawGo = false;
  for (const value of paths) {
    if (!isValidRepositoryPath(value)) {
      return {tier: 'full', reason: 'malformed repository path'};
    }
    if (isSensitivePolicyPath(value)) {
      return {tier: 'full', reason: `gate-defining path changed: ${value}`};
    }
    if (isDocumentationPath(value)) continue;
    if (value.toLowerCase().endsWith('.go')) {
      sawGo = true;
      continue;
    }
    return {tier: 'full', reason: `unclassified path changed: ${value}`};
  }

  return sawGo
    ? {tier: 'go', reason: 'Go and documentation changes only'}
    : {tier: 'docs', reason: 'documentation-only change set'};
}

function classifyEvent(eventName, paths) {
  if (eventName !== 'pull_request') {
    return {tier: 'full', reason: `event ${eventName || '<missing>'} requires full qualification`};
  }
  return classifyPaths(paths);
}

function parseNameStatusZ(buffer) {
  if (!Buffer.isBuffer(buffer)) throw new TypeError('name-status input must be a Buffer');
  if (buffer.length === 0) return [];

  const fields = buffer.toString('utf8').split('\0');
  if (fields.at(-1) !== '') throw new Error('name-status output is not NUL terminated');
  fields.pop();

  const paths = [];
  for (let index = 0; index < fields.length;) {
    const status = fields[index++];
    if (!/^[ACDMR][0-9]*$/.test(status)) throw new Error(`unsupported git status ${status}`);
    const kind = status[0];
    if (kind === 'R' || kind === 'C') {
      if (index + 1 >= fields.length) throw new Error(`incomplete ${kind} record`);
      paths.push(fields[index++], fields[index++]);
      continue;
    }
    if (index >= fields.length) throw new Error(`incomplete ${kind} record`);
    paths.push(fields[index++]);
  }
  return paths;
}

function parseLsTreeZ(buffer) {
  if (!Buffer.isBuffer(buffer)) throw new TypeError('ls-tree input must be a Buffer');
  if (buffer.length === 0) return new Map();

  const records = buffer.toString('utf8').split('\0');
  if (records.at(-1) !== '') throw new Error('ls-tree output is not NUL terminated');
  records.pop();

  const entries = new Map();
  for (const record of records) {
    const tab = record.indexOf('\t');
    if (tab <= 0) throw new Error('malformed ls-tree record');
    const header = record.slice(0, tab);
    const value = record.slice(tab + 1);
    const match = header.match(/^([0-7]{6}) ([a-z]+) ([0-9a-f]{40}|[0-9a-f]{64})$/i);
    if (!match || !isValidRepositoryPath(value) || entries.has(value)) {
      throw new Error('malformed ls-tree record');
    }
    entries.set(value, {mode: match[1], type: match[2]});
  }
  return entries;
}

function changedPathsUseOnlyRegularFiles(paths, ...trees) {
  if (!Array.isArray(paths) || paths.length === 0) return false;
  for (const value of paths) {
    if (!isValidRepositoryPath(value)) return false;
    for (const tree of trees) {
      if (!(tree instanceof Map)) return false;
      const entry = tree.get(value);
      if (!entry) continue;
      if (entry.type !== 'blob' || !['100644', '100755'].includes(entry.mode)) return false;
    }
  }
  return true;
}

function readTree(sha) {
  const result = spawnSync('git', ['ls-tree', '-r', '-z', sha], {
    encoding: null,
    maxBuffer: 16 * 1024 * 1024,
    shell: false,
  });
  if (result.error || result.status !== 0) throw new Error(`git ls-tree failed for ${sha}`);
  return parseLsTreeZ(result.stdout);
}

function requiredJobsForTier(tier) {
  if (tier === 'docs') return [...BASE_REQUIRED_JOBS];
  if (tier === 'go') return [...GO_REQUIRED_JOBS];
  if (tier === 'full') return [...FULL_REQUIRED_JOBS];
  throw new Error(`unknown evidence tier ${tier}`);
}

function matrixOutputsForTier(tier) {
  if (!TIERS.has(tier)) throw new Error(`unknown evidence tier ${tier}`);
  if (tier === 'full') {
    return {
      race: [
        {os: 'ubuntu-latest', scope: 'full'},
        {os: 'windows-latest', scope: 'full'},
        {os: 'macos-latest', scope: 'full'},
      ],
      platform: [
        {os: 'windows-latest', platform: 'windows', scope: 'full'},
        {os: 'macos-latest', platform: 'darwin', scope: 'full'},
      ],
    };
  }
  if (tier === 'go') {
    return {
      race: [{os: 'ubuntu-latest', scope: 'targeted'}],
      platform: [
        {os: 'windows-latest', platform: 'windows', scope: 'targeted'},
        {os: 'macos-latest', platform: 'darwin', scope: 'targeted'},
      ],
    };
  }
  return {
    race: [{os: 'ubuntu-latest', scope: 'targeted'}],
    platform: [{os: 'ubuntu-latest', platform: 'linux', scope: 'targeted'}],
  };
}

function selectGoPackages(evidenceMap, failureClass, platform) {
  if (!evidenceMap || !Array.isArray(evidenceMap.testGroups)) throw new Error('evidence map is missing testGroups');
  if (!['race', 'platform'].includes(failureClass)) throw new Error(`unsupported failure class ${failureClass}`);
  if (!['linux', 'windows', 'darwin'].includes(platform)) throw new Error(`unsupported platform ${platform}`);

  const packages = new Set();
  for (const group of evidenceMap.testGroups) {
    if (group?.kind !== 'go-package') continue;
    if (!Array.isArray(group.failureClasses) || !group.failureClasses.includes(failureClass)) continue;
    if (!Array.isArray(group.platforms) || !(group.platforms.includes(platform) || group.platforms.includes('all'))) continue;
    if (!isValidRepositoryPath(group.directory) || group.directory.startsWith('.')) {
      throw new Error(`invalid Go package directory in evidence map: ${group.directory}`);
    }
    packages.add(`./${group.directory}`);
  }
  const selected = [...packages].sort();
  if (selected.length === 0) throw new Error(`no ${failureClass} packages mapped for ${platform}`);
  return selected;
}

function normalizeJobResult(value) {
  if (typeof value === 'string') return value;
  if (value && typeof value === 'object' && typeof value.result === 'string') return value.result;
  return '<missing>';
}

function verifyJobResults(tier, results) {
  if (!results || typeof results !== 'object' || Array.isArray(results)) throw new Error('job results must be an object');
  const required = new Set(requiredJobsForTier(tier));

  for (const [job, value] of Object.entries(results)) {
    const result = normalizeJobResult(value);
    if (result !== 'success' && result !== 'skipped') {
      throw new Error(`job ${job} ended with ${result}`);
    }
  }
  for (const job of required) {
    if (normalizeJobResult(results[job]) !== 'success') {
      throw new Error(`required ${tier} evidence ${job} did not succeed`);
    }
  }
}

function appendOutput(file, name, value) {
  if (!file) return;
  if (value.includes('\n') || value.includes('\r')) throw new Error(`invalid multiline output ${name}`);
  fs.appendFileSync(file, `${name}=${value}\n`, 'utf8');
}

function classifyFromGitHubEvent(eventName, eventPath) {
  if (eventName !== 'pull_request') return classifyEvent(eventName, []);
  try {
    const event = JSON.parse(fs.readFileSync(eventPath, 'utf8'));
    const base = event?.pull_request?.base?.sha;
    const head = event?.pull_request?.head?.sha;
    if (!/^[0-9a-f]{40}$/i.test(base || '') || !/^[0-9a-f]{40}$/i.test(head || '')) {
      return {tier: 'full', reason: 'pull request event is missing exact base/head SHAs'};
    }
    const diff = spawnSync('git', ['diff', '--name-status', '-z', '--find-renames', `${base}...${head}`], {
      encoding: null,
      maxBuffer: 16 * 1024 * 1024,
      shell: false,
    });
    if (diff.error || diff.status !== 0) {
      return {tier: 'full', reason: 'git diff failed; selecting full qualification'};
    }
    const paths = parseNameStatusZ(diff.stdout);
    const baseTree = readTree(base);
    const headTree = readTree(head);
    if (!changedPathsUseOnlyRegularFiles(paths, baseTree, headTree)) {
      return {tier: 'full', reason: 'non-regular or malformed changed path requires full qualification'};
    }
    return classifyEvent(eventName, paths);
  } catch {
    return {tier: 'full', reason: 'change classification failed; selecting full qualification'};
  }
}

function runCLI(argv = process.argv.slice(2), env = process.env) {
  const command = argv[0];
  if (command === 'classify') {
    const result = classifyFromGitHubEvent(env.GITHUB_EVENT_NAME, env.GITHUB_EVENT_PATH);
    const matrices = matrixOutputsForTier(result.tier);
    appendOutput(env.GITHUB_OUTPUT, 'tier', result.tier);
    appendOutput(env.GITHUB_OUTPUT, 'reason', result.reason);
    appendOutput(env.GITHUB_OUTPUT, 'race_matrix', JSON.stringify(matrices.race));
    appendOutput(env.GITHUB_OUTPUT, 'platform_matrix', JSON.stringify(matrices.platform));
    console.log(`CI evidence tier: ${result.tier} (${result.reason})`);
    return;
  }

  if (command === 'run-targeted') {
    const failureClassIndex = argv.indexOf('--failure-class');
    const platformIndex = argv.indexOf('--platform');
    const failureClass = failureClassIndex >= 0 ? argv[failureClassIndex + 1] : '';
    const platform = platformIndex >= 0 ? argv[platformIndex + 1] : '';
    const useRace = argv.includes('--race');
    if (useRace && failureClass !== 'race') throw new Error('--race requires --failure-class race');
    const evidenceMap = JSON.parse(fs.readFileSync('testarchitecture/evidence-map.json', 'utf8'));
    const packages = selectGoPackages(evidenceMap, failureClass, platform);
    const args = ['test'];
    if (useRace) args.push('-race');
    args.push('-count=1', ...packages);
    const result = spawnSync('go', args, {stdio: 'inherit', shell: false});
    if (result.error || result.status !== 0) throw new Error(`targeted go test failed for ${failureClass}/${platform}`);
    return;
  }

  if (command === 'verify') {
    const tier = env.CI_EVIDENCE_TIER;
    const results = JSON.parse(env.CI_JOB_RESULTS || '{}');
    verifyJobResults(tier, results);
    console.log(`Release candidate evidence verified for tier ${tier}`);
    return;
  }

  throw new Error('usage: test-ci-policy.js <classify|run-targeted|verify>');
}

module.exports = {
  changedPathsUseOnlyRegularFiles,
  classifyEvent,
  classifyPaths,
  matrixOutputsForTier,
  parseLsTreeZ,
  parseNameStatusZ,
  requiredJobsForTier,
  selectGoPackages,
  verifyJobResults,
};

if (require.main === module) {
  try {
    runCLI();
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
