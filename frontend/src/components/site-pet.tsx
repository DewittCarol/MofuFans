'use client';
import {useMemo} from 'react';
import {useSiteTheme} from '@/lib/site-theme';
import {Desktop3dPet} from './desktop-3d-pet';

// The application ships no character model. A site owner may supply a licensed VRM.
export function SitePet() {
  const {theme,ready}=useSiteTheme();
  const definition=useMemo(()=>({family:0,name:theme.pet.label||'桌宠',url:theme.pet.model_url}),[theme.pet.label,theme.pet.model_url]);
  return ready && theme.appearance.pet && definition.url
    ? <Desktop3dPet key={definition.url} model={definition} label={definition.name} storageKey="fans-pet-v1"/>
    : null;
}
