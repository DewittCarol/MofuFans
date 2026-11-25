import { voyageTiers, type VoyageIssue, type VoyageTier } from './voyage';
import {inheritedAddressMessage,type VoyageDelivery} from './voyage-delivery';

export interface VoyageRecipient {
  message?:{status:string;note:string};
  delivery?:VoyageDelivery;
  batch_items?: {key:string;roster_id:number;tier:string;label:string}[];
  id: number; issue_id: number; uid: string; tier: VoyageTier | ''; name: string; avatar: string;
  profile_checked_at: string | null; profile_error: string; disabled: boolean;
  prepared_at: string | null; sent_at: string | null; version: number;
  code?: string; code_error?: boolean;
}
export interface DispatchConfig { issue_id: number; template: string; entry_url: string; version: number }
export interface DispatchData { config: DispatchConfig; recipients: VoyageRecipient[] }
export interface ImportPreview {
  added: number; skipped: number; conflicts: number;
  rows: { line: number; uid: string; tier: VoyageTier | ''; status: 'new' | 'existing' | 'duplicate' | 'conflict'; previous_tier?: VoyageTier | '' }[];
}
export function whisperURL(uid: string) { return `https://message.bilibili.com/#/whisper/mid${uid}`; }
export function voyageDispatchBlockReason(published:boolean, row:VoyageRecipient, dirty:boolean, busy:boolean, messageLength:number) {
  if(busy)return '正在处理，请稍候';
  if(!published)return '先发布这个批次，用户才能打开登记入口';
  if(dirty)return '私信模板有修改，请先保存';
  if(row.disabled)return '这个用户已移出名单';
  if(row.code_error||!row.code)return '认证码未读取成功，请刷新名单后重试';
  if(messageLength>500)return '私信超过 500 字，请缩短模板';
  return '';
}
export function recipientMessage(config: DispatchConfig, issue: VoyageIssue, recipient: VoyageRecipient, streamerName = '主播') {
  const addressNotice=inheritedAddressMessage(recipient.delivery);
  const values: Record<string, string> = {
    '主播': streamerName,
    '地址提示':addressNotice,
    '昵称': recipient.name || '小伙伴', 'UID': recipient.uid,
    '档位': voyageTiers.find(tier => tier.id === recipient.tier)?.name || '',
    '期次': issue.title, '批次名称': issue.title, '本次礼物': (recipient.batch_items || []).map(item=>item.label).join('、'), '本次期次': (recipient.batch_items || []).map(item=>item.label).join('、'), '认证码': recipient.code || '', '领取码': recipient.code || '',
    '登记地址': config.entry_url ? `本期登记入口：${config.entry_url}` : `本期登记入口：${typeof location === 'undefined' ? '' : location.origin}/voyage/collections/?batch=${issue.id}`,
  };
  let template = config.template.replace(/\r\n?/g, '\n');
  if (!recipient.tier) template = template.replace(/^[\t ]*本期档位[：:][\t ]*\{档位\}[\t ]*(?:\n|$)/gm, '');
  const message=template.replace(/\{(主播|昵称|UID|档位|期次|批次名称|本次礼物|本次期次|认证码|领取码|登记地址|地址提示)\}/g, (_, key: string) => values[key]);
  return addressNotice&&!template.includes('{地址提示}')?`${message}\n\n${addressNotice}`:message;
}
export const voyageField = 'w-full rounded-md border border-input bg-background px-3 py-2 text-sm leading-6 focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50';
