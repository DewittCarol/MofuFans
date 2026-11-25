import { defaultTheme } from "../../../../shared/theme";
import type { Metadata } from 'next';
export const metadata: Metadata = { title: `${defaultTheme.modules.ask.label} | ${defaultTheme.brand.tab_title}` };
export default function AskLayout({ children }: { children: React.ReactNode }) { return children; }
