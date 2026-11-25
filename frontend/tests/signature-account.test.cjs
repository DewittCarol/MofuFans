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
const memberAPI = load('../src/lib/member-api.ts', require);

function mountDialog(mode, service = {}) {
  let cursor = 0, effectCursor = 0, tree, changed = false;
  const slots = [], effects = [], calls = [], authenticated = [], intervals = new Map();
  let timerID = 0, closes = 0;
  const session = service.session || { current: null }, storage = service.storage || new Map();
  const localStorage = service.storageDisabled ? { getItem(){throw new Error('storage disabled');},setItem(){throw new Error('storage disabled');},removeItem(){throw new Error('storage disabled');} } : { getItem:key=>storage.get(key)||null,setItem:(key,value)=>storage.set(key,value),removeItem:key=>storage.delete(key) };
  const progress = load('../src/lib/signature-progress.ts', name => name === './member-api' ? memberAPI : require(name), {localStorage});
  const challenge = { challenge_id: 'challenge-A', purpose: mode, status: 'pending', bilibili_uid: '10001', verification_text: 'MofuFun验证xhs3sijs', expires_at: new Date(Date.now() + 600000).toISOString() };
  const props = {open:true,initialMode:mode,onClose(){closes++;props.open=false;},onAuthenticated:user=>authenticated.push(user),currentUser:{bilibili_uid:'10001'}};
  const success = data => ({ code: 200, data, message: '' });
  const hooks = {
    useState(initial) { const index = cursor++; if (!(index in slots)) slots[index] = typeof initial === 'function' ? initial() : initial; return [slots[index], value => { const next = typeof value === 'function' ? value(slots[index]) : value; if (!Object.is(next, slots[index])) { slots[index] = next; changed = true; } }]; },
    useRef(initial) { const index = cursor++; if (!(index in slots)) slots[index] = { current: initial }; return slots[index]; },
    useEffect(work, deps) { const index = effectCursor++, old = effects[index]; if (!old || deps.some((value, i) => !Object.is(value, old.deps[i]))) effects[index] = { deps, work, cleanup: old?.cleanup, pending: true }; },
  };
  const element = (type, props) => ({ type, props: props || {} });
  const api = {
    ...memberAPI,
    getSignatureStatus: async () => { calls.push(['status']); return service.status ? service.status() : session.current ? success({...session.current}) : {code:404,message:'没有进行中的验证'}; },
    startSignature: async (uid, purpose) => { calls.push(['start', uid, purpose]); const response=await(service.start ? service.start(uid,purpose) : success({ ...challenge, purpose, bilibili_uid: uid }));if(response.code===200)session.current={...response.data};return response; },
    checkSignature: async challengeID => { calls.push(['check', challengeID]); const response=await(service.check ? service.check(challengeID) : success({ challenge_id: challengeID, status: 'verified', bilibili_uid: '10001', bilibili_name: '离线测试', confirmation_until: new Date(Date.now() + 300000).toISOString() }));if(response.code===200)session.current={...session.current,...response.data};return response; },
    finishSignature: async (challengeID,password) => { calls.push(['finish', challengeID,password]); const response=await(service.finish ? service.finish(challengeID,password) : success({ bilibili_uid: '10001', signature_verified: true }));if(response.code===200)session.current={...session.current,status:'consumed'};return response; },
  };
  const { AccountDialog } = load('../src/components/account-dialog.tsx', name => {
    if (name === 'react') return hooks;
    if (name === 'react/jsx-runtime') return { jsx: element, jsxs: element, Fragment: 'fragment' };
    if (name === '@/lib/member-api') return api;
    if (name === '@/lib/signature-progress') return progress;
    if (name === '@/lib/api') return { loginAccount: async (username, password) => { calls.push(['login', username, password]); return success({ bilibili_uid: '10001', signature_verified: true }); } };
    if (name === 'lucide-react' || name.startsWith('@/components/ui/')) return new Proxy({}, { get: (_, key) => String(key) });
    return require(name);
  }, { document: {activeElement:null}, window: {setTimeout:()=>0,clearTimeout(){},setInterval:(callback,delay)=>{const id=++timerID;intervals.set(id,{callback,delay});return id;},clearInterval:id=>intervals.delete(id)} });
  const render = () => {
    for (let pass=0;pass<10;pass++) {
      cursor=0;effectCursor=0;changed=false;
      tree=AccountDialog(props);
      for (const effect of effects) if(effect.pending){effect.pending=false;effect.cleanup?.();effect.cleanup=effect.work();}
      if(!changed)return;
    }
    assert.fail('component did not settle');
  };
  const nodes = node => Array.isArray(node) ? node.flatMap(nodes) : node && typeof node === 'object' ? [node, ...nodes(node.props.children)] : [];
  const text = node => Array.isArray(node) ? node.map(text).join('') : node && typeof node === 'object' ? text(node.props.children) : typeof node === 'string' || typeof node === 'number' ? String(node) : '';
  const find = predicate => { const node = nodes(tree).find(predicate); assert.ok(node, 'control exists'); return node; };
  const settle=async()=>{for(let pass=0;pass<3;pass++){await new Promise(resolve=>setImmediate(resolve));render();}};
  const act = async fn => { await settle(); await fn(); await settle(); };
  render();
  return { calls, authenticated, intervals, storage, session, text: () => text(tree), field: id => find(node => node.props.id === id),
    change: (id,value) => act(() => find(node => node.props.id === id).props.onChange({ target: { value } })),
    submit: () => act(() => find(node => node.type === 'form').props.onSubmit({ preventDefault() {} })),
    click: label => act(() => find(node => ['button','Button'].includes(node.type) && text(node) === label).props.onClick()),
    close: () => act(() => find(node=>node.props['aria-label']==='关闭').props.onClick()),
    escape: () => act(() => find(node=>node.props.role==='dialog').props.onKeyDown({key:'Escape'})),
    backdrop: () => act(() => tree.props.onMouseDown?.()),
    reopen: (next=mode) => act(() => {props.open=true;props.initialMode=next;}),
    closed: () => !props.open, closes: () => closes,
    hasField: id => nodes(tree).some(node=>node.props.id===id),
    unmount: () => { for(const effect of effects)effect.cleanup?.(); },
    flush: () => act(() => {}),
  };
}

test('registration checks signature only on explicit click, then sets a site password without a second login name', async () => {
  const app = mountDialog('register');
  await app.change('member-signature-uid', '10001'); await app.submit();
  assert.deepEqual(app.calls, [['status'],['start', '10001', 'register']]);
  assert.equal(app.field('member-verification-text').type,'Input');
  assert.equal(app.field('member-verification-text').props.value, 'MofuFun验证xhs3sijs');
  await app.flush(); assert.equal(app.calls.length, 2, 'rendering never polls Bilibili');
  for (const timer of app.intervals.values()) timer.callback(); await app.flush();
  assert.equal(app.calls.length,2,'the countdown does not query Bilibili');
  await app.click('已修改签名，检查验证');
  assert.match(app.text(), /可以恢复原来的 B 站签名/);
  await app.change('member-new-password', 'offline-password'); await app.change('member-confirm-password', 'not-the-same'); await app.submit();
  assert.equal(app.calls.filter(row => row[0] === 'finish').length, 0);
  assert.match(app.text(), /两次输入的密码不一致/);
  await app.change('member-confirm-password', 'offline-password'); await app.submit();
  assert.deepEqual(app.calls.at(-1), ['finish', 'challenge-A', 'offline-password']); assert.equal(app.authenticated.length, 1);
  assert.doesNotMatch(app.text(), /设置登录名|取关|重新关注/);
});

test('legacy account upgrade verifies the current UID and does not ask for a replacement password', async () => {
  const app=mountDialog('upgrade');await app.flush();assert.equal(app.field('member-signature-uid').props.value,'10001');assert.equal(app.field('member-signature-uid').props.disabled,true);
  await app.submit();await app.click('已修改签名，检查验证');await app.submit();
  assert.deepEqual(app.calls[1],['start','10001','upgrade']);assert.deepEqual(app.calls.at(-1),['finish','challenge-A',undefined]);assert.equal(app.authenticated.length,1);
});

test('recovery uses a fresh recovery challenge and completes with a new password', async () => {
  const app = mountDialog('recover');
  await app.change('member-signature-uid', '10001'); await app.submit(); await app.click('已修改签名，检查验证');
  await app.change('member-new-password', 'new-offline-password'); await app.change('member-confirm-password', 'new-offline-password'); await app.submit();
  assert.deepEqual(app.calls[1], ['start', '10001', 'recover']);
  assert.deepEqual(app.calls.at(-1), ['finish', 'challenge-A', 'new-offline-password']);
});

test('password login retains legacy login names while presenting UID as the normal identifier', async () => {
  const app = mountDialog('login'); await app.change('member-login-name', 'legacy-name'); await app.change('member-login-password', 'offline-password'); await app.submit();
  assert.deepEqual(app.calls, [['status'],['login','legacy-name','offline-password']]); assert.equal(app.authenticated.length, 1);
});

test('password validation handles UTF-8 limits and expired challenges cannot retain time', () => {
  assert.equal(memberAPI.passwordProblem('八个字符仅是测试', '八个字符仅是测试'), '');
  assert.match(memberAPI.passwordProblem('字'.repeat(25), '字'.repeat(25)), /密码过长/);
  assert.match(memberAPI.passwordProblem('1234567', '1234567'), /至少/);
  assert.equal(memberAPI.signatureSecondsLeft({ status: 'pending', expires_at: '2026-10-27T00:00:00Z' }, Date.parse('2026-10-27T00:01:00Z')), 0);
  assert.equal(memberAPI.signatureSecondsLeft({ status: 'verified', expires_at: '2026-10-27T00:00:00Z', confirmation_until: '2026-10-27T00:05:00Z' }, Date.parse('2026-10-27T00:01:00Z')), 0,'confirmation cannot extend the challenge expiry');
});

test('member API binds reads and writes to the displayed account, without carrying batch notes', async () => {
  const calls = [];
  const api = load('../src/lib/member-api.ts', require, { fetch: async (url, options) => { calls.push({ url, options }); return { json: async () => ({ code: 200, data: {} }) }; } });
  await api.saveMemberAddress({ name: '离线测试', phone: '13800000000', address: '离线测试地址', note: '不应沿用的备注' }, '10001');
  assert.deepEqual(JSON.parse(calls[0].options.body), { action: 'save', uid: '10001', address: { name: '离线测试', phone: '13800000000', address: '离线测试地址', note: '' } });
  await api.setMemberDefaultAddress(12, 3, '10001');
  assert.deepEqual(JSON.parse(calls[1].options.body), { action: 'set_default', uid: '10001', entry_id: 12, long_term_version: 3 });
  await api.clearMemberDefaultAddress(4, '10001');
  assert.deepEqual(JSON.parse(calls[2].options.body), { action: 'clear_default', uid: '10001', long_term_version: 4 });
  await api.loadMemberAddressBook('10001'); await api.loadMemberCollections('10001');
  assert.equal(calls[3].url, '/api/account/address-book?expected_uid=10001');
  assert.equal(calls[4].url, '/api/account/collections?expected_uid=10001');
  await api.changeMemberPassword('old-password', 'new-password', '10001'); await api.revokeOtherMemberSessions('old-password', '10001');
  assert.equal(JSON.parse(calls[5].options.body).uid,'10001'); assert.equal(JSON.parse(calls[6].options.body).uid,'10001');
  await api.checkSignature('challenge-A'); await api.finishSignature('challenge-A','new-password');
  assert.deepEqual(JSON.parse(calls[7].options.body), {challenge_id:'challenge-A'});
  assert.deepEqual(JSON.parse(calls[8].options.body), {challenge_id:'challenge-A',password:'new-password'});
  assert.ok(calls.every(call => call.options.credentials === 'include' && call.options.cache === 'no-store'));
});

test('a second-tab challenge cannot replace the challenge attached to the password form', async () => {
  let activeChallenge='challenge-A', writes=0;
  const app=mountDialog('register',{finish:async(id)=>{if(id!==activeChallenge)return{code:409,message:'验证申请已变化，请重新开始'};writes++;return{code:200,data:{bilibili_uid:'10001'}};}});
  await app.change('member-signature-uid','10001');await app.submit();await app.click('已修改签名，检查验证');
  activeChallenge='challenge-B';
  await app.change('member-new-password','offline-password');await app.change('member-confirm-password','offline-password');await app.submit();
  assert.deepEqual(app.calls.at(-1),['finish','challenge-A','offline-password']);assert.equal(writes,0);assert.equal(app.authenticated.length,0);assert.match(app.text(),/验证申请已变化/);
});

const progressKey='mofu-signature-progress-v1';
const pendingProgress=(extra={})=>({challenge_id:'challenge-A',purpose:'register',status:'pending',bilibili_uid:'10001',verification_text:'MofuFun验证xhs3sijs',expires_at:new Date(Date.now()+600000).toISOString(),...extra});
const verifiedProgress=(extra={})=>pendingProgress({status:'verified',confirmation_until:new Date(Date.now()+300000).toISOString(),...extra});

test('clicking the backdrop never closes the dialog; explicit close and reopen restore the same pending challenge',async()=>{
  const app=mountDialog('register');await app.change('member-signature-uid','10001');await app.submit();
  const expiry=app.session.current.expires_at;
  await app.backdrop();assert.equal(app.closed(),false);assert.equal(app.closes(),0);
  await app.close();assert.equal(app.closed(),true);assert.ok(app.storage.has(progressKey));
  await app.reopen();assert.equal(app.field('member-verification-text').props.value,'MofuFun验证xhs3sijs');assert.match(app.text(),/已恢复上次的注册进度/);
  assert.equal(app.session.current.expires_at,expiry);assert.equal(app.calls.filter(call=>call[0]==='start').length,1);assert.equal(app.calls.filter(call=>call[0]==='check').length,0);
});

test('Escape preserves verified progress but never caches passwords; reopening returns directly to an empty password form',async()=>{
  const app=mountDialog('register');await app.change('member-signature-uid','10001');await app.submit();await app.click('已修改签名，检查验证');
  await app.change('member-new-password','sensitive-secret');await app.change('member-confirm-password','sensitive-secret');
  await app.escape();assert.equal(app.closed(),true);
  const cached=JSON.parse(app.storage.get(progressKey));assert.equal('status'in cached,false);assert.doesNotMatch(app.storage.get(progressKey),/sensitive-secret|password/);
  await app.reopen();assert.equal(app.field('member-new-password').props.value,'');assert.equal(app.field('member-confirm-password').props.value,'');assert.match(app.text(),/签名验证通过/);
  assert.equal(app.calls.filter(call=>call[0]==='check').length,1,'restoring the verified phase does not query Bilibili');
  await app.change('member-new-password','new-password');await app.change('member-confirm-password','new-password');await app.submit();assert.equal(app.storage.has(progressKey),false);
});

test('a reload restores pending text and legacy verified challenges from server status without polling Bilibili',async()=>{
  const first=mountDialog('register');await first.change('member-signature-uid','10001');await first.submit();first.unmount();
  const reloaded=mountDialog('register',{storage:first.storage,session:first.session});await reloaded.flush();
  assert.equal(reloaded.field('member-verification-text').props.value,'MofuFun验证xhs3sijs');assert.deepEqual(reloaded.calls,[['status']]);
  const oldVerified=verifiedProgress();delete oldVerified.verification_text;
  const legacy=mountDialog('register',{session:{current:oldVerified}});await legacy.flush();
  assert.equal(legacy.field('member-new-password').props.value,'');assert.deepEqual(legacy.calls,[['status']]);assert.match(legacy.text(),/签名验证通过/);
});

test('the login dialog offers the unfinished purpose explicitly instead of silently switching modes',async()=>{
  const app=mountDialog('login',{session:{current:verifiedProgress({purpose:'recover'})}});await app.flush();
  assert.ok(app.hasField('member-login-name'));assert.equal(app.hasField('member-new-password'),false);assert.match(app.text(),/有一项未完成的找回密码/);
  await app.click('继续上次找回密码');assert.ok(app.hasField('member-new-password'));assert.equal(app.hasField('member-login-name'),false);assert.match(app.text(),/重设密码并登录/);assert.deepEqual(app.calls,[['status']]);
});

test('pending old challenges can use a matching cached long string or check the already-edited signature without it',async()=>{
  const old=pendingProgress();delete old.verification_text;
  const storage=new Map([[progressKey,JSON.stringify({...old,verification_text:'MFV-legacy-long-verification-string'})]]);
  const cached=mountDialog('register',{session:{current:old},storage});await cached.flush();assert.equal(cached.field('member-verification-text').props.value,'MFV-legacy-long-verification-string');
  const noCache=mountDialog('register',{session:{current:{...old}}});await noCache.flush();assert.equal(noCache.hasField('member-verification-text'),false);assert.match(noCache.text(),/原验证文字仍然有效/);
  await noCache.click('已修改签名，检查验证');assert.ok(noCache.hasField('member-new-password'));assert.deepEqual(noCache.calls,[['status'],['check','challenge-A']]);
});

test('cached phase, extra fields and deadlines cannot override server status',async()=>{
  const authoritative=pendingProgress({verification_text:'来自服务器的文字',expires_at:new Date(Date.now()+120000).toISOString()});
  const storage=new Map([[progressKey,JSON.stringify({...authoritative,status:'verified',password:'do-not-store-this',verification_text:'被修改的缓存文字',expires_at:new Date(Date.now()+86400000).toISOString(),confirmation_until:new Date(Date.now()+86400000).toISOString()})]]);
  const app=mountDialog('register',{storage,session:{current:authoritative}});await app.flush();
  assert.equal(app.hasField('member-new-password'),false);assert.equal(app.field('member-verification-text').props.value,'来自服务器的文字');
  const saved=JSON.parse(storage.get(progressKey));assert.equal(saved.expires_at,authoritative.expires_at);assert.equal('status'in saved,false);assert.equal('password'in saved,false);assert.equal('confirmation_until'in saved,false);
});

test('a different server challenge is offered explicitly and never receives the old cached verification text',async()=>{
  const storage=new Map([[progressKey,JSON.stringify(pendingProgress({verification_text:'旧账号的验证文字'}))]]);
  const other=pendingProgress({challenge_id:'challenge-B',bilibili_uid:'10002'});delete other.verification_text;
  const app=mountDialog('register',{storage,session:{current:other}});await app.flush();assert.equal(app.hasField('member-verification-text'),false);assert.match(app.text(),/UID 10002/);assert.doesNotMatch(app.text(),/旧账号的验证文字/);
  await app.click('继续上次注册');assert.equal(app.hasField('member-verification-text'),false);assert.match(app.text(),/原验证文字仍然有效/);assert.equal(JSON.parse(storage.get(progressKey)).challenge_id,'challenge-B');
});

test('an in-progress password form does not switch accounts on a different tab challenge, and a stale failure preserves the newer public cache',async()=>{
  const session={current:verifiedProgress()},storage=new Map();
  const app=mountDialog('register',{session,storage,finish:async id=>id===session.current.challenge_id?{code:200,data:{bilibili_uid:session.current.bilibili_uid}}:{code:409,message:'验证申请已变化，请重新开始'}});await app.flush();
  await app.change('member-new-password','same-password');await app.change('member-confirm-password','same-password');
  session.current=verifiedProgress({challenge_id:'challenge-B',bilibili_uid:'10002'});storage.set(progressKey,JSON.stringify(session.current));await app.flush();
  assert.equal(app.field('member-new-password').props.value,'same-password');assert.equal(app.calls.filter(call=>call[0]==='status').length,1);
  await app.submit();assert.deepEqual(app.calls.at(-1),['finish','challenge-A','same-password']);assert.equal(app.authenticated.length,0);assert.equal(app.hasField('member-new-password'),false);assert.equal(JSON.parse(storage.get(progressKey)).challenge_id,'challenge-B');
});

test('expired, consumed, missing and rejected server challenges clear cached progress and cannot show password entry',async()=>{
  const responses=[{code:200,data:pendingProgress({status:'expired',expires_at:new Date(Date.now()-1000).toISOString()})},{code:200,data:verifiedProgress({status:'consumed'})},{code:404,message:'验证不存在'},{code:409,message:'验证已失效'}];
  for(const response of responses){const storage=new Map([[progressKey,JSON.stringify(verifiedProgress())]]);const app=mountDialog('register',{storage,status:async()=>response});await app.flush();assert.equal(storage.has(progressKey),false);assert.equal(app.hasField('member-new-password'),false);assert.ok(app.hasField('member-signature-uid'));}
});

test('network failure never trusts the cached verified phase, and retry restores from server status',async()=>{
  const storage=new Map([[progressKey,JSON.stringify({...verifiedProgress(),status:'verified'})]]);let unavailable=true;
  const app=mountDialog('register',{storage,status:async()=>{if(unavailable)throw new Error('offline');return{code:200,data:verifiedProgress()};}});await app.flush();
  assert.equal(app.hasField('member-new-password'),false);assert.equal(app.hasField('member-verification-text'),false);assert.ok(storage.has(progressKey));assert.match(app.text(),/重试恢复/);
  unavailable=false;await app.click('重试恢复');assert.ok(app.hasField('member-new-password'));assert.deepEqual(app.calls,[['status'],['status']]);
});

test('explicit restart clears public progress after failed restoration',async()=>{
  const storage=new Map([[progressKey,JSON.stringify(pendingProgress())]]);const app=mountDialog('register',{storage,status:async()=>{throw new Error('offline');}});await app.flush();await app.click('重新开始');assert.equal(storage.has(progressKey),false);assert.ok(app.hasField('member-signature-uid'));
});

test('explicitly abandoning a pending challenge prevents automatic restoration but leaves a clearly labelled continue option',async()=>{
  const app=mountDialog('register',{session:{current:pendingProgress()}});await app.flush();
  await app.click('重新选择 UID 或生成验证文字');assert.equal(app.storage.has(progressKey),false);
  await app.close();await app.reopen();assert.equal(app.hasField('member-verification-text'),false);assert.ok(app.hasField('member-signature-uid'));assert.match(app.text(),/有一项未完成的注册/);assert.equal(app.storage.has(progressKey),false);
  await app.click('继续上次注册');assert.ok(app.hasField('member-verification-text'));assert.ok(app.storage.has(progressKey));
});

test('server-cookie recovery remains usable when local storage is disabled',async()=>{
  const app=mountDialog('register',{storageDisabled:true,session:{current:verifiedProgress()}});await app.flush();assert.ok(app.hasField('member-new-password'));await app.close();await app.reopen();assert.ok(app.hasField('member-new-password'));assert.deepEqual(app.calls,[['status'],['status']]);
});

test('closing during restoration invalidates its late response, including after reopening a different purpose',async()=>{
  let resolveOld,count=0;const app=mountDialog('register',{status:()=>{count++;return count===1?new Promise(resolve=>{resolveOld=resolve;}):Promise.resolve({code:404});}});
  await app.close();await app.reopen('recover');resolveOld({code:200,data:verifiedProgress()});await app.flush();assert.equal(app.hasField('member-new-password'),false);assert.ok(app.hasField('member-signature-uid'));assert.match(app.text(),/找回密码/);assert.doesNotMatch(app.text(),/签名验证通过/);
});
