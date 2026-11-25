'use client';
import {useState} from 'react';
import {UserRound} from 'lucide-react';
import {bilibiliProfiles,profileName,type BilibiliProfile} from '@/lib/bilibili-profiles';

export function BilibiliPerson({uid,name,avatar,profile}:{uid:string;name?:string;avatar?:string;profile?:BilibiliProfile}){
 const image=profile?.avatar||avatar;
 const [failedImage,setFailedImage]=useState<string>();
 const pending=!profile||profile.status==='queued'||profile.status==='loading';
 return <div className="flex min-w-0 items-center gap-3">
  <span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-full bg-secondary text-muted-foreground">{image&&failedImage!==image?<img src={image} alt="" className="size-full object-cover" onError={()=>setFailedImage(image)}/>:<UserRound size={20} aria-hidden="true"/>}</span>
  <div className="min-w-0"><a href={`https://space.bilibili.com/${uid}`} target="_blank" rel="noreferrer" className="break-words text-sm font-medium hover:underline focus-visible:outline-2 focus-visible:outline-ring">{profileName(uid,name,profile)}</a><p className="mt-1 text-xs tabular-nums text-muted-foreground">UID {uid}</p>
   {name?.trim()&&profile?.name&&name.trim()!==profile.name&&<p className="mt-1 break-words text-xs text-muted-foreground">B 站昵称：{profile.name}</p>}
   {pending&&!name&&!profile?.name&&<p className="mt-1 text-xs text-muted-foreground">正在获取昵称与头像…</p>}
   {profile?.error&&<p className="mt-1 text-xs leading-5 text-muted-foreground">{profile.error}{(!profile.retry_at||Date.parse(profile.retry_at)<=Date.now())&&<button type="button" className="ml-2 text-primary underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-ring" onClick={()=>bilibiliProfiles.ensure([uid])}>重试</button>}</p>}
  </div>
 </div>
}
