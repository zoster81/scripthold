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

test('OpenSSF Scorecard workflow stays on supported default-branch triggers', () => {
  const scorecard = read('.github/workflows/scorecard.yml');
  assert.match(scorecard, /^name: OpenSSF Scorecard$/m);
  assert.match(scorecard, /push:\s*\n\s*branches:\s*\n\s*- main/m);
  assert.doesNotMatch(scorecard, /tags:\s*\n\s*- ['"]v\*['"]/m);
  assert.match(scorecard, /schedule:\s*\n\s*- cron:/m);
  assert.match(scorecard, /permissions:\s*\n\s*contents: read\s*\n\s*security-events: write\s*\n\s*id-token: write/m);
  assert.match(scorecard, /ossf\/scorecard-action@fa7117db83c52d787ae5a89f350999701505d78e/);
  assert.match(scorecard, /publish_results:\s*true/);
  assert.match(scorecard, /results_file:\s*results\.sarif/);
  assert.match(scorecard, /persist-credentials:\s*false/);
  assert.match(scorecard, /github\/codeql-action\/upload-sarif@6f2ac81f5cd81fe351a197ba0b00d91d03cd2aee/);
});

test('Glama ownership metadata belongs to the current Scripthold maintainer', () => {
  const metadata = readJSON('glama.json');
  assert.equal(metadata.$schema, 'https://glama.ai/mcp/schemas/server.json');
  assert.deepEqual(metadata.maintainers, ['zoster81']);
});

test('Smithery metadata stays aligned with the actual stdio container runtime without hard-coded tool-count drift', () => {
  const smithery = read('smithery.yaml');
  assert.match(smithery, /^runtime:\s*"?container"?$/m);
  assert.match(smithery, /command:\s*['"]\/usr\/local\/bin\/scripthold['"]/);
  assert.match(smithery, /--transport=stdio/);
});

function assertPinnedUbuntuRunners(yaml, name) {
  const runners = [...yaml.matchAll(/^\s*runs-on:\s*([^\r\n#]+)(?:\s*#.*)?$/gm)]
    .map((match) => match[1].trim());
  assert.ok(runners.length > 0, `${name}: no runner declared`);
  assert.ok(runners.every((runner) => runner === 'ubuntu-26.04'),
    `${name}: every runner must pin ubuntu-26.04: ${runners.join(', ')}`);
}

test('publication and Scorecard jobs pin Ubuntu 26 without an exact job-count oracle', () => {
  for (const name of [
    'release.yml',
    'publish-mcpb-assets.yml',
    'publish-registry.yml',
    'scorecard.yml',
    'release-finalizer-preflight.yml',
  ]) {
    const yaml = read('.github/workflows/' + name);
    assertPinnedUbuntuRunners(yaml, name);
  }
});

test('runner pin validation accepts added pinned jobs but rejects floating or missing runners', () => {
  const pinned = 'jobs:\n  first:\n    runs-on: ubuntu-26.04\n';
  assert.doesNotThrow(() => assertPinnedUbuntuRunners(
    pinned + '  second:\n    runs-on: ubuntu-26.04\n', 'fixture'));
  for (const unsafe of [
    pinned + '  second:\n    runs-on: ubuntu-latest\n',
    pinned + '  second:\n    runs-on: ubuntu-24.04\n',
    'jobs:\n  first:\n    steps: []\n',
  ]) {
    assert.throws(() => assertPinnedUbuntuRunners(unsafe, 'fixture'));
  }
});

test('all publishing jobs require disk-backed scratch before handling release data', () => {
  for (const name of ['release.yml', 'publish-mcpb-assets.yml', 'publish-registry.yml']) {
    const yaml = read('.github/workflows/' + name);
    const steps = yaml.split('      - name: Prepare disk-backed scratch');
    assert.equal(steps.length, 2, name);
    assert.match(steps[1], /findmnt -n -T "\$\{RUNNER_TEMP\}" -o FSTYPE/);
    assert.match(steps[1], /ext4\|xfs\|btrfs\|zfs/);
    assert.match(steps[1], /TMPDIR=%s/);
    assert.match(steps[1], /GOTMPDIR=%s/);
    assert.match(steps[1], /\$\{GITHUB_ENV\}/);
    assert.ok(steps[0].includes('runs-on: ubuntu-26.04'), name);
  }
});

test('signed normal handoff creates a private destination before raw artifact download', () => {
  const workflow = read('.github/workflows/publish-mcpb-assets.yml');
  const setup = workflow.indexOf('      - name: Prepare disk-backed scratch');
  const download = workflow.indexOf('      - name: Download signed original private normal artifacts');
  const verify = workflow.indexOf('      - name: Verify authentic same-run normal inputs');
  assert.ok(setup >= 0 && setup < download && download < verify,
    'private destination must precede the untrusted downloaded bytes');
  const beforeDownload = workflow.slice(setup, download);
  assert.match(beforeDownload, /umask 077/);
  assert.match(beforeDownload, /mkdir -m 700 -- "\$RUNNER_TEMP\/private-normal-raw"/);
  assert.doesNotMatch(beforeDownload, /chmod [0-9]+.*private-normal-raw/,
    'repairing permissions after an unsafe download is not an acceptable preflight');
});

test('future GitHub-only MCPB producer has read-only repository access while publication is disabled', () => {
  const workflow = read('.github/workflows/publish-mcpb-assets.yml');
  const release = read('.github/workflows/release.yml');
  const beforeJobs = workflow.split(/^jobs:\s*$/m, 1)[0];
  assert.match(beforeJobs, /permissions:\s*\n\s*contents:\s*read/m);
  assert.doesNotMatch(workflow, /^\s*contents:\s*write\s*$/m);
  assert.match(workflow,
    /jobs:\s*\n\s*publish:.*?permissions:\s*\n\s*contents:\s*read\s*\n\s*actions:\s*read/s);
  assert.match(release,
    /\n  verify-complete:\s*\n\s*needs: \[release, publish-mcpb\]\s*\n(?:\s*# [^\n]*\n)*\s*permissions:\s*\n\s*contents: read\s*\n\s*actions: read/);
  assert.match(release,
    /\n  finalize:\s*\n\s*needs: \[release, publish-mcpb, verify-complete\]\s*\n(?:\s*# [^\n]*\n)*\s*environment: scripthold-immutable-publish\s*\n\s*permissions:\s*\n\s*contents: read\s*\n\s*actions: read/);
  assert.match(release, /Publish one verified immutable release/);
  assert.match(release, /publish-registry:\s*\n\s*needs: finalize/);
  assert.doesNotMatch(workflow, /gh release upload/);
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

test('tagged producer keeps exact-commit Scorecard and authentic Sigstore evidence without publishing', () => {
  const workflow = read('.github/workflows/release.yml');
  assert.match(workflow, /actions\/workflows\/scorecard\.yml\/runs/);
  assert.match(workflow, /-f branch=main/);
  assert.match(workflow, /-f event=push/);
  assert.match(workflow, /-f head_sha="\$\{TAG_COMMIT\}"/);
  assert.match(workflow, /select-scorecard-run/);
  assert.match(workflow, /release:\s*\n\s*needs: verify.*?permissions:\s*\n\s*contents: read\s*\n\s*id-token: write\s*\n\s*attestations: write\s*\n\s*artifact-metadata: write/s);
  assert.match(workflow, /args: release --clean --skip=publish,announce/);
  assert.match(workflow, /actions\/attest@1e69f48acb82d1966a394da916b4c1698aa569d6\s+# v4\.2\.2/);
  assert.match(workflow, /subject-checksums:\s*dist\/checksums\.txt/);
  assert.match(workflow, /provenance\.sigstore\.json/);
  assert.match(workflow, /provenance\.intoto\.jsonl/);
  assert.match(workflow, /group: scripthold-immutable-release-\$\{\{ github.ref_name \}\}/);
  assert.doesNotMatch(workflow, /gh release upload/);
  assert.doesNotMatch(workflow, /^\s*contents: write\s*$/m);
  assert.match(workflow, /Publish one verified immutable release/);
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
