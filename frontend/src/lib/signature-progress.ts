import { signatureSecondsLeft, type SignatureChallenge, type SignaturePurpose } from './member-api';

export const signatureProgressKey = 'mofu-signature-progress-v1';
const dismissalKey = 'mofu-signature-dismissed-v1';
export interface SignatureProgress {
  challenge_id: string;
  bilibili_uid: string;
  purpose: SignaturePurpose;
  verification_text?: string;
  expires_at: string;
  confirmation_until?: string;
}
const purposes = ['register', 'recover', 'upgrade'];
function publicFields(value: unknown): SignatureProgress | null {
  if (!value || typeof value !== 'object') return null;
  const row = value as Record<string, unknown>;
  if (typeof row.challenge_id !== 'string' || !row.challenge_id || row.challenge_id.length > 200
    || typeof row.bilibili_uid !== 'string' || !/^[1-9]\d{0,18}$/.test(row.bilibili_uid)
    || typeof row.purpose !== 'string' || !purposes.includes(row.purpose)
    || typeof row.expires_at !== 'string' || !Number.isFinite(Date.parse(row.expires_at))) return null;
  return {
    challenge_id: row.challenge_id, bilibili_uid: row.bilibili_uid, purpose: row.purpose as SignaturePurpose, expires_at: row.expires_at,
    ...(typeof row.verification_text === 'string' && row.verification_text.length <= 2048 ? { verification_text: row.verification_text } : {}),
    ...(typeof row.confirmation_until === 'string' && Number.isFinite(Date.parse(row.confirmation_until)) ? { confirmation_until: row.confirmation_until } : {}),
  };
}
export function sameSignatureProgress(a: SignatureProgress | null, b: SignatureProgress): boolean {
  return !!a && a.challenge_id === b.challenge_id && a.bilibili_uid === b.bilibili_uid && a.purpose === b.purpose;
}
export function clearSignatureProgress(challengeID?: string): void {
  try {
    const raw = localStorage.getItem(signatureProgressKey);
    if (!raw) return;
    if (challengeID) {
      let row: SignatureProgress | null = null;
      try { row = publicFields(JSON.parse(raw)); } catch { /* Invalid public cache can be removed. */ }
      if (row && row.challenge_id !== challengeID) return;
    }
    localStorage.removeItem(signatureProgressKey);
  } catch { /* Cookies still allow recovery when browser storage is disabled. */ }
}
export function readSignatureProgress(): SignatureProgress | null {
  try {
    const raw = localStorage.getItem(signatureProgressKey);
    if (!raw) return null;
    const progress = publicFields(JSON.parse(raw));
    if (!progress || Date.parse(progress.expires_at) <= Date.now()) { clearSignatureProgress(progress?.challenge_id); return null; }
    return progress;
  } catch { clearSignatureProgress(); return null; }
}
export function saveSignatureProgress(challenge: SignatureChallenge): void {
  const progress = publicFields(challenge);
  if (!progress || signatureSecondsLeft(challenge) <= 0 || !['pending', 'verified'].includes(challenge.status)) { clearSignatureProgress(challenge.challenge_id); return; }
  try { localStorage.setItem(signatureProgressKey, JSON.stringify(progress)); } catch { /* The server cookie is the recovery fallback. */ }
}
export function dismissSignatureProgress(progress: SignatureProgress | null): void {
  if (!progress) return;
  clearSignatureProgress(progress.challenge_id);
  try { localStorage.setItem(dismissalKey, JSON.stringify({ challenge_id: progress.challenge_id, expires_at: progress.expires_at })); } catch { /* Recovery still depends on server status. */ }
}
export function clearSignatureDismissal(challengeID?: string): void {
  try {
    const raw = localStorage.getItem(dismissalKey);
    if (!raw) return;
    if (challengeID && JSON.parse(raw)?.challenge_id !== challengeID) return;
    localStorage.removeItem(dismissalKey);
  } catch { /* Optional public preference. */ }
}
export function isSignatureDismissed(progress: SignatureProgress): boolean {
  try {
    const raw = localStorage.getItem(dismissalKey);
    if (!raw) return false;
    const row = JSON.parse(raw);
    if (!row || !Number.isFinite(Date.parse(row.expires_at)) || Date.parse(row.expires_at) <= Date.now()) { clearSignatureDismissal(); return false; }
    return row.challenge_id === progress.challenge_id;
  } catch { return false; }
}
// Only the server supplies the phase and deadlines. Cache may supply an old
// public verification string, and only for exactly the same challenge.
export function recoverSignatureProgress(value: unknown, cached: SignatureProgress | null): SignatureChallenge | null {
  const progress = publicFields(value);
  if (!progress) return null;
  const row = value as Record<string, unknown>;
  if (row.status !== 'pending' && row.status !== 'verified') return null;
  const result: SignatureChallenge = { ...progress, status: row.status,
    ...(typeof row.bilibili_name === 'string' ? { bilibili_name: row.bilibili_name } : {}),
  };
  if (signatureSecondsLeft(result) <= 0) return null;
  if (!result.verification_text && sameSignatureProgress(cached, result) && Date.parse(cached!.expires_at) > Date.now()) result.verification_text = cached!.verification_text;
  return result;
}
