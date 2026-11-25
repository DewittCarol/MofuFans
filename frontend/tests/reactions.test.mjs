import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';

const source = readFileSync(new URL('../src/lib/reactions.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2017 } }).outputText;
const { mergeReactions } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);

test('HTTP and WebSocket delivery in either order emits only once', () => {
  const first = mergeReactions([], [{ value: '💖', count: 1 }]);
  assert.deepEqual(first.increased, ['💖']);
  assert.deepEqual(mergeReactions(first.emojis, [{ value: '💖', count: 1 }]).increased, []);
});
test('out-of-order snapshots do not roll counts back or replay effects', () => {
  const first = mergeReactions([{ value: '💖', count: 1 }], [{ value: '💖', count: 3 }, { value: '👍', count: 1 }]);
  const late = mergeReactions(first.emojis, [{ value: '💖', count: 2 }]);
  assert.deepEqual(late.emojis, first.emojis);
  assert.deepEqual(late.increased, []);
  assert.deepEqual(mergeReactions(late.emojis, [{ value: '💖', count: 4 }]).increased, ['💖']);
});
test('invalid counts are ignored and mixed batches retain existing reactions', () => {
  const result = mergeReactions([{ value: '👍', count: 2 }], [{ value: '💖', count: NaN }, { value: '👍', count: -1 }, { value: '🥳', count: 1 }]);
  assert.deepEqual(result.emojis, [{ value: '👍', count: 2 }, { value: '🥳', count: 1 }]);
  assert.deepEqual(result.increased, ['🥳']);
});
