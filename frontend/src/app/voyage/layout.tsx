import { defaultTheme } from "../../../../shared/theme";
import type { Metadata } from 'next';
import { VoyageAccess } from '@/components/voyage-access';
export const metadata: Metadata={title: `${defaultTheme.modules.voyage.label} | ${defaultTheme.brand.tab_title}`};
export default function Layout({children}:{children:React.ReactNode}) {return <VoyageAccess>{children}</VoyageAccess>;}
