import { useEffect } from 'react';
import type { ReactNode } from 'react';

interface DrawerProps {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  children: ReactNode;
}

// Drawer is a right-side slide-in panel. Pure CSS transition (~250ms), no
// animation library. Always mounted so the slide-out animates too; pointer
// events and the backdrop are gated on `open`.
//
// Dismissal: Esc key, backdrop click, or the close button (spec § 7.3).
export function Drawer({ open, onClose, title, children }: DrawerProps) {
  // Esc to close. Listener is only attached while open.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  return (
    <div
      // The outer container ignores pointer events when closed so the rest of
      // the page stays interactive; the panel re-enables them for itself.
      className={`fixed inset-0 z-50 ${open ? '' : 'pointer-events-none'}`}
      aria-hidden={!open}
    >
      {/* Backdrop — fades in, click to dismiss. */}
      <div
        onClick={onClose}
        className={`absolute inset-0 bg-black/50 transition-opacity duration-200 ${
          open ? 'opacity-100' : 'opacity-0'
        }`}
      />

      {/* Panel — slides in from the right. */}
      <aside
        className={`absolute right-0 top-0 flex h-full w-full max-w-md flex-col border-l border-zinc-800 bg-zinc-950 shadow-xl transition-transform duration-[250ms] ease-out ${
          open ? 'translate-x-0' : 'translate-x-full'
        }`}
      >
        <header className="flex items-start justify-between border-b border-zinc-800 px-4 py-3">
          <div className="min-w-0">{title}</div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="ml-3 shrink-0 rounded-md px-2 py-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
          >
            &times;
          </button>
        </header>
        <div className="flex-1 overflow-y-auto px-4 py-4">{children}</div>
      </aside>
    </div>
  );
}
