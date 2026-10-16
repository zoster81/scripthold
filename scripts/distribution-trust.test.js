'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const root = path.resolve(__dirname, '..');

function read(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), 'utf8');
}

function readJSON(relativePath) {
  return JSON.parse(read(relativePath));
}

function workflowFiles() {
  const directory = path.join(root, '.github', 'workflows');
  return fs.readdirSync(directory)
    .filter((name) => name.endsWith('.yml') || name.endsWith('.yaml'))
    .sort()
    .map((name) => ({ name, content: fs.readFileSync(path.join(directory, name), 'utf8') }));
}

test('all external GitHub Actions are pinned to immutable commit SHAs', () => {
  const externalUse = /^\s*uses:\s*([^\s]+)@([^\s#]+)(?:\s+#.*)?$/gm;
  const violations = [];
  for (const workflow of workflowFiles()) {
    for (const match of workflow.content.matchAll(externalUse)) {
      const action = match[1];
      const ref = match[2];
      if (action.startsWith('./')) continue;
      if (!/^[0-9a-f]{40}$/.test(ref)) violations.push(`${workflow.name}: ${action}@${ref}`);
    }
  }
  assert.deepEqual(violations, []);
});

test('OpenSSF Scorecard workflow stays on supported default-branch triggers', () => {
  const scorecard = read('.github/workflows/scorecard.yml');
  assert.match(scorecard, /^name: OpenSSF Scorecard$/m);
  assert.match(scorecard, /push:\s*\n\s*branches:\s*\n\s*- main/m);
  assert.doesNotMatch(scorecard, /tags:\s*\n\s*- ['"]v\*['"]/m);
  assert.match(scorecard, /schedule:\s*\n\s*- cron:/m);
  assert.match(scorecard, /permissions:\s*\n\s*contents: read\s*\n\s*security-events: write\s*\n\s*id-token: write/m);
  assert.match(scorecard, /ossf\/scorecard-action@2d1146689b8cda280b9bc96326124645441f03bc/);
  assert.match(scorecard, /publish_results:\s*true/);
  assert.match(scorecard, /results_file:\s*results\.sarif/);
  assert.match(scorecard, /persist-credentials:\s*false/);
  assert.match(scorecard, /github\/codeql-action\/upload-sarif@24c54180a607b1449ed407dd24f251e4e9147c8d/);
});

test('README trust badges are evidence-backed and avoid unverified directory claims', () => {
  const readme = read('README.md');
  assert.match(readme, /OpenSSF Scorecard/);
  assert.match(readme, /api\.scorecard\.dev\/projects\/github\.com\/zoster81\/scripthold\/badge/);
  assert.match(readme, /Release downloads/);
  assert.match(readme, /github\/downloads\/zoster81\/scripthold\/total/);
  assert.match(readme, /glama\.ai\/mcp\/servers\/zoster81\/scripthold\/badges\/score\.svg/);
  assert.doesNotMatch(readme, /Smithery[^\n]*badge/i);
  assert.doesNotMatch(readme, /MCP\.Directory[^\n]*badge/i);
  assert.doesNotMatch(readme, /PulseMCP[^\n]*badge/i);
});

test('Glama ownership metadata belongs to the current Scripthold maintainer', () => {
  const metadata = readJSON('glama.json');
  assert.equal(metadata.$schema, 'https://glama.ai/mcp/schemas/server.json');
  assert.deepEqual(metadata.maintainers, ['zoster81']);
});

test('Smithery metadata stays aligned with the actual stdio container runtime without hard-coded tool-count drift', () => {
  const smithery = read('smithery.yaml');
  assert.doesNotMatch(smithery, /\b30 tools\b/i);
  assert.match(smithery, /^runtime:\s*"?container"?$/m);
  assert.match(smithery, /command:\s*['"]\/usr\/local\/bin\/scripthold['"]/);
  assert.match(smithery, /--transport=stdio/);
});

test('distribution documentation preserves core release and third-party syndication boundaries', () => {
  const publishing = read('docs/PUBLISHING.md');
  assert.match(publishing, /Core release/i);
  assert.match(publishing, /Discovery.*syndication/is);
  assert.match(publishing, /SMITHERY_API_KEY/);
  assert.match(publishing, /Docker MCP Catalog/);
  assert.match(publishing, /GPL-3\.0/);
  assert.match(publishing, /Streamable HTTP/);
  assert.match(publishing, /TLS|trusted proxy/i);
  assert.match(publishing, /OpenSSF Scorecard.*default(?:-| )branch/is);
  assert.match(publishing, /does not support release-tag pushes/i);
});

test('release publishing defaults to read-only and grants write permission only at the job that needs it', () => {
  const workflow = read('.github/workflows/publish-mcpb-assets.yml');
  const beforeJobs = workflow.split(/^jobs:\s*$/m, 1)[0];
  assert.match(beforeJobs, /permissions:\s*\n\s*contents:\s*read/m);
  assert.doesNotMatch(beforeJobs, /^\s*contents:\s*write\s*$/m);
  assert.match(workflow, /jobs:\s*\n\s*publish:.*?permissions:\s*\n\s*contents:\s*write/s);
});

test('CodeQL uses one immutable action release across every workflow step', () => {
  const steps = [];
  const pattern = /^\s*uses:\s*github\/codeql-action\/(init|autobuild|analyze|upload-sarif)@([0-9a-f]{40})\s+#\s+v(\d+\.\d+\.\d+)$/gm;
  for (const workflow of workflowFiles()) {
    for (const match of workflow.content.matchAll(pattern)) {
      steps.push({ workflow: workflow.name, action: match[1], sha: match[2], version: match[3] });
    }
  }
  assert.deepEqual(steps.map(({ workflow, action }) => `${workflow}:${action}`), [
    'codeql.yml:init',
    'codeql.yml:autobuild',
    'codeql.yml:analyze',
    'scorecard.yml:upload-sarif',
  ]);
  assert.equal(new Set(steps.map(({ sha, version }) => `${sha}:${version}`)).size, 1);
  assert.match(read('.github/dependabot.yml'), /codeql-actions:\s*\n\s*patterns:\s*\n\s*- "github\/codeql-action\/\*"/);
});

test('GitHub workflows share one verified Node release', () => {
  const versions = workflowFiles().flatMap(({ name, content }) =>
    [...content.matchAll(/^\s*node-version:\s*['"]?(\d+\.\d+\.\d+)['"]?\s*$/gm)]
      .map((match) => `${name}:${match[1]}`));
  assert.ok(versions.length >= 4, 'CI and publication workflows should declare Node explicitly');
  assert.equal(new Set(versions.map((value) => value.split(':')[1])).size, 1);
});

test('setup-node stays on one verified immutable release across CI and publication', () => {
  const steps = workflowFiles().flatMap(({ name, content }) =>
    content.split(/\r?\n/).filter((line) =>
      /^\s*uses:\s*actions\/setup-node@/.test(line)).map((line) => {
      const match = /^\s*uses:\s*actions\/setup-node@([0-9a-f]{40})\s+#\s+v(\d+\.\d+\.\d+)\s*$/.exec(line);
      assert.ok(match, `setup-node in ${name} must pin its exact SHA and version`);
      return { name, sha: match[1], version: match[2] };
    }));
  assert.ok(steps.length >= 8, 'CI and publication must retain pinned Node setup coverage');
  assert.ok(steps.some((step) => step.name === 'release-finalizer-preflight.yml'));
  assert.equal(new Set(steps.map(({ sha, version }) => `${sha}:${version}`)).size, 1);
});

test('CodeQL scans pull requests targeting maintained branches and every pushed commit', () => {
  const workflow = read('.github/workflows/codeql.yml');
  assert.match(workflow, /pull_request:\s*\n\s*branches:\s*\n\s*- main\s*\n\s*- master/m);
  assert.match(workflow, /^\s{2}push:\s*$/m);
  assert.doesNotMatch(workflow, /push:\s*\n\s*branches:/m);
});

test('release workflow requires exact-commit Scorecard evidence and publishes pinned Sigstore provenance', () => {
  const workflow = read('.github/workflows/release.yml');
  assert.match(workflow, /actions\/workflows\/scorecard\.yml\/runs/);
  assert.match(workflow, /-f branch=main/);
  assert.match(workflow, /-f event=push/);
  assert.match(workflow, /-f head_sha="\$\{TAG_COMMIT\}"/);
  assert.match(workflow, /select-scorecard-run/);
  assert.match(workflow, /release:\s*\n\s*needs: verify.*?permissions:\s*\n\s*contents: write\s*\n\s*id-token: write\s*\n\s*attestations: write\s*\n\s*artifact-metadata: write/s);
  assert.match(workflow, /actions\/attest@1e69f48acb82d1966a394da916b4c1698aa569d6\s+# v4\.2\.2/);
  assert.match(workflow, /subject-checksums:\s*dist\/checksums\.txt/);
  assert.match(workflow, /provenance\.sigstore\.json/);
  assert.match(workflow, /gh release upload/);
});

test('release asset manifest contract is always exercised by CI policy', () => {
  const workflow = read('.github/workflows/test.yml');
  assert.match(workflow, /node --test [^\n]*scripts\/release-asset-contract\.test\.js/m);
});

test('container base images are pinned to immutable digests', () => {
  const dockerfile = read('Dockerfile');
  const declaredGoVersion = read('go.mod').match(/^go (1\.\d+\.\d+)$/m)?.[1];
  assert.ok(declaredGoVersion, 'go.mod must pin a Go patch release');
  const builder = dockerfile.match(/^FROM golang:(1\.\d+\.\d+)-alpine3\.24@sha256:[0-9a-f]{64} AS builder$/m);
  assert.ok(builder, 'Go builder must pin a patch release and an immutable image digest');
  assert.equal(builder[1], declaredGoVersion, 'Docker builder must use the Go release declared in go.mod');
  assert.match(dockerfile, /^FROM alpine:3\.24\.1@sha256:[0-9a-f]{64}$/m);
});

test('security policy links directly to private vulnerability reporting', () => {
  const policy = read('SECURITY.md');
  assert.match(policy, /https:\/\/github\.com\/zoster81\/scripthold\/security\/advisories\/new/);
});
