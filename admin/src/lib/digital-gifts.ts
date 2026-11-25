export type GiftMode = 'shared' | 'individual' | 'open';
export interface GiftFile { id: number; name: string; size: number; media_type: string }
export interface DigitalGift { id: number; title: string; intro: string; content: string; mode: GiftMode; published: boolean; version: number; files: GiftFile[] }
export interface GiftRecipient { message?:{status:string;note:string}; id: number; uid: string; name: string; disabled: boolean; roster_active: boolean; sources?: {id:number;roster_id:number;roster_title:string;tier:string;name:string;disabled:boolean;version:number}[]; version: number; sent_at: string | null }
export const giftField = 'w-full rounded-md border border-input bg-background px-3 py-2 text-sm leading-6 focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50';

export async function giftAPI<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api/admin/gifts${path}`, { method, signal, credentials: 'include', cache: 'no-store', headers: body instanceof FormData ? undefined : { 'Content-Type': 'application/json' }, body: body instanceof FormData ? body : body === undefined ? undefined : JSON.stringify(body) });
  if (!response.ok) throw new Error(response.status === 413 ? '上传请求未被接收，请重新选择文件重试' : '请求失败，请稍后重试');
  const result = await response.json();
  if (result.code !== 200) throw new Error(result.message || '操作失败，请重试');
  return result.data;
}

export function giftMessage(title: string, password: string, uid: string, individual: boolean, open = false) {
  return `「${title}」已开放领取。\n请打开 本站，在「电子礼物」中找到这份礼物。\n${open ? '无需密码，可以直接查看和下载。' : `${individual ? `UID：${uid}\n` : ''}礼物密码：${password}\n输入${individual ? ' UID 和' : ''}密码即可查看和下载。密码区分大小写，请妥善保管。`}`;
}

export function giftSize(size: number) {
 if(size<1024)return `${size} B`;
 const units=['KB','MB','GB','TB'];let value=size/1024,index=0;
 while(value>=1024&&index<units.length-1){value/=1024;index++}
 return `${value.toFixed(value>=100?0:1)} ${units[index]}`;
}
