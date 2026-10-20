"use client";

import React, { createContext, useContext, useState, useCallback, useRef } from "react";
import { AlertTriangle, AlertCircle, CheckCircle2, Info, X } from "lucide-react";

export type DialogVariant = "danger" | "warning" | "info" | "success";

export interface ConfirmOptions {
  title?: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: DialogVariant;
}

export interface AlertOptions {
  title?: string;
  message: string;
  confirmText?: string;
  variant?: DialogVariant;
}

export interface ToastItem {
  id: string;
  message: string;
  variant: DialogVariant;
}

interface DialogContextType {
  confirm: (options: ConfirmOptions | string) => Promise<boolean>;
  alert: (options: AlertOptions | string) => Promise<void>;
  toast: (message: string, variant?: DialogVariant) => void;
}

const DialogContext = createContext<DialogContextType | undefined>(undefined);

export function DialogProvider({ children }: { children: React.ReactNode }) {
  // Confirm/Alert state
  const [dialogState, setDialogState] = useState<{
    isOpen: boolean;
    isConfirm: boolean;
    title: string;
    message: string;
    confirmText: string;
    cancelText: string;
    variant: DialogVariant;
  }>({
    isOpen: false,
    isConfirm: true,
    title: "",
    message: "",
    confirmText: "Xác nhận",
    cancelText: "Hủy",
    variant: "info",
  });

  const resolverRef = useRef<((value: boolean) => void) | null>(null);

  // Toast state
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const toast = useCallback((message: string, variant: DialogVariant = "info") => {
    const id = Math.random().toString(36).substring(2, 9);
    setToasts((prev) => [...prev, { id, message, variant }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 4000);
  }, []);

  const confirm = useCallback((options: ConfirmOptions | string): Promise<boolean> => {
    const opts: ConfirmOptions = typeof options === "string" ? { message: options } : options;
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
      setDialogState({
        isOpen: true,
        isConfirm: true,
        title: opts.title || (opts.variant === "danger" ? "Xác nhận hành động" : "Xác nhận"),
        message: opts.message,
        confirmText: opts.confirmText || (opts.variant === "danger" ? "Xác nhận xóa / hủy" : "Đồng ý"),
        cancelText: opts.cancelText || "Hủy bỏ",
        variant: opts.variant || "info",
      });
    });
  }, []);

  const alert = useCallback((options: AlertOptions | string): Promise<void> => {
    const opts: AlertOptions = typeof options === "string" ? { message: options } : options;
    return new Promise<void>((resolve) => {
      resolverRef.current = () => resolve();
      setDialogState({
        isOpen: true,
        isConfirm: false,
        title: opts.title || (opts.variant === "danger" ? "Đã xảy ra lỗi" : opts.variant === "success" ? "Thành công" : "Thông báo"),
        message: opts.message,
        confirmText: opts.confirmText || "Đã hiểu",
        cancelText: "",
        variant: opts.variant || (opts.title?.includes("lỗi") || opts.title?.includes("Lỗi") ? "danger" : "info"),
      });
    });
  }, []);

  const handleClose = (result: boolean) => {
    setDialogState((prev) => ({ ...prev, isOpen: false }));
    if (resolverRef.current) {
      resolverRef.current(result);
      resolverRef.current = null;
    }
  };

  const renderIcon = (variant: DialogVariant) => {
    switch (variant) {
      case "danger":
        return (
          <div className="w-12 h-12 rounded-2xl bg-red-500/15 border border-red-500/30 flex items-center justify-center text-red-400 shrink-0 shadow-lg shadow-red-500/10">
            <AlertTriangle className="w-6 h-6 animate-pulse" />
          </div>
        );
      case "warning":
        return (
          <div className="w-12 h-12 rounded-2xl bg-amber-500/15 border border-amber-500/30 flex items-center justify-center text-amber-400 shrink-0 shadow-lg shadow-amber-500/10">
            <AlertCircle className="w-6 h-6" />
          </div>
        );
      case "success":
        return (
          <div className="w-12 h-12 rounded-2xl bg-emerald-500/15 border border-emerald-500/30 flex items-center justify-center text-emerald-400 shrink-0 shadow-lg shadow-emerald-500/10">
            <CheckCircle2 className="w-6 h-6" />
          </div>
        );
      default:
        return (
          <div className="w-12 h-12 rounded-2xl bg-blue-500/15 border border-blue-500/30 flex items-center justify-center text-blue-400 shrink-0 shadow-lg shadow-blue-500/10">
            <Info className="w-6 h-6" />
          </div>
        );
    }
  };

  const getConfirmButtonClasses = (variant: DialogVariant) => {
    switch (variant) {
      case "danger":
        return "bg-gradient-to-r from-red-600 to-rose-600 hover:from-red-500 hover:to-rose-500 text-white shadow-lg shadow-red-600/30 border border-red-500/40";
      case "warning":
        return "bg-gradient-to-r from-amber-600 to-orange-600 hover:from-amber-500 hover:to-orange-500 text-white shadow-lg shadow-amber-600/30 border border-amber-500/40";
      case "success":
        return "bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white shadow-lg shadow-emerald-600/30 border border-emerald-500/40";
      default:
        return "bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 text-white shadow-lg shadow-blue-600/30 border border-blue-500/40";
    }
  };

  return (
    <DialogContext.Provider value={{ confirm, alert, toast }}>
      {children}

      {/* Modern Confirm / Alert Box Modal */}
      {dialogState.isOpen && (
        <div
          className="fixed inset-0 z-[100] flex items-center justify-center p-4 bg-black/60 backdrop-blur-md animate-in fade-in duration-200"
          onClick={() => handleClose(false)}
        >
          <div
            className="relative w-full max-w-md bg-white dark:bg-[#111827] border border-slate-200 dark:border-slate-700/80 rounded-2xl shadow-2xl p-6 text-slate-800 dark:text-slate-100 overflow-hidden transform transition-all scale-100 animate-in zoom-in-95 duration-200"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Ambient background glow */}
            <div
              className={`absolute -top-24 -left-24 w-48 h-48 rounded-full blur-3xl pointer-events-none opacity-20 ${
                dialogState.variant === "danger"
                  ? "bg-red-500"
                  : dialogState.variant === "warning"
                  ? "bg-amber-500"
                  : dialogState.variant === "success"
                  ? "bg-emerald-500"
                  : "bg-blue-500"
              }`}
            />

            <div className="relative flex items-start gap-4">
              {renderIcon(dialogState.variant)}
              <div className="flex-1 min-w-0 pt-0.5">
                <h3 className="text-lg font-bold text-slate-900 dark:text-white tracking-tight mb-1">
                  {dialogState.title}
                </h3>
                <p className="text-sm text-slate-600 dark:text-slate-300 leading-relaxed whitespace-pre-line">
                  {dialogState.message}
                </p>
              </div>
            </div>

            {/* Action buttons */}
            <div className="mt-6 flex items-center justify-end gap-3">
              {dialogState.isConfirm && (
                <button
                  type="button"
                  onClick={() => handleClose(false)}
                  className="px-4 py-2.5 rounded-xl text-sm font-medium text-slate-700 dark:text-slate-300 bg-slate-100 hover:bg-slate-200 dark:bg-slate-800/90 dark:hover:bg-slate-700/90 border border-slate-200 dark:border-slate-700/80 transition-all hover:scale-[1.02] active:scale-[0.98]"
                >
                  {dialogState.cancelText}
                </button>
              )}
              <button
                type="button"
                autoFocus
                onClick={() => handleClose(true)}
                className={`px-5 py-2.5 rounded-xl text-sm font-semibold transition-all hover:scale-[1.02] active:scale-[0.98] ${getConfirmButtonClasses(
                  dialogState.variant
                )}`}
              >
                {dialogState.confirmText}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Floating Toast Notifications */}
      {toasts.length > 0 && (
        <div className="fixed bottom-6 right-6 z-[110] flex flex-col gap-2 pointer-events-none max-w-sm w-full">
          {toasts.map((t) => (
            <div
              key={t.id}
              className={`pointer-events-auto flex items-center gap-3 p-4 rounded-xl border backdrop-blur-lg shadow-2xl animate-in slide-in-from-bottom-5 duration-200 ${
                t.variant === "success"
                  ? "bg-emerald-50 dark:bg-emerald-950/90 border-emerald-200 dark:border-emerald-500/40 text-emerald-800 dark:text-emerald-200"
                  : t.variant === "danger"
                  ? "bg-red-50 dark:bg-red-950/90 border-red-200 dark:border-red-500/40 text-red-800 dark:text-red-200"
                  : t.variant === "warning"
                  ? "bg-amber-50 dark:bg-amber-950/90 border-amber-200 dark:border-amber-500/40 text-amber-800 dark:text-amber-200"
                  : "bg-white dark:bg-slate-900/90 border-blue-200 dark:border-blue-500/40 text-blue-900 dark:text-blue-200"
              }`}
            >
              {t.variant === "success" && <CheckCircle2 className="w-5 h-5 text-emerald-600 dark:text-emerald-400 shrink-0" />}
              {t.variant === "danger" && <AlertTriangle className="w-5 h-5 text-red-600 dark:text-red-400 shrink-0" />}
              {t.variant === "warning" && <AlertCircle className="w-5 h-5 text-amber-600 dark:text-amber-400 shrink-0" />}
              {t.variant === "info" && <Info className="w-5 h-5 text-blue-600 dark:text-blue-400 shrink-0" />}
              <span className="text-sm font-medium flex-1">{t.message}</span>
            </div>
          ))}
        </div>
      )}
    </DialogContext.Provider>
  );
}

export function useDialog() {
  const context = useContext(DialogContext);
  if (!context) {
    throw new Error("useDialog must be used within a DialogProvider");
  }
  return context;
}
