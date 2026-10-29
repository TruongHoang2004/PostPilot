"use client";
import { useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";

export function AuthGate() {
  const { refreshOrgs, error: workspaceError, logout } = useOrg();
  const [register, setRegister] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return <main className="dispatch-app auth-layout min-h-screen grid md:grid-cols-2">
    <section className="auth-intro bg-blue-900 text-white p-8 flex flex-col justify-between gap-12"><span className="dispatch-brand text-xl font-bold">POSTPILOT ↗</span><div><p className="dispatch-eyebrow text-xs uppercase tracking-widest text-blue-200 mb-5">Nội dung có kế hoạch</p><h1 className="text-4xl sm:text-6xl font-medium tracking-tight">Mỗi bài viết.<br />Đúng thời điểm.</h1><p className="auth-description mt-6 max-w-sm text-blue-100 leading-relaxed">Soạn nội dung, cộng tác cùng đội nhóm và quản lý lịch đăng Facebook trong một không gian.</p></div><span className="text-xs">Lên lịch. Kết nối. Chia sẻ.</span></section>
    <section className="auth-form-area flex items-center justify-center p-6 sm:p-12 bg-white dark:bg-slate-900">
    <div className="w-full max-w-sm">
      <h2 className="text-3xl font-medium tracking-tight">{register ? "Bắt đầu với PostPilot." : "Chào mừng trở lại."}</h2>
      <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">{register ? "Tạo tài khoản và workspace của bạn." : "Đăng nhập để quản lý Page và lịch đăng của bạn."}</p>
      {(error || workspaceError) && <p role="alert" className="mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-800">{error || workspaceError}</p>}
      {api.getToken() && workspaceError ? <div className="mt-5 flex gap-4"><button onClick={() => void refreshOrgs()} className="text-blue-600">Thử lại</button><button onClick={logout}>Đăng xuất</button></div> :
      <form className="mt-6 space-y-4" onSubmit={async e => {
        e.preventDefault(); setBusy(true); setError("");
        try { if (register) await api.register(email.trim(), password, name.trim()); else await api.login(email.trim(), password); await refreshOrgs(); }
        catch (e) { setError(e instanceof Error ? e.message : "Không thể đăng nhập"); }
        finally { setBusy(false); }
      }}>
        {register && <label className="dispatch-field block text-sm">Tên của bạn<input value={name} onChange={e => setName(e.target.value)} autoComplete="name" required className="mt-1 w-full rounded-md border p-3" /></label>}
        <label className="dispatch-field block text-sm">Email<input type="email" value={email} onChange={e => setEmail(e.target.value)} autoComplete="email" required className="mt-1 w-full rounded-md border p-3" /></label>
        <label className="dispatch-field block text-sm">Mật khẩu<input type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete={register ? "new-password" : "current-password"} minLength={register ? 12 : undefined} required className="mt-1 w-full rounded-md border p-3" /></label>
        {register && <p className="text-xs text-slate-500">Ít nhất 12 ký tự. Tài khoản mới có workspace riêng; dữ liệu cũ không tự chuyển sang.</p>}
        <button disabled={busy} className="dispatch-button primary w-full bg-blue-600 p-3 rounded-md font-medium text-white">{busy ? "Đang xử lý…" : register ? "Tạo tài khoản" : "Đăng nhập"}</button>
        <button type="button" onClick={() => { setRegister(!register); setError(""); }} className="w-full text-sm text-blue-600">{register ? "Đã có tài khoản? Đăng nhập" : "Chưa có tài khoản? Đăng ký"}</button>
      </form>}
      <nav className="mt-8 flex flex-wrap gap-x-4 gap-y-2 text-xs dispatch-muted"><Link href="/privacy">Quyền riêng tư</Link><Link href="/terms">Điều khoản</Link><Link href="/data-deletion">Xóa dữ liệu</Link></nav>
    </div>
    </section>
  </main>;
}
