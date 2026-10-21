import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(dateStr?: string): string {
  if (!dateStr) return "-";
  try {
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return "-";
    return new Intl.DateTimeFormat("vi-VN", {
      dateStyle: "short",
      timeStyle: "medium",
    }).format(d);
  } catch {
    return dateStr;
  }
}

export function formatBytes(bytes?: number): string {
  if (!bytes || bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
}

export function getStatusBadgeClass(status: string): string {
  switch (status.toUpperCase()) {
    case "PUBLISHED":
    case "ACTIVE":
      return "bg-emerald-500/15 text-emerald-400 border border-emerald-500/30";
    case "SCHEDULED":
    case "QUEUED":
    case "RATE_LIMITED":
    case "WARNING":
      return "bg-amber-500/15 text-amber-400 border border-amber-500/30";
    case "PROCESSING":
      return "bg-sky-500/15 text-sky-400 border border-sky-500/30 animate-pulse";
    case "FAILED":
    case "EXPIRED":
      return "bg-rose-500/15 text-rose-400 border border-rose-500/30";
    case "CANCELLED":
    case "REVOKED":
      return "bg-slate-700/50 text-slate-400 border border-slate-600/30";
    default:
      return "bg-slate-800 text-slate-300 border border-slate-700";
  }
}
