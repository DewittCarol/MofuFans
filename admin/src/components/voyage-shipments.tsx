"use client";
import {useState} from 'react';
import {Copy,Check} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {voyageField,type VoyageRecipient} from '@/lib/voyage-recipients';
import type {VoyageDelivery} from '@/lib/voyage-delivery';
export type DeliveryRow=VoyageRecipient & {delivery:VoyageDelivery};
export type ShippingAction='draft'|'ship'|'unship';
export type ShippingItem={id:number;recipient_version:number;version:number;courier:string;tracking:string;shipping_note:string};
export function shippingItem(row:DeliveryRow):ShippingItem{return {id:row.id,recipient_version:row.version,version:row.delivery.version,courier:row.delivery.courier,tracking:row.delivery.tracking,shipping_note:row.delivery.shipping_note};}

export function ShipmentAddress({delivery}:{delivery:VoyageDelivery}) {
 const a=delivery.address;
 const [copied,setCopied]=useState(false),[copying,setCopying]=useState(false),[copyError,setCopyError]=useState('');
 const text=a?`姓名：${a.name}\n手机号码：${a.phone}\n地址：${a.address}${a.note?`\n备注：${a.note}`:''}`:'';
 const copy=async()=>{setCopying(true);setCopyError('');try{if(!navigator.clipboard?.writeText)throw new Error();await navigator.clipboard.writeText(text);setCopied(true);}catch{setCopyError('未能自动复制，请选中下方文字手动复制。');}finally{setCopying(false);}};
 return <section aria-label="收货地址" className="space-y-2 text-sm leading-6"><div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-medium">收货地址</h3>{a&&<Button type="button" size="sm" variant="outline" disabled={copying} onClick={()=>void copy()}>{copied?<Check size={14}/>:<Copy size={14}/>}<span role="status">{copied?'已复制':'一键复制'}</span></Button>}</div>{a?<dl className="space-y-1.5"><div className="flex"><dt className="shrink-0">姓名：</dt><dd className="break-words">{a.name}</dd></div><div className="flex"><dt className="shrink-0">手机号码：</dt><dd className="break-all">{a.phone}</dd></div><div className="flex"><dt className="shrink-0">地址：</dt><dd className="whitespace-pre-wrap break-words">{a.address}</dd></div>{a.note&&<div className="flex text-muted-foreground"><dt className="shrink-0">备注：</dt><dd className="whitespace-pre-wrap break-words">{a.note}</dd></div>}</dl>:<p className="text-muted-foreground">用户尚未填写地址，可先保存运单。</p>}{copyError&&<p role="alert" className="text-xs text-destructive">{copyError}</p>}</section>;
}

export function ShipmentEditor({row,busy,onSave}:{row:DeliveryRow;busy:boolean;onSave:(items:ShippingItem[],action:ShippingAction)=>Promise<boolean>}){
 const [courier,setCourier]=useState(row.delivery.courier),[tracking,setTracking]=useState(row.delivery.tracking),[note,setNote]=useState(row.delivery.shipping_note);
 const [error,setError]=useState('');
 const save=async()=>{setError('');if(!courier.trim()||!tracking.trim()){setError('请填写快递公司和运单号');return;}await onSave([{...shippingItem(row),courier,tracking,shipping_note:note}],'draft');};
 return <div className="mt-4 grid w-full gap-6 rounded-xl bg-secondary/30 p-4 lg:grid-cols-2">
  <ShipmentAddress delivery={row.delivery}/>
  <form noValidate className="space-y-4" onSubmit={e=>{e.preventDefault();void save();}}><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor={`courier-${row.id}`}>快递公司</Label><Input id={`courier-${row.id}`} maxLength={40} value={courier} onChange={e=>setCourier(e.target.value)} disabled={busy}/></div><div className="space-y-2"><Label htmlFor={`tracking-${row.id}`}>运单号</Label><Input id={`tracking-${row.id}`} maxLength={100} value={tracking} onChange={e=>setTracking(e.target.value)} disabled={busy}/></div></div>
   <div className="space-y-2"><Label htmlFor={`shipping-note-${row.id}`}>物流备注（选填）</Label><Input id={`shipping-note-${row.id}`} maxLength={300} value={note} onChange={e=>setNote(e.target.value)} disabled={busy}/></div>
   <Button type="submit" disabled={busy}>{busy?'正在保存…':'保存运单'}</Button>{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
  </form>
 </div>;
}

// Read CSV quoting (including embedded commas/newlines) and Excel clipboard TSV.
export function parseTable(text:string):string[][]{
 const input=text.replace(/^\uFEFF/,'').replace(/\r\n?/g,'\n');
 const separator=input.split('\n')[0].includes('\t')?'\t':',';
 const table:string[][]=[];let row:string[]=[],field='',quoted=false;
 for(let i=0;i<input.length;i++){
  const c=input[i];
  if(c==='"'){if(quoted&&input[i+1]==='"'){field+='"';i++;}else if(!quoted&&field.trim()!==''){throw new Error('CSV 引号格式不正确');}else quoted=!quoted;}
  else if(!quoted&&(c===separator||c==='\n')){row.push(field.trim());field='';if(c==='\n'){if(row.some(Boolean))table.push(row);row=[];}}
  else field+=c;
 }
 if(quoted)throw new Error('CSV 引号没有闭合');
 row.push(field.trim());if(row.some(Boolean))table.push(row);return table;
}
function importRows(text:string,rows:DeliveryRow[]):{items:ShippingItem[];uids:string[]}{
 const table=parseTable(text);if(!table.length)throw new Error('请选择 CSV 或粘贴表格');
 const headers=table[0].map(h=>h.toLowerCase().replace(/\s/g,''));
 let uidCol=0,courierCol=1,trackingCol=2,noteCol=3;
 const locate=(names:string[])=>headers.findIndex(h=>names.includes(h));
 const headerUID=locate(['uid','用户uid','b站uid']);
 if(headerUID>=0){uidCol=headerUID;courierCol=locate(['快递公司','物流公司','courier']);trackingCol=locate(['运单号','快递单号','物流单号','tracking']);noteCol=locate(['备注','物流备注','shipping_note']);table.shift();if(courierCol<0||trackingCol<0)throw new Error('表头需要 UID、快递公司、运单号三列');}
 if(!table.length||table.length>500)throw new Error('每批可导入 1–500 行');
 const seen=new Set<string>(),items:ShippingItem[]=[],uids:string[]=[];
 for(let i=0;i<table.length;i++){
  const cells=table[i],uid=cells[uidCol]||'',courier=cells[courierCol]||'',tracking=cells[trackingCol]||'';
  const fail=(reason:string)=>{throw new Error(`第 ${i+(headerUID>=0?2:1)} 行：${reason}`);};
  if(!/^[1-9][0-9]{0,18}$/.test(uid))fail('UID 需为完整数字，不能使用科学计数法');
  if(seen.has(uid))fail(`UID ${uid} 重复`);seen.add(uid);
  const person=rows.find(r=>r.uid===uid);if(!person)fail(`UID ${uid} 不在本期名单中`);
  if(person!.disabled||!person!.sent_at)fail(`UID ${uid} 已停用或尚未发私信私信`);
  if(!courier||!/^[- A-Za-z0-9]{3,100}$/.test(tracking))fail('快递公司或运单号无效');
  items.push({...shippingItem(person!),courier,tracking,shipping_note:noteCol>=0?cells[noteCol]||'':''});uids.push(uid);
 }
 return {items,uids};
}
export function ShipmentImport({rows,busy,onSave}:{rows:DeliveryRow[];busy:boolean;onSave:(items:ShippingItem[],action:ShippingAction)=>Promise<boolean>}){
 const [text,setText]=useState(''),[error,setError]=useState('');
 const [preview,setPreview]=useState<{items:ShippingItem[];uids:string[]}|null>(null);
 const previewRows=()=>{setError('');try{setPreview(importRows(text,rows));}catch(e){setPreview(null);setError(e instanceof Error?e.message:'表格无法读取');}};
 const submit=async(action:ShippingAction)=>{if(preview&&await onSave(preview.items,action)){setPreview(null);setText('');}};
 return <details className="mb-5 rounded-xl border border-border bg-card p-4"><summary className="cursor-pointer text-sm font-medium">批量导入发货结果</summary><div className="mt-4 space-y-4">
  <p className="text-xs leading-5 text-muted-foreground">选择 UTF-8 CSV 或粘贴 Excel 表格，列为 UID、快递公司、运单号、备注（选填）。单号请设为文本格式。<a className="ml-2 text-primary underline underline-offset-4" href="/admin/voyage-shipping-template.csv" download>下载模板</a></p>
  <Input type="file" accept=".csv,.tsv,.txt" aria-label="导入发货结果文件" disabled={busy} onChange={async e=>{const file=e.target.files?.[0];setPreview(null);setError('');if(!file)return;if(file.size>1024*1024){setError('文件最多 1 MB');return;}try{const data=await file.arrayBuffer();setText(new TextDecoder('utf-8',{fatal:true}).decode(data));}catch{setError('无法读取文件，请另存为 CSV UTF-8 后重试');}}}/>
  <textarea className={voyageField} rows={5} value={text} disabled={busy} aria-label="粘贴运单表格" placeholder="UID    快递公司    运单号    备注" onChange={e=>{setText(e.target.value);setPreview(null);}}/>
  <Button variant="outline" disabled={busy||!text.trim()} onClick={previewRows}>预览匹配结果</Button>
  {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
  {preview&&<div className="space-y-3"><p className="text-sm">匹配 {preview.items.length} 人；已有运单将被本次内容更新。</p><div className="max-h-64 overflow-auto rounded-lg border border-border"><table className="w-full text-left text-sm"><thead className="bg-secondary"><tr><th className="p-2">UID</th><th className="p-2">快递公司</th><th className="p-2">运单号</th></tr></thead><tbody>{preview.items.map((item,i)=><tr key={item.id} className="border-t border-border"><td className="p-2">{preview.uids[i]}</td><td className="p-2">{item.courier}</td><td className="break-all p-2">{item.tracking}</td></tr>)}</tbody></table></div><div className="flex flex-wrap gap-2"><Button variant="outline" disabled={busy} onClick={()=>void submit('draft')}>仅保存运单</Button><Button disabled={busy} onClick={()=>void submit('ship')}>导入并确认发货（{preview.items.length} 人）</Button></div></div>}
 </div></details>;
}
