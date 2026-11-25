'use client';
import {useEffect,useSyncExternalStore} from 'react';
import {bilibiliProfiles,validProfileUID} from './bilibili-profiles';

export function useBilibiliProfiles(uids:string[]){
 useSyncExternalStore(bilibiliProfiles.subscribe,bilibiliProfiles.snapshot,()=>0);
 const key=Array.from(new Set(uids.filter(validProfileUID))).join(',');
 useEffect(()=>{
  const ids=key?key.split(','):[];
  const update=()=>bilibiliProfiles.ensure(ids);
  // Avoid querying every intermediate value while typing a UID.
  const timer=setTimeout(update,400);
  window.addEventListener('focus',update);
  return()=>{clearTimeout(timer);window.removeEventListener('focus',update)};
 },[key]);
 return bilibiliProfiles.profiles;
}
