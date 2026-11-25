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
const nodes = value => Array.isArray(value) ? value.flatMap(nodes) : value && typeof value === 'object' ? [value, ...nodes(value.props.children)] : [];
const text = value => Array.isArray(value) ? value.map(text).join('') : value && typeof value === 'object' ? text(value.props.children) : typeof value === 'string' || typeof value === 'number' ? String(value) : '';
const element = (type, props, key) => ({ type, props: props || {}, key });
const account = (extra={}) => ({username:'legacy-name',bilibili_uid:'10001',bilibili_name:'当前昵称',bilibili_avatar:'/api/avatars/member.webp',signature_verified:true,...extra});
const tick = () => new Promise(resolve => setImmediate(resolve));

function host() {
  let cursor=0,effectCursor=0,changed=false,tree;
  const slots=[],effects=[],focus=[];
  const hooks={
    useState(initial){const i=cursor++;if(!(i in slots))slots[i]=typeof initial==='function'?initial():initial;return[slots[i],value=>{const next=typeof value==='function'?value(slots[i]):value;if(!Object.is(next,slots[i])){slots[i]=next;changed=true;}}];},
    useRef(initial){const i=cursor++;if(!(i in slots))slots[i]={current:initial};return slots[i];},
    useCallback(fn,deps){const i=cursor++,old=slots[i];if(!old||deps.some((v,n)=>!Object.is(v,old.deps[n])))slots[i]={fn,deps};return slots[i].fn;},
    useEffect(work,deps){const i=effectCursor++,old=effects[i];if(!old||deps.some((v,n)=>!Object.is(v,old.deps[n])))effects[i]={work,deps,cleanup:old?.cleanup,pending:true};},
  };
  return {hooks,focus,get tree(){return tree;},render(component){for(let pass=0;pass<20;pass++){cursor=0;effectCursor=0;changed=false;tree=component();for(const node of nodes(tree))if(node.props.role==='tab'&&node.props.ref)node.props.ref({focus(){focus.push(node.props.id);node.props.onFocus?.();}});for(const effect of effects)if(effect.pending){effect.pending=false;effect.cleanup?.();effect.cleanup=effect.work();}if(!changed)return tree;}assert.fail('render did not settle');},unmount(){for(const effect of effects)effect.cleanup?.();}};
}

function mount(kind='workspace',options={}) {
  const hooks=host(),dialogs=load('../src/lib/site-dialog.ts',require),requests=[],notices=[],states=[],selected=[],authenticated=[],opened=[];
  const listeners=new Map(),mediaListeners=new Set(),media={matches:options.desktop??true,addEventListener:(_,fn)=>mediaListeners.add(fn),removeEventListener:(_,fn)=>mediaListeners.delete(fn)};
  const window={matchMedia:()=>media,addEventListener(name,fn){if(!listeners.has(name))listeners.set(name,new Set());listeners.get(name).add(fn);},removeEventListener(name,fn){listeners.get(name)?.delete(fn);}};
  const auth={user:account(),loading:false,accountsEnabled:true,authRevision:1,setUser:user=>authenticated.push(user),refresh:async()=>requests.push(['refresh']),logout:async()=>requests.push(['logout']),openAccountDialog:mode=>opened.push(mode),...options.auth};
  const memberAPI=load('../src/lib/member-api.ts',require);
  const book={uid:'10001',entries:[],long_term:{enabled:false,version:1}};
  const api={...memberAPI,
    changeMemberPassword:async(...args)=>{requests.push(['password',...args]);return options.password?options.password(...args):{code:200,data:auth.user};},
    revokeOtherMemberSessions:async(...args)=>{requests.push(['revoke',...args]);return{code:200,data:auth.user};},
    loadMemberAddressBook:async uid=>{requests.push(['book',uid]);return book;},
    saveMemberAddress:async(...args)=>{requests.push(['save',...args]);return options.save?options.save(...args):book;},
    loadMemberCollections:async uid=>{requests.push(['collections',uid]);return[{id:7,title:'已发布批次',note:'',collecting:true,closed:false}];},
  };
  const resolve=name=>{
    if(name==='react')return hooks.hooks;
    if(name==='react/jsx-runtime')return{jsx:element,jsxs:element,Fragment:'fragment'};
    if(name==='next/image')return{__esModule:true,default:'Image'};
    if(name==='lucide-react'||name.startsWith('@/components/ui/'))return new Proxy({},{get:(_,key)=>String(key)});
    if(name==='@/lib/account-auth')return{useAccountAuth:()=>auth};
    if(name==='@/lib/site-dialog')return dialogs;
    if(name==='@/lib/member-api')return api;
    if(name==='@/lib/voyage-claim')return{blankAddress:{name:'',phone:'',address:'',note:''}};
    if(name==='@/components/member-address-book')return{MemberAddressBookPanel:'MemberAddressBookPanel'};
    if(name==='@/components/member-collections')return{MemberCollections:'MemberCollections'};
    if(name==='@/components/member-security-panel')return{MemberSecurityPanel:'MemberSecurityPanel'};
    if(name==='@/components/voyage-claim')return{VoyageClaim:'VoyageClaim'};
    if(name==='@/components/member-home')return{SignedInMemberHome:'SignedInMemberHome'};
    return require(name);
  };
  const home=load('../src/components/member-home.tsx',resolve,{window});
  let Component,props;
  if(kind==='workspace'){Component=home.MemberHomeWorkspace;props={user:auth.user,initialSection:'addresses',onSectionChange:value=>selected.push(value),onNotice:value=>notices.push(value),...options.props};}
  if(kind==='header'){Component=home.MemberAccountHeader;props={user:auth.user,onLogout:auth.logout,...options.props};}
  if(kind==='signed'){Component=home.SignedInMemberHome;props={user:auth.user,authRevision:auth.authRevision,...options.props};}
  if(kind==='page'){Component=load('../src/app/account/page.tsx',resolve,{window}).default;props={};}
  if(kind==='security'){Component=load('../src/components/member-security-panel.tsx',resolve,{window}).MemberSecurityPanel;props={uid:auth.user.bilibili_uid,onDone:value=>notices.push(value),onStateChange:value=>states.push(value)};}
  if(kind==='book'){Component=load('../src/components/member-address-book.tsx',resolve,{window}).MemberAddressBookPanel;props={uid:auth.user.bilibili_uid,onStateChange:value=>states.push(value)};}
  if(kind==='collections'){Component=load('../src/components/member-collections.tsx',resolve,{window}).MemberCollections;props={uid:auth.user.bilibili_uid,onStateChange:value=>states.push(value)};}
  const render=()=>hooks.render(()=>Component(props));
  const find=predicate=>{const node=nodes(hooks.tree).find(predicate);assert.ok(node,'expected control exists');return node;};
  const flush=async()=>{await tick();render();await tick();render();};
  const act=async work=>{await work();await flush();};
  render();
  return{auth,props,requests,states,selected,notices,authenticated,opened,dialogs:dialogs.siteDialogs,focus:hooks.focus,render,flush,act,nodes:()=>nodes(hooks.tree),text:()=>text(hooks.tree),find,
    click:label=>act(()=>find(node=>['button','Button'].includes(node.type)&&text(node)===label).props.onClick()),
    change:(id,value)=>act(()=>find(node=>node.props.id===id).props.onChange({target:{value}})),
    submit:(index=0)=>act(()=>nodes(hooks.tree).filter(node=>node.type==='form')[index].props.onSubmit({preventDefault(){}})),
    panel:()=>find(node=>['MemberAddressBookPanel','MemberCollections','MemberSecurityPanel'].includes(node.type)),
    resize:desktop=>act(()=>{media.matches=desktop;for(const fn of mediaListeners)fn();}),
    unmount:()=>hooks.unmount(),
    mountAvatar:()=>{const avatar=find(node=>typeof node.type==='function');Component=avatar.type;props=avatar.props;render();},
  };
}

test('only the selected module mounts, and clean switching does not request confirmation or submit forms',async()=>{
  const app=mount();assert.equal(app.panel().type,'MemberAddressBookPanel');assert.equal(app.nodes().filter(node=>node.props.role==='tabpanel'&&!node.props.hidden).length,1);
  await app.click('礼物与物流');assert.equal(app.panel().type,'MemberCollections');assert.equal(app.nodes().some(node=>node.type==='MemberAddressBookPanel'),false);
  await app.click('账户安全');assert.equal(app.panel().type,'MemberSecurityPanel');assert.deepEqual(app.selected,['collections','security']);assert.equal(app.dialogs.getSnapshot(),null);assert.deepEqual(app.requests,[]);
  assert.ok(app.nodes().filter(node=>node.props.role==='tab').every(node=>node.props.type==='button'));
});

test('saving blocks switching; a dirty form stays intact on cancellation and unmounts only after confirmation',async()=>{
  const app=mount();const report=app.panel().props.onStateChange;
  await app.act(()=>report({busy:true,dirty:true}));await app.click('礼物与物流');assert.equal(app.panel().type,'MemberAddressBookPanel');assert.equal(app.dialogs.getSnapshot(),null);assert.match(app.text(),/操作进行中/);
  await app.act(()=>report({busy:false,dirty:true}));await app.click('礼物与物流');const first=app.dialogs.getSnapshot();assert.ok(first);assert.equal(app.panel().type,'MemberAddressBookPanel');
  await app.act(()=>app.dialogs.finish(first.id,false));assert.equal(app.panel().type,'MemberAddressBookPanel');assert.deepEqual(app.selected,[]);
  await app.click('礼物与物流');await app.act(()=>app.dialogs.finish(app.dialogs.getSnapshot().id,true));assert.equal(app.panel().type,'MemberCollections');assert.deepEqual(app.selected,['collections']);
  await app.act(()=>report({busy:true,dirty:true}));assert.equal(app.find(node=>node.props.id==='member-tab-security').props.disabled,false,'a previous module cannot lock the new module');
  await app.click('地址簿');await app.act(()=>report({busy:true,dirty:true}));assert.equal(app.find(node=>node.props.id==='member-tab-security').props.disabled,false,'returning to the same section does not revive its old callbacks');
});

test('identity replacement cancels its pending discard prompt and ignores old module callbacks',async()=>{
  const app=mount();const report=app.panel().props.onStateChange;await app.act(()=>report({busy:false,dirty:true}));await app.click('账户安全');assert.ok(app.dialogs.getSnapshot());
  app.unmount();await tick();assert.equal(app.dialogs.getSnapshot(),null);assert.deepEqual(app.selected,[]);
  report({busy:true,dirty:true});assert.deepEqual(app.selected,[]);
});

test('unmount does not cancel an unrelated site dialog',async()=>{
  const app=mount();const other=app.dialogs.open('其他操作',{title:'其他确认'});const id=app.dialogs.getSnapshot().id;
  await app.act(()=>app.panel().props.onStateChange({busy:false,dirty:true}));await app.click('账户安全');app.unmount();assert.equal(app.dialogs.getSnapshot().id,id);app.dialogs.finish(id,false);await other;
});

test('keyboard navigation moves focus without discarding a draft, and orientation follows the layout',async()=>{
  const app=mount();await app.act(()=>app.panel().props.onStateChange({busy:false,dirty:true}));
  let prevented=false;await app.act(()=>app.find(node=>node.props.id==='member-tab-addresses').props.onKeyDown({key:'ArrowDown',preventDefault(){prevented=true;}}));
  assert.equal(prevented,true);assert.equal(app.focus.at(-1),'member-tab-collections');assert.equal(app.panel().type,'MemberAddressBookPanel');assert.equal(app.dialogs.getSnapshot(),null);
  assert.equal(app.find(node=>node.props.role==='tablist').props['aria-orientation'],'vertical');await app.resize(false);assert.equal(app.find(node=>node.props.role==='tablist').props['aria-orientation'],'horizontal');
  await app.act(()=>app.find(node=>node.props.id==='member-tab-collections').props.onKeyDown({key:'End',preventDefault(){}}));assert.equal(app.focus.at(-1),'member-tab-security');assert.equal(app.find(node=>node.props.id==='member-tab-security').props.tabIndex,0);
});

test('unverified users can reach security but never mount address or delivery modules',async()=>{
  const app=mount('workspace',{auth:{user:account({signature_verified:false})},props:{initialSection:'security'}});assert.equal(app.panel().type,'MemberSecurityPanel');
  await app.click('地址簿');assert.equal(app.nodes().some(node=>node.type==='MemberAddressBookPanel'),false);assert.match(app.text(),/完成签名验证/);await app.click('验证 B 站身份');assert.deepEqual(app.opened,['upgrade']);
  await app.click('礼物与物流');assert.equal(app.nodes().some(node=>node.type==='MemberCollections'),false);
});

test('the identity header remains outside module replacement, with a keyed private workspace after session changes',async()=>{
  const app=mount('signed');const workspace=app.find(node=>typeof node.type==='function'&&node.type.name==='MemberHomeWorkspace');const oldKey=workspace.key;
  await app.act(()=>workspace.props.onSectionChange('security'));app.props.authRevision=2;app.render();
  const next=app.find(node=>typeof node.type==='function'&&node.type.name==='MemberHomeWorkspace');assert.notEqual(next.key,oldKey);assert.equal(next.props.initialSection,'security');
  assert.equal(app.nodes().filter(node=>typeof node.type==='function'&&node.type.name==='MemberAccountHeader').length,1);
  const page=mount('page');assert.equal(page.find(node=>node.type==='SignedInMemberHome').key,'10001');page.auth.user=account({bilibili_uid:'10002'});page.render();assert.equal(page.find(node=>node.type==='SignedInMemberHome').key,'10002');
  page.auth.user=null;page.render();assert.equal(page.nodes().some(node=>node.type==='SignedInMemberHome'),false);
});

test('the profile header displays account identity and logout, with a safe avatar fallback',async()=>{
  const app=mount('header');assert.match(app.text(),/当前昵称/);assert.match(app.text(),/UID 10001/);assert.ok(app.find(node=>node.type==='header').props.className.includes('bg-card'));await app.click('退出登录');assert.deepEqual(app.requests,[['logout']]);
  app.mountAvatar();const image=app.find(node=>node.type==='Image');assert.equal(image.props.src,'/api/avatars/member.webp');assert.equal(image.props.alt,'');await app.act(()=>image.props.onError());assert.equal(app.nodes().some(node=>node.type==='Image'),false);assert.equal(app.text(),'当');
});

test('security edits report dirty and busy states, capture UID, and cannot restore a session after unmount',async()=>{
  let complete;const app=mount('security',{password:()=>new Promise(resolve=>{complete=resolve;})});
  await app.change('current-member-password','old-password');await app.change('new-member-password','new-password');await app.change('confirm-member-password','new-password');assert.equal(app.states.at(-1).dirty,true);
  await app.submit();assert.equal(app.states.at(-1).busy,true);assert.deepEqual(app.requests,[['password','old-password','new-password','10001']]);await app.submit();assert.equal(app.requests.length,1);
  app.unmount();complete({code:200,data:account()});await tick();assert.deepEqual(app.authenticated,[]);assert.deepEqual(app.notices,[]);
  const fresh=mount('security');assert.equal(fresh.find(node=>node.props.id==='new-member-password').props.value,'');assert.equal(fresh.states.at(-1).dirty,false);
});

test('address-book and collection modules report their existing editing state through the optional callback',async()=>{
  let saved;const app=mount('book',{save:()=>new Promise(resolve=>{saved=resolve;})});await app.flush();await app.click('添加地址');await app.change('book-name','收件人');assert.equal(app.states.at(-1).dirty,true);await app.submit();assert.equal(app.states.at(-1).busy,true);saved({uid:'10001',entries:[],long_term:{enabled:false,version:1}});await app.flush();assert.equal(app.states.at(-1).busy,false);assert.equal(app.states.at(-1).dirty,false);
  const collections=mount('collections');await collections.flush();await collections.click('填写地址');await collections.act(()=>collections.find(node=>node.type==='VoyageClaim').props.onStateChange({busy:true,dirty:true}));assert.equal(collections.states.at(-1).busy,true);assert.equal(collections.states.at(-1).dirty,true);
});
