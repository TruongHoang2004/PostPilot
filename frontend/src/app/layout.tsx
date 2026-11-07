import type { Metadata } from "next";
import "./globals.css";
import { DialogProvider } from "@/context/DialogContext";
import { ThemeProvider } from "@/context/ThemeContext";
import { OrgProvider } from "@/context/OrgContext";

export const metadata: Metadata = {
  title: "PostPilot — Lịch đăng & Nội dung",
  description: "Quản lý nội dung, phê duyệt và lên lịch đăng bài Facebook trong cùng một không gian làm việc.",
  icons: {
    icon: "/logo.svg",
    apple: "/logo.svg",
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="vi" suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `
              (function() {
                try {
                  var saved = localStorage.getItem('postpilot-theme');
                  if (saved === 'light') {
                    document.documentElement.classList.remove('dark');
                    document.documentElement.classList.add('light');
                  } else if (saved === 'dark') {
                    document.documentElement.classList.add('dark');
                    document.documentElement.classList.remove('light');
                  } else {
                    document.documentElement.classList.add('light');
                  }
                } catch (e) {}
              })();
            `,
          }}
        />
      </head>
      <body className="antialiased min-h-screen text-slate-800 dark:text-slate-100 selection:bg-blue-600 selection:text-white font-sans">
        <ThemeProvider>
          <OrgProvider>
            <DialogProvider>{children}</DialogProvider>
          </OrgProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
