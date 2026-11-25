export interface VoyageAddress {name:string;phone:string;address:string;note:string}
export interface VoyageDelivery {address:VoyageAddress|null;courier:string;tracking:string;shipping_note:string;submitted_at:string|null;shipped_at:string|null;version:number;address_source?:string;long_term_saved_at?:string;long_term_changed?:boolean;long_term_origin?:string;has_history?:boolean}

export function inheritedAddressNotice(delivery?:Pick<VoyageDelivery,'address_source'|'long_term_changed'|'long_term_origin'>) {
 if(delivery?.address_source!=='long_term')return '';
 return delivery.long_term_changed?'常用地址已更新或停用，本次地址保持不变。发货前请与用户确认。':delivery.long_term_origin==='admin'?'本次已按用户委托使用常用地址，建议发送私信时与用户再次确认收货信息。':'本次已使用常用地址，建议发送私信时与用户再次确认收货信息。';
}
export function inheritedAddressMessage(delivery?:Pick<VoyageDelivery,'address_source'|'long_term_changed'>) {
 if(delivery?.address_source!=='long_term')return '';
 return delivery.long_term_changed?'本次仍保留此前填写的地址。常用地址已更新或停用，请在地址登记页核对本次收货信息。':'本次已使用你的常用收货地址，请核对收货信息。如需修改，请在地址登记页更新。';
}
export const blankAddress:VoyageAddress={name:'',phone:'',address:'',note:''};

export interface ShipmentRecipient {id:number;uid:string;name:string;tier:'captain'|'admiral'|'governor';disabled:boolean;delivery:VoyageDelivery}
