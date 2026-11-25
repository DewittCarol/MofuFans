const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = name => fs.readFileSync(path.join(__dirname, 'whisper-helper', name), 'utf8');

test('gift module is allowed on production and preview, without allowing other admin pages', () => {
  let listener;
  const chrome = { runtime: { onMessageExternal: { addListener(fn) { listener = fn; } } } };
  vm.runInNewContext(source('background.js'), { chrome, URL, Set, Date, setTimeout });
  for (const origin of ['https://example.com', 'https://preview.example.com']) {
    let reply;
    listener({ type: 'fans:ping' }, { url: `${origin}/admin/dashboard/gifts/` }, data => { reply = data; });
    assert.equal(reply.ok, true);
    for (const route of ['gifts-evil/', 'settings/', '../gifts/']) {
      reply = null;
      listener({ type: 'fans:ping' }, { url: `${origin}/admin/dashboard/${route}` }, data => { reply = data; });
      assert.equal(reply, null);
    }
  }
});
function mockDocument() {
  const attrs = new Map();
  return Object.assign(new EventTarget(), { documentElement: {
    setAttribute(key, value) { attrs.set(key, value); },
    getAttribute(key) { return attrs.get(key) || null; },
  } });
}

test('editor handoff keeps recipient scope, preserves drafts and never submits', async () => {
  const editor = { innerText:'', getClientRects:() => [1], focus() {} };
  const commands = [];
  let now=0;
  const context = vm.createContext({ location:{hostname:'message.bilibili.com',hash:'#/whisper/mid123456'}, Date:{now:()=>now}, setTimeout: (fn,ms) => { now+=ms; queueMicrotask(fn); return 1; }, document:{
    querySelector:() => editor,
    execCommand:(command,_,text) => { commands.push(command); editor.innerText+=command==='insertLineBreak'?'\n':text; return true; },
    createElement:() => ({ style:{}, setAttribute(){}, remove(){} }), body:{appendChild(){}},
  }});
  vm.runInContext(source('fill-editor.js'),context);
  const job={uid:'123456',text:'UID：123456\n领取码：TEST-TEST-TEST-TEST\n<不能被解析为HTML>'};
  assert.equal((await context.fillMofuWhisper(job)).ok,true);
  assert.equal(editor.innerText,job.text);
  assert.deepEqual(commands,['insertText','insertLineBreak','insertText','insertLineBreak','insertText']);
  editor.innerText='用户已有草稿';
  assert.equal((await context.fillMofuWhisper(job)).ok,false);
  assert.equal(editor.innerText,'用户已有草稿');
  editor.innerText=''; context.location.hash='#/whisper/mid610010';
  assert.equal((await context.fillMofuWhisper(job)).ok,false);
  assert.equal(editor.innerText,'');
  context.location.hash='#/whisper/mid123456';
  assert.equal((await context.fillMofuWhisper({...job,text:'字'.repeat(501)})).ok,false);
  assert.equal(commands.length,5);
});

test('extension accepts only the configured admin origins and validated jobs', async () => {
  let listener; const opened=[]; const forwarded=[];
  const chrome={runtime:{onMessageExternal:{addListener(fn){listener=fn;}}},tabs:{
    async create(options){opened.push(options);return {id:7};},
    async sendMessage(id,message){forwarded.push(message);return {ok:true};},
  }};
  vm.runInNewContext(source('background.js'),{chrome,URL,Set,Date,setTimeout});
  const sourcePage={url:'http://127.0.0.1:18081/admin/dashboard/voyage/delivery/'};
  const job={type:'fans:prefill',uid:'123456',text:'test'};
  assert.equal(listener(job,{url:'https://evil.invalid/admin/dashboard/voyage/'},()=>{}),false);
  assert.equal(listener(job,{url:'https://example.com/ask/'},()=>{}),false);
  let bad; listener({...job,uid:'123456&x=1'},sourcePage,r=>{bad=r;}); assert.equal(bad.ok,false);
  assert.equal(opened.length,0);
  const result=await new Promise(resolve=>listener(job,sourcePage,resolve));
  assert.equal(result.ok,true);
  assert.equal(opened[0].url,'https://message.bilibili.com/#/whisper/mid123456');
  assert.equal(forwarded[1].text,'test');
  assert.ok(!JSON.stringify(opened).includes('test'));
});

test('userscript consumes a one-time job and keeps the actual code out of the URL', async () => {
  const id='12345678-1234-4234-8234-123456789abc';
  const key='fans-whisper-job:'+id;
  const stored=new Map([[key,{uid:'123456',text:'secret-test-code',expires:Date.now()+60000}]]);
  let received, cleanedURL;
  const timers=[];
  const context={URL,Set,Date,document:mockDocument(),installMofuWhisperComposerStyle(){},installMofuWhisperPasteFix(){},GM_openInTab(){},location:{hostname:'message.bilibili.com',href:`https://message.bilibili.com/?fans_draft=${id}#/whisper/mid123456`},history:{state:null,replaceState(_,__,url){cleanedURL=url;}},
    GM_listValues:async()=>[...stored.keys()],GM_getValue:async key=>stored.get(key),GM_setValue:async(key,val)=>{stored.set(key,val);},GM_deleteValue:async key=>{stored.delete(key);},
    setTimeout:fn=>{timers.push(fn);},fillMofuWhisper:async job=>{received=job;return {ok:true};},
  };
  await vm.runInNewContext(source('userscript-bridge.js'),context);
  assert.equal(received.text,'secret-test-code');
  assert.equal(cleanedURL,'https://message.bilibili.com/#/whisper/mid123456');
  assert.equal(stored.get(key).result.ok,true);
  assert.equal(stored.get(key).text,undefined);
  for(const timer of timers)timer();
  await Promise.resolve(); assert.equal(stored.has(key),false);
});

test('Windows paste is normalized only for claim notices and keeps intentional blank lines', () => {
  const listeners = new Map(); const inserted=[];
  const editor = { focus() {} };
  const context={location:{hostname:'message.bilibili.com'},document:{
    addEventListener(type,fn){listeners.set(type,fn);},
    execCommand(command,_,text){inserted.push({command,text});return true;},
  }};
  vm.runInNewContext(source('fill-editor.js'),context);
  context.installMofuWhisperPasteFix();
  let prevented=0, stopped=0;
  const paste = text => listeners.get('paste')({target:{closest:()=>editor},clipboardData:{getData:()=>text},preventDefault(){prevented++;},stopImmediatePropagation(){stopped++;}});
  paste('B站 UID：123456\r\n领取码：TEST\r\n\r\n请保存');
  assert.equal(inserted.map(x=>x.command==='insertLineBreak'?'\n':x.text).join(''),'B站 UID：123456\n领取码：TEST\n\n请保存');
  paste('普通聊天\r\n下一行');
  paste('UID：123456\n领取码：TEST');
  assert.equal(inserted.length,6); assert.equal(prevented,1); assert.equal(stopped,1);
});

test('new DOM bridge works across distinct window wrappers even when cleanup fails', async () => {
  const document=mockDocument(); const replies=[];
  const window=new EventTarget(); window.postMessage=()=>{};
  const context={document,window,URL,Set,Date,CustomEvent,setTimeout,location:{hostname:'127.0.0.1',origin:'http://127.0.0.1:18081',pathname:'/admin/dashboard/voyage/delivery/'},
    GM_listValues:async()=>{throw new Error('cleanup failure');},GM_getValue:async()=>undefined,GM_setValue:async()=>{},GM_deleteValue:async()=>{},GM_openInTab(){},
  };
  await vm.runInNewContext(source('userscript-bridge.js'),context);
  document.addEventListener('fans-whisper-response-v2',e=>replies.push(JSON.parse(e.detail)));
  document.dispatchEvent(new CustomEvent('fans-whisper-request-v2',{detail:JSON.stringify({channel:'fans-whisper-request',id:'12345678-1234-4234-8234-123456789abc',type:'fans:ping'})}));
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(replies[0].ok,true); assert.equal(replies[0].version,JSON.parse(source('manifest.json')).version);
  assert.equal(JSON.parse(document.documentElement.getAttribute('data-fans-whisper-helper')).state,'ready');
  context.location.pathname='/ask/'; replies.length=0;
  document.dispatchEvent(new CustomEvent('fans-whisper-request-v2',{detail:JSON.stringify({channel:'fans-whisper-request',id:'12345678-1234-4234-8234-123456789abc',type:'fans:ping'})}));
  await Promise.resolve(); assert.equal(replies.length,0);
});

test('unavailable userscript APIs produce a visible diagnostic instead of silent startup failure', async () => {
  const document=mockDocument();
  await vm.runInNewContext(source('userscript-bridge.js'),{document,Set,location:{hostname:'localhost',origin:'http://localhost:18081'}});
  const status=JSON.parse(document.documentElement.getAttribute('data-fans-whisper-helper'));
  assert.equal(status.state,'error'); assert.match(status.error,/存储/);
});

for (const automatic of [false, true]) test(automatic ? 'gift first click connects and prefills without a separate manual detection; concurrent probes share one request' : 'admin client and sandbox bridge complete one handoff without using window identity', async () => {
  const ts = require('../admin/node_modules/typescript');
  const document=mockDocument(); const stored=new Map(); const opened=[];
  const window=new EventTarget(); window.setTimeout=setTimeout; window.postMessage=()=>{};
  const bridge={document,window,URL,Set,Date,CustomEvent,Promise,
    setTimeout(fn,ms){if(ms>=60000)return 0; return setTimeout(fn,0);},
    location:automatic ? {hostname:'preview.example.com',origin:'https://preview.example.com',pathname:'/admin/dashboard/gifts/'} : {hostname:'127.0.0.1',origin:'http://127.0.0.1:18081',pathname:'/admin/dashboard/voyage/delivery/'},
    GM_listValues:async()=>[],GM_getValue:async key=>stored.get(key),GM_setValue:async(key,value)=>stored.set(key,value),GM_deleteValue:async key=>stored.delete(key),
    GM_openInTab(url){opened.push(url); const key='fans-whisper-job:'+new URL(url).searchParams.get('fans_draft'); stored.set(key,{result:{ok:true}});},
  };
  await vm.runInNewContext(source('userscript-bridge.js'),bridge);
  const exports={};
  const client={exports,document,window,CustomEvent,crypto:require('node:crypto').webcrypto,clearTimeout,setTimeout,location:bridge.location};
  const code=fs.readFileSync(path.join(__dirname,'../admin/src/lib/whisper-helper.ts'),'utf8');
  vm.runInNewContext(ts.transpileModule(code,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,client);
  let pings = 0;
  document.addEventListener('fans-whisper-request-v2', event => { if (JSON.parse(event.detail).type === 'fans:ping') pings++; });
  if (automatic) {
    await Promise.all([
      exports.connectAndPrefillWhisper('123456', 'MofuFans\r\n礼物密码：offline-test'),
      exports.detectWhisperHelper(),
    ]);
    assert.equal(pings, 1);
  } else {
    assert.equal(await exports.detectWhisperHelper(),'userscript');
    await exports.prefillWhisper('userscript','123456','UID：123456\r\n领取码：offline-test');
  }
  assert.ok(exports.whisperHelperDetail().includes(JSON.parse(source('manifest.json')).version));
  assert.equal(opened.length,1); assert.equal(new URL(opened[0]).hash,'#/whisper/mid123456');
  assert.ok(!opened[0].includes('offline-test')); assert.equal(stored.size,0);
});
