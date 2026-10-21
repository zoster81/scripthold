const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const {
  classifyEvent,
  classifyPaths,
  changedPathsUseOnlyRegularFiles,
  matrixOutputsForTier,
  parseLsTreeZ,
  parseNameStatusZ,
  requiredJobsForTier,
  selectGoPackages,
  verifyJobResults,
} = require('./test-ci-policy.js');

test('docs-only paths use the docs tier', () => {
  assert.equal(classifyPaths(['README.md', 'docs/HTTP_SECURITY.md']).tier, 'docs');
});

test('Go changes plus documentation use the go tier', () => {
  assert.equal(classifyPaths(['internal/security/path.go', 'docs/ARCHITECTURE.md']).tier, 'go');
  assert.equal(classifyPaths(['filetoolsserver/handler/path_test.go']).tier, 'go');
});

test('workflow, script, metadata, fixture, and unknown changes fail closed to full', () => {
  for (const paths of [
    ['.github/workflows/test.yml'],
    ['scripts/test-ci-policy.js'],
    ['testarchitecture/evidence-map.json'],
    ['go.mod'],
    ['Dockerfile'],
    ['internal/sourceintelligence/testdata/provider-contracts.json'],
    ['docs/HTTP_SECURITY.md', 'config/example.json'],
  ]) {
    assert.equal(classifyPaths(paths).tier, 'full', paths.join(', '));
  }
});

test('empty or malformed paths fail closed to full', () => {
  for (const paths of [[], ['../README.md'], ['/README.md'], ['docs\\README.md'], ['docs//README.md'], ['']]) {
    assert.equal(classifyPaths(paths).tier, 'full', JSON.stringify(paths));
  }
});

test('rename parsing evaluates both source and destination paths', () => {
  const goRename = Buffer.from('R100\0internal/security/path.go\0docs/path.md\0');
  assert.deepEqual(parseNameStatusZ(goRename), ['internal/security/path.go', 'docs/path.md']);
  assert.equal(classifyPaths(parseNameStatusZ(goRename)).tier, 'go');

  const scriptRename = Buffer.from('R087\0scripts/old.js\0docs/old.md\0');
  assert.equal(classifyPaths(parseNameStatusZ(scriptRename)).tier, 'full');
});

test('malformed and type-changing name-status data fails closed', () => {
  assert.throws(() => parseNameStatusZ(Buffer.from('R100\0only-one-path\0')));
  assert.throws(() => parseNameStatusZ(Buffer.from('X\0README.md\0')));
  assert.throws(() => parseNameStatusZ(Buffer.from('T\0docs/GUIDE.md\0')));
  assert.throws(() => parseNameStatusZ(Buffer.from('U\0docs/GUIDE.md\0')));
});

test('tree modes reject symlinks and gitlinks from narrow evidence tiers', () => {
  const regularTree = parseLsTreeZ(Buffer.from(
    '100644 blob 0123456789012345678901234567890123456789\tREADME.md\0' +
    '100755 blob 1111111111111111111111111111111111111111\tinternal/tool.go\0',
  ));
  const unsafeTree = parseLsTreeZ(Buffer.from(
    '120000 blob 2222222222222222222222222222222222222222\tdocs/GUIDE.md\0' +
    '160000 commit 3333333333333333333333333333333333333333\tvendor/tool\0',
  ));

  assert.equal(changedPathsUseOnlyRegularFiles(['README.md', 'internal/tool.go'], regularTree, new Map()), true);
  assert.equal(changedPathsUseOnlyRegularFiles(['docs/GUIDE.md'], regularTree, unsafeTree), false);
  assert.equal(changedPathsUseOnlyRegularFiles(['vendor/tool'], unsafeTree, new Map()), false);
  assert.throws(() => parseLsTreeZ(Buffer.from('100644 blob bad\tREADME.md\0')));
});

test('main pushes, manual runs, and unknown events are always full qualification', () => {
  assert.equal(classifyEvent('push', ['README.md']).tier, 'full');
  assert.equal(classifyEvent('workflow_dispatch', ['README.md']).tier, 'full');
  assert.equal(classifyEvent('schedule', ['README.md']).tier, 'full');
  assert.equal(classifyEvent('pull_request', ['README.md']).tier, 'docs');
});

test('required jobs are monotonic across evidence tiers', () => {
  const docs = requiredJobsForTier('docs');
  const go = requiredJobsForTier('go');
  const full = requiredJobsForTier('full');

  for (const job of docs) assert.ok(go.includes(job), `go tier missing docs job ${job}`);
  for (const job of go) assert.ok(full.includes(job), `full tier missing go job ${job}`);
  assert.ok(full.includes('native-smoke'));
  assert.ok(full.includes('cross-build'));
  assert.ok(full.includes('container-smoke'));
  assert.ok(full.includes('release-config'));
});

test('matrix selection preserves full exact-SHA breadth while narrowing ordinary Go evidence', () => {
  const go = matrixOutputsForTier('go');
  assert.deepEqual(go.race, [{os: 'ubuntu-26.04', scope: 'targeted'}]);
  assert.deepEqual(go.platform, [
    {os: 'windows-latest', platform: 'windows', scope: 'targeted'},
    {os: 'macos-latest', platform: 'darwin', scope: 'targeted'},
  ]);

  const full = matrixOutputsForTier('full');
  assert.deepEqual(full.race, [
    {os: 'ubuntu-26.04', scope: 'full'},
    {os: 'windows-latest', scope: 'full'},
    {os: 'macos-latest', scope: 'full'},
  ]);
  assert.deepEqual(full.platform, [
    {os: 'windows-latest', platform: 'windows', scope: 'full'},
    {os: 'macos-latest', platform: 'darwin', scope: 'full'},
  ]);
});

test('PR trusted-base matrices still route Linux jobs to Ubuntu 26.04', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows', 'test.yml'), 'utf8');
  const mapping = "runs-on: ${{ matrix.os == 'ubuntu-latest' && 'ubuntu-26.04' || matrix.os }}";
  assert.equal(workflow.split(mapping).length - 1, 2);
});

function assertNonpublishingUbuntu26BuildProbe(yaml) {
  assert.match(yaml, /^on:\r?\n  pull_request:\r?\n    paths:\r?\n      - '\.github\/workflows\/ubuntu26-goreleaser-build-probe\.yml'\r?\n  workflow_dispatch:\r?$/m);
  assert.match(yaml, /^permissions:\r?\n  contents: read\r?$/m);
  assert.match(yaml, /^    runs-on: ubuntu-26\.04\r?$/m);
  assert.match(yaml, /^          version: 'v2\.18\.1'\r?$/m);
  assert.match(yaml, /^          args: build --clean --skip=validate\r?$/m);
  assert.match(yaml, /df -hPT/);
  assert.match(yaml, /findmnt -T/);
  assert.doesNotMatch(yaml, /contents: write|id-token: write|GITHUB_TOKEN|gh release|upload-artifact|--snapshot|args: release/);
}

test('Ubuntu 26 GoReleaser rehearsal only builds locally on hosted runner', () => {
  const filename = path.join(__dirname, '..', '.github', 'workflows',
    'ubuntu26-goreleaser-build-probe.yml');
  const yaml = fs.readFileSync(filename, 'utf8');
  assertNonpublishingUbuntu26BuildProbe(yaml);
  for (const [expected, unsafe] of [
    ['    runs-on: ubuntu-26.04', '    runs-on: ubuntu-latest'],
    ['  contents: read', '  contents: write'],
    ['          args: build --clean --skip=validate', '          args: release --clean'],
    ["          version: 'v2.18.1'", "          version: 'latest'"],
  ]) {
    assert.ok(yaml.includes(expected));
    assert.throws(() => assertNonpublishingUbuntu26BuildProbe(
      yaml.replace(expected, unsafe)));
  }
});

test('manual Ubuntu 26 complete normal rehearsal cannot publish or trust a local tag alias', () => {
  const file = path.join(__dirname, '..', '.github', 'workflows',
    'ubuntu26-normal-release-rehearsal.yml');
  const yaml = fs.readFileSync(file, 'utf8');
  const assertSafe = text => {
    assert.match(text, /^on:\r?\n  workflow_dispatch:\r?\n    inputs:/m);
    assert.match(text, /^permissions:\r?\n  contents: read\r?$/m);
    assert.match(text, /^    runs-on: ubuntu-26\.04\r?$/m);
    assert.match(text, /test "\$\{GITHUB_REF\}" = refs\/heads\/main/);
    assert.match(text, /test "\$\(git cat-file -t "\$\{tagref\}"\)" = tag/);
    assert.match(text, /git ls-remote --tags origin/);
    assert.match(text, /test "\$\(git rev-parse HEAD\)" = "\$\{commit\}"/);
    assert.match(text, /test "\$\(git rev-parse HEAD\)" = "\$\{TAG_COMMIT\}"/);
    assert.equal(text.split('persist-credentials: false').length - 1, 2);
    assert.doesNotMatch(text, /persist-credentials: true/);
    assert.match(text, /workdir: source/);
    assert.match(text, /^          version: 'v2\.18\.1'\r?$/m);
    assert.match(text, /^          args: release --clean --skip=publish,announce\r?$/m);
    assert.match(text, /node \.\.\/verifier\/scripts\/release-normal-rehearsal\.js/);
    assert.doesNotMatch(text, /contents: write|packages: write|id-token: write|attestations: write/);
    assert.doesNotMatch(text, /GITHUB_TOKEN|GH_TOKEN|secrets\.|gh release|upload-artifact|--snapshot|gh api/);
  };
  assertSafe(yaml);
  for (const [safe, unsafe] of [
    ['  contents: read', '  contents: write'],
    ['release --clean --skip=publish,announce', 'release --clean'],
    ['persist-credentials: false', 'persist-credentials: true'],
    ['test "${GITHUB_REF}" = refs/heads/main', 'test "${GITHUB_REF}" = refs/tags/main'],
    ["version: 'v2.18.1'", "version: 'latest'"],
  ]) {
    assert.ok(yaml.includes(safe));
    assert.throws(() => assertSafe(yaml.replace(safe, unsafe)));
  }
});

test('real historical Sigstore proof is nonpublishing and pins source identity', () => {
  const file = path.join(__dirname, '..', '.github', 'workflows',
    'ubuntu26-authentic-provenance-proof.yml');
  const yaml = fs.readFileSync(file, 'utf8');
  const assertSafe = text => {
    assert.match(text, /^on:\r?\n  pull_request:\r?\n    paths:\r?\n      - '\.github\/workflows\/ubuntu26-authentic-provenance-proof\.yml'\r?\n  workflow_dispatch:\r?$/m);
    assert.match(text, /^permissions:\r?\n  contents: read\r?$/m);
    assert.match(text, /^    runs-on: ubuntu-26\.04\r?$/m);
    assert.match(text, /persist-credentials: false/);
    assert.match(text, /v3\.3\.0/);
    assert.match(text, /aac3a46bf71135f73594341f1b97dcf08c552832/);
    assert.match(text, /5d1cde029753e4b75fbe3236febaecb905a82820/);
    assert.match(text, /gh release download v3\.3\.0/);
    assert.match(text, /verifyWithGitHubCLI\(subject, bundlePath, repo, tag, sha\)/);
    const verifier = fs.readFileSync(path.join(__dirname, 'release-provenance.js'), 'utf8');
    assert.match(verifier, /'--deny-self-hosted-runners'/);
    assert.match(text, /projectBundle/);
    assert.doesNotMatch(text, /contents: write|id-token: write|attestations: write|packages: write/);
    assert.doesNotMatch(text, /gh release (?:upload|create|edit|delete)|goreleaser|upload-artifact|secrets\./);
  };
  assertSafe(yaml);
  for (const [safe, unsafe] of [
    ['  contents: read', '  contents: write'],
    ['    runs-on: ubuntu-26.04', '    runs-on: ubuntu-latest'],
    ['persist-credentials: false', 'persist-credentials: true'],
    ['verifyWithGitHubCLI(subject, bundlePath, repo, tag, sha)', 'console.log(subject)'],
  ]) {
    assert.ok(yaml.includes(safe));
    assert.throws(() => assertSafe(yaml.replace(safe, unsafe)));
  }
});

test('targeted package selection is derived from the evidence map', () => {
  const evidenceMap = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'testarchitecture', 'evidence-map.json'), 'utf8'));
  const race = selectGoPackages(evidenceMap, 'race', 'linux');
  assert.ok(race.includes('./filetoolsserver/handler'));
  assert.ok(race.includes('./internal/backupstore'));
  assert.ok(race.includes('./internal/sourceintelligence'));
  assert.ok(!race.includes('./internal/security'));

  const windowsPlatform = selectGoPackages(evidenceMap, 'platform', 'windows');
  assert.ok(windowsPlatform.includes('./cmd/scripthold'));
  assert.ok(windowsPlatform.includes('./internal/filesystem'));
  assert.ok(!windowsPlatform.includes('./internal/httptransport'));
});

test('the policy job explicitly executes every Node regression test in scripts', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows', 'test.yml'), 'utf8');
  const policyJob = workflow.split(/^  policy:\r?$/m)[1]?.split(/^  gitleaks:\r?$/m)[0];
  assert.ok(policyJob, 'missing CI policy job');
  const commands = [...policyJob.matchAll(/^ {8}run: node --test ([^\r\n]+)$/gm)];
  assert.ok(commands.length > 0, 'missing Node regression execution in policy job');
  const selected = new Set(commands.flatMap(([, args]) => args.trim().split(/\s+/)));
  const required = fs.readdirSync(__dirname)
    .filter(name => name.endsWith('.test.js'))
    .map(name => 'scripts/' + name);
  assert.deepEqual(required.filter(name => !selected.has(name)), [],
    'unselected Node regression tests must fail the CI policy');
});

test('future tagged normal producer stages 15 genuine originals before any publication', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows',
    'release.yml'), 'utf8');
  const producer = workflow.split(/^  release:\r?$/m)[1]
    ?.split(/^  publish-mcpb:\r?$/m)[0];
  assert.ok(producer, 'missing original release.yml tagged normal producer');
  assert.match(producer, /args: release --clean --skip=publish,announce/);
  assert.match(producer, /id-token: write/);
  assert.match(producer, /attestations: write/);
  assert.match(producer, /uses: actions\/attest@/);
  assert.match(producer, /release-normal-source/);
  assert.match(producer, /release-normal-provenance-pair/);
  assert.equal((producer.match(/uses: actions\/upload-artifact@/g) || []).length, 15);
  assert.equal((producer.match(/archive: false/g) || []).length, 15);
  assert.match(producer, /needs: verify/);
  assert.doesNotMatch(producer, /contents: write|gh release (?:upload|create|edit)|GITHUB_TOKEN: \\$\\{\\{ secrets/);
});

test('raw upload output binding does not mask Bash command-substitution failures', () => {
  for (const filename of ['release.yml', 'publish-mcpb-assets.yml']) {
    const workflow = fs.readFileSync(path.join(__dirname, '..',
      '.github', 'workflows', filename), 'utf8');
    assert.doesNotMatch(workflow, /export STAGE_(?:IDS|DIGESTS)=/,
      'export with assignment masks command substitution failures');
    assert.match(workflow, /^\s+STAGE_IDS="\$\(IFS=,; echo "\$\{ids\[\*\]\}"\)"$/m);
    assert.match(workflow, /^\s+STAGE_DIGESTS="\$\(IFS=,; echo "\$\{digests\[\*\]\}"\)"$/m);
    assert.match(workflow, /^\s+export STAGE_IDS STAGE_DIGESTS$/m);
  }
});

test('tagged producer preserves genuine signed DSSE without duplicating consumer signature checks', () => {
  const root = path.join(__dirname, '..', '.github', 'workflows');
  const workflow = fs.readFileSync(path.join(root, 'release.yml'), 'utf8');
  const producer = workflow.split(/^  release:\r?$/m)[1]
    ?.split(/^  publish-mcpb:\r?$/m)[0];
  const consumer = fs.readFileSync(path.join(root, 'publish-mcpb-assets.yml'), 'utf8');
  assert.ok(producer);
  assert.match(producer, /projectBundle/);
  assert.match(producer, /parseChecksums/);
  assert.doesNotMatch(producer,
    /node scripts\/release-provenance\.js|verifyWithGitHubCLI|gh attestation verify/);
  assert.match(consumer, /verifyStagedNormalAttestations/);
});

test('tagged producer sends unchanged GoReleaser v2.18.1 notes directly to read-only finalizer', () => {
  const yaml = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows',
    'release.yml'), 'utf8');
  const producer = yaml.split(/^  release:\r?$/m)[1]
    ?.split(/^  publish-mcpb:\r?$/m)[0];
  const finalizer = yaml.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(producer && finalizer);
  assert.ok(producer.includes('dist/CHANGELOG.md'));
  assert.match(producer, /encodeGoReleaserNotes/);
  assert.ok(producer.includes('notes_metadata: ${{ steps.stage.outputs.notes_metadata }}'));
  assert.ok(finalizer.includes('SOURCE_RELEASE_NOTES_METADATA: ${{ needs.release.outputs.notes_metadata }}'));
  assert.match(finalizer, /decodeGoReleaserNotes/);
  assert.doesNotMatch(producer, /--release-notes|generate_release_notes: true/);
  assert.match(finalizer, /publication is disabled until the single-writer finalizer is qualified/);
});

test('future signed normal producer has an independently authenticated fifteen-file Linux consumer', () => {
  const consumer = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'publish-mcpb-assets.yml'), 'utf8');
  assert.match(consumer, /jobs:\s*publish:/);
  assert.match(consumer, /contents: read/);
  assert.match(consumer, /actions: read/);
  assert.ok(consumer.includes('artifact-ids: ${{ inputs.normal_ids }}'));
  assert.match(consumer, /skip-decompress: true/);
  assert.match(consumer, /digest-mismatch: error/);
  assert.match(consumer, /readJobVerifiedRawNormalStaging/);
  assert.match(consumer, /verifyRawNormalHandoffPayloads/);
  assert.match(consumer, /verifyStagedNormalAttestations/);
  assert.doesNotMatch(consumer, /contents: write|id-token: write|attestations: write|gh release (?:upload|create|edit)/);
});

test('future GitHub-only MCPB job consumes signed private originals and emits seven raw assets', () => {
  const root = path.join(__dirname, '..', '.github', 'workflows');
  const workflow = fs.readFileSync(path.join(root, 'release.yml'), 'utf8');
  const mcpb = fs.readFileSync(path.join(root, 'publish-mcpb-assets.yml'), 'utf8');
  const caller = workflow.split(/^  publish-mcpb:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(caller && caller.includes('needs: release'));
  assert.ok(caller.includes('normal_ids: ${{ needs.release.outputs.ids }}'));
  assert.ok(caller.includes('normal_digests: ${{ needs.release.outputs.digests }}'));
  assert.ok(caller.includes('normal_manifest: ${{ needs.release.outputs.manifest }}'));
  assert.ok(caller.includes('normal_attempt: ${{ needs.release.outputs.attempt }}'));
  assert.match(mcpb, /readJobVerifiedRawNormalStaging/);
  assert.match(mcpb, /verifyRawNormalHandoffPayloads/);
  assert.match(mcpb, /verifyStagedNormalAttestations/);
  assert.match(mcpb, /verifyPreparedMcpbBinaryPayloads/);
  assert.ok(mcpb.includes('artifact-ids: ${{ inputs.normal_ids }}'));
  assert.match(mcpb, /skip-decompress: true/);
  assert.match(mcpb, /digest-mismatch: error/);
  assert.equal((mcpb.match(/uses: actions\/upload-artifact@/g) || []).length, 7);
  assert.equal((mcpb.match(/archive: false/g) || []).length, 7);
  assert.doesNotMatch(mcpb, /gh release upload|curl -fSL --retry 3 --retry-all-errors[\s\S]*releases\/download/);
  assert.doesNotMatch(mcpb, /contents: write/);
});

test('future tagged finalizer requires an approved release environment but cannot publish yet', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const assertSafe = source => {
    const finalizer = source.split(/^  finalize:\r?$/m)[1]
      ?.split(/^  publish-registry:\r?$/m)[0];
    assert.ok(finalizer, 'missing single Release finalizer');
    assert.match(finalizer, /^    environment: scripthold-immutable-publish\r?$/m);
    assert.match(finalizer, /^    permissions:\r?\n      contents: read\r?\n      actions: read\r?$/m);
    assert.doesNotMatch(finalizer, /contents: write|id-token: write|secrets\.|finalizeCompleteRelease|prepareDraftAssets/);
    assert.match(finalizer, /publication is disabled until the single-writer finalizer is qualified/);
    assert.match(finalizer, /^          exit 1\r?$/m);
  };
  assertSafe(workflow);
  for (const [safe, unsafe] of [
    ['    environment: scripthold-immutable-publish', '    environment: unprotected'],
    ['    environment: scripthold-immutable-publish\n', ''],
    ['    environment: scripthold-immutable-publish\n    permissions:\n      contents: read\n      actions: read',
      '    environment: scripthold-immutable-publish\n    permissions:\n      contents: write\n      actions: read'],
    ["          echo 'publication is disabled until the single-writer finalizer is qualified' >&2",
      "          echo 'publication is permitted'"],
  ]) {
    assert.ok(workflow.includes(safe));
    assert.throws(() => assertSafe(workflow.replace(safe, unsafe)));
  }
});

test('protected read-only finalizer binds GitHub-owned tagged context before staging authority', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(finalizer);
  const importGuard = /requireHostedTaggedFinalizerContext\s*\}\s*=\s*\r?\n\s*require\('\.\/scripts\/release-finalizer-preflight'\)/;
  const call = /requireHostedTaggedFinalizerContext\(process\.env,\s*\r?\n?\s*trusted\.manifest,\s*process\.env\.GITHUB_SHA\)/;
  assert.match(finalizer, importGuard);
  assert.match(finalizer, call);
  const guardPosition = finalizer.search(call);
  const draftSourcePosition = finalizer.indexOf('verifyDraftSourceInventory(trusted.manifest');
  const authorityPosition = finalizer.indexOf('await readPublicationAuthority');
  assert.ok(guardPosition >= 0 && draftSourcePosition > guardPosition &&
    authorityPosition > guardPosition, 'verify tag source before authorizing any draft source');
  assert.doesNotMatch(finalizer, /release-finalizer-write|contents: write|secrets\.|finalizeCompleteRelease/);
  assert.throws(() => {
    const unguarded = finalizer.replace(call, '');
    assert.match(unguarded, call);
  }, /regular expression/);
});

test('future finalizer gates Registry on all 22 authenticated original files', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(finalizer, 'missing separate finalizer after genuine MCPB producer');
  assert.match(finalizer, /needs: \[release, publish-mcpb\]/);
  assert.match(finalizer, /contents: read/);
  assert.match(finalizer, /actions: read/);
  assert.match(finalizer, /readJobVerifiedRawCompleteStaging/);
  assert.match(finalizer, /verifyRawCompleteHandoffPayloads/);
  assert.match(finalizer, /verifyDraftSourceInventory/,
    'the verified 22 originals must match the future draft writer input contract');
  assert.doesNotMatch(finalizer, /release-finalizer-write|prepareDraftAssets/,
    'read-only finalizer cannot import or invoke a release mutator');
  assert.doesNotMatch(finalizer, /verifyStagedNormalAttestations/,
    'the successful MCPB dependency already verified all 12 genuine signatures');
  assert.match(finalizer, /readPublicationAuthority/);
  assert.ok(finalizer.includes('needs.release.outputs.ids'));
  assert.ok(finalizer.includes('needs.publish-mcpb.outputs.ids'));
  assert.match(finalizer, /skip-decompress: true/);
  assert.match(finalizer, /digest-mismatch: error/);
  assert.doesNotMatch(finalizer, /gh release (?:upload|create|edit)|contents: write/);
  assert.match(finalizer, /publication is disabled until the single-writer finalizer is qualified/);
  const registry = workflow.split(/^  publish-registry:\r?$/m)[1];
  assert.ok(registry.includes('needs: finalize'),
    'Registry must never publish after merely building MCPB assets');
});

test('aggregate verification rejects failed or missing required evidence', () => {
  const success = Object.fromEntries(requiredJobsForTier('docs').map((job) => [job, 'success']));
  success['linux-regression'] = 'skipped';
  assert.doesNotThrow(() => verifyJobResults('docs', success));

  assert.throws(() => verifyJobResults('docs', {...success, gitleaks: 'failure'}));
  assert.throws(() => verifyJobResults('docs', {...success, policy: 'skipped'}));
  assert.throws(() => verifyJobResults('unknown', success));
});
