"use client";

import React, { createContext, useContext, useEffect, useState, useCallback, useRef } from "react";
import { Organization, ChannelGroup, User, OrganizationMember, GroupRole } from "@/lib/types";
import { api } from "@/lib/api";

interface OrgContextType {
  currentOrg: Organization | null;
  organizations: Organization[];
  currentUser: User | null;
  currentMember: OrganizationMember | null;
  channelGroups: ChannelGroup[];
  channelRoles: Record<string, GroupRole>;
  currentRole: string;
  actualRole: string;
  canManageOrg: boolean;
  canCreatePost: boolean;
  canPublishDirectly: boolean;
  canApprovePost: boolean;
  loading: boolean;
  error: string | null;
  switchOrg: (org: Organization) => void;
  createOrg: (name: string, slug?: string) => Promise<Organization>;
  refreshOrgs: () => Promise<void>;
  refreshGroups: () => Promise<void>;
  logout: () => void;
}
const OrgContext = createContext<OrgContextType | undefined>(undefined);

export function OrgProvider({ children }: { children: React.ReactNode }) {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [currentOrg, setCurrentOrg] = useState<Organization | null>(null);
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [currentMember, setCurrentMember] = useState<OrganizationMember | null>(null);
  const [channelGroups, setChannelGroups] = useState<ChannelGroup[]>([]);
  const [channelRoles, setChannelRoles] = useState<Record<string, GroupRole>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const generation = useRef(0);
  const reset = useCallback(() => {
    generation.current++;
    setCurrentUser(null); setCurrentMember(null); setCurrentOrg(null);
    setOrganizations([]); setChannelGroups([]); setChannelRoles({}); setError(null);
  }, []);
  const logout = useCallback(() => { api.clearAuth(); reset(); setLoading(false); }, [reset]);
  const refreshGroups = useCallback(async () => {
    if (!api.getToken()) return;
    const epoch = generation.current;
    try { const groups = await api.getChannelGroups(); if (epoch === generation.current) setChannelGroups(groups); }
    catch (e) { if (epoch === generation.current) setError(e instanceof Error ? e.message : "Không tải được nhóm kênh"); }
  }, []);
  const refreshOrgs = useCallback(async () => {
    const epoch = ++generation.current;
    if (!api.getToken()) { reset(); setLoading(false); return; }
    setLoading(true); setError(null);
    // Clear the previous workspace immediately, so errors never retain another tenant's UI.
    setCurrentOrg(null); setCurrentMember(null); setChannelGroups([]); setChannelRoles({});
    try {
      const orgs = await api.getOrganizations();
      if (epoch !== generation.current) return;
      setOrganizations(orgs);
      const active = orgs.find(o => o.id === api.getActiveOrgId()) || orgs[0];
      if (!active) throw new Error("Tài khoản chưa có workspace. Hãy liên hệ quản trị viên.");
      api.setActiveOrgId(active.id);
      const me = await api.getMe();
      const groups = await api.getChannelGroups();
      if (epoch !== generation.current) return;
      if (!me.member || me.organization.id !== active.id) throw new Error("Không xác minh được quyền trong workspace.");
      setCurrentUser(me.user); setCurrentMember(me.member); setCurrentOrg(active);
      setChannelRoles(me.channel_roles || {}); setChannelGroups(groups);
    } catch (e) {
      if (epoch === generation.current) setError(e instanceof Error ? e.message : "Không tải được workspace");
    } finally { if (epoch === generation.current) setLoading(false); }
  }, [reset]);
  useEffect(() => {
    refreshOrgs();
    const expired = () => { reset(); setLoading(false); };
    window.addEventListener("postpilot-session-expired", expired);
    return () => window.removeEventListener("postpilot-session-expired", expired);
  }, [refreshOrgs, reset]);
  const switchOrg = useCallback((org: Organization) => { api.setActiveOrgId(org.id); void refreshOrgs(); }, [refreshOrgs]);
  const createOrg = useCallback(async (name: string, slug?: string) => {
    const created = await api.createOrganization(name, slug); api.setActiveOrgId(created.id); await refreshOrgs(); return created;
  }, [refreshOrgs]);
  const actualRole = currentMember?.org_role || "";
  const canManageOrg = actualRole === "OWNER" || actualRole === "ADMIN";
  const roles = Object.values(channelRoles);
  const canCreatePost = canManageOrg || roles.some(r => ["MANAGER", "APPROVER", "EDITOR"].includes(r));
  const canApprovePost = canManageOrg || roles.some(r => ["MANAGER", "APPROVER"].includes(r));
  return <OrgContext.Provider value={{ currentOrg, organizations, currentUser, currentMember, channelGroups, channelRoles,
    currentRole: actualRole, actualRole, canManageOrg, canCreatePost, canPublishDirectly: canApprovePost,
    canApprovePost, loading, error, switchOrg, createOrg, refreshOrgs, refreshGroups, logout }}>{children}</OrgContext.Provider>;
}
export function useOrg() { const context = useContext(OrgContext); if (!context) throw new Error("useOrg requires OrgProvider"); return context; }
