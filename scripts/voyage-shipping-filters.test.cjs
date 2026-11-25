const {test} = require('node:test');
const assert = require('node:assert/strict');
// The deployed Node 22.23+ toolchain can load this type-only TypeScript module directly.
const {parcelExceptions,parcelFilterView,parcelState,parcelChange} = require('../admin/src/lib/voyage-fulfillment.ts');
const plain = value => JSON.parse(JSON.stringify(value));
const address = {name:'离线收件人',phone:'00000000000',address:'离线地址甲',note:''};
const alternate = {...address,address:'离线地址乙'};
const choice = (id,extra={}) => ({recipient_id:id,collection:'离线收集',version:1,address:{...address},...extra});
const inherited = (id,extra={}) => choice(id,{address_source:'long_term',long_term_changed:true,...extra});
const parcel = (id,extra={}) => ({id,plan_id:1,uid:String(800000+id),name:'离线包裹',items:[{label:'十月舰礼'}],address:null,choices:[],address_recipient_id:0,address_version:0,confirmed_at:null,exported_at:null,shipped_at:null,cancelled_at:null,courier:'',tracking:'',note:'',version:1,address_changed:false,block_reason:'',...extra});
const view = (rows,extra={},name) => parcelFilterView(rows,{status:'pending',query:'',exception:'all',...extra},name);

test('缺地址与冲突地址分别筛选；唯一地址和内容相同的多个地址无需处理',()=>{
 assert.deepEqual(plain(parcelExceptions(parcel(1))),['missing']);
 assert.deepEqual(plain(parcelExceptions(parcel(2,{choices:[choice(1)]}))),[]);
 assert.deepEqual(plain(parcelExceptions(parcel(3,{choices:[choice(1),choice(2)]}))),[]);
 assert.deepEqual(plain(parcelExceptions(parcel(4,{choices:[choice(1),choice(2,{address:alternate})]}))),['confirm']);
});

test('已寄出和已取消记录始终排除在全部待处理异常之外',()=>{
 for(const closed of [{shipped_at:'2026-10-25'},{cancelled_at:'2026-10-25'}]){
  const row=parcel(1,{...closed,address_changed:true,block_reason:'资格已停用',choices:[inherited(1)]});
  assert.deepEqual(plain(parcelExceptions(row)),[]);
  assert.equal(view([row],{status:'all',exception:'attention'}).filtered.length,0);
  assert.equal(view([row],{status:'all'}).filtered.length,1);
 }
});

test('长期变化只核对已采用的来源，不误报未选中的其他收集',()=>{
 const row=parcel(1,{address:{...address},address_recipient_id:1,choices:[choice(1,{address_source:'manual'}),inherited(2,{address:alternate})]});
 assert.deepEqual(plain(parcelExceptions(row)),[]);
 assert.equal(view([row],{exception:'long_term_changed'}).filtered.length,0);
 const used={...row,address_recipient_id:2,address:alternate};
 assert.deepEqual(plain(parcelExceptions(used)),['long_term_changed']);
});

test('长期标记与实际来源同时匹配，用户已重新手填的不继续误报',()=>{
 for(const source of ['manual',undefined]){
  const row=parcel(1,{choices:[choice(1,{address_source:source,long_term_changed:true})]});
  assert.deepEqual(plain(parcelExceptions(row)),[]);
 }
 assert.deepEqual(plain(parcelExceptions(parcel(2,{choices:[inherited(1,{long_term_changed:false})]}))),[]);
});

test('尚未选定冲突地址时保留长期变化提醒；唯一自动来源与现行发货规则一致',()=>{
 assert.deepEqual(plain(parcelExceptions(parcel(1,{choices:[choice(1),inherited(2,{address:alternate})]}))),['long_term_changed','confirm']);
 assert.deepEqual(plain(parcelExceptions(parcel(2,{choices:[inherited(1),choice(2)]}))),['long_term_changed']);
 assert.deepEqual(plain(parcelExceptions(parcel(3,{choices:[choice(1),inherited(2)]}))),[]);
});

test('资格问题不会隐藏同时存在的缺地址问题，全部异常按包裹去重',()=>{
 const row=parcel(1,{block_reason:'资格已停用'});
 assert.deepEqual(plain(parcelExceptions(row)),['missing','blocked']);
 const result=view([row],{exception:'attention'});
 assert.equal(result.filtered.length,1);
 assert.equal(result.exceptionCounts.attention,1);
 assert.equal(result.exceptionCounts.missing,1);
 assert.equal(result.exceptionCounts.blocked,1);
 assert.equal(result.exceptionCounts.all,1);
});

test('地址版本更新与长期设置变化是独立提醒，不更改发货状态或提交内容',()=>{
 const row=parcel(1,{address:{...address},address_recipient_id:1,address_version:1,address_changed:true,choices:[inherited(1,{version:2})]});
 const before=JSON.stringify(row),state=parcelState(row),change=plain(parcelChange(row));
 assert.deepEqual(plain(parcelExceptions(row)),['long_term_changed','changed']);
 view([row],{exception:'attention'});
 assert.equal(JSON.stringify(row),before);
 assert.equal(parcelState(row),state);
 assert.deepEqual(plain(parcelChange(row)),change);
});

test('搜索、状态、异常取交集；选项数量使用另两项条件，不使用当前页数量',()=>{
 const rows=[
  parcel(1,{name:'白桃'}),
  parcel(2,{name:'白桃',choices:[inherited(2)]}),
  parcel(3,{name:'白桃',shipped_at:'2026-10-25'}),
  parcel(4,{name:'白桃',cancelled_at:'2026-10-25'}),
  parcel(5,{name:'山莓'}),
 ];
 const result=view(rows,{query:' 白桃 ',exception:'missing'});
 assert.deepEqual(plain(result.filtered.map(p=>p.id)),[1]);
 assert.deepEqual(plain(result.tabCounts),{pending:1,shipped:0,all:1});
 assert.equal(result.exceptionCounts.all,2);
 assert.equal(result.exceptionCounts.attention,2);
 assert.equal(result.exceptionCounts.missing,1);
 assert.equal(result.exceptionCounts.long_term_changed,1);
 assert.equal(view(rows,{status:'all',query:'白桃'}).exceptionCounts.all,4);
});

test('已发货与异常筛选组合为空，清除异常恢复已寄记录且计数一致',()=>{
 const rows=[parcel(1),parcel(2,{shipped_at:'2026-10-25'})];
 const empty=view(rows,{status:'shipped',exception:'missing'});
 assert.equal(empty.filtered.length,0);
 assert.equal(empty.exceptionCounts.missing,0);
 assert.equal(empty.exceptionCounts.all,1);
 assert.equal(empty.tabCounts.pending,1);
 assert.deepEqual(plain(view(rows,{status:'shipped'}).filtered.map(p=>p.id)),[2]);
});

test('搜索保留 UID、个人资料显示名和礼物内容，未加载昵称也可用原名',()=>{
 const rows=[parcel(1,{name:'原名字'}),parcel(2,{items:[{label:'活动奖品'}]})];
 assert.deepEqual(plain(view(rows,{query:'800001'}).filtered.map(p=>p.id)),[1]);
 assert.deepEqual(plain(view(rows,{query:'原名字'}).filtered.map(p=>p.id)),[1]);
 assert.deepEqual(plain(view(rows,{query:'资料名字'},p=>p.id===1?'资料名字':p.name).filtered.map(p=>p.id)),[1]);
 assert.deepEqual(plain(view(rows,{query:'活动奖品'}).filtered.map(p=>p.id)),[2]);
});

test('大于一页时异常数量覆盖完整结果，零结果保持全部选项为零',()=>{
 const rows=Array.from({length:31},(_,i)=>parcel(i+1));
 assert.equal(view(rows,{exception:'missing'}).exceptionCounts.missing,31);
 assert.equal(view(rows,{exception:'missing'}).filtered.length,31);
 const empty=view(rows,{query:'不存在'});
 assert.equal(empty.filtered.length,0);
 assert.ok(Object.values(empty.exceptionCounts).every(count=>count===0));
 assert.ok(Object.values(empty.tabCounts).every(count=>count===0));
});
