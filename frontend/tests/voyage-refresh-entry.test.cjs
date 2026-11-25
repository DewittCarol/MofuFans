const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

function load(relative, resolve) {
  const filename = path.join(__dirname, relative), module = { exports: {} };
  const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  vm.runInNewContext(source, { module, exports: module.exports, require: resolve, Error }, { filename });
  return module.exports;
}
const claim = load('../src/lib/voyage-claim.ts', require);
const nodes = node => Array.isArray(node) ? node.flatMap(nodes) : node && typeof node === 'object' ? [node, ...nodes(node.props.children)] : [];
const text = node => Array.isArray(node) ? node.map(text).join('') : node && typeof node === 'object' ? text(node.props.children) : typeof node === 'string' ? node : '';

function mount(memberUID) {
  let cursor = 0, tree, mode = 'shipping';
  const slots = [], calls = [];
  const record = { address: { name: '离线测试', phone: '13800000000', address: '隔离测试原地址', note: '' }, version: 1, collecting: true, can_edit_address: true, history: [], items: [], parcels: [] };
  const hooks = {
    useState(initial) { const index = cursor++; if (!(index in slots)) slots[index] = initial; return [slots[index], value => { slots[index] = typeof value === 'function' ? value(slots[index]) : value; }]; },
    useRef(initial) { const index = cursor++; if (!(index in slots)) slots[index] = { current: initial }; return slots[index]; },
    useEffect() {},
  };
  const element = (type, props) => ({ type, props: props || {} });
  const { VoyageClaim } = load('../src/components/voyage-claim.tsx', name => {
    if (name === 'react') return hooks;
    if (name === 'react/jsx-runtime') return { jsx: element, jsxs: element, Fragment: 'fragment' };
    if (name === '@/lib/voyage-claim') return claim;
    if (name === '@/lib/voyage') return { voyageRequest: async (url, method, body) => { calls.push({ url, method, body }); return JSON.parse(JSON.stringify(record)); } };
    if (name === '@/lib/site-dialog') return { confirmDiscard: async () => true };
    if (name === 'lucide-react' || name.startsWith('@/components/ui/')) return new Proxy({}, { get: (_, key) => String(key) });
    return require(name);
  });
  const render = () => { cursor = 0; tree = VoyageClaim({ issueID: 17, memberUID, mode, onModeChange: value => { mode = value; }, onClose() {} }); };
  const find = predicate => { const node = nodes(tree).find(predicate); assert.ok(node, 'expected control exists'); return node; };
  const act = async work => { await work(); await new Promise(resolve => setImmediate(resolve)); render(); };
  render();
  return { calls, record, labels: () => nodes(tree).filter(node => node.type === 'Button').map(text),
    value: id => find(node => node.props.id === id).props.value,
    change: (id, value) => act(() => find(node => node.props.id === id).props.onChange({ target: { value } })),
    submit: () => act(() => find(node => node.type === 'form').props.onSubmit({ preventDefault() {} })),
    click: label => act(() => find(node => node.type === 'Button' && text(node) === label).props.onClick()),
  };
}

test('signed-in users have one address-and-logistics refresh that preserves an unsaved address', async () => {
  const app = mount('10001'); await app.submit();
  assert.deepEqual(app.labels().filter(label => label.startsWith('刷新')), ['刷新地址与物流']);
  assert.equal(app.labels().includes('重新验证'), false);
  await app.click('收货地址'); await app.change('address-17', '本页尚未保存的地址'); await app.click('物流信息');
  await app.click('刷新地址与物流');
  assert.equal(app.calls.length, 2); assert.equal(app.calls[1].url, '/api/account/collections/17/claim');
  assert.equal(app.calls[1].body.action, 'view'); assert.equal(app.calls[1].body.uid, '10001');
  await app.click('收货地址'); assert.equal(app.value('address-17'), '本页尚未保存的地址');
});

test('private-message users keep distinct refresh and revalidation actions', async () => {
  const app = mount(); await app.change('uid-17', '10001'); await app.change('code-17', 'TEST01'); await app.submit();
  assert.deepEqual(app.labels().filter(label => label.startsWith('刷新')), ['刷新寄送进度']);
  assert.ok(app.labels().includes('重新验证'));
  await app.click('刷新寄送进度');
  assert.equal(app.calls.length, 2); assert.equal(app.calls[1].url, '/api/voyage/issues/17/claim');
  assert.equal(app.calls[1].body.action, 'view'); assert.equal(app.calls[1].body.code, 'TEST01');
  await app.click('重新验证');
  assert.equal(app.calls.length, 2, 'revalidation returns to credentials without sending another refresh');
  assert.equal(app.value('code-17'), ''); assert.ok(app.labels().includes('验证并查看'));
});
