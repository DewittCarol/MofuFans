const API_BASE = '/api';

export interface Tag {
  id: number;
  tag_name: string;
  description: string;
  question_count: number;
  created_at: string;
  updated_at: string;
}

export interface EmojiData {
  value: string;
  count: number;
}

export interface Question {
  id: number;
  tag_id: number;
  tag: Tag;
  content: string;
  images: string;
  images_num: number;
  is_hide: boolean;
  is_rainbow: boolean;
  is_archive: boolean;
  is_publish: boolean;
  is_spam: boolean;
  is_real_name: boolean;
  bilibili_uid?: string;
  bilibili_name?: string;
  bilibili_avatar?: string;
  emojis: string;
  likes: number;
  created_at: string;
  updated_at: string;
}

export interface CursorTheme {
  id: string;
  name: string;
  manifest?: string;
}

export interface Config {
  cursor_theme: string;
  cursor_themes: CursorTheme[];
  announcement: string;
  require_verified_user_to_post: boolean;
  voyage_public_enabled: boolean;
  pet_default_expanded: boolean;
  allow_repeated_reactions: boolean;
}

export interface AccountUser {
  username: string;
  bilibili_uid: string;
  bilibili_name: string;
  bilibili_avatar: string;
  verified_at: string;
  signature_verified: boolean;
  signature_verified_at?: string | null;
  is_disabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface VerificationStatus {
  status: 'pending' | 'verified' | 'expired' | 'consumed';
  target_uid?: string;
  bilibili_uid?: string;
  bilibili_name?: string;
  bilibili_avatar?: string;
  requested_at: string;
  expires_at: string;
  confirmation_until?: string;
}

export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

export interface QuestionsResponse {
  min_id: number;
  max_id: number;
  questions: Question[];
  page: number;
  page_size: number;
  total: number;
}

export async function getInfo(): Promise<ApiResponse<{ id: number; username: string }>> {
  const res = await fetch(`${API_BASE}/info`, {
    method: 'GET',
    credentials: 'include',
  });
  return res.json();
}

export async function getConfig(): Promise<ApiResponse<Config>> {
  const res = await fetch(`${API_BASE}/config`, {
    method: 'GET',
    credentials: 'include',
  });
  return res.json();
}

export async function getAccountInfo(): Promise<ApiResponse<AccountUser>> {
  const res = await fetch(`${API_BASE}/account/info`, { credentials: 'include', cache: 'no-store' });
  return res.json();
}

export async function loginAccount(username: string, password: string, useLegacyName = false): Promise<ApiResponse<AccountUser>> {
  const res = await fetch(`${API_BASE}/account/login`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(!useLegacyName && /^[1-9]\d{0,18}$/.test(username) ? { bilibili_uid: username, password } : { username, password }),
  });
  return res.json();
}

export async function logoutAccount(uid: string): Promise<ApiResponse<null>> {
  const res = await fetch(`${API_BASE}/account/logout`, { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ uid }) });
  return res.json();
}

export async function startAccountVerification(bilibiliUid: string): Promise<ApiResponse<VerificationStatus>> {
  const res = await fetch(`${API_BASE}/account/verification`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bilibili_uid: bilibiliUid }),
  });
  return res.json();
}

export async function getAccountVerification(): Promise<ApiResponse<VerificationStatus>> {
  const res = await fetch(`${API_BASE}/account/verification`, { credentials: 'include' });
  return res.json();
}

export async function registerAccount(username: string, password: string): Promise<ApiResponse<AccountUser>> {
  const res = await fetch(`${API_BASE}/account/register`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  return res.json();
}

export async function getQuestions(params: {
  locate_id?: number;
  page?: number;
  size?: number;
  order_by?: string;
  order?: string;
  tag_id?: number;
  publish?: boolean;
  rainbow?: boolean;
  archive?: boolean;
  search?: string;
}, signal?: AbortSignal): Promise<ApiResponse<QuestionsResponse>> {
  const searchParams = new URLSearchParams();
  if (params.locate_id) searchParams.set("locate_id", params.locate_id.toString());
  if (params.page) searchParams.set('page', params.page.toString());
  if (params.size) searchParams.set('page_size', params.size.toString());
  if (params.order_by) searchParams.set('order_by', params.order_by);
  if (params.order) searchParams.set('order', params.order);
  if (params.tag_id) searchParams.set('tag_id', params.tag_id.toString());
  if (params.publish !== undefined) searchParams.set('publish', params.publish.toString());
  if (params.rainbow !== undefined) searchParams.set('rainbow', params.rainbow.toString());
  if (params.archive !== undefined) searchParams.set('archive', params.archive.toString());
  if (params.search) searchParams.set('search', params.search);

  const res = await fetch(`${API_BASE}/question?${searchParams.toString()}`, {
    method: 'GET',
    signal,
    credentials: 'include',
  });
  return res.json();
}

export async function createQuestion(data: FormData): Promise<Response> {
  return fetch(`${API_BASE}/question`, {
    method: 'POST',
    credentials: 'include',
    body: data,
  });
}

export async function updateQuestion(id: number, data: {
  tag_id?: number;
  is_hide?: boolean;
  is_rainbow?: boolean;
  is_archive?: boolean;
  is_publish?: boolean;
}): Promise<ApiResponse<null>> {
  const res = await fetch(`${API_BASE}/question/${id}`, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  return res.json();
}

export async function addEmoji(questionId: number, emoji: string): Promise<ApiResponse<{ card_id: number; emojis: EmojiData[] }>> {
  const formData = new FormData();
  formData.append('emoji', emoji);
  const res = await fetch(`${API_BASE}/question/${questionId}/emoji`, {
    method: 'POST',
    credentials: 'include',
    body: formData,
  });
  return res.json();
}

export async function getTags(): Promise<ApiResponse<Tag[]>> {
  const res = await fetch(`${API_BASE}/tag`, {
    method: 'GET',
    credentials: 'include',
  });
  return res.json();
}
