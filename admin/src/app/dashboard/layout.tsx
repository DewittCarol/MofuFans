"use client";

import { useSiteTheme } from "@/lib/site-theme";
import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import { Dialog } from "radix-ui";
import { Menu, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sidebar } from "@/components/sidebar";
import { useAuth } from "@/lib/auth";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const {theme}=useSiteTheme();
  const { isAuthenticated, isLoading } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  useEffect(() => { setMenuOpen(false); }, [pathname]);

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push("/");
    }
  }, [isAuthenticated, isLoading, router]);

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="text-muted-foreground text-lg">加载中...</div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return null;
  }

  return (
    <div className="flex h-[100dvh] min-h-0 flex-col overflow-hidden sm:flex-row">
      <div className="hidden h-full sm:block"><Sidebar /></div>
      <div className="flex shrink-0 items-center gap-3 border-b border-border bg-card px-4 py-2 sm:hidden">
        <Dialog.Root open={menuOpen} onOpenChange={setMenuOpen}>
          <Dialog.Trigger asChild><Button variant="ghost" size="icon" aria-label="打开管理菜单"><Menu size={20} /></Button></Dialog.Trigger>
          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 z-50 bg-black/40" />
            <Dialog.Content aria-describedby={undefined} className="fixed inset-y-0 left-0 z-50 h-[100dvh] w-56 outline-none"
              onClick={event => { if ((event.target as Element).closest('a')) setMenuOpen(false); }}>
              <Dialog.Title className="sr-only">管理菜单</Dialog.Title>
              <Sidebar />
              <Dialog.Close asChild><button type="button" aria-label="关闭管理菜单" className="absolute -right-12 top-3 rounded-lg bg-card p-2.5 text-foreground focus-visible:outline-2 focus-visible:outline-ring"><X size={20} /></button></Dialog.Close>
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
        <span className="text-sm font-medium">{theme.brand.admin_title}</span>
      </div>
      <main className="relative min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain bg-background">
        <div className={`mx-auto px-4 py-6 sm:px-8 sm:py-8 lg:px-10 ${pathname.startsWith('/dashboard/voyage') ? 'max-w-[1520px]' : pathname.startsWith('/dashboard/settings') ? 'max-w-[1480px]' : 'max-w-6xl'}`}>
          {children}
        </div>
      </main>
    </div>
  );
}
