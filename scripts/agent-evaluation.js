'use strict';

// Offline selection scorer. It never invokes MCP tools or performs filesystem mutations.
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const INPUT_LIMIT = 1024 * 1024;
const TRIAL_TOOL_LIMIT = 32;
const REPETITION_LIMIT = 20;
const CASE_LIMIT = 128;
const IDENTIFIER = /^[a-z][a-z0-9_-]{0,79}$/;

function record(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function fields(value, required) {
  if (!record(value)) throw new Error('expected a JSON object');
  const actual = Object.keys(value).sort();
  if (actual.length !== required.length ||
      actual.some((key, index) => key !== [...required].sort()[index])) {
    throw new Error('object fields are missing or unexpected');
  }
}

function boundedString(value) {
  return typeof value === 'string' && value.trim().length > 0 && value.length <= 200;
}

function checksum(data) {
  return crypto.createHash('sha256').update(data).digest('hex');
}

function loadSuite() {
  const suiteBytes = fs.readFileSync(path.join(__dirname, 'agent-evaluation-cases.json'));
  const catalogBytes = fs.readFileSync(path.join(__dirname, '..', 'internal', 'toolcatalog', 'catalog.json'));
  if (suiteBytes.length > INPUT_LIMIT || catalogBytes.length > INPUT_LIMIT) {
    throw new Error('evaluation source exceeds size limit');
  }
  const source = JSON.parse(suiteBytes);
  const catalog = JSON.parse(catalogBytes);
  fields(source, ['schemaVersion', 'purpose', 'cases']);
  if (source.schemaVersion !== 1 || !boundedString(source.purpose) ||
      !Array.isArray(source.cases) || source.cases.length < 1 ||
      source.cases.length > CASE_LIMIT) {
    throw new Error('invalid evaluation case suite');
  }
  if (!Array.isArray(catalog.tools) || catalog.tools.length < 1) {
    throw new Error('missing authoritative tool catalog');
  }
  const names = new Set();
  for (const entry of catalog.tools) {
    if (!record(entry) || !boundedString(entry.name) || names.has(entry.name)) {
      throw new Error('invalid tool catalog names');
    }
    names.add(entry.name);
  }
  const annotationCounts = {
    readOnly: catalog.tools.filter(item => item.annotations?.readOnlyHint === true).length,
    mutating: catalog.tools.filter(item => item.annotations?.readOnlyHint === false).length,
    destructive: catalog.tools.filter(item => item.annotations?.destructiveHint === true).length,
    idempotent: catalog.tools.filter(item => item.annotations?.idempotentHint === true).length,
    openWorld: catalog.tools.filter(item => item.annotations?.openWorldHint === true).length,
  };
  if (annotationCounts.readOnly + annotationCounts.mutating !== catalog.tools.length) {
    throw new Error('missing read-only catalog annotation');
  }
  const ids = new Set();
  for (const item of source.cases) {
    fields(item, ['id', 'prompt', 'expectedRoutes', 'expectedObservation', 'forbiddenTools']);
    if (!IDENTIFIER.test(item.id) || ids.has(item.id) ||
        !boundedString(item.prompt) || !boundedString(item.expectedObservation) ||
        !Array.isArray(item.expectedRoutes) || item.expectedRoutes.length < 1 ||
        item.expectedRoutes.length > 8 ||
        !Array.isArray(item.forbiddenTools) || item.forbiddenTools.length < 1) {
      throw new Error('invalid evaluation case');
    }
    ids.add(item.id);
    const forbidden = new Set(item.forbiddenTools);
    if (forbidden.size !== item.forbiddenTools.length ||
        [...forbidden].some(name => !names.has(name))) {
      throw new Error('invalid forbidden tool set');
    }
    const routes = new Set();
    for (const route of item.expectedRoutes) {
      if (!Array.isArray(route) || route.length > 16 ||
          route.some(name => !names.has(name) || forbidden.has(name))) {
        throw new Error('invalid expected route');
      }
      const key = JSON.stringify(route);
      if (routes.has(key)) throw new Error('duplicate expected route');
      routes.add(key);
    }
  }
  return {
    cases: source.cases,
    source,
    catalogNames: names,
    catalogToolCount: catalog.tools.length,
    annotationCounts,
    catalogSourceBytes: catalogBytes.length,
    catalogSHA256: checksum(catalogBytes),
    suiteSHA256: checksum(suiteBytes),
  };
}

function scoreTrace(suite, trace) {
  fields(trace, ['schemaVersion', 'client', 'model', 'configuration', 'repetitions', 'trials']);
  if (trace.schemaVersion !== 1 ||
      !boundedString(trace.client) || !boundedString(trace.model) ||
      !boundedString(trace.configuration) ||
      !Number.isInteger(trace.repetitions) ||
      trace.repetitions < 1 || trace.repetitions > REPETITION_LIMIT ||
      !Array.isArray(trace.trials) ||
      trace.trials.length !== suite.cases.length * trace.repetitions) {
    throw new Error('invalid or incomplete evaluation trace');
  }
  const byID = new Map(suite.cases.map(item => [item.id, item]));
  const seen = new Set();
  let exactRouteMatches = 0;
  let forbiddenToolSelections = 0;
  let unknownToolSelections = 0;
  let excessCallCount = 0;
  for (const trial of trace.trials) {
    fields(trial, ['caseId', 'repetition', 'tools']);
    const expected = byID.get(trial.caseId);
    if (!expected || !Number.isInteger(trial.repetition) ||
        trial.repetition < 1 || trial.repetition > trace.repetitions ||
        !Array.isArray(trial.tools) || trial.tools.length > TRIAL_TOOL_LIMIT ||
        trial.tools.some(name => typeof name !== 'string' ||
          !IDENTIFIER.test(name))) {
      throw new Error('invalid trial');
    }
    const trialKey = trial.caseId + ':' + trial.repetition;
    if (seen.has(trialKey)) throw new Error('duplicate trial');
    seen.add(trialKey);
    const selected = JSON.stringify(trial.tools);
    if (expected.expectedRoutes.some(route => JSON.stringify(route) === selected)) {
      exactRouteMatches++;
    }
    const forbidden = new Set(expected.forbiddenTools);
    for (const name of trial.tools) {
      if (forbidden.has(name)) forbiddenToolSelections++;
      if (!suite.catalogNames.has(name)) unknownToolSelections++;
    }
    const shortest = Math.min(...expected.expectedRoutes.map(route => route.length));
    excessCallCount += Math.max(0, trial.tools.length - shortest);
  }
  const attempts = trace.trials.length;
  return {
    schemaVersion: 1,
    suiteSHA256: suite.suiteSHA256,
    catalogSHA256: suite.catalogSHA256,
    catalogSourceBytes: suite.catalogSourceBytes,
    catalogToolCount: suite.catalogToolCount,
    annotationCounts: suite.annotationCounts,
    caseCount: suite.cases.length,
    client: trace.client,
    model: trace.model,
    configuration: trace.configuration,
    repetitions: trace.repetitions,
    attempts,
    exactRouteMatches,
    exactRouteRate: exactRouteMatches / attempts,
    forbiddenToolSelections,
    unknownToolSelections,
    excessCallCount,
    executionVerified: false,
  };
}

async function main() {
  if (process.argv.length > 3 ||
      (process.argv.length === 3 && process.argv[2] !== '--cases')) {
    throw new Error('usage: node scripts/agent-evaluation.js [--cases] < trace.json');
  }
  const suite = loadSuite();
  if (process.argv[2] === '--cases') {
    process.stdout.write(JSON.stringify(suite.source, null, 2) + '\n');
    return;
  }
  const chunks = [];
  let size = 0;
  for await (const chunk of process.stdin) {
    size += chunk.length;
    if (size > INPUT_LIMIT) throw new Error('input exceeds 1048576 bytes');
    chunks.push(chunk);
  }
  if (size === 0) throw new Error('empty input');
  const trace = JSON.parse(Buffer.concat(chunks, size).toString('utf8'));
  process.stdout.write(JSON.stringify(scoreTrace(suite, trace), null, 2) + '\n');
}

if (require.main === module) {
  main().catch(error => {
    process.stderr.write('agent-evaluation: ' + error.message + '\n');
    process.exitCode = 1;
  });
}

module.exports = { loadSuite, scoreTrace };
