import type { Metadata } from "next";
import "./globals.css";
import { DialogProvider } from "@/context/DialogContext";
import { ThemeProvider } from "@/context/ThemeContext";

export const metadata: Metadata = {
  title: "PostPilot - Facebook Multi-Page Automation & Studio",
  description: "Bảng điều khiển đăng bài tự động đa Fanpage Facebook với Golang Backend, Redis Queue và AI Gemini",
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
                  var prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
                  if (saved === 'light') {
                    document.documentElement.classList.remove('dark');
                    document.documentElement.classList.add('light');
                  } else if (saved === 'dark') {
                    document.documentElement.classList.add('dark');
                    document.documentElement.classList.remove('light');
                  } else if (prefersDark) {
                    document.documentElement.classList.add('dark');
                  } else {
                    document.documentElement.classList.add('dark');
                  }
                } catch (e) {}
              })();
            `,
          }}
        />
      </head>
      <body className="antialiased min-h-screen text-slate-800 dark:text-slate-100 selection:bg-blue-600 selection:text-white font-sans">
        <ThemeProvider>
          <DialogProvider>{children}</DialogProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}

