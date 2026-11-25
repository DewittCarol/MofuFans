export type LeaveDecision = boolean | Promise<boolean>;
type NavigationEvent = {
  defaultPrevented: boolean;
  button: number;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  altKey: boolean;
  preventDefault: () => void;
  stopPropagation: () => void;
};

export const isPlainNavigation = (event: NavigationEvent) => !event.defaultPrevented && event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;

export function guardNavigation(event: NavigationEvent, canLeave: () => LeaveDecision, navigate: () => void) {
  const decision = canLeave();
  if (decision === true) return;
  // This must happen synchronously: awaiting first would let the original link navigate.
  event.preventDefault();
  event.stopPropagation();
  if (decision !== false) void decision.then(allowed => { if (allowed) navigate(); }).catch(() => { /* Failed confirmation keeps the draft open. */ });
}
