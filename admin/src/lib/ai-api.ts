export async function aiAPI<T>(path: string, method = "GET", body?: unknown, _token?: string, timeout=30000):Promise<T>{
 const response=await fetch(`/api/${path}`,{method,credentials:"same-origin",cache:"no-store",headers:{"Content-Type":"application/json"},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(timeout)});
 const result=await response.json();if(!response.ok||result.code!==200)throw new Error(result.message||"请求失败，请稍后重试");return result.data as T;
}
