// Minimal toast notification — one stack, auto-dismissing.

const STACK_ID = 'conduit-toast-stack';

function ensureStack(): HTMLDivElement {
  let stack = document.getElementById(STACK_ID) as HTMLDivElement | null;
  if (stack) return stack;
  stack = document.createElement('div');
  stack.id = STACK_ID;
  stack.className = 'toast-stack';
  document.body.appendChild(stack);
  return stack;
}

export type ToastKind = 'info' | 'success' | 'error';

export function toast(message: string, kind: ToastKind = 'info', durationMs = 3500): void {
  const stack = ensureStack();
  const el = document.createElement('div');
  el.className = `toast toast--${kind}`;
  el.textContent = message;
  stack.appendChild(el);
  // Trigger transition
  requestAnimationFrame(() => el.classList.add('toast--visible'));
  setTimeout(() => {
    el.classList.remove('toast--visible');
    setTimeout(() => el.remove(), 250);
  }, durationMs);
}
