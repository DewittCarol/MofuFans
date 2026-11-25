const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');const ts=require('typescript');
const mod={exports:{}};const timers=[];vm.runInNewContext(ts.transpileModule(fs.readFileSync('src/lib/bilibili-profiles.ts','utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,{exports:mod.exports,module:mod,require:()=>({voyageRequest:()=>{throw Error('unexpected network')}}),setTimeout:(fn,ms)=>{timers.push({fn,ms});return timers.length;},Date,BigInt});
const {BilibiliProfileStore,profileName,validProfileUID,syncBilibiliProfiles}=mod.exports;
const tick=()=>new Promise(resolve=>setImmediate(resolve));
test('UID queue deduplicates across tiers and screens while remaining sequential',async()=>{
 let running=0,max=0;const calls=[];const store=new BilibiliProfileStore(async uid=>{calls.push(uid);max=Math.max(max,++running);await tick();running--;return{uid,name:'昵称'+uid,avatar:'/a.png',error:'',checked_at:new Date().toISOString()};});
 store.ensure(['10001','10001','10002']);store.ensure(['10001','10002']);for(let i=0;i<5;i++)await tick();
 assert.equal(max,1);assert.deepEqual(calls,['10001','10002']);assert.equal(store.profiles['10001'].status,'ready');store.ensure(['10001']);await tick();assert.equal(calls.length,2);
});
test('upstream cooldown pauses other users; explicit names remain separate from fetched names',async()=>{
 let calls=0;const now=Date.now();const store=new BilibiliProfileStore(async uid=>{calls++;return{uid,name:'已有昵称',avatar:'/a.png',error:'暂不可用',retry_at:new Date(now+300000).toISOString(),pause_until:new Date(now+60000).toISOString()};},()=>now);
 store.ensure(['10001','10002']);await tick();assert.equal(calls,1);assert.equal(store.profiles['10002'].status,'queued');store.ensure(['10001']);await tick();assert.equal(calls,1);
 assert.equal(profileName('10001','手动备注',store.profiles['10001']),'手动备注');assert.equal(profileName('10001','',store.profiles['10001']),'已有昵称');assert.ok(timers.some(t=>t.ms===60000));
});
test('large UIDs remain strings and invalid intermediate values are never fetched',async()=>{
 const calls=[];const store=new BilibiliProfileStore(async uid=>{calls.push(uid);return{uid,name:'用户',avatar:'/a.png',error:'',checked_at:new Date().toISOString()};});
 store.ensure(['0','001','123x','9223372036854775808','9007199254740993']);await tick();assert.deepEqual(calls,['9007199254740993']);assert.equal(validProfileUID('9223372036854775807'),true);
});

test('seven-day cache expires on time and batch reads show cached users before a slow update',async()=>{
 let now=Date.now(),calls=0;
 const store=new BilibiliProfileStore(async uid=>{calls++;return{uid,name:'昵称',avatar:'/a.png',error:'',checked_at:new Date(now).toISOString()}},()=>now);
 store.ensure(['10001']);await tick();now+=6*86400000;store.ensure(['10001']);await tick();assert.equal(calls,1);
 now+=2*86400000;store.ensure(['10001']);await tick();assert.equal(calls,2);
 const seen=[];let release;
 const bulk=new BilibiliProfileStore(async uid=>{seen.push(uid);if(uid==='10002')await new Promise(r=>release=r);return{uid,name:'新昵称',avatar:'/new.png',error:'',checked_at:new Date(now).toISOString()}},()=>now,async()=>[
  {uid:'10001',name:'已有昵称',avatar:'/cached.png',error:'',checked_at:new Date(now-6*86400000).toISOString()},
  {uid:'10002',name:'旧昵称',avatar:'/old.png',error:'',checked_at:new Date(now-8*86400000).toISOString()},
 ]);
 bulk.ensure(['10001','10002','10003']);await tick();assert.equal(bulk.profiles['10001'].status,'ready');assert.equal(bulk.profiles['10002'].name,'旧昵称');assert.deepEqual(seen,['10002']);
 release();for(let i=0;i<4;i++)await tick();assert.deepEqual(seen,['10002','10003']);assert.equal(bulk.profiles['10002'].name,'新昵称');
});


test('manual sync bypasses daily cache, updates both fields and reuses the latest minute',async()=>{
 let now=Date.now();const calls=[];
 const store=new BilibiliProfileStore(async(uid,refresh)=>{calls.push({uid,refresh});return{uid,name:refresh?'新昵称':'原昵称',avatar:refresh?'/new.png':'/old.png',error:'',checked_at:new Date(now).toISOString()};},()=>now);
 store.ensure(['10001']);await tick();now+=120000;
 store.ensure(['10001']);await tick();assert.equal(calls.length,1);
 const profile=await store.refresh('10001');assert.equal(profile.name,'新昵称');assert.equal(profile.avatar,'/new.png');
 assert.equal(calls.length,2);assert.equal(calls[1].refresh,true);
 await store.refresh('10001');assert.equal(calls.length,2);
 assert.equal(profileName('10001','手填名字',profile),'手填名字');
});

test('manual and automatic requests stay serialized and a queued UID is refreshed only once',async()=>{
 let release;let active=0,max=0;const calls=[],now=Date.now();
 const store=new BilibiliProfileStore(async(uid,refresh)=>{
  calls.push([uid,refresh]);max=Math.max(max,++active);
  if(uid==='10001')await new Promise(resolve=>{release=resolve});
  active--;return{uid,name:'用户',avatar:'/a.png',error:'',checked_at:new Date(now).toISOString()};
 },()=>now);
 store.ensure(['10001','10002','10003']);
 const a=store.refresh('10003'),b=store.refresh('10003');release();
 await Promise.all([a,b]);await tick();
 assert.equal(max,1);assert.deepEqual(calls,[['10001',false],['10003',true],['10002',false]]);
});

test('manual sync joins an in-flight cached lookup then refreshes its stale result',async()=>{
 let release;const now=Date.now(),calls=[];
 const store=new BilibiliProfileStore(async(uid,refresh)=>{
  calls.push(refresh);if(!refresh)await new Promise(resolve=>{release=resolve});
  return{uid,name:refresh?'新昵称':'原昵称',avatar:'/a.png',error:'',checked_at:new Date(now-(refresh?0:120000)).toISOString()};
 },()=>now);
 store.ensure(['10001']);const a=store.refresh('10001'),b=store.refresh('10001');release();
 const results=await Promise.all([a,b]);assert.deepEqual(calls,[false,true]);assert.ok(results.every(p=>p.name==='新昵称'));
});

test('cooldown also releases queued manual work without a delayed forced request',async()=>{
 let release;const now=Date.now(),calls=[];
 const store=new BilibiliProfileStore(async(uid,refresh)=>{
  calls.push([uid,refresh]);await new Promise(resolve=>{release=resolve});
  return{uid,name:'原昵称',avatar:'/old.png',error:'暂不可用',retry_at:new Date(now+300000).toISOString(),pause_until:new Date(now+60000).toISOString()};
 },()=>now);
 store.ensure(['10001','10002']);const manual=store.refresh('10002');release();
 const result=await manual;assert.equal(result.status,'error');assert.ok(result.pause_until);
 await store.refresh('10001');await store.refresh('10003');assert.equal(calls.length,1);
 assert.equal(store.profiles['10001'].avatar,'/old.png');assert.equal(store.profiles['10002'].status,'error');
});

test('batch sync covers the full unique UID list and stops after the active user',async()=>{
 const calls=[],progress=[];let stop=false;
 const result=await syncBilibiliProfiles(['10001','10001','10002','invalid','10003'],{
  refresh:async uid=>{calls.push(uid);if(uid==='10002')stop=true;return{uid,name:'用户',avatar:'/a.png',error:''}},
  isStopped:()=>stop,onProgress:value=>progress.push(value),
 });
 assert.deepEqual(calls,['10001','10002']);assert.equal(result.total,3);assert.equal(result.completed,2);assert.equal(result.stopped,true);
 assert.equal(progress[0].completed,0);assert.equal(progress.at(-1).completed,2);
});

test('batch keeps successful profiles, counts partial errors and pauses on upstream restriction',async()=>{
 const calls=[];
 const result=await syncBilibiliProfiles(['10001','10002','10003','10004'],{
  refresh:async uid=>{calls.push(uid);return{uid,name:'用户',avatar:'/a.png',error:uid==='10001'?'':'稍后重试',...(uid==='10003'?{pause_until:new Date(Date.now()+60000).toISOString()}: {})}},
  isStopped:()=>false,onProgress:()=>{},
 });
 assert.deepEqual(calls,['10001','10002','10003']);assert.equal(result.succeeded,1);assert.equal(result.failed,2);assert.equal(result.paused,true);assert.equal(result.total-result.completed,1);
});
