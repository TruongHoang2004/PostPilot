import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        slate: { 50: "#f6f7f8", 100: "#f0f2f4", 200: "#dce1e6", 300: "#c5cdd6", 400: "#8593a3", 500: "#637181", 600: "#4d5b6c", 700: "#354454", 800: "#303b47", 900: "#1d232a", 950: "#15191e" },
        blue: { 50: "#edf2fc", 100: "#dce7fc", 200: "#c6d9fc", 300: "#a8c4ff", 400: "#86acfa", 500: "#4a78d7", 600: "#2554b8", 700: "#20479a", 800: "#1b3b7d", 900: "#1a325e", 950: "#172745" },
        background: "var(--background)",
        foreground: "var(--foreground)",
      },
    },
  },
  plugins: [],
};
export default config;
