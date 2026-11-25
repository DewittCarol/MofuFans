import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';

const source = readFileSync(new URL('../src/lib/moderation-preferences.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2017 } }).outputText;
const { imageIdentity, imageReviewKey, moderationStoragePrefix, readPreferences, readReviewedImages } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);
const origin = 'http://localhost:18081';
const prefix = moderationStoragePrefix({ id: 1, username: 'admin' });
const fakeStorage = () => {
  const data = new Map();
  return { get length() { return data.size; }, key: i => [...data.keys()][i] ?? null,
    getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) };
};

test('missing or corrupt preferences default to protected list view', () => {
  for (const value of [null, '{', 'null', '[]', '{"blurEnabled":"false","view":"unknown"}']) {
    assert.deepEqual(readPreferences(value), { blurEnabled: true, view: 'list' });
  }
  assert.deepEqual(readPreferences('{"blurEnabled":false,"view":"cards"}'), { blurEnabled: false, view: 'cards' });
});

test('same image has the same identity across relative, absolute, reordered and duplicate attachments', () => {
  const identity = imageIdentity('/upload-img/hash.webp', origin);
  assert.equal(imageIdentity('upload-img/hash.webp', origin), identity);
  assert.equal(imageIdentity(`${origin}/upload-img/hash.webp#detail`, origin), identity);
  assert.notEqual(imageIdentity('https://other.example/upload-img/hash.webp', origin), identity);
  assert.notEqual(imageIdentity('/upload-img/hash.webp?image=2', origin), identity);
  assert.doesNotThrow(() => imageIdentity('https://', origin));
});

test('revealing one image persists independently and does not reveal its neighbor or another moderator', () => {
  const storage = fakeStorage();
  const first = imageIdentity('/upload-img/first.webp', origin);
  const second = imageIdentity('/upload-img/second.webp', origin);
  storage.setItem(imageReviewKey(prefix, first), '1');
  assert.equal(readReviewedImages(storage, prefix).has(first), true);
  assert.equal(readReviewedImages(storage, prefix).has(second), false);
  assert.equal(readReviewedImages(storage, moderationStoragePrefix({ id: 2, username: 'reviewer' })).size, 0);
  storage.setItem(imageReviewKey(prefix, second), '1');
  assert.equal(readReviewedImages(storage, prefix).size, 2);
});

test('turning protection off and back on does not mark unseen images as reviewed', () => {
  const storage = fakeStorage();
  const first = imageIdentity('/first.webp', origin);
  storage.setItem(imageReviewKey(prefix, first), '1');
  storage.setItem(`${prefix}preferences`, '{"blurEnabled":false,"view":"cards"}');
  storage.setItem(`${prefix}preferences`, '{"blurEnabled":true,"view":"cards"}');
  assert.deepEqual([...readReviewedImages(storage, prefix)], [first]);
});

test('malformed review keys and invalid review values do not uncover images', () => {
  const storage = fakeStorage();
  storage.setItem(`${prefix}seen:%ZZ`, '1');
  storage.setItem(imageReviewKey(prefix, '/one.webp'), 'true');
  assert.equal(readReviewedImages(storage, prefix).size, 0);
});
