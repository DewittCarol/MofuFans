export interface VoyageAddress {name:string;phone:string;address:string;note:string}
export interface VoyageDelivery {
	address_source?:string;long_term_saved_at?:string;long_term_changed?:boolean;
	long_term?:{enabled:boolean;version:number;matches_address:boolean;updated_at:string|null;entry_id?:number;source?:string};
	address_book?:{id:number;address:VoyageAddress;is_default:boolean;created_at:string;last_used_at:string;uses:{issue_id:number;title:string;note?:string;source:string;recorded_at:string}[]}[];
 can_edit_address?:boolean; items?:{label:string;status:string}[]; parcels?:{id:number;labels:string[];status:string;courier:string;tracking:string;note:string;shipped_at:string|null}[];
 labels?: string[]; collecting?: boolean;history?:{issue_title:string;address:VoyageAddress}[];address:VoyageAddress|null;courier:string;tracking:string;shipping_note:string;submitted_at:string|null;shipped_at:string|null;version:number}
export const blankAddress:VoyageAddress={name:'',phone:'',address:'',note:''};
export type VoyageLongTermState = NonNullable<VoyageDelivery['long_term']>;
export type VoyageAddressBookEntry = NonNullable<VoyageDelivery['address_book']>[number];

export function usesLongTermAddress(record:VoyageDelivery|null) {
 return !!(record?.long_term?.enabled&&record.long_term.matches_address);
}
export type ClaimAction='view'|'save'|'set_long_term'|'cancel_long_term'|'set_book_default';
export function claimPayload(record:VoyageDelivery|null,address:VoyageAddress,uid:string,code:string,action:ClaimAction,bookEntryID?:number) {
 const base={uid:uid.trim(),code,action,long_term_version:record?.long_term?.version||0};
 if(action==='save')return {...base,address,version:record?.version||0,long_term_action:'keep' as const};
 if(action==='set_long_term')return {...base,version:record?.version||0};
 if(action==='set_book_default')return {...base,book_entry_id:bookEntryID};
 return base;
}
export function memberClaimPayload(record:VoyageDelivery|null,address:VoyageAddress,expectedUID:string,action:ClaimAction,bookEntryID?:number) {
 const body:Record<string,unknown> = {...claimPayload(record,address,expectedUID,'',action,bookEntryID)};
 delete body.code;
 return body;
}
export function longTermSetDisabledReason(record:VoyageDelivery|null,dirty:boolean) {
 if(!record?.address)return '请先保存收货地址，再启用此选项。';
 if(dirty)return '上方地址有未保存的修改，请先保存，再启用或更新常用地址。';
 return '';
}

export function refreshClaimRecord(current:VoyageDelivery|null,fresh:VoyageDelivery,dirty:boolean):VoyageDelivery {
 if(!current||!dirty)return fresh;
 // Keep the address and consent versions the draft was based on. A logistics
 // refresh must not let an old draft overwrite a concurrent edit as a new one.
 return {...fresh,address:current.address,version:current.version,submitted_at:current.submitted_at,
  address_source:current.address_source,long_term_saved_at:current.long_term_saved_at,
  long_term_changed:current.long_term_changed,long_term:current.long_term};
}
