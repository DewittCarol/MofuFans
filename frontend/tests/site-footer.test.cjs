const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
let footer = {};
function load(file) {
  const exports = {};
  const code = ts.transpileModule(fs.readFileSync(file, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX, esModuleInterop: true } }).outputText;
  vm.runInNewContext(code, { exports, URL, require: name => {
    if (name === '@/lib/emoji-catalog') return { useEmojiCatalog: () => ({ footer }) };
    if (name.startsWith('.')) { const target = path.resolve(path.dirname(file), name); return target.endsWith('.json') ? require(target) : load(target + '.ts'); }
    return require(name);
  } }, { filename: file });
  return exports;
}
const shared = load(path.resolve(__dirname, '../../shared/footer.ts'));
const { SiteFooter } = load(path.resolve(__dirname, '../src/components/site-footer.tsx'));
const render = value => { footer = { ...shared.emptyFooter, ...value }; return renderToStaticMarkup(React.createElement(SiteFooter)); };
test('footer is absent until filing information is configured', () => {
  assert.equal(render({}), '');
  assert.equal(render({ footer_police_icon: '/icon.png', footer_police_url: 'https://beian.mps.gov.cn/' }), '');
});
test('filing text is escaped and official links and optional icon render', () => {
  const html = render({ footer_icp_number: '<script>bad</script>', footer_police_number: '测试公安备案号', footer_police_url: 'https://beian.mps.gov.cn/#/query/webSearch?code=12345678901234', footer_police_icon: '/api/theme-assets/test.png' });
  assert.ok(html.includes('&lt;script&gt;bad&lt;/script&gt;'));
  assert.ok(html.includes('https://beian.miit.gov.cn/'));
  assert.ok(html.includes('https://beian.mps.gov.cn/'));
  assert.ok(html.includes('noopener noreferrer'));
  assert.ok(html.includes('/api/theme-assets/test.png'));
});
test('footer refuses unsafe links and image URLs', () => {
  for (const url of ['javascript:alert(1)', 'https://beian.mps.gov.cn.evil.test/', 'https://evil@beian.mps.gov.cn/', 'http://beian.mps.gov.cn/']) assert.equal(shared.policeLink(url), undefined);
  const html = render({ footer_police_number: '测试公安备案号', footer_police_url: 'javascript:alert(1)', footer_police_icon: 'javascript:alert(1)' });
  assert.ok(html.includes('测试公安备案号'));
  assert.equal(html.includes('javascript:'), false);
});
