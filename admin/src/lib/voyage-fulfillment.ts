import type {VoyageAddress} from './voyage-delivery';
export interface ParcelItem {key:string;roster_id:number;tier:string;label:string;recipient_id:number;collection_id:number}
export interface AddressChoice {recipient_id:number;collection:string;version:number;address:VoyageAddress|null;address_source?:string;long_term_changed?:boolean;long_term_origin?:string}
export interface Parcel {id:number;plan_id:number;uid:string;name:string;items:ParcelItem[];address:VoyageAddress|null;choices:AddressChoice[];address_recipient_id:number;address_version:number;confirmed_at:string|null;exported_at:string|null;shipped_at:string|null;cancelled_at:string|null;courier:string;tracking:string;note:string;version:number;address_changed:boolean;block_reason:string;has_history?:boolean}
export interface Plan {total:number;shipped:number;cancelled:number;id:number;title:string;version:number}
export interface PendingGroup {collection_id:number;roster_id:number;tier:string;label:string;collection:string;category:string;month:string;people:number;addressed:number}
export interface ParcelChange {id:number;version:number;address_recipient_id?:number;address_version?:number;courier?:string;tracking?:string;note?:string}
export const selectionKey=(g:PendingGroup)=>`${g.collection_id}/${g.roster_id}/${g.tier}`;
export function parcelState(p:Parcel) {
 if(p.cancelled_at)return 'cancelled';if(p.shipped_at)return 'shipped';if(p.block_reason)return 'blocked';if(p.address_changed)return 'changed';
 if(!p.address&&!uniqueAddress(p))return p.choices.some(a=>a.address)?'confirm':'missing';return p.tracking?'ready':'packing';
}
export const parcelStateLabels:Record<string,string>={cancelled:'已取消',shipped:'已寄出',blocked:'需核对资格',changed:'地址有更新',confirm:'需选择地址',missing:'待补地址',ready:'待确认寄出',packing:'待发货'};
export function uniqueAddress(p:Parcel):AddressChoice|undefined {
 const choices=p.choices.filter(a=>a.address);if(!choices.length)return;
 const keys=new Set(choices.map(a=>JSON.stringify(a.address)));return keys.size===1?choices[0]:undefined;
}
export const parcelChange=(p:Parcel):ParcelChange=>({id:p.id,version:p.version,courier:p.courier,tracking:p.tracking,note:p.note,...(!p.address&&uniqueAddress(p)?{address_recipient_id:uniqueAddress(p)!.recipient_id,address_version:uniqueAddress(p)!.version}:{})});

export function parcelMatchesTab(p:Parcel,tab:string){return tab==='all'||tab==='shipped'&&!!p.shipped_at||tab==='pending'&&!p.shipped_at&&!p.cancelled_at;}

export const parcelExceptionOptions = [
 {id:'all',label:'不限异常'},
 {id:'attention',label:'所有异常'},
 {id:'missing',label:'缺地址'},
  {id:'long_term_changed',label:'常用地址设置已变更'},
 {id:'changed',label:'地址有更新'},
 {id:'confirm',label:'需选择地址'},
 {id:'blocked',label:'需核对资格'},
] as const;
export type ParcelExceptionFilter = typeof parcelExceptionOptions[number]['id'];
type ParcelException = Exclude<ParcelExceptionFilter,'all'|'attention'>;
export interface ParcelFilters {status:string;query:string;exception:ParcelExceptionFilter}

export function parcelExceptions(p:Parcel):ParcelException[] {
 if(p.shipped_at||p.cancelled_at)return [];
 const issues:ParcelException[]=[];
 const available=p.choices.filter(choice=>choice.address);
 const automatic=!p.address?uniqueAddress(p):undefined;
 // A confirmed package only needs review of its chosen source, not every alternative.
 const relevant=p.address?p.choices.filter(choice=>choice.recipient_id===p.address_recipient_id):automatic?[automatic]:available;
 if(!p.address&&!available.length)issues.push('missing');
 if(relevant.some(choice=>choice.address_source==='long_term'&&choice.long_term_changed))issues.push('long_term_changed');
 if(p.address_changed)issues.push('changed');
 if(!p.address&&!automatic&&available.length)issues.push('confirm');
 if(p.block_reason)issues.push('blocked');
 return issues;
}

export function parcelFilterView(rows:Parcel[],filters:ParcelFilters,displayName:(parcel:Parcel)=>string=parcel=>parcel.name) {
 const query=filters.query.trim();
 const searched=rows.filter(p=>`${p.uid} ${displayName(p)} ${p.items.map(item=>item.label).join(' ')}`.includes(query));
 const issues=new Map(searched.map(p=>[p.id,parcelExceptions(p)]));
 const matches=(p:Parcel,filter:ParcelExceptionFilter)=>filter==='all'||(filter==='attention'?issues.get(p.id)!.length>0:issues.get(p.id)!.includes(filter));
 const scoped=searched.filter(p=>parcelMatchesTab(p,filters.status));
 const exceptionCounts=Object.fromEntries(parcelExceptionOptions.map(({id})=>[id,scoped.filter(p=>matches(p,id)).length])) as Record<ParcelExceptionFilter,number>;
 const matching=searched.filter(p=>matches(p,filters.exception));
 const tabCounts=Object.fromEntries(['pending','shipped','all'].map(tab=>[tab,matching.filter(p=>parcelMatchesTab(p,tab)).length]));
 return {filtered:scoped.filter(p=>matches(p,filters.exception)),exceptionCounts,tabCounts};
}
