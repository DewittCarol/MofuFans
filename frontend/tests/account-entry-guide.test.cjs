const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

function load(relative, resolve, globals = {}) {
  const filename = path.join(__dirname, relative), module = { exports: {} };
  const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  vm.runInNewContext(source, { module, exports: module.exports, require: resolve, Error, Date, TextEncoder, ...globals }, { filename });
  return module.exports;
}
const element = (type, props) => ({ type, props: props || {} });
const nodes = node => Array.isArray(node) ? node.flatMap(nodes) : node && typeof node === 'object' ? [node, ...nodes(node.props.children)] : [];
const text = node => Array.isArray(node) ? node.map(text).join('') : node && typeof node === 'object' ? text(node.props.children) : typeof node === 'string' || typeof node === 'number' ? String(node) : '';
const find = (tree, predicate) => { const node = nodes(tree).find(predicate); assert.ok(node, 'expected control exists'); return node; };

function renderNavbar(overrides = {}) {
  const calls = [], account = { user: { bilibili_uid: '10001', bilibili_name: '用于检查较长昵称的测试账号', bilibili_avatar: '/avatars/offline.png' }, loading: false, accountsEnabled: true, openAccountDialog: mode => calls.push(['open', mode]), logout: () => calls.push(['logout']), ...overrides };
  const { Navbar } = load('../src/components/navbar.tsx', name => {
    if (name === 'react/jsx-runtime') return { jsx: element, jsxs: element, Fragment: 'fragment' };
    if (name === 'next/link') return { default: 'Link' };
    if (name === 'next/navigation') return { usePathname: () => '/account/' };
    if (name === '@/lib/admin-session') return { useAdminSession: () => false, useVoyageAccess: () => true };
    if (name === '@/lib/account-auth') return { useAccountAuth: () => account };
    if (name === '@/lib/site-theme') return { useSiteTheme: () => ({ theme: {
      brand: { logo: '/offline-brand.png', streamer_name: '离线测试主播', site_name: '离线测试站点' },
      modules: { ask: { label: '提问箱' }, voyage: { label: '大航海' }, songs: { label: '歌单' }, gallery: { label: '画廊' } },
    } }) };
    if (name === './account-entry.module.css') return { default: { entry: 'entry', identity: 'identity', name: 'name', hint: 'hint' } };
    if (name === 'lucide-react') return new Proxy({}, { get: (_, key) => String(key) });
    return require(name);
  });
  return { tree: Navbar(), calls, account };
}

test('the account entry keeps a native profile link, the full accessible nickname and a separate logout action', () => {
  const { tree, calls, account } = renderNavbar();
  const link = find(tree, node => node.props.href === '/account/');
  assert.equal(link.type, 'Link');
  assert.equal(link.props.onClick, undefined, 'touch and modified clicks use ordinary link navigation');
  assert.equal(link.props['aria-current'], 'page');
  assert.equal(link.props['aria-label'], `${account.user.bilibili_name}，进入主页`);
  assert.equal(find(link, node => node.props.className === 'name').props.children, account.user.bilibili_name);
  assert.equal(find(link, node => node.props.className === 'hint').props['aria-hidden'], 'true');
  assert.doesNotMatch(text(link), /我的账户|·/);
  assert.match(find(link, node => node.props.style?.backgroundImage).props.style.backgroundImage, /offline\.png/);
  const logout = find(tree, node => node.props['aria-label'] === '退出登录');
  assert.equal(logout.props.type, 'button'); logout.props.onClick(); assert.deepEqual(calls, [['logout']]);
});

test('the account entry retains login, disabled-account and missing-nickname states', () => {
  const anonymous = renderNavbar({ user: null });
  find(anonymous.tree, node => node.props.title === '登录 / 注册').props.onClick();
  assert.deepEqual(anonymous.calls, [['open', 'login']]);
  assert.equal(nodes(renderNavbar({ accountsEnabled: false }).tree).some(node => node.props.href === '/account/'), false);
  const fallback = renderNavbar({ user: { bilibili_uid: '10001', bilibili_name: '', bilibili_avatar: '' } });
  assert.equal(find(fallback.tree, node => node.props.className === 'name').props.children, 'UID 10001');
});

function mountSignature(purpose, extra = {}) {
  let cursor = 0, effectCursor = 0, changed = false, tree;
  const slots = [], effects = [], copied = [], calls = [], storage = new Map();
  const document = { activeElement: null };
  const challenge = { challenge_id: 'offline-guide-challenge', purpose, bilibili_uid: '10001', status: 'pending', verification_text: '本站验证abcd2345', expires_at: new Date(Date.now() + 600000).toISOString(), ...extra };
  const memberAPI = load('../src/lib/member-api.ts', require);
  const progress = load('../src/lib/signature-progress.ts', name => name === './member-api' ? memberAPI : require(name), { localStorage: { getItem: key => storage.get(key) || null, setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key) } });
  const hooks = {
    useState(initial) { const index = cursor++; if (!(index in slots)) slots[index] = initial; return [slots[index], value => { const next = typeof value === 'function' ? value(slots[index]) : value; if (!Object.is(next, slots[index])) { slots[index] = next; changed = true; } }]; },
    useRef(initial) { const index = cursor++; if (!(index in slots)) slots[index] = { current: initial }; return slots[index]; },
    useEffect(work, deps) { const index = effectCursor++, old = effects[index]; if (!old || deps.some((value, i) => !Object.is(value, old.deps[i]))) effects[index] = { deps, work, cleanup: old?.cleanup, pending: true }; },
  };
  const { AccountDialog } = load('../src/components/account-dialog.tsx', name => {
    if (name === 'react') return hooks;
    if (name === 'react/jsx-runtime') return { jsx: element, jsxs: element, Fragment: 'fragment' };
    if (name === '@/lib/member-api') return { ...memberAPI, getSignatureStatus: async () => { calls.push('status'); return { code: 200, data: challenge }; } };
    if (name === '@/lib/signature-progress') return progress;
    if (name === '@/lib/api') return {};
    if (name === 'lucide-react' || name.startsWith('@/components/ui/')) return new Proxy({}, { get: (_, key) => String(key) });
    return require(name);
  }, { document, navigator: { clipboard: { writeText: async value => copied.push(value) } }, window: { setTimeout: () => 0, clearTimeout() {}, setInterval: () => 0, clearInterval() {} } });
  const render = () => {
    for (let pass = 0; pass < 10; pass++) {
      cursor = 0; effectCursor = 0; changed = false;
      tree = AccountDialog({ open: true, initialMode: purpose, currentUser: { bilibili_uid: '10001' }, onClose() {}, onAuthenticated() {} });
      for (const effect of effects) if (effect.pending) { effect.pending = false; effect.cleanup?.(); effect.cleanup = effect.work(); }
      if (!changed) return;
    }
    assert.fail('dialog did not settle');
  };
  render();
  return { get tree() { return tree; }, challenge, calls, copied, document, async flush() { for (let pass = 0; pass < 3; pass++) { await new Promise(resolve => setImmediate(resolve)); render(); } } };
}

for (const purpose of ['register', 'recover', 'upgrade']) {
  test(`${purpose} shows a collapsible location example and copies only the current generated text`, async () => {
    const app = mountSignature(purpose); await app.flush();
    const field = find(app.tree, node => node.props.id === 'member-verification-text');
    assert.equal(field.type, 'Input', 'verification text remains single-line');
    assert.equal(field.props.value, app.challenge.verification_text);
    const details = find(app.tree, node => node.type === 'details');
    assert.equal(details.props.open, undefined, 'the example does not initially expand the mobile dialog');
    assert.match(text(find(details, node => node.type === 'summary')), /填写位置示例/);
    assert.match(text(details), /图片仅为填写位置示例，请使用上方本次生成的验证文字/);
    const picture = find(details, node => node.type === 'img');
    assert.equal(picture.props.src, '/images/signature-verification-example.svg');
    const image = fs.readFileSync(path.join(__dirname, '../public', picture.props.src), 'utf8');
    assert.match(image, new RegExp(`width="${picture.props.width}" height="${picture.props.height}"`));
    assert.match(picture.props.className, /max-w-full/);
    assert.match(picture.props.alt, /个人签名填写位置/);
    await find(app.tree, node => node.type === 'Button' && text(node) === '复制验证文字').props.onClick();
    assert.deepEqual(app.copied, [app.challenge.verification_text]);
    assert.deepEqual(app.calls, ['status'], 'reading instructions never calls Bilibili');
    assert.equal(app.tree.props.onMouseDown, undefined, 'backdrop clicks still do not dismiss progress');
  });
}

test('legacy pending challenges without restored text do not misleadingly point to a missing code above', async () => {
  const app = mountSignature('register', { verification_text: undefined }); await app.flush();
  assert.equal(nodes(app.tree).some(node => node.type === 'details' || node.type === 'img'), false);
  assert.match(text(app.tree), /原验证文字仍然有效/);
  assert.ok(find(app.tree, node => node.type === 'Button' && text(node) === '已修改签名，检查验证'));
});

test('the example disclosure participates in the dialog keyboard focus boundary', async () => {
  const app = mountSignature('register'); await app.flush();
  const dialog = find(app.tree, node => node.props.role === 'dialog');
  let selector, prevented = 0, focused = '';
  const first = { focus: () => { focused = 'first'; } }, summary = {}, last = { focus: () => { focused = 'last'; } };
  dialog.props.ref.current = { querySelectorAll: value => { selector = value; return [first, summary, last]; } };
  app.document.activeElement = last;
  dialog.props.onKeyDown({ key: 'Tab', preventDefault: () => prevented++ });
  assert.ok(selector.split(',').includes('summary'));
  assert.equal(focused, 'first'); assert.equal(prevented, 1);
  app.document.activeElement = first;
  dialog.props.onKeyDown({ key: 'Tab', shiftKey: true, preventDefault: () => prevented++ });
  assert.equal(focused, 'last'); assert.equal(prevented, 2);
});
