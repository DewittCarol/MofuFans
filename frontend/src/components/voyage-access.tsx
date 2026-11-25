'use client';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useVoyageAccess } from '@/lib/admin-session';
export function VoyageAccess({children}:{children:React.ReactNode}) {
 const allowed=useVoyageAccess(),router=useRouter();
 useEffect(()=>{if(allowed===false)router.replace('/');},[allowed,router]);
 return allowed ? <>{children}</> : null;
}
