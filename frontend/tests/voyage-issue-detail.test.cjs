const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');

// Render the production component. Only interactive image overlays and the
// decorative tier badge are replaced; the issue and gift layout stay real.
function loadTS(relativePath) {
  const filename = path.join(__dirname, relativePath);
  const source = fs.readFileSync(filename, 'utf8');
  const moduleObject = { exports: {} };
  const requireForComponent = name => {
    if (name === '@/lib/voyage') return loadTS('../src/lib/voyage.ts');
    if (name === '@/components/image-preview') return { ImagePreview: () => null };
    if (name === '@/components/voyage-tier-badge') return {
      VoyageTierBadge: ({ tier }) => React.createElement('span', { 'data-tier': tier }, tier),
    };
    return require(name);
  };
  const compiled = ts.transpileModule(source, { compilerOptions: {
    module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX,
  } }).outputText;
  vm.runInNewContext(compiled, {
    exports: moduleObject.exports, module: moduleObject, require: requireForComponent,
  }, { filename });
  return moduleObject.exports;
}
const { VoyageIssueContents } = loadTS('../src/components/voyage-issue-detail.tsx');
const gift = (values = {}) => ({
  name: '礼物名称标记', section: 'guard', tiers: ['captain'], description: '', images: [],
  event: '活动名称标记', audience: '参与条件标记', ...values,
});
const issue = (values = {}) => ({
  id: 1, kind: 'guard', title: '九月回馈', month: '2026-09', note: '', shipping_note: '',
  published: true, version: 1, images: [], gifts: [], ...values,
});
const render = value => renderToStaticMarkup(React.createElement(VoyageIssueContents, { issue: value }));

for (const kind of ['guard', 'lottery']) {
  test(`${kind} uses the card category and never mixes audience or tier fields`, () => {
    const record = issue({ kind, gifts: [gift({section: 'other', tiers: ['governor']})] });
    const before = structuredClone(record);
    const html = render(record);
    assert.ok(html.includes('礼物名称标记'));
    assert.equal(html.includes('活动名称标记'), false, 'activity name belongs to the card title');
    assert.equal(html.includes('参与条件标记'), kind === 'lottery');
    assert.equal(html.includes('data-tier'), kind === 'guard');
    assert.ok(html.includes(kind === 'lottery' ? '活动奖品' : '大航海回馈'));
    assert.deepEqual(record, before);
  });
}

test('period introduction precedes gifts and overall shipping follows them', () => {
  const html = render(issue({
    note: '本期介绍标记', shipping_note: '整体发货标记', gifts: [gift({
      description: '单件介绍标记', shipping_note: '单件发货标记',
    })],
  }));
  const ordered = ['本期介绍标记', '航海回馈', '礼物名称标记', '单件介绍标记', '单件发货标记', '发货说明', '整体发货标记'];
  const positions = ordered.map(value => html.indexOf(value));
  assert.ok(positions.every(position => position >= 0), 'all configured public descriptions must be visible');
  assert.deepEqual(positions, [...positions].sort((a, b) => a - b));
});

test('a reused image remains visible at each explicitly selected poster and gift placement', () => {
  const html = render(issue({
    images: ['/api/voyage-assets/period.png', '/api/voyage-assets/gift.png'],
    gifts: [gift({ images: ['/api/voyage-assets/gift.png'] })],
  }));
  const periodImage = 'src="/api/voyage-assets/period.png"';
  const giftImage = 'src="/api/voyage-assets/gift.png"';
  assert.equal(html.split(periodImage).length - 1, 1);
  assert.equal(html.split(giftImage).length - 1, 2);
  assert.ok(html.indexOf(periodImage) < html.indexOf('航海回馈'));
  assert.ok(html.indexOf(giftImage) < html.indexOf('航海回馈'));
  assert.ok(html.lastIndexOf(giftImage) > html.indexOf('礼物名称标记'));
});

test('old electronic gift fields do not add a type label or an experience entry', () => {
  const html = render(issue({ gifts: [
    gift({ name: '旧指针礼物', kind: 'digital', preview: 'cursor:mofu' }),
    gift({ name: '旧桌宠礼物', kind: 'mixed', preview: 'pet:3d' }),
  ] }));
  assert.ok(html.includes('旧指针礼物'));
  assert.ok(html.includes('旧桌宠礼物'));
  assert.doesNotMatch(html, /电子礼物|试用这套指针|展示这个桌宠|用电脑打开|<button/);
});