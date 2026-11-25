'use client';

import { useEffect, useSyncExternalStore } from 'react';
import { usePathname } from 'next/navigation';
import { AlertDialog } from 'radix-ui';
import { buttonVariants } from '@/components/ui/button';
import { siteDialogs } from '@/lib/site-dialog';

const emptySnapshot = () => null;

export function SiteDialogHost() {
  const dialog = useSyncExternalStore(siteDialogs.subscribe, siteDialogs.getSnapshot, emptySnapshot);
  const pathname = usePathname();
  // A dialog from the previous screen must never approve an action after navigation.
  useEffect(() => () => siteDialogs.cancel(), [pathname]);
  useEffect(() => {
    window.addEventListener('popstate', siteDialogs.cancel);
    return () => window.removeEventListener('popstate', siteDialogs.cancel);
  }, []);
  if (!dialog) return null;

  const close = (accepted: boolean) => siteDialogs.finish(dialog.id, accepted);
  return <AlertDialog.Root key={dialog.id} open onOpenChange={open => { if (!open) close(false); }}>
    <AlertDialog.Portal>
      <AlertDialog.Overlay className="fixed inset-0 z-[3000] bg-black/40" />
      <AlertDialog.Content
        className="fixed left-1/2 top-1/2 z-[3001] max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-xl bg-card p-6 text-card-foreground shadow-xl outline-none sm:p-7"
        onCloseAutoFocus={event => { event.preventDefault(); if (!siteDialogs.getSnapshot() && dialog.opener?.isConnected) dialog.opener.focus(); }}
      >
        <AlertDialog.Title className="text-xl font-semibold leading-snug">{dialog.title}</AlertDialog.Title>
        <AlertDialog.Description className="mt-3 whitespace-pre-wrap break-words text-sm leading-7 text-muted-foreground">{dialog.description}</AlertDialog.Description>
        <div className="mt-7 flex flex-wrap justify-end gap-3">
          {dialog.kind === 'confirm' && <AlertDialog.Cancel type="button" className={buttonVariants({variant: 'outline', className: 'min-h-10'})} onClick={() => close(false)}>{dialog.cancelLabel || '取消'}</AlertDialog.Cancel>}
          {dialog.kind === 'message'
            ? <AlertDialog.Cancel type="button" className={buttonVariants({className: 'min-h-10'})} onClick={() => close(true)}>{dialog.confirmLabel}</AlertDialog.Cancel>
            : <AlertDialog.Action type="button" className={buttonVariants({variant: dialog.destructive ? 'destructive' : 'default', className: 'min-h-10'})} onClick={() => close(true)}>{dialog.confirmLabel || '确认'}</AlertDialog.Action>}
        </div>
      </AlertDialog.Content>
    </AlertDialog.Portal>
  </AlertDialog.Root>;
}
