import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "PostPilot - Facebook Multi-Page Automation & Studio",
  description: "Bảng điều khiển đăng bài tự động đa Fanpage Facebook với Golang Backend, Redis Queue và AI Gemini",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="vi" className="dark">
      <body className="antialiased min-h-screen text-slate-100 selection:bg-blue-600 selection:text-white font-sans">
        {children}
      </body>
    </html>
  );
}
