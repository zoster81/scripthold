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

test('tagged producer sends unchanged GoReleaser notes to the independent read-only verifier', () => {
  const yaml = fs.readFileSync(path.join(__dirname, '..', '.github', 'workflows',
    'release.yml'), 'utf8');
  const producer = yaml.split(/^  release:\r?$/m)[1]
    ?.split(/^  publish-mcpb:\r?$/m)[0];
  const verifier = yaml.split(/^  verify-complete:\r?$/m)[1]
    ?.split(/^  finalize:\r?$/m)[0];
  const finalizer = yaml.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(producer && verifier && finalizer);
  assert.ok(producer.includes('dist/CHANGELOG.md'));
  assert.match(producer, /encodeGoReleaserNotes/);
  assert.ok(producer.includes('notes_metadata: ${{ steps.stage.outputs.notes_metadata }}'));
  assert.ok(verifier.includes('SOURCE_RELEASE_NOTES_METADATA: ${{ needs.release.outputs.notes_metadata }}'));
  assert.match(verifier, /node scripts\/release-finalizer-complete-stage\.js/);
  const localStage = fs.readFileSync(path.join(__dirname,
    'release-finalizer-complete-stage.js'), 'utf8');
  assert.match(localStage, /decodeGoReleaserNotes/);
  assert.doesNotMatch(producer, /--release-notes|generate_release_notes: true/);
  assert.match(finalizer, /Publish one verified immutable release/);
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

test('MCPB consumer pins both checkouts to the signed producer commit before tag code', () => {
  const yaml = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'publish-mcpb-assets.yml'), 'utf8');
  const trusted = yaml.indexOf('      - name: Checkout packaging workflow source');
  const tagged = yaml.indexOf('      - name: Checkout immutable release tag');
  const verified = yaml.indexOf('      - name: Verify checked-out release tag and metadata');
  const downloaded = yaml.indexOf('      - name: Download signed original private normal artifacts');
  assert.ok(trusted > 0 && tagged > trusted && verified > tagged &&
    downloaded > verified);
  for (const start of [trusted, tagged]) {
    const section = yaml.slice(start, yaml.indexOf('      - name:', start + 10));
    assert.ok(section.includes('persist-credentials: false'),
      'no checkout may persist workflow credentials');
  }
  const lines = [
    'test "${VERSION_TAG}" = "${GITHUB_REF_NAME}"',
    'test "$(git -C publication-source rev-parse HEAD)" = "${GITHUB_SHA}"',
    'test "$(git -C release-source rev-parse HEAD)" = "${GITHUB_SHA}"',
  ];
  const assertPinned = source => {
    const block = source.slice(
      source.indexOf('      - name: Verify checked-out release tag and metadata'),
      source.indexOf('      - name: Download signed original private normal artifacts'));
    const code = block.indexOf('node scripts/verify-release-version.js');
    assert.ok(code > 0);
    for (const line of lines) {
      const check = block.indexOf(line);
      assert.ok(check >= 0 && check < code,
        'source check must precede tagged code: ' + line);
    }
  };
  assertPinned(yaml);
  for (const line of lines) {
    assert.throws(() => assertPinned(yaml.replace(line, '')), /source check/);
  }
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

test('verified bytes and privileged release gate stay in distinct fail-closed jobs', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const verify = workflow.split(/^  verify-complete:\r?$/m)[1]
    ?.split(/^  finalize:\r?$/m)[0];
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(verify && finalizer, 'require separate read-only verification and protected finalization');
  assert.match(verify, /^    needs: \[release, publish-mcpb\]\r?$/m);
  assert.match(verify, /^    permissions:\r?\n      contents: read\r?\n      actions: read\r?$/m);
  assert.doesNotMatch(verify, /^    environment:|contents: write|id-token: write|secrets\.|release-finalizer-write|prepareDraftAssets/);
  assert.match(verify, /node scripts\/release-finalizer-complete-stage\.js/);
  const source = fs.readFileSync(path.join(__dirname,
    'release-finalizer-complete-stage.js'), 'utf8');
  for (const check of ['readJobVerifiedRawCompleteStaging',
    'verifyRawCompleteHandoffPayloads', 'verifyDraftSourceInventory',
    'readPublicationAuthority']) assert.ok(source.includes(check));
  assert.match(finalizer, /^    needs: \[release, publish-mcpb, verify-complete\]\r?$/m);
  assert.match(finalizer, /^    environment: scripthold-immutable-publish\r?$/m);
  assert.match(finalizer, /^    permissions:\r?\n      contents: read\r?\n      actions: read\r?$/m);
  assert.doesNotMatch(finalizer, /^      contents: write$|id-token: write/m);
  assert.match(finalizer, /Publish one verified immutable release/);
  assert.match(finalizer, /const result = await finalizeCompleteRelease\(input\)/);
  assert.match(workflow, /^  publish-registry:\r?\n    needs: finalize\r?$/m);
  // Removing the authenticated mutator must invalidate the publication gate.
  const requiredCall = /const result = await finalizeCompleteRelease\(input\)/;
  const unsafe = finalizer.replace('await finalizeCompleteRelease(input)', 'undefined');
  assert.match(finalizer, requiredCall);
  assert.throws(() => assert.match(unsafe, requiredCall),
    /regular expression/);
});

test('protected writer invokes the existing authenticated draft coordinator after approval', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(finalizer);
  const check = finalizer.indexOf('node scripts/release-finalizer-complete-stage.js');
  const admin = finalizer.indexOf('      - name: Mint short-lived administration reader');
  const token = finalizer.indexOf('      - name: Mint short-lived release writer token');
  const publish = finalizer.indexOf('      - name: Publish one verified immutable release');
  assert.ok(check >= 0 && check < admin && admin < token && token < publish);
  assert.match(finalizer, /const input = buildProtectedPublicationInput\(process\.env\);/);
  assert.match(finalizer, /const result = await finalizeCompleteRelease\(input\);/);
  assert.match(finalizer, /result\?\.kind !== 'immutable-publication'/);
  assert.doesNotMatch(finalizer, /Fail closed until single-writer|gh release (?:upload|create|edit)/);
  assert.match(workflow, /^  publish-registry:\r?\n    needs: finalize\r?$/m);
});

test('protected finalizer can mint only two repository-scoped App tokens after byte verification', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const verifier = workflow.split(/^  verify-complete:\r?$/m)[1]
    ?.split(/^  finalize:\r?$/m)[0];
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(verifier && finalizer);
  assert.doesNotMatch(verifier, /secrets\.|create-github-app-token/);
  const verified = finalizer.indexOf('node scripts/release-finalizer-complete-stage.js');
  const minted = finalizer.indexOf('      - name: Mint short-lived administration reader');
  const second = finalizer.indexOf('      - name: Mint short-lived release writer token');
  const checked = finalizer.indexOf('      - name: Publish one verified immutable release');
  assert.ok(verified >= 0 && minted > verified && second > minted &&
    checked > second,
  'protected writer must follow independent byte verification and token issuance');
  const pin = 'actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1';
  assert.equal(finalizer.split('uses: ' + pin).length - 1, 2);
  assert.equal((finalizer.match(/uses: actions\/create-github-app-token@/g) || []).length, 2);
  const sections = [finalizer.slice(minted, second), finalizer.slice(second, checked)];
  const bridge = finalizer.slice(checked);
  for (const required of [
    'SOURCE_NORMAL_MANIFEST: ${{ needs.release.outputs.manifest }}',
    'SOURCE_MCPB_MANIFEST: ${{ needs.publish-mcpb.outputs.manifest }}',
    'SOURCE_RELEASE_NOTES_METADATA: ${{ needs.release.outputs.notes_metadata }}',
    'SCRIPTHOLD_RELEASE_ADMIN_READ_TOKEN: ${{ steps.admin-reader-token.outputs.token }}',
    'SCRIPTHOLD_RELEASE_WRITE_TOKEN: ${{ steps.release-writer-token.outputs.token }}',
    'GH_TOKEN: ${{ github.token }}',
    "require('./scripts/release-finalizer-write')",
    'buildProtectedPublicationInput(process.env)',
  ]) assert.ok(bridge.includes(required), 'missing protected binding: ' + required);
  assert.match(bridge, /^        shell: bash$/m);
  assert.match(bridge, /^          set -euo pipefail$/m);
  assert.match(bridge, /await finalizeCompleteRelease\(input\)/);
  assert.doesNotMatch(bridge, /gh release|GITHUB_OUTPUT|contents: write/);
  for (const section of sections) {
    assert.match(section, /app-id: \$\{\{ secrets\.SCRIPTHOLD_RELEASE_APP_ID \}\}/);
    assert.match(section, /private-key: \$\{\{ secrets\.SCRIPTHOLD_RELEASE_APP_PRIVATE_KEY \}\}/);
    assert.match(section, /owner: \$\{\{ github\.repository_owner \}\}/);
    assert.match(section, /^          repositories: scripthold$/m);
    assert.doesNotMatch(section, /skip-token-revoke|\.\/release-finalizer-write|GH_TOKEN:|run:/);
  }
  assert.match(sections[0], /^          permission-administration: read$/m);
  assert.doesNotMatch(sections[0], /permission-contents:/);
  assert.match(sections[1], /^          permission-contents: write$/m);
  assert.doesNotMatch(sections[1], /permission-administration:/);
  assert.deepEqual((finalizer.match(/secrets\.[A-Za-z_][A-Za-z0-9_]*/g) || []).sort(),
    ['secrets.SCRIPTHOLD_RELEASE_APP_ID', 'secrets.SCRIPTHOLD_RELEASE_APP_ID',
      'secrets.SCRIPTHOLD_RELEASE_APP_PRIVATE_KEY',
      'secrets.SCRIPTHOLD_RELEASE_APP_PRIVATE_KEY'].sort());
  assert.match(finalizer, /result\?\.kind !== 'immutable-publication'/);
  assert.doesNotMatch(finalizer, /id-token: write/);
});

test('protected finalizer independently verifies 22 downloaded originals on its runner', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const verifier = workflow.split(/^  verify-complete:\r?$/m)[1]
    ?.split(/^  finalize:\r?$/m)[0];
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(verifier && finalizer);
  const required = [
    /Require isolated disk-backed complete release staging/,
    /actions\/download-artifact@[0-9a-f]{40}/,
    /artifact-ids: \$\{\{ needs\.release\.outputs\.ids \}\},\$\{\{ needs\.publish-mcpb\.outputs\.ids \}\}/,
    /skip-decompress: true/,
    /digest-mismatch: error/,
    /persist-credentials: false/,
    /node scripts\/release-finalizer-complete-stage\.js/,
    /SOURCE_NORMAL_MANIFEST: \$\{\{ needs\.release\.outputs\.manifest \}\}/,
    /SOURCE_MCPB_MANIFEST: \$\{\{ needs\.publish-mcpb\.outputs\.manifest \}\}/,
    /GH_TOKEN: \$\{\{ github\.token \}\}/,
  ];
  for (const job of [verifier, finalizer]) {
    for (const pattern of required) assert.match(job, pattern);
    assert.match(job, /^    permissions:\r?\n      contents: read\r?\n      actions: read\r?$/m);
    assert.doesNotMatch(job, /^      contents: write$|id-token: write/m);
    if (job === verifier) assert.doesNotMatch(job, /secrets\./);
  }
  assert.match(finalizer, /^    environment: scripthold-immutable-publish\r?$/m);
  assert.ok(finalizer.indexOf('node scripts/release-finalizer-complete-stage.js') <
    finalizer.indexOf('      - name: Publish one verified immutable release'));
  const implementation = fs.readFileSync(path.join(__dirname,
    'release-finalizer-complete-stage.js'), 'utf8');
  for (const check of [
    'readJobVerifiedRawCompleteStaging', 'verifyRawCompleteHandoffPayloads',
    'verifyDraftSourceInventory', 'readPublicationAuthority',
    'requireHostedTaggedVerifierContext', 'requireHostedTaggedWriterContext',
  ]) assert.ok(implementation.includes(check), 'missing independent check: ' + check);
  assert.doesNotMatch(implementation,
    /require\('\.\/release-finalizer-write'\)|finalizeCompleteRelease|prepareDraftAssets/);
});

test('future tagged publisher requires protected approval and cannot bypass the single writer', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const assertSafe = source => {
    const finalizer = source.split(/^  finalize:\r?$/m)[1]
      ?.split(/^  publish-registry:\r?$/m)[0];
    assert.ok(finalizer, 'missing protected Release finalizer');
    assert.match(finalizer, /^    needs: \[release, publish-mcpb, verify-complete\]\r?$/m);
    assert.match(finalizer, /^    environment: scripthold-immutable-publish\r?$/m);
    assert.match(finalizer, /^    permissions:\r?\n      contents: read\r?\n      actions: read\r?$/m);
    assert.doesNotMatch(finalizer, /^      contents: write$|id-token: write/m);
    assert.match(finalizer, /Publish one verified immutable release/);
    assert.match(finalizer, /const result = await finalizeCompleteRelease\(input\)/);
  };
  assertSafe(workflow);
  for (const [safe, unsafe] of [
    ['    environment: scripthold-immutable-publish', '    environment: unprotected'],
    ['    environment: scripthold-immutable-publish\n', ''],
    ['    environment: scripthold-immutable-publish\n    permissions:\n      contents: read\n      actions: read',
      '    environment: scripthold-immutable-publish\n    permissions:\n      contents: write\n      actions: read'],
    ["      - name: Publish one verified immutable release",
      "      - name: Skip authenticated publication"],
  ]) {
    assert.ok(workflow.includes(safe));
    assert.throws(() => assertSafe(workflow.replace(safe, unsafe)));
  }
});

test('unprivileged verifier binds GitHub-owned tagged context before accepting complete source authority', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const verifier = workflow.split(/^  verify-complete:\r?$/m)[1]
    ?.split(/^  finalize:\r?$/m)[0];
  assert.ok(verifier);
  const source = fs.readFileSync(path.join(__dirname,
    'release-finalizer-complete-stage.js'), 'utf8');
  assert.match(verifier, /node scripts\/release-finalizer-complete-stage\.js/);
  const guard = /contextCheck\(process\.env, trusted\.manifest, process\.env\.GITHUB_SHA\)/;
  assert.match(source, /requireHostedTaggedVerifierContext/);
  assert.match(source, /requireHostedTaggedWriterContext/);
  assert.match(source, guard);
  const guardPosition = source.search(guard);
  const draftSourcePosition = source.indexOf('verifyDraftSourceInventory(trusted.manifest');
  const authorityPosition = source.indexOf('await readPublicationAuthority');
  assert.ok(guardPosition >= 0 && draftSourcePosition > guardPosition &&
    authorityPosition > guardPosition, 'verify tag source before authorizing any draft source');
  assert.doesNotMatch(verifier, /release-finalizer-write|contents: write|secrets\.|finalizeCompleteRelease/);
  assert.throws(() => assert.match(source.replace(guard, ''), guard),
    /regular expression/);
});

test('future finalizer gates Registry on all 22 authenticated original files', () => {
  const workflow = fs.readFileSync(path.join(__dirname, '..', '.github',
    'workflows', 'release.yml'), 'utf8');
  const verifier = workflow.split(/^  verify-complete:\r?$/m)[1]
    ?.split(/^  finalize:\r?$/m)[0];
  const finalizer = workflow.split(/^  finalize:\r?$/m)[1]
    ?.split(/^  publish-registry:\r?$/m)[0];
  assert.ok(verifier && finalizer, 'missing separate verifier or finalizer');
  assert.match(verifier, /needs: \[release, publish-mcpb\]/);
  assert.match(verifier, /contents: read/);
  assert.match(verifier, /actions: read/);
  assert.match(verifier, /node scripts\/release-finalizer-complete-stage\.js/);
  const source = fs.readFileSync(path.join(__dirname,
    'release-finalizer-complete-stage.js'), 'utf8');
  assert.match(source, /readJobVerifiedRawCompleteStaging/);
  assert.match(source, /verifyRawCompleteHandoffPayloads/);
  assert.match(source, /verifyDraftSourceInventory/,
    'the verified 22 originals must match the future draft writer input contract');
  assert.doesNotMatch(source, /release-finalizer-write|prepareDraftAssets/,
    'read-only verification cannot import or invoke a release mutator');
  assert.doesNotMatch(source, /verifyStagedNormalAttestations/,
    'the successful MCPB dependency already verified all 12 genuine signatures');
  assert.match(source, /readPublicationAuthority/);
  assert.ok(verifier.includes('needs.release.outputs.ids'));
  assert.ok(verifier.includes('needs.publish-mcpb.outputs.ids'));
  assert.match(verifier, /skip-decompress: true/);
  assert.match(verifier, /digest-mismatch: error/);
  assert.doesNotMatch(verifier, /gh release (?:upload|create|edit)|contents: write/);
  assert.match(finalizer, /needs: \[release, publish-mcpb, verify-complete\]/);
  assert.match(finalizer, /Publish one verified immutable release/);
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
