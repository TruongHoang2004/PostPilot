"use client";

import React, { createContext, useContext, useEffect, useState, useCallback, useMemo } from "react";
import { Organization, ChannelGroup, User, OrganizationMember } from "@/lib/types";
import { api } from "@/lib/api";

export interface OrgContextType {
  currentOrg: Organization | null;
  organizations: Organization[];
  switchOrg: (org: Organization) => void;
  createOrg: (name: string, slug?: string) => Promise<Organization>;
  currentRole: string; // Effective role ('OWNER' | 'ADMIN' | 'MEMBER' | 'MANAGER' | 'APPROVER' | 'EDITOR' | 'VIEWER')
  actualRole: string;
  isSimulating: boolean;
  simulateRole: (role: string | null) => void;
  channelGroups: ChannelGroup[];
  refreshOrgs: () => Promise<void>;
  refreshGroups: () => Promise<void>;
  canManageOrg: boolean;
  canCreatePost: boolean;
  canPublishDirectly: boolean;
  canApprovePost: boolean;
  currentUser: User | null;
  currentMember: OrganizationMember | null;
  loading: boolean;
}

const OrgContext = createContext<OrgContextType | undefined>(undefined);

const DEFAULT_FALLBACK_ORG: Organization = {
  id: "00000000-0000-0000-0000-000000000001",
  name: "Không gian làm việc chính",
  slug: "main-workspace",
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

export function OrgProvider({ children }: { children: React.ReactNode }) {
  const [organizations, setOrganizations] = useState<Organization[]>([]);
  const [currentOrg, setCurrentOrg] = useState<Organization | null>(null);
  const [channelGroups, setChannelGroups] = useState<ChannelGroup[]>([]);
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [currentMember, setCurrentMember] = useState<OrganizationMember | null>(null);
  const [simulatedRole, setSimulatedRoleState] = useState<string | null>(null);
  const [loading, setLoading] = useState<boolean>(true);

  // 1. Fetch channel groups for the currently selected org
  const refreshGroups = useCallback(async () => {
    try {
      const groups = await api.getChannelGroups();
      setChannelGroups(groups);
    } catch (err) {
      console.warn("Lỗi tải danh sách Channel Groups:", err);
      setChannelGroups([]);
    }
  }, []);

  // 2. Fetch all organizations and profile
  const refreshOrgs = useCallback(async () => {
    setLoading(true);
    try {
      const [orgsList, meData] = await Promise.allSettled([
        api.getOrganizations(),
        api.getMe(),
      ]);

      let orgs: Organization[] = [];
      if (orgsList.status === "fulfilled" && Array.isArray(orgsList.value)) {
        orgs = orgsList.value;
      }

      if (meData.status === "fulfilled" && meData.value) {
        if (meData.value.user) setCurrentUser(meData.value.user);
        if (meData.value.member) setCurrentMember(meData.value.member);
      }

      setOrganizations(orgs);

      // Determine active organization
      const savedOrgId = api.getActiveOrgId();
      let active: Organization | null = null;

      if (savedOrgId && orgs.length > 0) {
        active = orgs.find((o) => o.id === savedOrgId) || null;
      }

      if (!active && orgs.length > 0) {
        active = orgs[0];
        api.setActiveOrgId(orgs[0].id);
      } else if (!active && orgs.length === 0) {
        active = DEFAULT_FALLBACK_ORG;
        api.setActiveOrgId(DEFAULT_FALLBACK_ORG.id);
      }

      setCurrentOrg(active);
    } catch (err) {
      console.warn("Lỗi đồng bộ Organization:", err);
      setCurrentOrg(DEFAULT_FALLBACK_ORG);
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial load
  useEffect(() => {
    const savedSimRole = api.getSimulateRole();
    if (savedSimRole) {
      setSimulatedRoleState(savedSimRole);
    }
    refreshOrgs();
  }, [refreshOrgs]);

  // When active org changes, refresh channel groups
  useEffect(() => {
    if (currentOrg) {
      refreshGroups();
    }
  }, [currentOrg, refreshGroups]);

  // Switch Active Workspace
  const switchOrg = useCallback(
    (org: Organization) => {
      setCurrentOrg(org);
      api.setActiveOrgId(org.id);
      refreshGroups();
    },
    [refreshGroups]
  );

  // Create Workspace
  const createOrg = useCallback(
    async (name: string, slug?: string): Promise<Organization> => {
      const created = await api.createOrganization(name, slug);
      setOrganizations((prev) => [...prev, created]);
      switchOrg(created);
      return created;
    },
    [switchOrg]
  );

  // Simulate Role
  const simulateRole = useCallback((role: string | null) => {
    api.setSimulateRole(role);
    setSimulatedRoleState(role);
  }, []);

  // Compute effective role & RBAC permissions
  const actualRole = useMemo(() => {
    return currentMember?.org_role || "OWNER";
  }, [currentMember]);

  const currentRole = useMemo(() => {
    return simulatedRole || actualRole;
  }, [simulatedRole, actualRole]);

  const isSimulating = Boolean(simulatedRole);

  const canManageOrg = useMemo(() => {
    const roleUpper = currentRole.toUpperCase();
    return roleUpper === "OWNER" || roleUpper === "ADMIN";
  }, [currentRole]);

  const canCreatePost = useMemo(() => {
    const roleUpper = currentRole.toUpperCase();
    return roleUpper !== "VIEWER";
  }, [currentRole]);

  const canPublishDirectly = useMemo(() => {
    const roleUpper = currentRole.toUpperCase();
    return ["OWNER", "ADMIN", "MANAGER", "APPROVER"].includes(roleUpper);
  }, [currentRole]);

  const canApprovePost = useMemo(() => {
    const roleUpper = currentRole.toUpperCase();
    return ["OWNER", "ADMIN", "MANAGER", "APPROVER"].includes(roleUpper);
  }, [currentRole]);

  const value = useMemo(
    () => ({
      currentOrg,
      organizations,
      switchOrg,
      createOrg,
      currentRole,
      actualRole,
      isSimulating,
      simulateRole,
      channelGroups,
      refreshOrgs,
      refreshGroups,
      canManageOrg,
      canCreatePost,
      canPublishDirectly,
      canApprovePost,
      currentUser,
      currentMember,
      loading,
    }),
    [
      currentOrg,
      organizations,
      switchOrg,
      createOrg,
      currentRole,
      actualRole,
      isSimulating,
      simulateRole,
      channelGroups,
      refreshOrgs,
      refreshGroups,
      canManageOrg,
      canCreatePost,
      canPublishDirectly,
      canApprovePost,
      currentUser,
      currentMember,
      loading,
    ]
  );

  return <OrgContext.Provider value={value}>{children}</OrgContext.Provider>;
}

export function useOrg() {
  const context = useContext(OrgContext);
  if (!context) {
    throw new Error("useOrg must be used within an OrgProvider");
  }
  return context;
}
