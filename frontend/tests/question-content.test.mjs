import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import ts from 'typescript';
const source = readFileSync(new URL('../src/lib/question-content.ts', import.meta.url), 'utf8');
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2017 } }).outputText;
const { renderQuestionContent: render } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`);
const emoji = { kind: 'input', value: '[示例+开心]', name: '示例开心', url: '/api/emoji-assets/test.png', is_deleted: true };

test('uploaded and deleted emoji tokens remain renderable without treating names as regexes', () => {
  const html = render('你好[示例+开心]！[未添加]', [emoji]);
  assert.ok(html.includes('src="/api/emoji-assets/test.png"'));
  assert.ok(html.includes('[未添加]'));
  assert.equal((html.match(/<img /g) || []).length, 1);
});
test('question HTML and quotes cannot become executable attributes', () => {
  const html = render('<img src=x onerror="alert(1)"> https://x.test/"onclick="x [示例+开心]', [emoji]);
  assert.ok(html.includes('&lt;img'));
  assert.ok(!html.includes(' onclick="'));
  assert.equal((html.match(/<img /g) || []).length, 1);
  assert.ok(!render('[bad]', [{ ...emoji, value: '[bad]', url: 'javascript:alert(1)' }]).includes('<img'));
});
test('links, BV links, newlines and hidden text are preserved', () => {
  const html = render('第一行\nhttps://x.test/?a=1&b=2 BV1234567890 {{[示例+开心]秘密}}', [emoji]);
  assert.ok(html.includes('\n'));
  assert.ok(html.includes('href="https://x.test/?a=1&amp;b=2"'));
  assert.ok(html.includes('href="https://www.bilibili.com/video/BV1234567890"'));
  assert.ok(html.includes('class="question-spoiler" tabindex="0"'));
});
test('reaction emoji names do not change question text', () => {
  assert.equal(render('[示例+开心]', [{ ...emoji, kind: 'reaction' }]), '[示例+开心]');
});
