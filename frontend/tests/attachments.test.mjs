import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';

const source = readFileSync(new URL('../src/lib/attachments.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2017 } }).outputText;
const { attachmentUrls } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);

test('legacy uploads resolve at the site root, including under admin and topic routes', () => {
  const paths = attachmentUrls('upload-img/a.png;./upload-img/b.webp;/upload-img/c.jpg');
  assert.deepEqual(paths, ['/upload-img/a.png', '/upload-img/b.webp', '/upload-img/c.jpg']);
  for (const page of ['http://test.local/', 'http://test.local/admin/dashboard/questions/', 'http://test.local/tags/detail/']) {
    assert.equal(new URL(paths[0], page).pathname, '/upload-img/a.png');
  }
});
test('external HTTP images and order are preserved; blank and unsupported entries are ignored', () => {
  assert.deepEqual(attachmentUrls(';https://cdn.example/a.png; ;http://cdn.example/b.jpg;'), ['https://cdn.example/a.png', 'http://cdn.example/b.jpg']);
  assert.deepEqual(attachmentUrls('javascript:alert(1);file:///tmp/a;ftp://example/a'), []);
  assert.deepEqual(attachmentUrls(''), []);
});
