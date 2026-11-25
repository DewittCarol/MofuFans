import { defaultTheme, themeVariables } from "../../../shared/theme";
import { SiteDialogHost } from '@/components/site-dialog-host';
import type { Metadata } from "next";
import "./globals.css";
import { SiteThemeProvider } from "@/lib/site-theme";
import { EmojiCatalogProvider } from "@/lib/emoji-catalog";
import { ReactionBurstProvider } from "@/components/reaction-burst";
import { Navbar } from "@/components/navbar";
import { AccountAuthProvider } from "@/lib/account-auth";
import { SitePet } from "@/components/site-pet";
import { MofuCursor as SiteCursor } from "@/components/site-cursor";
import { AdminSessionProvider } from "@/lib/admin-session";
import { SiteBackground } from "@/components/site-background";
import { InstallationGate } from "@/components/installation-gate";
import { SiteFooter } from "@/components/site-footer";


export const metadata: Metadata = {
  title: defaultTheme.brand.tab_title,
  description: defaultTheme.brand.description,
  icons: {
    icon: defaultTheme.brand.favicon,
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" data-theme-texture={String(defaultTheme.appearance.texture)} style={{ ...themeVariables(defaultTheme, false), scrollbarGutter: 'stable' }}>
      <body
        className={`antialiased fabric-linen isolate`}
      >
        <SiteThemeProvider><InstallationGate><EmojiCatalogProvider><AccountAuthProvider><AdminSessionProvider>
          <SiteBackground />
          <SiteCursor />
          <SiteDialogHost />
          <SitePet />
          <ReactionBurstProvider>
            <Navbar />
            {children}
          </ReactionBurstProvider>
          <SiteFooter />
        </AdminSessionProvider></AccountAuthProvider></EmojiCatalogProvider></InstallationGate></SiteThemeProvider>
      </body>
    </html>
  );
}
