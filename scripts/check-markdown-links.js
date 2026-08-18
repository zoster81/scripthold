#!/usr/bin/env node

const fs = require('node:fs');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

function stripFencedCode(markdown) {
  const lines = markdown.split(/\r?\n/);
  let fence = null;
  return lines.map((line) => {
    const match = line.match(/^\s*(```+|~~~+)/);
    if (match) {
      const marker = match[1][0];
      if (fence === null) fence = marker;
      else if (fence === marker) fence = null;
      return '';
    }
    return fence === null ? line : '';
  }).join('\n');
}

function normalizedTarget(raw) {
  let value = raw.trim();
  if (value.startsWith('<')) {
    const close = value.indexOf('>');
    if (close < 0) return null;
    value = value.slice(1, close);
  } else {
    value = value.split(/\s+/)[0];
  }
  if (!value || value.startsWith('#') || value.startsWith('//')) return null;
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) return null;
  if (value.startsWith('/')) return null;
  return value;
}

function extractLocalTargets(markdown) {
  const text = stripFencedCode(markdown);
  const targets = [];
  const seen = new Set();

  const add = (raw) => {
    const target = normalizedTarget(raw);
    if (target !== null && !seen.has(target)) {
      seen.add(target);
      targets.push(target);
    }
  };

  for (const match of text.matchAll(/!?\[[^\]\n]*\]\(([^)\n]+)\)/g)) add(match[1]);
  for (const match of text.matchAll(/^\s{0,3}\[[^\]\n]+\]:\s*(<[^>]+>|\S+)/gm)) add(match[1]);
  return targets;
}

function isWithinRoot(root, candidate) {
  const relative = path.relative(root, candidate);
  return relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative));
}

function validateLocalTarget(root, markdownPath, target) {
  const pathPart = target.split('#', 1)[0].split('?', 1)[0];
  if (!pathPart) return {ok: true};

  let decoded;
  try {
    decoded = decodeURIComponent(pathPart);
  } catch {
    return {ok: false, reason: 'malformed percent-encoding'};
  }

  const resolvedRoot = path.resolve(root);
  const candidate = path.resolve(path.dirname(markdownPath), decoded);
  if (!isWithinRoot(resolvedRoot, candidate)) return {ok: false, reason: 'target escapes repository root'};
  if (!fs.existsSync(candidate)) return {ok: false, reason: 'target does not exist'};

  try {
    const realRoot = fs.realpathSync(resolvedRoot);
    const realCandidate = fs.realpathSync(candidate);
    if (!isWithinRoot(realRoot, realCandidate)) return {ok: false, reason: 'target resolves outside repository root'};
  } catch {
    return {ok: false, reason: 'target cannot be resolved safely'};
  }
  return {ok: true};
}

function trackedMarkdownFiles(root) {
  const result = spawnSync('git', ['ls-files', '-z', '--', '*.md'], {
    cwd: root,
    encoding: null,
    maxBuffer: 16 * 1024 * 1024,
    shell: false,
  });
  if (result.error || result.status !== 0) throw new Error('git ls-files failed while enumerating Markdown files');
  return result.stdout.toString('utf8').split('\0').filter(Boolean);
}

function validateRepository(root) {
  const failures = [];
  for (const relative of trackedMarkdownFiles(root)) {
    const markdownPath = path.resolve(root, relative);
    let metadata;
    try {
      metadata = fs.lstatSync(markdownPath);
    } catch {
      failures.push(`${relative}: tracked Markdown source cannot be inspected safely`);
      continue;
    }
    if (!metadata.isFile()) {
      failures.push(`${relative}: tracked Markdown source is not a regular file`);
      continue;
    }
    const markdown = fs.readFileSync(markdownPath, 'utf8');
    for (const target of extractLocalTargets(markdown)) {
      const result = validateLocalTarget(root, markdownPath, target);
      if (!result.ok) failures.push(`${relative}: ${target} (${result.reason})`);
    }
  }
  if (failures.length > 0) throw new Error(`invalid local Markdown links:\n${failures.join('\n')}`);
}

module.exports = {
  extractLocalTargets,
  validateLocalTarget,
  validateRepository,
};

if (require.main === module) {
  try {
    const root = path.resolve(process.argv[2] || process.cwd());
    validateRepository(root);
    console.log('Local Markdown links are valid.');
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
