import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';
const source = readFileSync(new URL('../src/lib/moderation-update.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2017 } }).outputText;
const { moderationUpdate } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);
const question = { tag_id: 8, is_hide: true, is_rainbow: true, is_archive: false, is_publish: true, is_spam: false };

test('intercepting a published question also cancels publication and preserves other flags', () => {
  assert.deepEqual(moderationUpdate(question, { is_spam: true }), { ...question, is_publish: false, is_spam: true });
});
test('approving an intercepted question publishes and restores it to the inbox', () => {
  assert.deepEqual(moderationUpdate({ ...question, is_publish: false, is_spam: true }, { is_publish: true }), question);
});
test('restoring an intercepted question alone never publishes it', () => {
  assert.deepEqual(moderationUpdate({ ...question, is_publish: false, is_spam: true }, { is_spam: false }), { ...question, is_publish: false });
});
