'use strict';

const assert = require('node:assert/strict');
const { spawnSync } = require('node:child_process');
const path = require('node:path');
const test = require('node:test');
const { loadSuite, scoreTrace } = require('./agent-evaluation');

function traceFor(suite, repetitions = 2) {
  const trials = [];
  for (let repetition = 1; repetition <= repetitions; repetition++) {
    for (const item of suite.cases) {
      trials.push({
        caseId: item.id,
        repetition,
        tools: [...item.expectedRoutes[0]],
      });
    }
  }
  return {
    schemaVersion: 1,
    client: 'offline-test-client',
    model: 'synthetic-test-model',
    configuration: 'fixed-offline-fixture',
    repetitions,
    trials,
  };
}

test('versioned suite is catalog-bound and contains positive and refusal oracles', () => {
  const suite = loadSuite();
  assert.equal(suite.catalogToolCount, 43);
  assert.equal(suite.cases.length, 22);
  assert.equal(suite.annotationCounts.readOnly + suite.annotationCounts.mutating, 43);
  assert.ok(suite.annotationCounts.destructive > 0);
  assert.match(suite.catalogSHA256, /^[a-f0-9]{64}$/);
  assert.match(suite.suiteSHA256, /^[a-f0-9]{64}$/);
  assert.ok(suite.cases.some(item => item.expectedRoutes.some(route => route.length === 0)));
  assert.ok(suite.cases.some(item => item.expectedRoutes.some(route => route.length > 1)));
  assert.ok(suite.cases.every(item => item.expectedObservation && item.forbiddenTools.length));
});

test('complete repeated synthetic traces score routes only, never execution results', () => {
  const suite = loadSuite();
  const result = scoreTrace(suite, traceFor(suite));
  assert.equal(result.attempts, 44);
  assert.equal(result.exactRouteMatches, 44);
  assert.equal(result.exactRouteRate, 1);
  assert.equal(result.forbiddenToolSelections, 0);
  assert.equal(result.unknownToolSelections, 0);
  assert.equal(result.excessCallCount, 0);
  assert.equal(result.executionVerified, false);
  assert.equal(result.catalogSHA256, suite.catalogSHA256);
  assert.deepEqual(result.annotationCounts, suite.annotationCounts);
  assert.equal(result.suiteSHA256, suite.suiteSHA256);
});

test('incorrect selection and prohibited apply are reported without asserting mutation', () => {
  const suite = loadSuite();
  const trace = traceFor(suite, 1);
  const trial = trace.trials.find(item => item.caseId === 'markdown-preview');
  trial.tools = ['markdown_apply'];
  const result = scoreTrace(suite, trace);
  assert.equal(result.exactRouteMatches, suite.cases.length - 1);
  assert.equal(result.forbiddenToolSelections, 1);
  assert.equal(result.executionVerified, false);
});

test('unregistered tool proposals count as routing failures, not tool execution', () => {
  const suite = loadSuite();
  const trace = traceFor(suite, 1);
  trace.trials[0].tools = ['imaginary_tool'];
  const result = scoreTrace(suite, trace);
  assert.equal(result.unknownToolSelections, 1);
  assert.equal(result.exactRouteMatches, suite.cases.length - 1);
  assert.equal(result.executionVerified, false);
});

test('malformed, incomplete, ambiguous or oversized trials fail closed', () => {
  const suite = loadSuite();
  const valid = traceFor(suite, 1);
  for (const mutate of [
    trace => { trace.trials.pop(); },
    trace => { trace.trials[1].caseId = trace.trials[0].caseId; },
    trace => { trace.trials[0].tools = new Array(33).fill('read_text_file'); },
    trace => { trace.trials[0].caseId = 'unknown-case'; },
    trace => { trace.trials[0].repetition = 9; },
    trace => { trace.repetitions = 0; },
    trace => { trace.extra = true; },
    trace => { trace.trials[0].tools = [5]; },
  ]) {
    const sample = structuredClone(valid);
    mutate(sample);
    assert.throws(() => scoreTrace(suite, sample));
  }
});

test('CLI scores stdin without filesystem output and rejects bad input', () => {
  const suite = loadSuite();
  const binary = path.join(__dirname, 'agent-evaluation.js');
  const good = spawnSync(process.execPath, [binary], {
    encoding: 'utf8',
    input: JSON.stringify(traceFor(suite, 1)),
    timeout: 10000,
  });
  assert.equal(good.status, 0, good.stderr);
  assert.equal(JSON.parse(good.stdout).exactRouteMatches, suite.cases.length);
  const cases = spawnSync(process.execPath, [binary, '--cases'], {
    encoding: 'utf8',
    timeout: 10000,
  });
  assert.equal(cases.status, 0, cases.stderr);
  assert.equal(JSON.parse(cases.stdout).cases.length, 22);
  const bad = spawnSync(process.execPath, [binary], {
    encoding: 'utf8',
    input: '{"schemaVersion":1,"trials":[]}',
    timeout: 10000,
  });
  assert.notEqual(bad.status, 0);
  assert.match(bad.stderr, /agent-evaluation:/);
  const huge = spawnSync(process.execPath, [binary], {
    encoding: 'utf8',
    input: 'x'.repeat(1048577),
    timeout: 10000,
  });
  assert.notEqual(huge.status, 0);
  assert.match(huge.stderr, /input exceeds/);
});
