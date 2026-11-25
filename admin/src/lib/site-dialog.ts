'use client';

export type DialogOptions = {
  title: string;
  confirmLabel?: string;
  cancelLabel?: string;
  destructive?: boolean;
};
export type SiteDialog = DialogOptions & {
  id: number;
  description: string;
  kind: 'confirm' | 'message';
  opener: HTMLElement | null;
};

// Only one confirmation can run at a time. Repeated clicks must not queue mutations.
export class SiteDialogStore {
  private active: SiteDialog | null = null;
  private resolve: ((accepted: boolean) => void) | null = null;
  private listeners = new Set<() => void>();
  private messages: { dialog: SiteDialog; resolve: (accepted: boolean) => void }[] = [];
  private sequence = 0;
  getSnapshot = () => this.active;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  };
  open(description: string, options: DialogOptions, kind: SiteDialog['kind'] = 'confirm'): Promise<boolean> {
    if (this.active && kind === 'confirm') return Promise.resolve(false);
    return new Promise(resolve => {
      const dialog: SiteDialog = {
        ...options, description, kind, id: ++this.sequence,
        opener: typeof document !== 'undefined' && document.activeElement instanceof HTMLElement ? document.activeElement : null,
      };
      if (this.active) { this.messages.push({ dialog, resolve }); return; }
      this.resolve = resolve;
      this.active = dialog;
      this.listeners.forEach(listener => listener());
    });
  }
  finish = (id: number, accepted: boolean) => {
    if (this.active?.id !== id) return;
    const resolve = this.resolve;
    const next = this.messages.shift();
    this.active = next?.dialog || null;
    this.resolve = next?.resolve || null;
    this.listeners.forEach(listener => listener());
    resolve?.(accepted);
  };
  cancel = () => {
    const messages = this.messages.splice(0);
    messages.forEach(message => message.resolve(false));
    if (this.active) this.finish(this.active.id, false);
  };
}

export const siteDialogs = new SiteDialogStore();
export const confirmAction = (description: string, options: DialogOptions) => siteDialogs.open(description, options);
export const confirmDiscard = (description: string) => confirmAction(description, {
  title: '放弃未保存的修改？', confirmLabel: '放弃修改', cancelLabel: '继续编辑',
});
export const showMessage = (description: string, title = '操作未完成') => siteDialogs.open(description, { title, confirmLabel: '知道了' }, 'message');
