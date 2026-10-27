"use client";

import React, { useState, useEffect, useCallback } from "react";
import {
  Channel,
  ChannelGroup,
  ChannelGroupPermission,
  GroupRole,
  OrganizationMember,
  OrgRole,
  AuditLog,
} from "@/lib/types";
import { api } from "@/lib/api";
import { useOrg } from "@/context/OrgContext";
import { useDialog } from "@/context/DialogContext";
import {
  Users,
  Layers,
  UserPlus,
  Plus,
  Trash2,
  Shield,
  ShieldCheck,
  CheckCircle2,
  Clock,
  RefreshCw,
  FolderTree,
  Sliders,
  X,
  History,
  Check,
} from "lucide-react";

export function TeamAndGroupsManager() {
  const { currentOrg, canManageOrg, channelGroups, refreshGroups } = useOrg();
  const { confirm, toast } = useDialog();

  const [activeTab, setActiveTab] = useState<"members" | "groups">("members");

  // Members state
  const [members, setMembers] = useState<OrganizationMember[]>([]);
  const [loadingMembers, setLoadingMembers] = useState<boolean>(true);
  const [newMemberEmail, setNewMemberEmail] = useState<string>("");
  const [newMemberRole, setNewMemberRole] = useState<OrgRole>("MEMBER");
  const [isAddingMember, setIsAddingMember] = useState<boolean>(false);
  const [showAddMemberModal, setShowAddMemberModal] = useState<boolean>(false);
  const [invitationTokenResult, setInvitationTokenResult] = useState<{ email: string; token: string } | null>(null);

  // Channels & Groups state
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loadingChannels, setLoadingChannels] = useState<boolean>(true);

  // Audit Logs state
  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);
  const [loadingLogs, setLoadingLogs] = useState<boolean>(false);

  // Create Group Modal
  const [showCreateGroupModal, setShowCreateGroupModal] = useState<boolean>(false);
  const [newGroupName, setNewGroupName] = useState<string>("");
  const [newGroupDesc, setNewGroupDesc] = useState<string>("");
  const [newGroupColor, setNewGroupColor] = useState<string>("blue");
  const [newGroupRequireApproval, setNewGroupRequireApproval] = useState<boolean>(false);
  const [isCreatingGroup, setIsCreatingGroup] = useState<boolean>(false);

  // Group Details / Config Modal
  const [configGroup, setConfigGroup] = useState<ChannelGroup | null>(null);
  const [selectedGroupChannelIds, setSelectedGroupChannelIds] = useState<string[]>([]);
  const [groupPermissions, setGroupPermissions] = useState<ChannelGroupPermission[]>([]);
  const [loadingGroupPerms, setLoadingGroupPerms] = useState<boolean>(false);
  const [isSavingChannels, setIsSavingChannels] = useState<boolean>(false);

  // Add Permission Modal/Inline
  const [assignUserId, setAssignUserId] = useState<string>("");
  const [assignGroupRole, setAssignGroupRole] = useState<GroupRole>("EDITOR");
  const [isAssigningPerm, setIsAssigningPerm] = useState<boolean>(false);

  // 1. Fetch Members
  const fetchMembers = useCallback(async () => {
    if (!currentOrg) return;
    setLoadingMembers(true);
    try {
      const data = await api.getOrgMembers(currentOrg.id);
      setMembers(data);
    } catch (err) {
      console.warn("Lỗi tải thành viên:", err);
    } finally {
      setLoadingMembers(false);
    }
  }, [currentOrg]);

  // 2. Fetch Channels
  const fetchChannels = useCallback(async () => {
    setLoadingChannels(true);
    try {
      const data = await api.getChannels();
      setChannels(data);
    } catch (err) {
      console.warn("Lỗi tải danh sách kênh:", err);
    } finally {
      setLoadingChannels(false);
    }
  }, []);

  // 3. Fetch Audit Logs
  const fetchAuditLogs = useCallback(async () => {
    setLoadingLogs(true);
    try {
      const logs = await api.getAuditLogs(15);
      setAuditLogs(logs);
    } catch (err) {
      console.warn("Lỗi tải audit logs:", err);
    } finally {
      setLoadingLogs(false);
    }
  }, []);

  useEffect(() => {
    fetchMembers();
    fetchChannels();
    fetchAuditLogs();
  }, [fetchMembers, fetchChannels, fetchAuditLogs]);

  // Handle Add Member
  const handleAddMember = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentOrg) return;
    if (!newMemberEmail.trim()) {
      toast("Vui lòng nhập email thành viên", "warning");
      return;
    }

    setIsAddingMember(true);
    try {
      const res = await api.addOrgMember(currentOrg.id, newMemberEmail.trim(), newMemberRole);
      if (res.token) {
        setInvitationTokenResult({ email: newMemberEmail.trim(), token: res.token });
        toast("✉️ Đã tạo lời mời thành viên thành công!", "success");
      } else {
        toast("🎉 Đã thêm thành viên mới vào tổ chức!", "success");
        setShowAddMemberModal(false);
      }
      setNewMemberEmail("");
      fetchMembers();
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi khi thêm thành viên", "danger");
    } finally {
      setIsAddingMember(false);
    }
  };

  // Handle Remove Member
  const handleRemoveMember = async (memberId: string, email?: string) => {
    if (!currentOrg) return;
    const ok = await confirm({
      title: "Xác nhận xóa thành viên",
      message: `Bạn có chắc chắn muốn xóa thành viên ${email || ""} khỏi tổ chức ${currentOrg.name}?`,
      confirmText: "Xóa thành viên",
      variant: "danger",
    });
    if (!ok) return;

    try {
      await api.removeOrgMember(currentOrg.id, memberId);
      toast("Đã xóa thành viên khỏi tổ chức", "success");
      fetchMembers();
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi khi xóa thành viên", "danger");
    }
  };

  // Handle Create Channel Group
  const handleCreateGroup = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newGroupName.trim()) {
      toast("Vui lòng nhập tên cụm kênh", "warning");
      return;
    }

    setIsCreatingGroup(true);
    try {
      await api.createChannelGroup({
        name: newGroupName.trim(),
        description: newGroupDesc.trim(),
        color_code: newGroupColor,
        require_approval: newGroupRequireApproval,
      });

      toast("✨ Đã tạo Cụm kênh mới thành công!", "success");
      setShowCreateGroupModal(false);
      setNewGroupName("");
      setNewGroupDesc("");
      setNewGroupColor("blue");
      setNewGroupRequireApproval(false);
      refreshGroups();
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi khi tạo cụm kênh", "danger");
    } finally {
      setIsCreatingGroup(false);
    }
  };

  // Handle Delete Channel Group
  const handleDeleteGroup = async (groupId: string, name: string) => {
    const ok = await confirm({
      title: "Xóa Cụm kênh phát sóng",
      message: `Bạn có chắc muốn xóa cụm "${name}"? Các kênh bên trong sẽ không bị xóa.`,
      confirmText: "Xóa Cụm",
      variant: "danger",
    });
    if (!ok) return;

    try {
      await api.deleteChannelGroup(groupId);
      toast("Đã xóa cụm kênh", "success");
      refreshGroups();
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi xóa cụm kênh", "danger");
    }
  };

  // Open Config Modal for Group
  const openConfigModal = async (group: ChannelGroup) => {
    setConfigGroup(group);
    setSelectedGroupChannelIds(group.channels?.map((c) => c.id) || []);
    setLoadingGroupPerms(true);
    try {
      const perms = await api.getChannelGroupPermissions(group.id);
      setGroupPermissions(perms);
    } catch (err) {
      console.warn("Lỗi tải phân quyền cụm:", err);
      setGroupPermissions([]);
    } finally {
      setLoadingGroupPerms(false);
    }
  };

  // Save Channels in Group
  const handleSaveGroupChannels = async () => {
    if (!configGroup) return;
    setIsSavingChannels(true);
    try {
      await api.setChannelGroupChannels(configGroup.id, selectedGroupChannelIds);
      toast("✅ Đã cập nhật danh sách kênh trong cụm!", "success");
      refreshGroups();
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi cập nhật kênh trong cụm", "danger");
    } finally {
      setIsSavingChannels(false);
    }
  };

  // Toggle channel in selectedGroupChannelIds
  const toggleChannelSelection = (chId: string) => {
    setSelectedGroupChannelIds((prev) =>
      prev.includes(chId) ? prev.filter((id) => id !== chId) : [...prev, chId]
    );
  };

  // Assign Group Permission
  const handleAssignGroupPermission = async () => {
    if (!configGroup) return;
    if (!assignUserId) {
      toast("Vui lòng chọn thành viên", "warning");
      return;
    }

    setIsAssigningPerm(true);
    try {
      await api.setChannelGroupPermission(configGroup.id, assignUserId, assignGroupRole);
      toast("Đã cấp quyền trên cụm kênh thành công!", "success");
      const perms = await api.getChannelGroupPermissions(configGroup.id);
      setGroupPermissions(perms);
      setAssignUserId("");
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi gán quyền cụm kênh", "danger");
    } finally {
      setIsAssigningPerm(false);
    }
  };

  // Delete Group Permission
  const handleDeleteGroupPermission = async (memberId: string) => {
    if (!configGroup) return;
    try {
      await api.deleteChannelGroupPermission(configGroup.id, memberId);
      toast("Đã thu hồi quyền cụm kênh", "success");
      setGroupPermissions((prev) => prev.filter((p) => p.user_id !== memberId && p.id !== memberId));
      fetchAuditLogs();
    } catch (err: any) {
      toast(err.message || "Lỗi thu hồi quyền", "danger");
    }
  };

  const colorVariants: Record<string, { bg: string; text: string; border: string }> = {
    blue: { bg: "bg-blue-500/10 dark:bg-blue-500/20", text: "text-blue-600 dark:text-blue-400", border: "border-blue-500/30" },
    emerald: { bg: "bg-emerald-500/10 dark:bg-emerald-500/20", text: "text-emerald-600 dark:text-emerald-400", border: "border-emerald-500/30" },
    amber: { bg: "bg-amber-500/10 dark:bg-amber-500/20", text: "text-amber-600 dark:text-amber-400", border: "border-amber-500/30" },
    purple: { bg: "bg-purple-500/10 dark:bg-purple-500/20", text: "text-purple-600 dark:text-purple-400", border: "border-purple-500/30" },
    rose: { bg: "bg-rose-500/10 dark:bg-rose-500/20", text: "text-rose-600 dark:text-rose-400", border: "border-rose-500/30" },
  };

  return (
    <div className="space-y-6">
      {/* Top Header Card */}
      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 sm:p-6 shadow-sm dark:shadow-xl space-y-4">
        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <span className="p-2.5 rounded-xl bg-blue-500/10 text-blue-600 dark:text-blue-400 border border-blue-500/20">
              <Users className="h-6 w-6" />
            </span>
            <div>
              <h2 className="text-base font-bold text-slate-900 dark:text-white flex items-center gap-2">
                <span>Quản Lý Đội Ngũ & Cụm Kênh (Team & Channel Groups)</span>
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                Không gian làm việc: <span className="font-semibold text-slate-700 dark:text-slate-300">{currentOrg?.name}</span> ({currentOrg?.slug})
              </p>
            </div>
          </div>

          {/* Tab buttons */}
          <div className="flex items-center gap-1.5 p-1 bg-slate-100 dark:bg-slate-800 rounded-xl border border-slate-200 dark:border-slate-700">
            <button
              onClick={() => setActiveTab("members")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                activeTab === "members"
                  ? "bg-blue-600 text-white shadow-sm"
                  : "text-slate-600 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white"
              }`}
            >
              <Users className="h-3.5 w-3.5" />
              <span>Thành Viên ({members.length})</span>
            </button>
            <button
              onClick={() => setActiveTab("groups")}
              className={`px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5 ${
                activeTab === "groups"
                  ? "bg-blue-600 text-white shadow-sm"
                  : "text-slate-600 dark:text-slate-300 hover:text-slate-900 dark:hover:text-white"
              }`}
            >
              <Layers className="h-3.5 w-3.5" />
              <span>Cụm Kênh & Phân Quyền ({channelGroups.length})</span>
            </button>
          </div>
        </div>
      </div>

      {/* Tab 1: Thành Viên Tổ Chức */}
      {activeTab === "members" && (
        <div className="space-y-6">
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 sm:p-6 shadow-sm dark:shadow-xl space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <ShieldCheck className="h-4 w-4 text-blue-500" />
                  <span>Danh Sách Thành Viên ({members.length})</span>
                </h3>
                <p className="text-xs text-slate-500">
                  Phân quyền cấp Tổ chức: OWNER (Chủ sở hữu), ADMIN (Quản trị viên), MEMBER (Thành viên).
                </p>
              </div>

              {canManageOrg && (
                <button
                  type="button"
                  onClick={() => {
                    setInvitationTokenResult(null);
                    setShowAddMemberModal(true);
                  }}
                  className="px-3.5 py-2 rounded-xl text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 shadow-md shadow-blue-600/20 transition-all flex items-center gap-1.5"
                >
                  <UserPlus className="h-3.5 w-3.5" />
                  <span>Mời Thành Viên Mới</span>
                </button>
              )}
            </div>

            {loadingMembers ? (
              <div className="py-12 text-center text-xs text-slate-500">
                <RefreshCw className="h-5 w-5 animate-spin mx-auto mb-2 text-blue-500" />
                Đang tải danh sách thành viên...
              </div>
            ) : members.length === 0 ? (
              <div className="p-8 text-center text-xs text-slate-500 border border-dashed rounded-xl">
                Chưa có thành viên nào khác. Nhấn Mời thành viên mới để mở rộng đội ngũ!
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-xs">
                  <thead>
                    <tr className="border-b border-slate-200 dark:border-slate-800 text-slate-500 font-semibold uppercase tracking-wider text-[11px]">
                      <th className="pb-3 pl-1">Thành viên</th>
                      <th className="pb-3">Email</th>
                      <th className="pb-3">Vai trò Org</th>
                      <th className="pb-3">Ngày tham gia</th>
                      {canManageOrg && <th className="pb-3 pr-1 text-right">Thao tác</th>}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800/60">
                    {members.map((m) => (
                      <tr key={m.id} className="hover:bg-slate-50/60 dark:hover:bg-slate-800/30 transition-colors">
                        <td className="py-3 pl-1">
                          <div className="flex items-center gap-2.5">
                            <span className="h-8 w-8 rounded-lg bg-blue-100 dark:bg-blue-500/20 text-blue-600 dark:text-blue-400 font-bold flex items-center justify-center shrink-0">
                              {(m.user_full_name || m.user_email || "U").charAt(0).toUpperCase()}
                            </span>
                            <div>
                              <span className="font-bold text-slate-900 dark:text-white block">
                                {m.user_full_name || "Thành viên"}
                              </span>
                              <span className="text-[10px] text-slate-500 font-mono">
                                ID: {m.user_id ? m.user_id.slice(0, 8) : "N/A"}
                              </span>
                            </div>
                          </div>
                        </td>
                        <td className="py-3 font-mono text-slate-700 dark:text-slate-300">
                          {m.user_email || "Chưa cập nhật"}
                        </td>
                        <td className="py-3">
                          <span
                            className={`px-2.5 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                              m.org_role === "OWNER"
                                ? "bg-purple-100 dark:bg-purple-500/20 text-purple-700 dark:text-purple-300 border border-purple-300 dark:border-purple-500/30"
                                : m.org_role === "ADMIN"
                                ? "bg-blue-100 dark:bg-blue-500/20 text-blue-700 dark:text-blue-300 border border-blue-300 dark:border-blue-500/30"
                                : "bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-700"
                            }`}
                          >
                            {m.org_role}
                          </span>
                        </td>
                        <td className="py-3 text-slate-500 text-[11px]">
                          {m.created_at ? new Date(m.created_at).toLocaleDateString("vi-VN") : "Gần đây"}
                        </td>
                        {canManageOrg && (
                          <td className="py-3 pr-1 text-right">
                            {m.org_role !== "OWNER" && (
                              <button
                                type="button"
                                onClick={() => handleRemoveMember(m.user_id, m.user_email)}
                                title="Xóa thành viên"
                                className="p-1.5 rounded-lg text-rose-500 hover:text-rose-700 hover:bg-rose-50 dark:hover:bg-rose-950/40 transition-colors"
                              >
                                <Trash2 className="h-4 w-4" />
                              </button>
                            )}
                          </td>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>

          {/* Audit Logs Section */}
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 sm:p-6 shadow-sm dark:shadow-xl space-y-3">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                <History className="h-4 w-4 text-slate-500" />
                <span>Nhật Ký Hoạt Động & Bảo Mật (Audit Logs)</span>
              </h3>
              <button
                type="button"
                onClick={fetchAuditLogs}
                disabled={loadingLogs}
                className="text-xs text-blue-600 dark:text-blue-400 hover:underline flex items-center gap-1"
              >
                <RefreshCw className={`h-3 w-3 ${loadingLogs ? "animate-spin" : ""}`} />
                Làm mới nhật ký
              </button>
            </div>

            {loadingLogs ? (
              <p className="text-xs text-slate-500 py-4 text-center">Đang tải nhật ký...</p>
            ) : auditLogs.length === 0 ? (
              <p className="text-xs text-slate-500 py-4 text-center">Chưa có bản ghi hoạt động nào.</p>
            ) : (
              <div className="space-y-2 max-h-56 overflow-y-auto pr-1">
                {auditLogs.map((log) => (
                  <div
                    key={log.id}
                    className="flex items-center justify-between p-2.5 rounded-xl bg-slate-50 dark:bg-slate-950/50 border border-slate-200 dark:border-slate-800 text-xs"
                  >
                    <div className="flex items-center gap-2 truncate">
                      <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-blue-50 dark:bg-blue-950/50 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-900 font-mono">
                        {log.action}
                      </span>
                      <span className="text-slate-700 dark:text-slate-300 truncate">
                        {log.entity_type} {log.entity_id ? `(#${log.entity_id.slice(0, 8)})` : ""}
                      </span>
                    </div>
                    <span className="text-[10px] text-slate-500 shrink-0">
                      {log.created_at ? new Date(log.created_at).toLocaleString("vi-VN") : ""}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Tab 2: Cụm Kênh & Phân Quyền */}
      {activeTab === "groups" && (
        <div className="space-y-6">
          <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/60 backdrop-blur-sm p-5 sm:p-6 shadow-sm dark:shadow-xl space-y-4">
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
              <div>
                <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <FolderTree className="h-4 w-4 text-purple-500" />
                  <span>Danh Sách Cụm Kênh Phát Sóng (Channel Groups)</span>
                </h3>
                <p className="text-xs text-slate-500">
                  Nhóm các Fanpage/Kênh lại với nhau, cấu hình quy trình duyệt và gán quyền theo vai trò.
                </p>
              </div>

              {canManageOrg && (
                <button
                  type="button"
                  onClick={() => setShowCreateGroupModal(true)}
                  className="px-3.5 py-2 rounded-xl text-xs font-bold text-white bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 shadow-md shadow-blue-600/20 transition-all flex items-center gap-1.5"
                >
                  <Plus className="h-3.5 w-3.5" />
                  <span>Tạo Cụm Kênh Mới</span>
                </button>
              )}
            </div>

            {channelGroups.length === 0 ? (
              <div className="p-10 border border-dashed rounded-2xl text-center space-y-2">
                <Layers className="h-8 w-8 mx-auto text-slate-400" />
                <h4 className="text-sm font-bold text-slate-800 dark:text-slate-200">
                  Chưa có Cụm kênh nào
                </h4>
                <p className="text-xs text-slate-500 max-w-sm mx-auto">
                  Hãy tạo Cụm kênh để đăng bài đồng thời lên hàng loạt Fanpage hoặc áp dụng chính sách kiểm duyệt bài viết tự động.
                </p>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {channelGroups.map((g) => {
                  const color = colorVariants[g.color_code || "blue"] || colorVariants.blue;

                  return (
                    <div
                      key={g.id}
                      className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-950/40 p-4 space-y-3 flex flex-col justify-between hover:border-slate-300 dark:hover:border-slate-700 transition-all shadow-sm"
                    >
                      <div className="space-y-2">
                        <div className="flex items-start justify-between gap-2">
                          <span
                            className={`px-2.5 py-0.5 rounded-lg text-xs font-bold border flex items-center gap-1.5 ${color.bg} ${color.text} ${color.border}`}
                          >
                            <span className="h-1.5 w-1.5 rounded-full bg-current" />
                            {g.name}
                          </span>

                          <span
                            className={`px-2 py-0.5 rounded text-[10px] font-semibold ${
                              g.require_approval
                                ? "bg-amber-100 dark:bg-amber-500/20 text-amber-700 dark:text-amber-300 border border-amber-300 dark:border-amber-500/30"
                                : "bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 border border-emerald-300 dark:border-emerald-500/30"
                            }`}
                          >
                            {g.require_approval ? "Bắt buộc duyệt" : "Đăng trực tiếp"}
                          </span>
                        </div>

                        {g.description && (
                          <p className="text-xs text-slate-600 dark:text-slate-400 line-clamp-2">
                            {g.description}
                          </p>
                        )}

                        <div className="flex items-center gap-3 text-xs text-slate-500 pt-1">
                          <span>
                            📺 {g.channel_count !== undefined ? g.channel_count : g.channels?.length || 0} kênh
                          </span>
                          <span>•</span>
                          <span>👥 {g.member_count || 0} thành viên phân quyền</span>
                        </div>
                      </div>

                      <div className="pt-3 border-t border-slate-200 dark:border-slate-800 flex items-center justify-between gap-2">
                        <button
                          type="button"
                          onClick={() => openConfigModal(g)}
                          className="px-3 py-1.5 rounded-xl text-xs font-semibold text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-950/40 hover:bg-blue-100 border border-blue-200 dark:border-blue-900 transition-colors flex items-center gap-1.5"
                        >
                          <Sliders className="h-3.5 w-3.5" />
                          <span>Cấu hình Kênh & Quyền</span>
                        </button>

                        {canManageOrg && (
                          <button
                            type="button"
                            onClick={() => handleDeleteGroup(g.id, g.name)}
                            title="Xóa cụm kênh"
                            className="p-1.5 rounded-xl text-rose-500 hover:text-rose-700 hover:bg-rose-50 dark:hover:bg-rose-950/40 transition-colors"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* Add Member Modal */}
      {showAddMemberModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-md rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                <UserPlus className="h-4 w-4 text-blue-500" />
                <span>Mời Thành Viên Vào Tổ Chức</span>
              </h3>
              <button
                type="button"
                onClick={() => setShowAddMemberModal(false)}
                className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {invitationTokenResult ? (
              <div className="space-y-3 p-4 rounded-xl bg-blue-50 dark:bg-blue-950/40 border border-blue-200 dark:border-blue-900 text-xs">
                <p className="font-bold text-blue-800 dark:text-blue-300">
                  🎉 Lời mời đã được tạo cho email: {invitationTokenResult.email}
                </p>
                <p className="text-slate-600 dark:text-slate-400">
                  Người dùng có thể chấp nhận lời mời bằng mã token sau (hoặc đăng ký tài khoản mới):
                </p>
                <div className="p-2 rounded bg-white dark:bg-slate-950 border font-mono text-[11px] select-all break-all text-slate-900 dark:text-slate-200">
                  {invitationTokenResult.token}
                </div>
                <button
                  type="button"
                  onClick={() => {
                    navigator.clipboard.writeText(invitationTokenResult.token);
                    toast("Đã sao chép token lời mời!", "success");
                  }}
                  className="w-full py-1.5 rounded-lg bg-blue-600 text-white font-semibold text-xs"
                >
                  Sao chép Token lời mời
                </button>
              </div>
            ) : (
              <form onSubmit={handleAddMember} className="space-y-4">
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    Email thành viên
                  </label>
                  <input
                    type="email"
                    required
                    placeholder="dongnghiep@congty.com"
                    value={newMemberEmail}
                    onChange={(e) => setNewMemberEmail(e.target.value)}
                    className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                    Vai trò trong Tổ chức (Org Role)
                  </label>
                  <select
                    value={newMemberRole}
                    onChange={(e) => setNewMemberRole(e.target.value as OrgRole)}
                    className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
                  >
                    <option value="MEMBER">MEMBER (Thành viên - Quyền hạn theo Cụm kênh)</option>
                    <option value="ADMIN">ADMIN (Quản trị viên - Toàn quyền nhóm và duyệt)</option>
                    <option value="OWNER">OWNER (Đồng sở hữu tổ chức)</option>
                  </select>
                </div>

                <div className="flex items-center justify-end gap-2 pt-2">
                  <button
                    type="button"
                    onClick={() => setShowAddMemberModal(false)}
                    className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                  >
                    Hủy
                  </button>
                  <button
                    type="submit"
                    disabled={isAddingMember}
                    className="px-5 py-2 rounded-xl text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 shadow-md shadow-blue-600/25 transition-all disabled:opacity-50"
                  >
                    {isAddingMember ? "Đang gửi..." : "Gửi Lời Mời"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* Create Group Modal */}
      {showCreateGroupModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-md rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                <Plus className="h-4 w-4 text-blue-500" />
                <span>Tạo Cụm Kênh Mới</span>
              </h3>
              <button
                type="button"
                onClick={() => setShowCreateGroupModal(false)}
                className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <form onSubmit={handleCreateGroup} className="space-y-4">
              <div className="space-y-1">
                <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                  Tên Cụm Kênh (Bắt buộc)
                </label>
                <input
                  type="text"
                  required
                  placeholder="Ví dụ: Cụm Fanpage Bán Hàng Miền Nam"
                  value={newGroupName}
                  onChange={(e) => setNewGroupName(e.target.value)}
                  className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                  Mô tả
                </label>
                <textarea
                  rows={2}
                  placeholder="Mô tả mục đích sử dụng cụm kênh..."
                  value={newGroupDesc}
                  onChange={(e) => setNewGroupDesc(e.target.value)}
                  className="w-full px-3 py-2 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-300 dark:border-slate-800 text-xs text-slate-900 dark:text-slate-100 focus:outline-none focus:border-blue-500"
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold text-slate-700 dark:text-slate-300">
                  Màu sắc nhận diện
                </label>
                <div className="flex gap-2">
                  {Object.keys(colorVariants).map((col) => (
                    <button
                      key={col}
                      type="button"
                      onClick={() => setNewGroupColor(col)}
                      className={`h-7 w-7 rounded-lg border-2 capitalize text-[10px] font-bold flex items-center justify-center transition-all ${
                        newGroupColor === col
                          ? "border-blue-600 scale-110 shadow-sm"
                          : "border-transparent opacity-70 hover:opacity-100"
                      } ${colorVariants[col].bg} ${colorVariants[col].text}`}
                    >
                      {col.charAt(0).toUpperCase()}
                    </button>
                  ))}
                </div>
              </div>

              <label className="flex items-center gap-2.5 p-3 rounded-xl bg-amber-50 dark:bg-amber-950/30 border border-amber-200 dark:border-amber-800/60 cursor-pointer">
                <input
                  type="checkbox"
                  checked={newGroupRequireApproval}
                  onChange={(e) => setNewGroupRequireApproval(e.target.checked)}
                  className="rounded text-amber-600 focus:ring-amber-500 h-4 w-4"
                />
                <div className="text-xs text-amber-900 dark:text-amber-300">
                  <span className="font-bold">Bắt buộc Phê Duyệt bài đăng</span>
                  <p className="text-[11px] text-amber-700 dark:text-amber-400">
                    Khi bật, bài viết từ Editor phải được Approver hoặc Manager duyệt trước khi phát sóng.
                  </p>
                </div>
              </label>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateGroupModal(false)}
                  className="px-4 py-2 rounded-xl text-xs font-semibold text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  Hủy
                </button>
                <button
                  type="submit"
                  disabled={isCreatingGroup}
                  className="px-5 py-2 rounded-xl text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 shadow-md shadow-blue-600/25 transition-all disabled:opacity-50"
                >
                  {isCreatingGroup ? "Đang tạo..." : "Tạo Cụm"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Config Channels & Permissions Modal */}
      {configGroup && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
          <div className="w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 p-6 shadow-2xl space-y-6">
            <div className="flex items-center justify-between border-b pb-3 border-slate-200 dark:border-slate-800">
              <div>
                <h3 className="text-sm font-bold text-slate-900 dark:text-white flex items-center gap-2">
                  <Sliders className="h-4 w-4 text-blue-500" />
                  <span>Cấu hình Cụm: {configGroup.name}</span>
                </h3>
                <p className="text-xs text-slate-500">
                  Chọn kênh phát sóng và chỉ định phân quyền thành viên trong cụm này
                </p>
              </div>
              <button
                type="button"
                onClick={() => setConfigGroup(null)}
                className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {/* Section 1: Channels inside Group */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <h4 className="text-xs font-bold uppercase tracking-wider text-slate-700 dark:text-slate-300">
                  1. Kênh phát sóng thuộc cụm ({selectedGroupChannelIds.length}/{channels.length})
                </h4>
                {canManageOrg && (
                  <button
                    type="button"
                    onClick={handleSaveGroupChannels}
                    disabled={isSavingChannels}
                    className="px-3 py-1 rounded-lg text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 transition-colors disabled:opacity-50"
                  >
                    {isSavingChannels ? "Đang lưu..." : "Lưu danh sách kênh"}
                  </button>
                )}
              </div>

              {channels.length === 0 ? (
                <p className="text-xs text-slate-500">Chưa có kênh nào được kết nối trong hệ thống.</p>
              ) : (
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-48 overflow-y-auto p-1">
                  {channels.map((ch) => {
                    const isSelected = selectedGroupChannelIds.includes(ch.id);
                    return (
                      <div
                        key={ch.id}
                        onClick={() => canManageOrg && toggleChannelSelection(ch.id)}
                        className={`flex items-center justify-between p-2.5 rounded-xl border text-xs cursor-pointer transition-all ${
                          isSelected
                            ? "bg-blue-50 dark:bg-blue-950/40 border-blue-400 text-blue-900 dark:text-blue-300 font-semibold"
                            : "bg-slate-50 dark:bg-slate-950/40 border-slate-200 dark:border-slate-800 text-slate-700 dark:text-slate-400"
                        }`}
                      >
                        <div className="truncate pr-2">
                          <span className="block truncate">{ch.name}</span>
                          <span className="text-[10px] text-slate-500 block truncate">
                            {ch.platform || "FACEBOOK"} • {ch.external_id}
                          </span>
                        </div>
                        {isSelected && <Check className="h-4 w-4 text-blue-600 dark:text-blue-400 shrink-0" />}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            {/* Section 2: Group Permissions (RBAC) */}
            <div className="space-y-3 pt-3 border-t border-slate-200 dark:border-slate-800">
              <h4 className="text-xs font-bold uppercase tracking-wider text-slate-700 dark:text-slate-300">
                2. Phân quyền trên Cụm kênh (Channel Group RBAC)
              </h4>

              {canManageOrg && (
                <div className="flex flex-col sm:flex-row gap-2 items-center bg-slate-50 dark:bg-slate-950 p-3 rounded-xl border border-slate-200 dark:border-slate-800">
                  <select
                    value={assignUserId}
                    onChange={(e) => setAssignUserId(e.target.value)}
                    className="flex-1 px-3 py-1.5 rounded-lg bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 text-xs text-slate-800 dark:text-slate-200"
                  >
                    <option value="">-- Chọn thành viên để gán quyền --</option>
                    {members.map((m) => (
                      <option key={m.user_id} value={m.user_id}>
                        {m.user_full_name || m.user_email} ({m.org_role})
                      </option>
                    ))}
                  </select>

                  <select
                    value={assignGroupRole}
                    onChange={(e) => setAssignGroupRole(e.target.value as GroupRole)}
                    className="px-3 py-1.5 rounded-lg bg-white dark:bg-slate-900 border border-slate-300 dark:border-slate-700 text-xs text-slate-800 dark:text-slate-200"
                  >
                    <option value="MANAGER">MANAGER (Toàn quyền quản lý kênh & duyệt)</option>
                    <option value="APPROVER">APPROVER (Có quyền Phê duyệt bài đăng)</option>
                    <option value="EDITOR">EDITOR (Soạn thảo, gửi chờ duyệt nếu bật)</option>
                    <option value="VIEWER">VIEWER (Chỉ xem thống kê, không được tạo bài)</option>
                  </select>

                  <button
                    type="button"
                    onClick={handleAssignGroupPermission}
                    disabled={isAssigningPerm || !assignUserId}
                    className="px-4 py-1.5 rounded-lg text-xs font-bold text-white bg-blue-600 hover:bg-blue-500 disabled:opacity-50 shrink-0"
                  >
                    Gán Quyền
                  </button>
                </div>
              )}

              {loadingGroupPerms ? (
                <p className="text-xs text-slate-500 py-3 text-center">Đang tải danh sách phân quyền...</p>
              ) : groupPermissions.length === 0 ? (
                <p className="text-xs text-slate-500 py-2">
                  Chưa gán quyền riêng cho thành viên nào. Thành viên sẽ kế thừa quyền cấp Tổ chức.
                </p>
              ) : (
                <div className="space-y-2 max-h-48 overflow-y-auto">
                  {groupPermissions.map((perm) => (
                    <div
                      key={perm.id}
                      className="flex items-center justify-between p-2.5 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800 text-xs"
                    >
                      <div>
                        <span className="font-bold text-slate-800 dark:text-slate-200 block">
                          {perm.user_full_name || perm.user_email || perm.user_id}
                        </span>
                        <span className="text-[10px] text-slate-500">{perm.user_email}</span>
                      </div>

                      <div className="flex items-center gap-2">
                        <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-blue-100 dark:bg-blue-500/20 text-blue-700 dark:text-blue-300 border border-blue-200 dark:border-blue-500/30">
                          {perm.group_role}
                        </span>
                        {canManageOrg && (
                          <button
                            type="button"
                            onClick={() => handleDeleteGroupPermission(perm.user_id)}
                            className="p-1 rounded text-rose-500 hover:text-rose-700"
                            title="Xóa quyền"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="flex justify-end pt-2">
              <button
                type="button"
                onClick={() => setConfigGroup(null)}
                className="px-5 py-2 rounded-xl text-xs font-semibold text-slate-700 dark:text-slate-300 bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700"
              >
                Đóng
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
