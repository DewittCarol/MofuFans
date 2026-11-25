export type InstallationSettings = {
  storage_type: "local" | "oss";
  oss: { endpoint: string; region: string; bucket: string; prefix: string; has_access_key: boolean; has_secret_key: boolean };
  test_recipient_uid: string;
  secure_cookie: boolean;
  locked: string[];
  restart_required: boolean;
  active_storage_type: "local" | "oss";
  database_type: string;
};

export type InstallationUpdate = Pick<InstallationSettings, "storage_type" | "test_recipient_uid" | "secure_cookie"> & {
  oss: Pick<InstallationSettings["oss"], "endpoint" | "region" | "bucket" | "prefix"> & { access_key: string; secret_key: string };
  clear_credentials?: boolean;
  confirm_storage_change?: boolean;
};

export function editableSettings(settings: InstallationSettings): InstallationUpdate {
  const { endpoint, region, bucket, prefix } = settings.oss;
  return { storage_type: settings.storage_type, test_recipient_uid: settings.test_recipient_uid, secure_cookie: settings.secure_cookie,
    oss: { endpoint, region, bucket, prefix, access_key: "", secret_key: "" } };
}

export async function installationAPI<T>(path: string, method = "GET", body?: unknown, token?: string, timeout = 30000): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json", "X-MofuFans-Request": "settings" };
  if (token) headers["X-Setup-Token"] = token;
  const response = await fetch(`/api/${path}`, { method, credentials: "same-origin", cache: "no-store", headers,
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(timeout) });
  const result = await response.json();
  if (!response.ok || result.code !== 200) throw new Error(result.message || "请求未完成，请稍后重试");
  return result.data as T;
}

export function installationError(error: unknown): string {
  if (error instanceof Error && error.name === "TimeoutError") return "请求超时，请检查网络后重试；若正在完成初始化，可刷新页面确认结果。";
  return error instanceof Error ? error.message : "连接失败，请检查服务是否正常运行";
}
