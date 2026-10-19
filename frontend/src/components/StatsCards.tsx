"use client";

import React from "react";
import { SystemStats } from "@/lib/types";
import { Flag, Calendar, CheckCircle2, AlertTriangle, Layers } from "lucide-react";

interface StatsCardsProps {
  stats: SystemStats | null;
}

export function StatsCards({ stats }: StatsCardsProps) {
  const cards = [
    {
      title: "Fanpage Đang Kết Nối",
      value: stats ? `${stats.active_pages} / ${stats.total_pages}` : "0 / 0",
      subtext: "Hoạt động / Tổng số Fanpage",
      icon: Flag,
      color: "from-blue-600/20 to-blue-500/5",
      borderColor: "border-blue-500/20",
      textColor: "text-blue-400",
      iconBg: "bg-blue-500/10 text-blue-400",
    },
    {
      title: "Bài Viết Đang Hẹn Giờ",
      value: stats?.scheduled_posts ?? 0,
      subtext: "Đang chờ đến giờ phát sóng",
      icon: Calendar,
      color: "from-amber-600/20 to-amber-500/5",
      borderColor: "border-amber-500/20",
      textColor: "text-amber-400",
      iconBg: "bg-amber-500/10 text-amber-400",
    },
    {
      title: "Đã Đăng Thành Công",
      value: stats?.published_posts ?? 0,
      subtext: "Lên Meta Graph API chuẩn",
      icon: CheckCircle2,
      color: "from-emerald-600/20 to-emerald-500/5",
      borderColor: "border-emerald-500/20",
      textColor: "text-emerald-400",
      iconBg: "bg-emerald-500/10 text-emerald-400",
    },
    {
      title: "Lỗi / Đang Retry",
      value: stats?.failed_posts ?? 0,
      subtext: "Tự động Exponential Backoff",
      icon: AlertTriangle,
      color: "from-rose-600/20 to-rose-500/5",
      borderColor: "border-rose-500/20",
      textColor: "text-rose-400",
      iconBg: "bg-rose-500/10 text-rose-400",
    },
    {
      title: "Fast / Heavy Queue",
      value: stats ? `${stats.fast_queue_depth ?? 0} / ${stats.heavy_queue_depth ?? 0}` : "0 / 0",
      subtext: "Text, Link / Media, Video",
      icon: Layers,
      color: "from-purple-600/20 to-purple-500/5",
      borderColor: "border-purple-500/20",
      textColor: "text-purple-400",
      iconBg: "bg-purple-500/10 text-purple-400",
    },
  ];

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-3 sm:gap-4 mb-6">
      {cards.map((card, idx) => {
        const Icon = card.icon;
        return (
          <div
            key={idx}
            className={`relative overflow-hidden rounded-xl bg-gradient-to-br ${card.color} border ${card.borderColor} p-4 backdrop-blur-sm transition-all hover:translate-y-[-2px] hover:shadow-lg`}
          >
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium text-slate-400 line-clamp-1">{card.title}</span>
              <div className={`p-1.5 rounded-lg ${card.iconBg}`}>
                <Icon className="h-4 w-4" />
              </div>
            </div>
            <div className={`text-2xl sm:text-3xl font-extrabold tracking-tight ${card.textColor}`}>
              {card.value}
            </div>
            <p className="text-[11px] text-slate-500 mt-1 line-clamp-1">{card.subtext}</p>
          </div>
        );
      })}
    </div>
  );
}
