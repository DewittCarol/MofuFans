import { defaultTheme } from "../../../../shared/theme";
import type { Metadata } from 'next';
export const metadata:Metadata={title: `${defaultTheme.modules.songs.label} | ${defaultTheme.brand.tab_title}`};
export default function Layout({children}:{children:React.ReactNode}){return children;}
