const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

// Run the production component's handlers with a deterministic hook host. UI
// primitives are inert nodes; the claim library and request-building code stay real.
function loadTS(relativePath, resolve) {
  const filename = path.join(__dirname, relativePath);
  const module = { exports: {} };
  const compiled = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
  }).outputText;
  vm.runInNewContext(compiled, { module, exports: module.exports, require: resolve, Error }, { filename });
  return module.exports;
}
const claim = loadTS('../src/lib/voyage-claim.ts', require);
const copy = value => JSON.parse(JSON.stringify(value));
const address = value => ({ name: '离线测试', phone: '13800000000', address: value, note: '' });
const initialRecord = () => ({
  address: address('原地址 A'), version: 1, submitted_at: '2026-10-25T01:00:00Z',
  address_source: 'long_term', long_term_saved_at: '2026-10-25T01:00:00Z', long_term_changed: false,
  long_term: { enabled: true, version: 1, matches_address: true, updated_at: '2026-10-25T01:00:00Z' },
  collecting: true, can_edit_address: true, history: [], items: [], parcels: [],
  courier: '', tracking: '', shipping_note: '', shipped_at: null,
});

function server(record = initialRecord()) {
  const service = { current: copy(record), requests: [], urls: [], writes: 0 };
  service.request = async (_url, _method, body) => {
    service.requests.push(copy(body));
    service.urls.push(_url);
    if(service.failure)throw service.failure;
    const current = service.current;
    if (body.action === 'view') return copy(current);
    // This is the controller's optimistic-write contract, not a replacement
    // implementation of address encryption, eligibility or fulfillment.
    if ((['save','set_long_term'].includes(body.action) && body.version !== current.version)
      || (['cancel_long_term','set_long_term','set_book_default'].includes(body.action)
        && body.long_term_version !== current.long_term.version)) {
      throw new Error('记录已更新，请重新验证后再操作');
    }
    if (body.action === 'save') {
      current.address = copy(body.address);
      current.version++;
      current.address_source = '';
      current.long_term = {...current.long_term,matches_address:false};
    }
    if(body.action==='set_long_term'){
      assert.equal('address' in body,false,'setting must use the server-saved address');
      current.long_term = {...current.long_term,enabled:true,version:current.long_term.version+1,matches_address:true};
    }
    if(body.action==='set_book_default'){
      assert.equal('address' in body,false);
      const entry=current.address_book.find(e=>e.id===body.book_entry_id);
      assert.ok(entry);
      current.address_book=current.address_book.map(e=>({...e,is_default:e.id===entry.id}));
      current.long_term={...current.long_term,enabled:true,version:current.long_term.version+1,entry_id:entry.id,matches_address:entry.address.address===current.address?.address};
    }
    if (body.action === 'cancel_long_term') {
      current.long_term = { ...current.long_term, enabled: false, version: current.long_term.version + 1, matches_address: false };
    }
    service.writes++;
    return copy(current);
  };
  return service;
}

function mount(service,options={}) {
  const slots = [];
  let cursor = 0, mode = 'address', tree;
  const hooks = {
    useState(initial) {
      const index = cursor++;
      if (!(index in slots)) slots[index] = typeof initial === 'function' ? initial() : initial;
      return [slots[index], value => { slots[index] = typeof value === 'function' ? value(slots[index]) : value; }];
    },
    useRef(initial) {
      const index = cursor++;
      if (!(index in slots)) slots[index] = { current: initial };
      return slots[index];
    },
    useEffect() {},
  };
  const element = (type, props) => ({ type, props: props || {} });
  const { VoyageClaim } = loadTS('../src/components/voyage-claim.tsx', name => {
    if (name === 'react') return hooks;
    if (name === 'react/jsx-runtime') return { jsx: element, jsxs: element, Fragment: 'fragment' };
    if (name === '@/lib/voyage-claim') return claim;
    if (name === '@/lib/voyage') return { voyageRequest: service.request };
    if (name === '@/lib/site-dialog') return { confirmDiscard: options.confirmDiscard|| (async () => true) };
    if (name === 'lucide-react') return { X: 'icon-x', Check: 'icon-check', Truck: 'icon-truck' };
    if (name.startsWith('@/components/ui/')) return new Proxy({}, { get: (_target, key) => String(key) });
    return require(name);
  });
  const render = () => {
    cursor = 0;
    tree = VoyageClaim({ issueID: 17, memberUID: options.memberUID, mode, onModeChange: value => { mode = value; }, onClose() {} });
  };
  const nodes = node => {
    if (Array.isArray(node)) return node.flatMap(nodes);
    if (!node || typeof node !== 'object') return [];
    return [node, ...nodes(node.props.children)];
  };
  const text = node => {
    if (Array.isArray(node)) return node.map(text).join('');
    if (node && typeof node === 'object') return text(node.props.children);
    return typeof node === 'string' || typeof node === 'number' ? String(node) : '';
  };
  const find = predicate => {
    const node = nodes(tree).find(predicate);
    assert.ok(node, 'expected control was rendered');
    return node;
  };
  const act = async callback => {
    await callback();
    await new Promise(resolve => setImmediate(resolve));
    render();
  };
  const app = {
    field: id => find(node => node.props.id === id),
    checkbox: () => find(node => node.props.type === 'checkbox'),
    text: () => text(tree),
    preferenceText: () => text(find(node=>node.type==='section'&&node.props['aria-labelledby']==='long-term-title-17')),
    button: label => find(node => node.type === 'Button' && text(node) === label),
    flush: () => act(()=>{}),
    change: async (id, value) => act(() => app.field(id).props.onChange({ target: { value } })),
    remember: async checked => act(() => app.checkbox().props.onChange({ target: { checked } })),
    click: async label => act(() => find(node => node.type === 'Button' && text(node) === label).props.onClick()),
    selectHistory: async id => act(()=>find(node=>node.type==='Select').props.onValueChange(String(id))),
    submit: async () => act(() => find(node => node.type === 'form').props.onSubmit({ preventDefault() {} })),
    login: async () => {
      await app.change('uid-17', '12345');
      await app.change('code-17', 'TEST01');
      await app.submit();
    },
    refresh: async () => {
      await app.click('物流信息');
      await app.click(options.memberUID ? '刷新地址与物流' : '刷新寄送进度');
    },
  };
  render();
  return app;
}

test('signed-in claim uses the member route and binds the displayed UID without a private-message code',async()=>{
  const service=server(),app=mount(service,{memberUID:'12345'});
  await app.submit();
  assert.equal(service.urls[0],'/api/account/collections/17/claim');
  assert.equal(service.requests[0].uid,'12345');assert.equal('code' in service.requests[0],false);
  await app.change('address-17','登录后的新地址');await app.submit();
  assert.equal(service.current.address.address,'登录后的新地址');
  assert.equal(service.urls.at(-1),'/api/account/collections/17/claim');
  assert.equal(service.requests.at(-1).uid,'12345');assert.equal('code' in service.requests.at(-1),false);
});

test('expired member access clears the address view instead of falling back to private-message credentials',async()=>{
  const service=server(),app=mount(service,{memberUID:'12345'});await app.submit();
  assert.equal(app.field('address-17').props.value,'原地址 A');
  service.failure=Object.assign(new Error('请重新登录'),{code:408});await app.refresh();
  assert.throws(()=>app.field('address-17'));assert.match(app.text(),/请重新登录/);
  assert.ok(service.urls.every(url=>url==='/api/account/collections/17/claim'));
});

test('choosing an address, filling this batch and changing the default are independent actions',async()=>{
  const record=initialRecord();record.address.note='本次备注';record.long_term.enabled=false;record.long_term.matches_address=false;
  record.address_book=[{id:9,address:address('历史地址 B'),is_default:false,uses:[]}];
  const service=server(record),app=mount(service);
  assert.doesNotMatch(app.text(),/历史地址 B/);
  await app.login();await app.selectHistory(9);
  assert.equal(app.field('address-17').props.value,'原地址 A');assert.equal(service.writes,0);
  await app.click('设为常用地址');
  assert.equal(service.requests.at(-1).action,'set_book_default');assert.equal(service.requests.at(-1).book_entry_id,9);
  assert.equal(service.current.address.address,'原地址 A');assert.equal(service.current.address_book.filter(e=>e.is_default).length,1);
  assert.match(app.text(),/当前常用地址与本次地址不同/);
  await app.click('填入本次地址');
  assert.equal(app.field('address-17').props.value,'历史地址 B');assert.equal(app.field('note-17').props.value,'本次备注');
  assert.equal(service.current.address.address,'原地址 A');
});

test('two-page edit: refreshing logistics cannot grant a stale draft the other page\'s write versions', async () => {
  const service = server();
  const app = mount(service);
  await app.login();
  await app.change('address-17', '页一未保存地址 B');
  service.current = {
    ...service.current, address: address('页二已保存地址 C'), version: 2,
    long_term: { ...service.current.long_term, version: 2 },
    parcels: [{ id: 1, labels: ['测试礼物'], status: '已寄出', courier: '测试快递', tracking: 'NEW123', note: '', shipped_at: '2026-10-25T02:00:00Z' }],
  };
  await app.refresh();
  assert.match(app.text(), /NEW123/, 'logistics must still refresh');
  await app.click('收货地址');
  assert.equal(app.field('address-17').props.value, '页一未保存地址 B');
  await app.submit();
  const attempt = service.requests.at(-1);
  assert.equal(attempt.version, 1);
  assert.equal(attempt.long_term_version, 1);
  assert.equal(service.writes, 0);
  assert.equal(service.current.address.address, '页二已保存地址 C');
  assert.match(app.text(), /记录已更新，请重新验证后再操作/);
});

test('two-page consent: an immediate toggle with a stale consent version fails inside the independent box', async () => {
  const service = server();
  const app = mount(service);
  await app.login();
  service.current.long_term = { ...service.current.long_term, version: 2, matches_address: false };
  service.current.long_term_changed = true;
  await app.remember(false);
  const attempt = service.requests.at(-1);
  assert.equal(attempt.action, 'cancel_long_term');
  assert.equal(attempt.long_term_version, 1);
  assert.equal(service.writes, 0);
  assert.equal(service.current.long_term.version, 2);
  assert.equal(service.current.long_term.enabled, true);
  assert.equal(app.checkbox().props.checked, true);
  assert.match(app.preferenceText(), /记录已更新，请重新验证后再操作/);
});

test('a clean refresh adopts the latest address and versions for the next intentional save', async () => {
  const service = server();
  const app = mount(service);
  await app.login();
  service.current = { ...service.current, address: address('另一页新地址 C'), version: 2,
    long_term: { ...service.current.long_term, version: 2 } };
  await app.refresh();
  await app.click('收货地址');
  assert.equal(app.field('address-17').props.value, '另一页新地址 C');
  await app.change('note-17', '本次备注');
  await app.submit();
  assert.equal(service.requests.at(-1).version, 2);
  assert.equal(service.requests.at(-1).long_term_version, 2);
  assert.equal(service.writes, 1);
  assert.equal(service.current.address.note, '本次备注');
});

test('confirmed explicit cancellation still discards an unsaved draft and preserves the registered address', async () => {
  const service = server();
  const app = mount(service);
  await app.login();
  await app.change('address-17', '未保存地址 B');
  await app.remember(false);
  assert.equal(service.requests.at(-1).action, 'cancel_long_term');
  assert.equal(service.writes, 1);
  assert.equal(service.current.address.address, '原地址 A');
  assert.equal(service.current.long_term.enabled, false);
  assert.equal(app.field('address-17').props.value, '原地址 A');
  assert.equal(app.checkbox().props.checked, false);
  assert.match(app.text(), /已保存的地址不变/);
});

test('refreshing a dirty draft still applies current edit permissions and history', () => {
  const current = initialRecord();
  const fresh = { ...initialRecord(), can_edit_address: false, collecting: false,
    history: [{ issue_title: '其他批次', address: address('另一个历史地址') }] };
  const result = claim.refreshClaimRecord(current, fresh, true);
  assert.equal(result.can_edit_address, false);
  assert.equal(result.collecting, false);
  assert.equal(result.history, fresh.history);
  assert.equal(result.address, current.address);
  assert.equal(result.long_term, current.long_term);
});

test('saving this address keeps the enabled preference and offers an explicit replacement afterwards', async()=>{
  const service=server(),app=mount(service);
  await app.login();
  await app.change('address-17','本批临时地址 B');
  await app.submit();
  assert.equal(service.requests.at(-1).long_term_action,'keep');
  assert.equal(service.current.long_term.version,1);
  assert.equal(service.current.long_term.enabled,true);
  assert.equal(app.checkbox().props.checked,true,'enabled and matching are separate facts');
  assert.match(app.preferenceText(),/当前常用地址与本次地址不同/);
  assert.equal(app.button('更新常用地址').props.disabled,false);
});

test('enabling immediately uses only server-saved address versions and never changes this address version',async()=>{
  const initial=initialRecord();initial.long_term={...initial.long_term,enabled:false,matches_address:false};
  const service=server(initial),app=mount(service);
  await app.login();await app.remember(true);
  const body=service.requests.at(-1);
  assert.equal(body.action,'set_long_term');assert.equal(body.version,1);assert.equal(body.long_term_version,1);
  assert.equal('address' in body,false);
  assert.equal(service.current.version,1);
  assert.equal(service.current.address.address,'原地址 A');
  assert.equal(app.checkbox().props.checked,true);
  assert.match(app.preferenceText(),/已启用常用地址/);
});

test('replacing a different enabled preference is an explicit action using the saved address',async()=>{
  const initial=initialRecord();initial.long_term.matches_address=false;
  const service=server(initial),app=mount(service);
  await app.login();assert.equal(app.checkbox().props.checked,true);
  await app.click('更新常用地址');
  assert.equal(service.requests.at(-1).action,'set_long_term');
  assert.equal('address' in service.requests.at(-1),false);
  assert.equal(service.current.version,1);
  assert.match(app.preferenceText(),/常用地址已更新/);
  assert.doesNotMatch(app.preferenceText(),/当前常用地址与本次地址不同/);
});

test('no saved address or a dirty draft disables setting and the handler also blocks accidental requests',async()=>{
  for(const scenario of ['missing','draft','replacement']){
    const initial=initialRecord();
    initial.long_term={...initial.long_term,enabled:scenario==='replacement',matches_address:false};
    if(scenario==='missing')initial.address=null;
    const service=server(initial),app=mount(service);await app.login();
    if(scenario!=='missing')await app.change('address-17','未经保存的地址 B');
    if(scenario==='replacement'){
      assert.equal(app.button('更新常用地址').props.disabled,true);
      await app.click('更新常用地址');
    }else{
      assert.equal(app.checkbox().props.disabled,true);
      await app.remember(true);
    }
    assert.equal(service.requests.length,1);
    assert.equal(service.writes,0);
    assert.match(app.preferenceText(),/先.*保存/);
  }
});

test('closed collection can still enable a preference from its already-saved address',async()=>{
  const initial=initialRecord();initial.can_edit_address=false;initial.collecting=false;initial.long_term.enabled=false;
  const service=server(initial),app=mount(service);await app.login();
  assert.equal(app.field('address-17').props.disabled,true);
  assert.equal(app.checkbox().props.disabled,false);
  await app.remember(true);
  assert.equal(service.writes,1);assert.equal(service.current.version,1);
  assert.equal(app.checkbox().props.checked,true);
});

test('stale saved-address or consent versions reject enable without adopting new versions or changing the switch',async()=>{
  for(const changed of ['address','consent']){
    const initial=initialRecord();initial.long_term.enabled=false;
    const service=server(initial),app=mount(service);await app.login();
    if(changed==='address'){service.current.version=2;service.current.address=address('另一页新地址 C')}
    else service.current.long_term.version=2;
    await app.remember(true);
    assert.equal(service.requests.at(-1).version,1);
    assert.equal(service.requests.at(-1).long_term_version,1);
    assert.equal(service.writes,0);assert.equal(app.checkbox().props.checked,false);
    assert.equal(app.field('address-17').props.value,'原地址 A');
    assert.match(app.preferenceText(),/记录已更新，请重新验证后再操作/);
  }
});

test('failed enable keeps the switch off and reports failure only inside the preference box',async()=>{
  const initial=initialRecord();initial.long_term.enabled=false;
  const service=server(initial),realRequest=service.request;
  service.request=(url,method,body)=>body.action==='set_long_term'?Promise.reject(new Error('网络暂时不可用')):realRequest(url,method,body);
  const app=mount(service);await app.login();await app.remember(true);
  assert.equal(app.checkbox().props.checked,false);
  assert.equal(app.checkbox().props.disabled,false);
  assert.match(app.preferenceText(),/网络暂时不可用/);
  assert.equal(service.writes,0);
});

test('dirty cancellation declined by the user keeps the draft and consent without a request',async()=>{
  let confirmations=0;
  const service=server(),app=mount(service,{confirmDiscard:async()=>{confirmations++;return false}});
  await app.login();await app.change('address-17','继续编辑的草稿 B');await app.remember(false);
  assert.equal(confirmations,1);assert.equal(service.requests.length,1);assert.equal(service.writes,0);
  assert.equal(app.checkbox().props.checked,true);
  assert.equal(app.field('address-17').props.value,'继续编辑的草稿 B');
});

test('dirty cancellation failure preserves the draft even after the user approved discarding it',async()=>{
  const service=server(),app=mount(service);
  await app.login();await app.change('address-17','不能丢失的草稿 B');
  service.current.long_term.version=2;
  await app.remember(false);
  assert.equal(service.requests.at(-1).action,'cancel_long_term');
  assert.equal('address' in service.requests.at(-1),false);
  assert.equal(service.writes,0);assert.equal(app.checkbox().props.checked,true);
  assert.equal(app.field('address-17').props.value,'不能丢失的草稿 B');
  assert.match(app.preferenceText(),/记录已更新/);
});

test('the switch changes only after success and a pending operation cannot be submitted twice',async()=>{
  const initial=initialRecord();initial.long_term.enabled=false;
  const service=server(initial),realRequest=service.request;
  let complete,starts=0;
  service.request=(url,method,body)=>{
    if(body.action!=='set_long_term')return realRequest(url,method,body);
    starts++;
    return new Promise(resolve=>{complete=()=>resolve(realRequest(url,method,body))});
  };
  const app=mount(service);await app.login();await app.remember(true);
  assert.equal(app.checkbox().props.checked,false);assert.equal(app.checkbox().props.disabled,true);
  assert.match(app.preferenceText(),/正在启用常用地址/);
  await app.remember(true);assert.equal(starts,1);
  complete();await app.flush();
  assert.equal(app.checkbox().props.checked,true);assert.equal(app.checkbox().props.disabled,false);
  assert.equal(service.writes,1);
});
