import {voyageRequest} from './voyage';

export interface BilibiliProfile {
 uid:string; name:string; avatar:string; error:string;
 checked_at?:string; retry_at?:string|null; pause_until?:string;
 status:'queued'|'loading'|'ready'|'error';
}
export const validProfileUID=(uid:string)=>/^[1-9]\d{0,18}$/.test(uid)&&BigInt(uid)<=BigInt('9223372036854775807');
export const profileName=(uid:string,manual:string|undefined,profile?:BilibiliProfile)=>manual?.trim()||profile?.name||`UID ${uid}`;
export const profileCacheAge=7*24*60*60*1000;

// One browser queue shared by every admin module; the server also deduplicates
// profiles and throttles across tabs. No business record is written here.
export class BilibiliProfileStore {
 profiles:Record<string,BilibiliProfile>={};
 version=0;
 private listeners=new Set<()=>void>();
 private queue:string[]=[];
 private pending=new Set<string>();
 private forced=new Set<string>();
 private refreshes=new Map<string,Promise<BilibiliProfile>>();
 private running=false;
 private pauseUntil=0;
 private timer:ReturnType<typeof setTimeout>|undefined;
 constructor(private request=(uid:string,refresh=false)=>voyageRequest<BilibiliProfile>(`/api/bilibili/profiles/${uid}`,'POST',{refresh}),private now=()=>Date.now(),private readCache:((uids:string[])=>Promise<BilibiliProfile[]>)|null=null){}
 subscribe=(listener:()=>void)=>{this.listeners.add(listener);return()=>{this.listeners.delete(listener)}};
 snapshot=()=>this.version;
 private emit(){this.version++;for(const fn of this.listeners)fn();}
 ensure(uids:string[]){
  let changed=false;
  for(const uid of new Set(uids)){
   if(!validProfileUID(uid)||this.pending.has(uid))continue;
   const p=this.profiles[uid];
   if(p?.retry_at&&Date.parse(p.retry_at)>this.now())continue;
   if(p?.status==='ready'&&this.now()-Date.parse(p.checked_at||'')<profileCacheAge)continue;
   this.pending.add(uid);this.queue.push(uid);
   this.profiles[uid]={...p,uid,name:p?.name||'',avatar:p?.avatar||'',error:'',status:'queued'};changed=true;
  }
  if(changed)this.emit();
  void this.drain();
 }
 // Explicit refresh shares the automatic queue, but bypasses its seven-day cache.
 // Join an in-flight lookup first; a fresh response needs no second request.
 refresh(uid:string):Promise<BilibiliProfile>{
  const existing=this.refreshes.get(uid);if(existing)return existing;
  const task=this.refreshOne(uid).finally(()=>this.refreshes.delete(uid));
  this.refreshes.set(uid,task);return task;
 }
 private async refreshOne(uid:string):Promise<BilibiliProfile>{
  if(!validProfileUID(uid))throw Error('请输入有效的 B 站 UID');
  if(this.pending.has(uid)&&!this.queue.includes(uid))await this.waitFor(uid);
  const p=this.profiles[uid];
  if(p?.retry_at&&Date.parse(p.retry_at)>this.now())return p;
  if(this.pauseUntil>this.now())return this.paused(uid);
  if(p?.status==='ready'&&this.now()-Date.parse(p.checked_at||'')<60000)return p;
  this.queue=this.queue.filter(value=>value!==uid);
  this.queue.unshift(uid);this.pending.add(uid);this.forced.add(uid);
  this.profiles[uid]={...p,uid,name:p?.name||'',avatar:p?.avatar||'',error:'',status:'queued'};
  this.emit();
  const result=this.waitFor(uid);
  void this.drain();
  return result;
 }
 private waitFor(uid:string):Promise<BilibiliProfile>{
  if(!this.pending.has(uid))return Promise.resolve(this.profiles[uid]);
  return new Promise(resolve=>{
   const unsubscribe=this.subscribe(()=>{
    if(!this.pending.has(uid)){unsubscribe();resolve(this.profiles[uid])}
   });
  });
 }
 private paused(uid:string):BilibiliProfile{
  const p=this.profiles[uid],until=new Date(this.pauseUntil).toISOString();
  return {...p,uid,name:p?.name||'',avatar:p?.avatar||'',status:'error',error:'B 站资料暂时无法获取，请稍后重试',retry_at:until,pause_until:until};
 }
 private async drain(){
  if(this.running||!this.queue.length)return;
  const delay=this.pauseUntil-this.now();
  if(delay>0){if(!this.timer)this.timer=setTimeout(()=>{this.timer=undefined;void this.drain()},delay);return}
  this.running=true;
  try{
   const cachedUIDs=this.queue.filter(uid=>!this.forced.has(uid)).slice(0,500);
   if(this.readCache&&cachedUIDs.length){
    try{
     const cached=await this.readCache(cachedUIDs);
     for(const p of cached){
      if(!cachedUIDs.includes(p.uid)||this.forced.has(p.uid))continue;
      const reusable=!!(p.retry_at&&Date.parse(p.retry_at)>this.now())||(!p.error&&!!p.name&&!!p.avatar&&this.now()-Date.parse(p.checked_at||'')<profileCacheAge);
      this.profiles[p.uid]={...p,status:reusable?(p.error?'error':'ready'):'queued'};
      if(reusable){this.queue=this.queue.filter(uid=>uid!==p.uid);this.pending.delete(p.uid)}
     }
     this.emit();
    }catch{/* The normal per-user queue also reads the persisted cache. */}
   }
   while(this.queue.length&&this.pauseUntil<=this.now()){
    const uid=this.queue.shift()!;
    this.profiles[uid]={...this.profiles[uid],status:'loading'};this.emit();
    try{
     const data=await this.request(uid,this.forced.delete(uid));
     const error=data.error||(!data.name||!data.avatar?'资料暂未获取完整，可稍后重试':'');
     this.profiles[uid]={...data,uid,error,status:error?'error':'ready'};
     if(data.pause_until)this.pauseUntil=Math.max(this.pauseUntil,Date.parse(data.pause_until));
    }catch{
     this.pauseUntil=this.now()+60000;
     this.profiles[uid]={...this.profiles[uid],status:'error',error:'资料暂未获取成功，可稍后重试',retry_at:new Date(this.pauseUntil).toISOString(),pause_until:new Date(this.pauseUntil).toISOString()};
    }
    const retryAt=Date.parse(this.profiles[uid].retry_at||'');
    if(retryAt>this.now())setTimeout(()=>this.emit(),retryAt-this.now()+50);
    this.pending.delete(uid);this.emit();
   }
  }finally{
   // Do not leave a manual refresh waiting to run after an upstream cooldown.
   if(this.pauseUntil>this.now()){
    for(const uid of this.forced){
     this.queue=this.queue.filter(value=>value!==uid);this.pending.delete(uid);
     this.profiles[uid]=this.paused(uid);
    }
    this.forced.clear();this.emit();
   }
   this.running=false;void this.drain();
  }
 }
}

export const bilibiliProfiles=new BilibiliProfileStore(undefined,undefined,uids=>voyageRequest<BilibiliProfile[]>('/api/bilibili/profiles-cache','POST',{uids}));

export interface ProfileSyncProgress {total:number;completed:number;succeeded:number;failed:number}
export interface ProfileSyncResult extends ProfileSyncProgress {stopped:boolean;paused:boolean;error:string}
export async function syncBilibiliProfiles(uids:string[],options:{
 refresh:(uid:string)=>Promise<BilibiliProfile>;
 onProgress:(progress:ProfileSyncProgress)=>void;
 isStopped:()=>boolean;
}):Promise<ProfileSyncResult>{
 const ids=[...new Set(uids.filter(validProfileUID))];
 const result:ProfileSyncResult={total:ids.length,completed:0,succeeded:0,failed:0,stopped:false,paused:false,error:''};
 options.onProgress({...result});
 for(const uid of ids){
  if(options.isStopped()){result.stopped=true;break}
  try{
   const profile=await options.refresh(uid);
   if(profile.error||!profile.name||!profile.avatar){result.failed++;result.error=profile.error||'部分资料暂未获取完整'}else result.succeeded++;
   if(profile.pause_until&&Date.parse(profile.pause_until)>Date.now())result.paused=true;
  }catch{result.failed++;result.paused=true;result.error='资料暂未获取成功，请稍后重试'}
  result.completed++;options.onProgress({...result});
  if(result.paused)break;
 }
 if(options.isStopped()&&result.completed<result.total)result.stopped=true;
 return result;
}
