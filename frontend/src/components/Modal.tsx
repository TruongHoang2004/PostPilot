"use client";
import { ReactNode, useEffect, useRef } from "react";

/** Native modal supplies focus trapping, Escape handling and focus restoration. */
/** dismissOnBackdrop=false keeps long forms from closing on a stray mouse release over the backdrop (Escape still asks onClose). */
export function Modal({ children, onClose, labelledBy, dismissOnBackdrop = true }: { children: ReactNode; onClose: () => void; labelledBy: string; dismissOnBackdrop?: boolean }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => { if (dialog?.open) dialog.close(); };
  }, []);
  return <dialog ref={ref} className="dispatch-native-modal" aria-labelledby={labelledBy}
    onCancel={event => { event.preventDefault(); onClose(); }}
    onClick={event => { if (dismissOnBackdrop && event.target === event.currentTarget) onClose(); }}>
    {children}
  </dialog>;
}
