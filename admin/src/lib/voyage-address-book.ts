import type {VoyageAddress} from './voyage-delivery';

export interface AddressBookUse {issue_id:number;title:string;note?:string;source:string;recorded_at:string}
export interface AddressBookEntry {id:number;address:VoyageAddress;is_default:boolean;created_at:string;last_used_at:string;uses:AddressBookUse[]}
export interface AddressBookPerson {uid:string;name:string;count:number;updated_at:string;enabled:boolean;source:string}
export interface AddressBookDetail {
 uid:string;entries:AddressBookEntry[];
 long_term:{enabled:boolean;version:number;entry_id:number;source:string;updated_at:string|null};
 collections:{issue_id:number;recipient_id:number;title:string;version:number}[];
 events:{id:number;entry_id:number;action:string;admin_id?:number;channel?:string;confirmed_at?:string;occurred_at:string}[];
}
export const consentActionLabels:Record<string,string>={user_set:'用户设置常用地址',user_clear:'用户关闭自动填写',admin_set:'主播开启礼物托管',admin_clear:'主播关闭自动填写',admin_confirm_once:'主播确认本次使用'};
export const addressUseLabels:Record<string,string>={user:'用户填写',long_term:'常用地址自动填写',admin_confirmed:'主播确认本次使用',admin:'主播确认'};
export function addressBookAction(detail:AddressBookDetail,action:'set_default'|'clear_default'|'confirm_once',entryID:number,recipientID:number,channel:string,confirmedAt:string){
 const collection=detail.collections.find(c=>c.recipient_id===recipientID);
 return {action,entry_id:entryID,version:detail.long_term.version,recipient_id:recipientID,delivery_version:collection?.version||0,confirmed:!!confirmedAt,confirmed_at:confirmedAt||undefined,channel};
}
