import type { AccountUser, ApiResponse } from './api';
import type { VoyageAddress, VoyageAddressBookEntry, VoyageLongTermState } from './voyage-claim';

export type SignaturePurpose = 'register' | 'recover' | 'upgrade';
export type AccountDialogMode = 'login' | SignaturePurpose;
export interface SignatureChallenge {
  challenge_id: string;
  purpose: SignaturePurpose;
  status: 'pending' | 'verified' | 'expired' | 'consumed';
  bilibili_uid: string;
  bilibili_name?: string;
  verification_text?: string;
  expires_at: string;
  confirmation_until?: string;
}

export async function memberAccountRequest<T>(path: string, body?: unknown): Promise<ApiResponse<T>> {
  const response = await fetch(`/api/account${path}`, {
    method: body === undefined ? 'GET' : 'POST', credentials: 'include', cache: 'no-store',
    ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  });
  return response.json();
}
export const startSignature = (bilibili_uid: string, purpose: SignaturePurpose) => memberAccountRequest<SignatureChallenge>('/signature/start', { bilibili_uid, purpose });
export const getSignatureStatus = () => memberAccountRequest<SignatureChallenge | null>('/signature/status');
export const checkSignature = (challenge_id: string) => memberAccountRequest<SignatureChallenge>('/signature/check', { challenge_id });
export const finishSignature = (challenge_id: string, password?: string) => memberAccountRequest<AccountUser>('/signature/finish', password === undefined ? { challenge_id } : { challenge_id, password });
export const changeMemberPassword = (current_password: string, password: string, uid: string) => memberAccountRequest<AccountUser>('/password', { current_password, password, uid });
export const revokeOtherMemberSessions = (password: string, uid: string) => memberAccountRequest<AccountUser>('/logout-others', { password, uid });

export interface MemberAddressBook {
  uid: string;
  entries: VoyageAddressBookEntry[];
  long_term: VoyageLongTermState;
}
export interface MemberCollection {
  id: number; title: string; note: string; opens: string; closes: string; collecting: boolean; closed: boolean;
}
export class MemberRequestError extends Error {
  constructor(message: string, public code: number) { super(message); this.name = 'MemberRequestError'; }
}
export async function privateMemberRequest<T>(path: string, body?: unknown): Promise<T> {
  const response = await memberAccountRequest<T>(path, body);
  if (response.code !== 200) {
    if (typeof window !== 'undefined' && [401, 403, 408].includes(response.code)) window.dispatchEvent(new Event('member-auth-refresh'));
    throw new MemberRequestError(response.message || '暂时无法完成操作，请重试', response.code);
  }
  return response.data;
}
export const loadMemberAddressBook = (uid: string) => privateMemberRequest<MemberAddressBook>(`/address-book?expected_uid=${encodeURIComponent(uid)}`);
export const saveMemberAddress = (address: VoyageAddress, uid: string) => privateMemberRequest<MemberAddressBook>('/address-book', { action: 'save', uid, address: { ...address, note: '' } });
export const setMemberDefaultAddress = (entry_id: number, long_term_version: number, uid: string) => privateMemberRequest<MemberAddressBook>('/address-book', { action: 'set_default', uid, entry_id, long_term_version });
export const clearMemberDefaultAddress = (long_term_version: number, uid: string) => privateMemberRequest<MemberAddressBook>('/address-book', { action: 'clear_default', uid, long_term_version });
export const loadMemberCollections = (uid: string) => privateMemberRequest<MemberCollection[]>(`/collections?expected_uid=${encodeURIComponent(uid)}`);

export function passwordProblem(password: string, confirmation: string): string {
  if (password !== confirmation) return '两次输入的密码不一致';
  if (Array.from(password).length < 8) return '密码至少需要 8 个字符';
  if (new TextEncoder().encode(password).length > 72) return '密码过长，请缩短后重试（中文等字符占用更多长度）';
  return '';
}
export function signatureSecondsLeft(value: SignatureChallenge, now = Date.now()): number {
  const expires = Date.parse(value.expires_at);
  const end = value.status === 'verified' ? Math.min(expires, Date.parse(value.confirmation_until || '')) : expires;
  return Number.isFinite(end) ? Math.max(0, Math.ceil((end - now) / 1000)) : 0;
}
