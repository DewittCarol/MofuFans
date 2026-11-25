import { defaultTheme, themeVariables } from "../../../shared/theme";
import { SiteDialogHost } from '@/components/site-dialog-host';
import type { Metadata } from "next";
import "./globals.css";
import { SiteThemeProvider } from "@/lib/site-theme";
import { EmojiCatalogProvider } from "@/lib/emoji-catalog";
import { AuthProvider } from "@/lib/auth";
import { MofuCursor as SiteCursor } from "@/components/site-cursor";


export const metadata: Metadata = {
  title: defaultTheme.brand.admin_title,
  description: defaultTheme.brand.description,
  icons: {
    icon: defaultTheme.brand.admin_favicon,
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" className="h-full" style={{ ...themeVariables(defaultTheme, true), scrollbarGutter: 'stable' }}>
      <body
        className={`antialiased min-h-full`}
      >
        <SiteThemeProvider><SiteCursor /><SiteDialogHost />
        <AuthProvider><EmojiCatalogProvider>{children}</EmojiCatalogProvider></AuthProvider></SiteThemeProvider>
      </body>
    </html>
  );
}
