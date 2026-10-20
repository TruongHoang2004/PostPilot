import React from "react";

interface LogoProps {
  className?: string;
  size?: number;
  withText?: boolean;
  theme?: "dark" | "light" | "auto";
}

export function PostPilotLogo({
  className = "h-9 w-9",
  size,
  withText = false,
  theme = "dark",
}: LogoProps) {
  const isLight = theme === "light";

  const icon = (
    <svg
      viewBox="0 0 512 512"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      style={size ? { width: size, height: size } : undefined}
    >
      <defs>
        {isLight ? (
          <>
            <radialGradient id="ppBgLight" cx="50%" cy="30%" r="70%">
              <stop offset="0%" stopColor="#ffffff" />
              <stop offset="70%" stopColor="#f8fafc" />
              <stop offset="100%" stopColor="#f1f5f9" />
            </radialGradient>
            <linearGradient id="ppPrimaryLight" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#0284c7" />
              <stop offset="45%" stopColor="#2563eb" />
              <stop offset="100%" stopColor="#7c3aed" />
            </linearGradient>
            <linearGradient id="ppAccentLight" x1="10%" y1="0%" x2="90%" y2="100%">
              <stop offset="0%" stopColor="#0891b2" />
              <stop offset="100%" stopColor="#1d4ed8" />
            </linearGradient>
            <linearGradient id="ppTrailLight" x1="0%" y1="50%" x2="100%" y2="50%">
              <stop offset="0%" stopColor="#0284c7" stopOpacity="0.1" />
              <stop offset="50%" stopColor="#3b82f6" stopOpacity="0.25" />
              <stop offset="100%" stopColor="#8b5cf6" stopOpacity="0.45" />
            </linearGradient>
            <filter id="ppShadowLight" x="-15%" y="-15%" width="130%" height="135%">
              <feDropShadow dx="0" dy="16" stdDeviation="16" floodColor="#1e3a8a" floodOpacity="0.18" />
              <feDropShadow dx="0" dy="4" stdDeviation="6" floodColor="#1e3a8a" floodOpacity="0.1" />
            </filter>
          </>
        ) : (
          <>
            <radialGradient id="ppLogoBgDark" cx="50%" cy="40%" r="65%">
              <stop offset="0%" stopColor="#1e1b4b" stopOpacity="0.9" />
              <stop offset="100%" stopColor="#090d1a" stopOpacity="1" />
            </radialGradient>
            <linearGradient id="ppPrimaryDark" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#38bdf8" />
              <stop offset="45%" stopColor="#3b82f6" />
              <stop offset="100%" stopColor="#8b5cf6" />
            </linearGradient>
            <linearGradient id="ppAccentDark" x1="20%" y1="0%" x2="90%" y2="100%">
              <stop offset="0%" stopColor="#06b6d4" />
              <stop offset="100%" stopColor="#2563eb" />
            </linearGradient>
            <linearGradient id="ppTrailDark" x1="0%" y1="50%" x2="100%" y2="50%">
              <stop offset="0%" stopColor="#38bdf8" stopOpacity="0" />
              <stop offset="60%" stopColor="#60a5fa" stopOpacity="0.3" />
              <stop offset="100%" stopColor="#a855f7" stopOpacity="0.8" />
            </linearGradient>
            <filter id="ppShadowDark" x="-10%" y="-10%" width="120%" height="130%">
              <feDropShadow dx="0" dy="12" stdDeviation="14" floodColor="#0284c7" floodOpacity="0.35" />
            </filter>
          </>
        )}

        <linearGradient id="ppLightGlint" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stopColor="#ffffff" stopOpacity="0.95" />
          <stop offset="100%" stopColor="#bfdbfe" stopOpacity="0.25" />
        </linearGradient>
      </defs>

      {/* Squircle Background Base */}
      <rect
        width="512"
        height="512"
        rx="128"
        fill={isLight ? "url(#ppBgLight)" : "url(#ppLogoBgDark)"}
      />
      <rect
        width="508"
        height="508"
        x="2"
        y="2"
        rx="126"
        stroke={isLight ? "#e2e8f0" : "url(#ppPrimaryDark)"}
        strokeWidth={isLight ? "2.5" : "3"}
        strokeOpacity={isLight ? "1" : "0.4"}
      />

      {/* Orbit Track */}
      <circle
        cx="256"
        cy="256"
        r="170"
        stroke={isLight ? "url(#ppTrailLight)" : "url(#ppTrailDark)"}
        strokeWidth="3.5"
        strokeDasharray="12 16"
        opacity={isLight ? 0.8 : 0.45}
        transform="rotate(-25 256 256)"
      />

      {/* Main Origami Pilot Jet / Social Dispatch Symbol */}
      <g
        filter={isLight ? "url(#ppShadowLight)" : "url(#ppShadowDark)"}
        transform="translate(20, -10)"
      >
        <path
          d="M 140 330 C 180 320 220 295 240 270 L 190 275 Z"
          fill="#6366f1"
          opacity={isLight ? 0.7 : 0.5}
        />
        <path
          d="M 155 350 L 255 130 L 245 285 Z"
          fill={isLight ? "url(#ppLightAccent)" : "url(#ppAccentDark)"}
          opacity={0.95}
        />
        <path
          d="M 255 130 L 375 255 L 275 295 L 245 285 Z"
          fill={isLight ? "url(#ppPrimaryLight)" : "url(#ppPrimaryDark)"}
        />
        <path
          d="M 245 285 L 275 295 L 235 375 Z"
          fill={isLight ? "#312e81" : "#4338ca"}
        />

        {/* Highlights */}
        <path
          d="M 255 130 L 155 350 L 245 285 Z"
          stroke="url(#ppLightGlint)"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          opacity={0.8}
        />
        <path
          d="M 255 130 L 375 255"
          stroke="#ffffff"
          strokeWidth="3.5"
          strokeLinecap="round"
          opacity={0.9}
        />

        {/* Cockpit AI Node */}
        <circle cx="255" cy="130" r="6" fill="#ffffff" />
        <circle cx="255" cy="130" r="3.5" fill={isLight ? "#0284c7" : "#38bdf8"} />

        {/* Sonic Automation Broadcast Waves */}
        <path
          d="M 320 185 C 345 195 365 215 375 240"
          stroke={isLight ? "#0284c7" : "#38bdf8"}
          strokeWidth="4.5"
          strokeLinecap="round"
          strokeDasharray="1 1"
          opacity={0.9}
        />
        <path
          d="M 345 165 C 380 180 405 210 415 245"
          stroke={isLight ? "#6366f1" : "#818cf8"}
          strokeWidth="3.5"
          strokeLinecap="round"
          opacity={0.7}
        />
      </g>
    </svg>
  );

  if (!withText) {
    return icon;
  }

  return (
    <div className="flex items-center gap-3">
      <div className="relative group flex items-center justify-center">{icon}</div>
      <div>
        <div className="flex items-center gap-2">
          <span
            className={`font-extrabold text-lg tracking-tight ${
              isLight
                ? "text-slate-900"
                : "bg-gradient-to-r from-white via-slate-100 to-slate-300 bg-clip-text text-transparent"
            }`}
          >
            PostPilot
          </span>
          <span
            className={`text-[10px] font-bold px-1.5 py-0.5 rounded tracking-wider ${
              isLight
                ? "bg-blue-100 text-blue-700 border border-blue-200"
                : "bg-blue-500/15 text-blue-400 border border-blue-500/30"
            }`}
          >
            STUDIO
          </span>
        </div>
        <p
          className={`text-xs hidden sm:block ${
            isLight ? "text-slate-500" : "text-slate-400"
          }`}
        >
          Hệ thống Tự động hóa Facebook Multi-Page & Content Studio
        </p>
      </div>
    </div>
  );
}

export default PostPilotLogo;
