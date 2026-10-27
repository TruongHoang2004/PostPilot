import Link from "next/link";
import type { ReactNode } from "react";

export const privacyContactEmail = "xuantruong.tn.1712@gmail.com";

export function PolicyContact({ subject }: { subject: string }) {
  return (
    <a
      href={`mailto:${privacyContactEmail}?subject=${encodeURIComponent(subject)}`}
      className="break-all font-medium text-blue-700 underline decoration-blue-300 underline-offset-4 hover:text-blue-900 dark:text-blue-300 dark:decoration-blue-700 dark:hover:text-blue-200"
    >
      {privacyContactEmail}
    </a>
  );
}

export function PolicyLayout({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-800 dark:bg-[#090d16] dark:text-slate-200">
      <header className="border-b border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950">
        <nav aria-label="Điều hướng chính sách" className="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-4 px-5 py-5 sm:px-8">
          <Link href="/" className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">
            PostPilot
          </Link>
          <Link href="/" className="text-sm font-medium text-blue-700 underline-offset-4 hover:underline dark:text-blue-300">
            Về trang chủ
          </Link>
        </nav>
      </header>

      <main id="main-content" className="mx-auto max-w-3xl px-5 py-10 sm:px-8 sm:py-14">
        <p className="mb-3 text-xs font-semibold uppercase tracking-widest text-blue-700 dark:text-blue-300">PostPilot · Thông tin dữ liệu</p>
        <h1 className="text-3xl font-bold leading-tight tracking-tight text-slate-950 dark:text-white sm:text-4xl">{title}</h1>
        <p className="mt-4 text-base leading-7 text-slate-600 dark:text-slate-400">{description}</p>
        <p className="mt-4 text-sm text-slate-500 dark:text-slate-400">
          Cập nhật: <time dateTime="2026-10-25">25/10/2026</time>
        </p>
        <article className="mt-8 space-y-8 border-t border-slate-200 pt-8 text-base leading-7 dark:border-slate-800 [&_h2]:mb-3 [&_h2]:text-xl [&_h2]:font-semibold [&_h2]:text-slate-950 dark:[&_h2]:text-white [&_p+p]:mt-3 [&_ul]:list-disc [&_ul]:space-y-2 [&_ul]:pl-6 [&_ol]:list-decimal [&_ol]:space-y-3 [&_ol]:pl-6">
          {children}
        </article>
      </main>

      <footer className="mx-auto max-w-3xl px-5 pb-10 sm:px-8">
        <nav aria-label="Thông tin pháp lý" className="flex flex-wrap gap-x-6 gap-y-3 border-t border-slate-200 pt-6 text-sm dark:border-slate-800">
          <Link href="/privacy" className="text-blue-700 underline-offset-4 hover:underline dark:text-blue-300">Chính sách quyền riêng tư</Link>
          <Link href="/terms" className="text-blue-700 underline-offset-4 hover:underline dark:text-blue-300">Điều khoản dịch vụ</Link>
          <Link href="/data-deletion" className="text-blue-700 underline-offset-4 hover:underline dark:text-blue-300">Yêu cầu xóa dữ liệu</Link>
        </nav>
      </footer>
    </div>
  );
}
