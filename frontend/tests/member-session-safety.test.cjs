const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const ts = require('typescript');

function load(relative, resolve, globals = {}) {
  const filename = path.join(__dirname, relative), module = { exports: {} };
  const source = ts.transpileModule(fs.readFileSync(filename, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  vm.runInNewContext(source, { module, exports: module.exports, require: resolve, Error, Event, TextEncoder, ...globals }, { filename });
  return module.exports;
}
const element = (type, props) => ({ type, props: props || {} });
const nodes = node => Array.isArray(node) ? node.flatMap(nodes) : node && typeof node === 'object' ? [node, ...nodes(node.props.children)] : [];
const text = node => Array.isArray(node) ? node.map(text).join('') : node && typeof node === 'object' ? text(node.props.children) : typeof node === 'string' ? node : '';
const tick = () => new Promise(resolve => setImmediate(resolve));
function events() {
  const listeners = new Map();
  return {
    addEventListener(name, listener) { if (!listeners.has(name)) listeners.set(name,new Set()); listeners.get(name).add(listener); },
    removeEventListener(name, listener) { listeners.get(name)?.delete(listener); },
    dispatchEvent(event) { for (const listener of listeners.get(event.type) || []) listener(event); },
  };
}
function hookHost() {
  let cursor=0, effectCursor=0, changed=false, tree;
  const slots=[], effects=[];
  const hooks={
    createContext: () => ({ Provider:'AuthProvider' }),
    useState(initial) { const i=cursor++;if(!(i in slots))slots[i]=typeof initial==='function'?initial():initial;return[slots[i],value=>{const next=typeof value==='function'?value(slots[i]):value;if(!Object.is(next,slots[i])){slots[i]=next;changed=true;}}]; },
    useRef(initial) { const i=cursor++;if(!(i in slots))slots[i]={current:initial};return slots[i]; },
    useCallback(callback,deps) { const i=cursor++,old=slots[i];if(!old||deps.some((v,n)=>!Object.is(v,old.deps[n])))slots[i]={callback,deps};return slots[i].callback; },
    useEffect(work,deps) { const i=effectCursor++,old=effects[i];if(!old||deps.some((v,n)=>!Object.is(v,old.deps[n])))effects[i]={work,deps,cleanup:old?.cleanup,pending:true}; },
  };
  return { hooks, render(component) { for(let pass=0;pass<20;pass++){cursor=0;effectCursor=0;changed=false;tree=component();for(const effect of effects)if(effect.pending){effect.pending=false;effect.cleanup?.();effect.cleanup=effect.work();}if(!changed)return tree;}assert.fail('render did not settle'); }, get tree(){return tree;} };
}

function providerTabs() {
  const tabs=[], broadcasts=[], user={bilibili_uid:'10001',signature_verified:true,updated_at:'one'};
  let serverUser=user, completeLogout, rejectLogout;
  const env={broadcasts,user,get serverUser(){return serverUser;},commitLogout(){serverUser=null;completeLogout({code:200,data:null});},failLogout(){rejectLogout(new Error('offline'));}};
  const mount=()=>{
    const host=hookHost(), win=events(), calls=[];
    const tab={win,calls,deferInfo:null};
    const api={
      getAccountInfo:()=>{calls.push('info');if(tab.deferInfo)return tab.deferInfo;return Promise.resolve({code:serverUser?200:408,data:serverUser});},
      logoutAccount:uid=>{calls.push(['logout',uid]);return new Promise((resolve,reject)=>{completeLogout=resolve;rejectLogout=reject;});},
    };
    const {AccountAuthProvider}=load('../src/lib/account-auth.tsx',name=>{
      if(name==='react')return host.hooks;
      if(name==='react/jsx-runtime')return{jsx:element,jsxs:element,Fragment:'fragment'};
      if(name==='./api')return api;
      if(name==='./emoji-catalog')return{useEmojiCatalog:()=>({accountsEnabled:true})};
      if(name==='@/components/account-dialog')return{AccountDialog:'AccountDialog'};
      return require(name);
    },{window:win,localStorage:{setItem(key,newValue){broadcasts.push(JSON.parse(newValue));for(const other of tabs)if(other!==tab)other.win.dispatchEvent({type:'storage',key,newValue});}}});
    tab.render=()=>host.render(()=>AccountAuthProvider({children:'private children'}));
    tab.value=()=>tab.render().props.value;
    tab.text=()=>text(tab.render());
    tabs.push(tab);tab.render();return tab;
  };
  return{...env,mount,flush:async()=>{await tick();tabs.forEach(tab=>tab.render());}};
}

test('delayed logout clears locally first, then invalidates other tabs and their pending old info responses',async()=>{
  const env=providerTabs(),a=env.mount(),b=env.mount();await env.flush();
  assert.equal(a.value().user.bilibili_uid,'10001');assert.equal(b.value().user.bilibili_uid,'10001');
  let oldInfo; b.deferInfo=new Promise(resolve=>{oldInfo=resolve;});const pendingInfo=b.value().refresh();
  const logout=a.value().logout();assert.equal(a.value().user,null);assert.equal(env.broadcasts.length,0,'no broadcast before server logout finishes');
  const beforeFocus=a.calls.length;a.win.dispatchEvent({type:'focus'});assert.equal(a.calls.length,beforeFocus,'focus cannot repopulate the local view during logout');
  env.commitLogout();await logout;await env.flush();
  assert.equal(env.broadcasts.at(-1).kind,'signed-out');assert.equal(b.value().user,null);
  oldInfo({code:200,data:env.user});await pendingInfo;await env.flush();
  assert.equal(b.value().user,null,'a stale response cannot restore private data after the signed-out event');
  const beforeOtherFocus=b.calls.length;b.win.dispatchEvent({type:'focus'});assert.equal(b.calls.length,beforeOtherFocus);
  assert.deepEqual(a.calls.find(Array.isArray),['logout','10001']);
});

test('failed logout stays hidden locally, reports failure, and only a successful retry signs out other tabs',async()=>{
  const env=providerTabs(),a=env.mount(),b=env.mount();await env.flush();
  const logout=a.value().logout();env.failLogout();await logout;await env.flush();
  assert.equal(a.value().user,null);assert.match(a.text(),/本页个人信息已隐藏/);assert.match(a.text(),/重试退出/);
  assert.equal(b.value().user.bilibili_uid,'10001');assert.equal(env.broadcasts.length,0);
  const count=a.calls.length;a.win.dispatchEvent({type:'focus'});assert.equal(a.calls.length,count);
  const retry=a.value().logout();env.commitLogout();await retry;await env.flush();
  assert.equal(b.value().user,null);assert.doesNotMatch(a.text(),/重试退出/);
});

function mountBook(service) {
  const host=hookHost(),win=events(),requests=[];
  const api=load('../src/lib/member-api.ts',require,{window:win,fetch:async(url,options)=>{const request={url,...options,body:options.body?JSON.parse(options.body):undefined};requests.push(request);return{json:()=>service(request)};}});
  const {MemberAddressBookPanel}=load('../src/components/member-address-book.tsx',name=>{
    if(name==='react')return host.hooks;
    if(name==='react/jsx-runtime')return{jsx:element,jsxs:element,Fragment:'fragment'};
    if(name==='@/lib/member-api')return api;
    if(name==='@/lib/voyage-claim')return{blankAddress:{name:'',phone:'',address:'',note:''}};
    if(name==='@/lib/site-dialog')return{confirmDiscard:async()=>true};
    if(name.startsWith('@/components/ui/'))return new Proxy({},{get:(_,key)=>String(key)});
    return require(name);
  },{window:win});
  const render=()=>host.render(()=>MemberAddressBookPanel({uid:'10001'}));
  const find=predicate=>{const node=nodes(render()).find(predicate);assert.ok(node,'control exists');return node;};
  const act=async(callback)=>{await callback();await tick();render();};
  render();
  return{requests,flush:()=>act(()=>{}),text:()=>text(render()),change:(id,value)=>act(()=>find(node=>node.props.id===id).props.onChange({target:{value}})),click:label=>act(()=>find(node=>node.type==='Button'&&text(node)===label).props.onClick()),submit:()=>act(()=>find(node=>node.type==='form').props.onSubmit({preventDefault(){}})),hasAddress:()=>nodes(render()).some(node=>node.props.id==='book-address')};
}
const emptyBook=uid=>({uid,entries:[],long_term:{enabled:false,version:1}});
test('an old address draft keeps its displayed UID after another tab changes the cookie and is rejected without a write',async()=>{
  let cookieUID='10001',writes=0;
  const app=mountBook(async request=>{
    const expected=request.body?.uid||new URL(request.url,'https://offline.invalid').searchParams.get('expected_uid');
    if(expected!==cookieUID)return{code:403,message:'登录账号已变化，请刷新页面后重试'};
    if(request.method==='POST')writes++;
    return{code:200,data:emptyBook(cookieUID)};
  });
  await app.flush();await app.click('添加地址');await app.change('book-name','甲账号');await app.change('book-phone','13800000000');await app.change('book-address','甲账号的未保存地址');
  cookieUID='10002';await app.submit();
  assert.equal(app.requests.at(-1).body.uid,'10001');assert.equal(writes,0);assert.equal(app.hasAddress(),false);assert.match(app.text(),/登录账号已变化/);
});
test('a mismatched address-book response is cleared rather than displayed under the previous account',async()=>{
  const app=mountBook(async()=>({code:200,data:{...emptyBook('10002'),entries:[{id:1,address:{name:'不应显示的收件人',phone:'13800000000',address:'不应显示的地址'},is_default:false}]}}));
  await app.flush();assert.doesNotMatch(app.text(),/不应显示/);assert.match(app.text(),/登录账号已变化/);
});
