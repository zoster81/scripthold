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
  assert.match(scorecard, /github\/codeql-action\/upload-sarif@b96794f015dfd88f77b49b1c93e0fa7110f94c63/);
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

test('container base images are pinned to immutable digests', () => {
  const dockerfile = read('Dockerfile');
  assert.match(dockerfile, /^FROM golang:1\.27\.1-alpine3\.24@sha256:[0-9a-f]{64} AS builder$/m);
  assert.match(dockerfile, /^FROM alpine:3\.24\.1@sha256:[0-9a-f]{64}$/m);
});

test('security policy links directly to private vulnerability reporting', () => {
  const policy = read('SECURITY.md');
  assert.match(policy, /https:\/\/github\.com\/zoster81\/scripthold\/security\/advisories\/new/);
});
