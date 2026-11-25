const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),vm=require('node:vm'),ts=require('typescript');
function load(name){const mod={exports:{}};vm.runInNewContext(ts.transpileModule(fs.readFileSync(`src/lib/${name}.ts`,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,{module:mod,exports:mod.exports,require:n=>n.startsWith('./')?load(n.slice(2)):require(n)});return mod.exports;}
const {recipientMessage}=load('voyage-recipients');
const {inheritedAddressNotice,inheritedAddressMessage}=load('voyage-delivery');
const issue={id:42,title:'十月舰礼'},person={uid:'12345',name:'测试舰长',tier:'captain',code:'Ab12Cd',batch_items:[]};
test('automatic reuse adds a confirmation message without exposing address fields',()=>{
 const delivery={address_source:'long_term',address:{name:'PRIVATE-NAME',phone:'PRIVATE-PHONE',address:'PRIVATE-ADDRESS'}};
 const message=recipientMessage({template:'{昵称}\n{UID}\n{认证码}\n{登记地址}',entry_url:''},issue,{...person,delivery});
 assert.match(message,/常用收货地址/);assert.match(message,/如需修改/);assert.match(message,/12345/);assert.match(message,/Ab12Cd/);
 assert.doesNotMatch(message,/PRIVATE/);assert.equal(message.split(inheritedAddressMessage(delivery)).length,2);
});
test('placeholder positions the reminder once; manual registration is unchanged',()=>{
 const template='{地址提示}\n{UID} {认证码}';
 const delivery={address_source:'long_term'};
 const message=recipientMessage({template,entry_url:''},issue,{...person,delivery});
 assert.ok(message.startsWith(inheritedAddressMessage(delivery)));assert.equal(message.split(inheritedAddressMessage(delivery)).length,2);
 assert.equal(recipientMessage({template:'{UID} {认证码}',entry_url:''},issue,person),'12345 Ab12Cd');
 assert.equal(inheritedAddressNotice({address_source:''}),'');
});
test('changed or revoked preference is never described as currently valid',()=>{
 const delivery={address_source:'long_term',long_term_changed:true};
 assert.match(inheritedAddressNotice(delivery),/更新或停用/);
 assert.match(inheritedAddressMessage(delivery),/此前/);
 assert.doesNotMatch(inheritedAddressNotice(delivery),/标记为长期有效/);
});
