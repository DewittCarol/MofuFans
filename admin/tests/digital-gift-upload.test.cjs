const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const ts=require('../node_modules/typescript');
function setup(api,fetch){const module={exports:{}};vm.runInNewContext(ts.transpileModule(fs.readFileSync(require('node:path').join(__dirname,'../src/lib/digital-gift-upload.ts'),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,{module,exports:module.exports,require:()=>({giftAPI:api}),fetch,DOMException,setTimeout:fn=>setTimeout(fn,0)});return module.exports.uploadGiftFile}
function file(size){return {size,name:'large.bin',slice:(start,end)=>({size:end-start})}}
test('files over 25 MB use bounded chunks and complete once',async()=>{
 let received=0,complete=0;const chunks=[],progress=[];
 const upload=setup(async(path,method)=>{if(path.endsWith('/complete')){complete++;return {id:9,size:received}};return {token:'t',received:0}},async(url,options)=>{assert.equal(Number(options.headers['Upload-Offset']),received);assert.ok(options.body.size<=8*1024*1024);chunks.push(options.body.size);received+=options.body.size;return {ok:true,json:async()=>({code:200,data:{token:'t',received}})}});
 const result=await upload(1,file(26*1024*1024),n=>progress.push(n),new AbortController().signal);
 assert.equal(result.size,26*1024*1024);assert.equal(chunks.length,4);assert.equal(complete,1);assert.equal(progress.at(-1),result.size);
});
test('lost chunk response resumes from server offset, completion retries safely',async()=>{
 let received=0,chunkCalls=0,completions=0;
 const upload=setup(async(path,method)=>{if(path.endsWith('/complete')){if(++completions===1)throw Error('lost response');return {id:9}};return {token:'t',received}},async(url,options)=>{chunkCalls++;received+=options.body.size;throw Error('lost response')});
 const result=await upload(1,file(100),()=>{},new AbortController().signal);
 assert.equal(result.id,9);assert.equal(chunkCalls,1);assert.equal(completions,2);
});
test('temporary status failure allows another chunk attempt',async()=>{
 let attempt=0;
 const upload=setup(async(path,method)=>{if(method==='GET')throw Error('network');if(path.endsWith('/complete'))return {id:1};return {token:'t',received:0}},async()=>{if(++attempt===1)throw Error('network');return {ok:true,json:async()=>({code:200,data:{token:'t',received:100}})}});
 assert.equal((await upload(1,file(100),()=>{},new AbortController().signal)).id,1);assert.equal(attempt,2);
});
test('cancelling deletes partial upload without using aborted signal for cleanup',async()=>{
 const controller=new AbortController();let deleted=false;
 const upload=setup(async(path,method,body,signal)=>{if(method==='DELETE'){assert.equal(signal,undefined);deleted=true;return null}return {token:'t',received:0}},async()=>{throw Error('unexpected chunk')});
 await assert.rejects(upload(1,file(100),()=>controller.abort(),controller.signal),{name:'AbortError'});assert.ok(deleted);
});
