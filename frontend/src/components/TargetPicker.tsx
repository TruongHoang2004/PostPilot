"use client";

import { Channel, ChannelGroup } from "@/lib/types";
import { Layers, Users } from "lucide-react";

export type TargetMode = "CHANNEL" | "MULTI" | "GROUP";
export interface TargetSelection { mode: TargetMode; channelId: string; channelIds: string[]; groupId: string }

export function defaultTarget(channels: Channel[], groups: ChannelGroup[]): TargetSelection {
  return { mode: "CHANNEL", channelId: channels[0]?.id || "", channelIds: [], groupId: groups[0]?.id || "" };
}

/** Channel ids the selection resolves to (used for suggestions and role checks). */
export function targetChannelIds(sel: TargetSelection, channels: Channel[], groups: ChannelGroup[]): string[] {
  if (sel.mode === "GROUP") return (groups.find(g => g.id === sel.groupId)?.channels || []).map(c => c.id);
  if (sel.mode === "MULTI") return sel.channelIds.filter(id => channels.some(c => c.id === id));
  return sel.channelId ? [sel.channelId] : [];
}

/** Request fields for the selection. */
export function targetPayload(sel: TargetSelection): { channel_id?: string; channel_ids?: string[]; channel_group_id?: string } {
  if (sel.mode === "GROUP") return { channel_group_id: sel.groupId || undefined };
  if (sel.mode === "MULTI") return { channel_ids: sel.channelIds };
  return { channel_id: sel.channelId || undefined };
}

/**
 * Re-validates a selection against freshly loaded channels/groups. Returns the SAME object when nothing
 * changed so effects keyed on the selection do not re-run. preferMulti picks "many Fanpages" the first time
 * channels arrive when more than one exists (bulk composer).
 */
export function reconcileTarget(prev: TargetSelection, channels: Channel[], groups: ChannelGroup[], preferMulti = false): TargetSelection {
  const firstLoad = !prev.channelId && prev.channelIds.length === 0 && channels.length > 0;
  const groupId = groups.some(g => g.id === prev.groupId) ? prev.groupId : groups[0]?.id || "";
  if (firstLoad && preferMulti && channels.length > 1) {
    return { ...prev, mode: "MULTI", channelId: channels[0].id, channelIds: [channels[0].id], groupId };
  }
  const channelId = channels.some(c => c.id === prev.channelId) ? prev.channelId : channels[0]?.id || "";
  const channelIds = prev.channelIds.filter(id => channels.some(c => c.id === id));
  const same = channelId === prev.channelId && groupId === prev.groupId && channelIds.length === prev.channelIds.length && channelIds.every((id, i) => id === prev.channelIds[i]);
  return same ? prev : { ...prev, channelId, groupId, channelIds };
}

export function targetIsComplete(sel: TargetSelection): boolean {
  if (sel.mode === "GROUP") return Boolean(sel.groupId);
  if (sel.mode === "MULTI") return sel.channelIds.length > 0;
  return Boolean(sel.channelId);
}

export function targetLabel(sel: TargetSelection, channels: Channel[], groups: ChannelGroup[]): string {
  if (sel.mode === "GROUP") return groups.find(g => g.id === sel.groupId)?.name || "Nhóm Fanpage";
  if (sel.mode === "MULTI") {
    const names = channels.filter(c => sel.channelIds.includes(c.id)).map(c => c.name);
    return names.length ? (names.length > 2 ? `${names.slice(0, 2).join(", ")} +${names.length - 2}` : names.join(", ")) : "Chọn Fanpage";
  }
  return channels.find(c => c.id === sel.channelId)?.name || "Chọn Fanpage";
}

interface TargetPickerProps { channels: Channel[]; channelGroups: ChannelGroup[]; value: TargetSelection; onChange: (next: TargetSelection) => void; idPrefix: string }

export function TargetPicker({ channels, channelGroups, value, onChange, idPrefix }: TargetPickerProps) {
  const modes: { id: TargetMode; label: string }[] = [{ id: "CHANNEL", label: "Một Fanpage" }, { id: "MULTI", label: "Nhiều Fanpage" }, { id: "GROUP", label: `Nhóm (${channelGroups.length})` }];
  const group = channelGroups.find(g => g.id === value.groupId);
  const toggleChannel = (id: string) => {
    const ids = value.channelIds.includes(id) ? value.channelIds.filter(x => x !== id) : [...value.channelIds, id];
    onChange({ ...value, channelIds: ids });
  };
  return <div className="target-picker">
    <div className="composer-row-head"><span className="composer-label" id={`${idPrefix}-target-label`}>Đăng lên</span>
      <div className="chip-row" role="group" aria-labelledby={`${idPrefix}-target-label`}>{modes.map(mode => <button key={mode.id} type="button" className="chip" aria-pressed={value.mode === mode.id} onClick={() => onChange({ ...value, mode: mode.id })}>{mode.label}</button>)}</div>
    </div>
    {value.mode === "CHANNEL" && (channels.length === 0 ? <p className="dispatch-note warning">Chưa có Fanpage nào được kết nối. Thêm Fanpage trong tab Fanpage trước.</p> :
      <label className="dispatch-field"><span className="sr-only">Fanpage</span><select id={`${idPrefix}-channel`} value={value.channelId} onChange={e => onChange({ ...value, channelId: e.target.value })}>{channels.map(c => <option key={c.id} value={c.id}>{c.name} · {c.token_status === "ACTIVE" ? "sẵn sàng" : c.token_status}</option>)}</select></label>)}
    {value.mode === "MULTI" && (channels.length === 0 ? <p className="dispatch-note warning">Chưa có Fanpage nào được kết nối.</p> :
      <div className="check-list" role="group" aria-label="Chọn nhiều Fanpage">
        {channels.map(c => <label key={c.id} className="check-item"><input type="checkbox" checked={value.channelIds.includes(c.id)} onChange={() => toggleChannel(c.id)} /><span>{c.name}</span><small>{c.token_status === "ACTIVE" ? "sẵn sàng" : c.token_status}</small></label>)}
        <div className="check-actions"><button type="button" onClick={() => onChange({ ...value, channelIds: channels.map(c => c.id) })}>Chọn tất cả</button><button type="button" onClick={() => onChange({ ...value, channelIds: [] })}>Bỏ chọn</button></div>
      </div>)}
    {value.mode === "GROUP" && (channelGroups.length === 0 ? <p className="dispatch-note warning">Chưa có nhóm Fanpage. Tạo nhóm trong tab Đội nhóm.</p> : <>
      <label className="dispatch-field"><span className="sr-only">Nhóm Fanpage</span><select id={`${idPrefix}-group`} value={value.groupId} onChange={e => onChange({ ...value, groupId: e.target.value })}>{channelGroups.map(g => <option key={g.id} value={g.id}>{g.name} ({g.channel_count ?? g.channels?.length ?? 0} Fanpage){g.require_approval ? " · cần duyệt" : ""}</option>)}</select></label>
      {group && <p className="dispatch-note"><Users size={14} aria-hidden="true" />Đăng cùng lúc lên {group.channel_count ?? group.channels?.length ?? 0} Fanpage. {group.require_approval ? "Nhóm này yêu cầu phê duyệt trước khi đăng." : "Nhóm này cho phép đăng trực tiếp."}</p>}
    </>)}
    {value.mode === "MULTI" && value.channelIds.length > 1 && <p className="dispatch-note"><Layers size={14} aria-hidden="true" />Mỗi bài sẽ được tạo riêng cho {value.channelIds.length} Fanpage.</p>}
  </div>;
}
