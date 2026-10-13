#!/usr/bin/env node
'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const MAX_BUNDLE_BYTES = 4 * 1024 * 1024;
const MAX_METADATA_BYTES = 2 * 1024 * 1024;
const MAX_ARTIFACTS = 64;
const STATEMENT_TYPE = 'https://in-toto.io/Statement/v1';
const PREDICATE_TYPE = 'https://slsa.dev/provenance/v1';
const BUNDLE_TYPE = 'application/vnd.dev.sigstore.bundle.v0.3+json';
const REPOSITORY = 'zoster81/scripthold';
const WORKFLOW = '.github/workflows/release.yml';

function requireCondition(condition, message) {
  if (!condition) throw new Error(message);
}

function readBoundedJSON(filename, maximumBytes) {
  const stat = fs.lstatSync(filename);
  requireCondition(stat.isFile(), 'metadata must be a regular file');
  requireCondition(stat.size > 0 && stat.size <= maximumBytes, 'metadata exceeds size bounds');
  return JSON.parse(fs.readFileSync(filename, 'utf8'));
}

function parseChecksums(contents) {
  requireCondition(Buffer.byteLength(contents) <= MAX_METADATA_BYTES, 'checksums too large');
  const entries = new Map();
  for (const line of contents.trimEnd().split(/\r?\n/)) {
    const match = /^([0-9a-fA-F]{64})  ([A-Za-z0-9][A-Za-z0-9._-]{0,159})$/.exec(line);
    requireCondition(match, 'invalid checksum entry');
    requireCondition(!entries.has(match[2]), 'duplicate checksum artifact');
    entries.set(match[2], match[1].toLowerCase());
  }
  requireCondition(entries.size > 0 && entries.size <= MAX_ARTIFACTS, 'invalid checksum count');
  return entries;
}

function projectBundle(bundle, checksums, tag, sha, repo = REPOSITORY) {
  requireCondition(/^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$/.test(tag), 'invalid release tag');
  requireCondition(/^[0-9a-f]{40}$/.test(sha), 'invalid source commit');
  requireCondition(repo === REPOSITORY, 'unexpected source repository');
  requireCondition(bundle && bundle.mediaType === BUNDLE_TYPE, 'unsupported Sigstore bundle');
  requireCondition(
    bundle.verificationMaterial && bundle.verificationMaterial.certificate &&
    typeof bundle.verificationMaterial.certificate.rawBytes === 'string' &&
    bundle.verificationMaterial.certificate.rawBytes.length > 0 &&
    Array.isArray(bundle.verificationMaterial.tlogEntries) &&
    bundle.verificationMaterial.tlogEntries.length > 0,
    'missing Sigstore verification material',
  );
  const envelope = bundle.dsseEnvelope;
  requireCondition(envelope && envelope.payloadType === 'application/vnd.in-toto+json', 'invalid DSSE type');
  requireCondition(
    typeof envelope.payload === 'string' &&
    /^[A-Za-z0-9+/]+={0,2}$/.test(envelope.payload) &&
    Array.isArray(envelope.signatures) && envelope.signatures.length === 1 &&
    typeof envelope.signatures[0].sig === 'string' &&
    /^[A-Za-z0-9+/]+={0,2}$/.test(envelope.signatures[0].sig),
    'invalid DSSE signature or payload',
  );
  const statement = JSON.parse(Buffer.from(envelope.payload, 'base64').toString('utf8'));
  requireCondition(statement && statement._type === STATEMENT_TYPE &&
    statement.predicateType === PREDICATE_TYPE, 'not SLSA provenance v1');
  requireCondition(Array.isArray(statement.subject) &&
    statement.subject.length === checksums.size, 'subject count differs from checksums');
  const remaining = new Map(checksums);
  for (const subject of statement.subject) {
    requireCondition(subject && typeof subject.name === 'string' &&
      remaining.has(subject.name) && subject.digest &&
      subject.digest.sha256 === remaining.get(subject.name), 'subject checksum mismatch');
    remaining.delete(subject.name);
  }
  requireCondition(remaining.size === 0, 'unsigned checksums remain');
  const definition = statement.predicate && statement.predicate.buildDefinition;
  const params = definition && definition.externalParameters && definition.externalParameters.workflow;
  const dependencies = definition && definition.resolvedDependencies;
  const builder = statement.predicate && statement.predicate.runDetails &&
    statement.predicate.runDetails.builder;
  requireCondition(params && params.repository === 'https://github.com/' + repo &&
    params.path === WORKFLOW && params.ref === 'refs/tags/' + tag, 'release source mismatch');
  requireCondition(builder && builder.id ===
    'https://github.com/' + repo + '/' + WORKFLOW + '@refs/tags/' + tag, 'builder identity mismatch');
  requireCondition(Array.isArray(dependencies) && dependencies.some((dep) =>
    dep && dep.uri === 'git+https://github.com/' + repo + '@refs/tags/' + tag &&
    dep.digest && dep.digest.gitCommit === sha), 'source commit mismatch');
  // Preserve the original signed envelope; the separate Sigstore bundle supplies trust material.
  return JSON.stringify({
    payloadType: envelope.payloadType,
    payload: envelope.payload,
    signatures: envelope.signatures,
  }) + '\n';
}

async function hashFile(filename) {
  const digest = crypto.createHash('sha256');
  for await (const bytes of fs.createReadStream(filename)) digest.update(bytes);
  return digest.digest('hex');
}

async function validateArtifacts(entries, artifactMetadata, cwd) {
  requireCondition(Array.isArray(artifactMetadata), 'invalid GoReleaser artifact inventory');
  const root = path.resolve(cwd, 'dist');
  const rootReal = fs.realpathSync(root);
  const selected = new Map();
  for (const artifact of artifactMetadata) {
    if (!artifact || !entries.has(artifact.name)) continue;
    requireCondition(!selected.has(artifact.name) && typeof artifact.path === 'string',
      'ambiguous GoReleaser artifact');
    const candidate = path.resolve(cwd, artifact.path);
    requireCondition(candidate.startsWith(root + path.sep), 'artifact path outside dist');
    const stat = fs.lstatSync(candidate);
    requireCondition(stat.isFile(), 'artifact is not a regular file');
    const resolved = fs.realpathSync(candidate);
    requireCondition(resolved.startsWith(rootReal + path.sep), 'artifact escapes dist');
    requireCondition(await hashFile(candidate) === entries.get(artifact.name), 'local artifact digest mismatch');
    selected.set(artifact.name, candidate);
  }
  requireCondition(selected.size === entries.size, 'missing GoReleaser artifact');
  return selected;
}

function verificationArguments(filename, bundlePath, repo, tag, sha) {
  return [
    'attestation', 'verify', filename,
    '--bundle', bundlePath,
    '--repo', repo,
    '--cert-identity', 'https://github.com/' + repo + '/' + WORKFLOW + '@refs/tags/' + tag,
    '--source-ref', 'refs/tags/' + tag,
    '--source-digest', sha,
    '--deny-self-hosted-runners',
  ];
}

function verifyWithGitHubCLI(filename, bundlePath, repo, tag, sha) {
  const result = spawnSync('gh', verificationArguments(filename, bundlePath, repo, tag, sha),
    { encoding: 'utf8', timeout: 120000, maxBuffer: 1024 * 1024 });
  requireCondition(!result.error && result.status === 0,
    'cryptographic attestation verification failed for ' + path.basename(filename));
}

async function publish(options) {
  const { bundlePath, checksumsPath, artifactsPath, outputPath, tag, sha,
    repo = REPOSITORY, cwd = process.cwd(), verifier = verifyWithGitHubCLI } = options;
  const bundle = readBoundedJSON(bundlePath, MAX_BUNDLE_BYTES);
  const checksumStat = fs.lstatSync(checksumsPath);
  requireCondition(checksumStat.isFile() && checksumStat.size > 0 &&
    checksumStat.size <= MAX_METADATA_BYTES, 'checksums exceed size bounds');
  const checksums = parseChecksums(fs.readFileSync(checksumsPath, 'utf8'));
  const projection = projectBundle(bundle, checksums, tag, sha, repo);
  const artifacts = readBoundedJSON(artifactsPath, MAX_METADATA_BYTES);
  const selected = await validateArtifacts(checksums, artifacts, cwd);
  for (const [artifactName, filename] of selected) {
    await verifier(filename, bundlePath, repo, tag, sha);
    // Detect concurrent mutation between the initial checksum and final publication.
    requireCondition(await hashFile(filename) === checksums.get(artifactName),
      'artifact changed during verification');
  }
  // Never overwrite existing release evidence or publish a partial projection on verification failure.
  fs.writeFileSync(outputPath, projection, { encoding: 'utf8', flag: 'wx', mode: 0o600 });
  return selected.size;
}

if (require.main === module) {
  if (process.argv.length !== 8) {
    console.error('usage: node scripts/release-provenance.js BUNDLE CHECKSUMS ARTIFACTS TAG SHA OUTPUT');
    process.exitCode = 1;
  } else {
    publish({
      bundlePath: process.argv[2],
      checksumsPath: process.argv[3],
      artifactsPath: process.argv[4],
      tag: process.argv[5],
      sha: process.argv[6],
      outputPath: process.argv[7],
    }).then((count) => {
      console.log('verified release provenance for ' + count + ' artifacts');
    }).catch((error) => {
      console.error('release provenance: ' + error.message);
      process.exitCode = 1;
    });
  }
}

module.exports = { parseChecksums, projectBundle, validateArtifacts, verificationArguments, publish };
