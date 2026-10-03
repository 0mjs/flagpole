import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "./client";

export type User = Schemas["User"];
export type Role = Schemas["Role"];
export type Flag = Schemas["Flag"];
export type FlagSummary = Schemas["FlagSummary"];
export type EnvironmentConfig = Schemas["EnvironmentConfig"];
export type ChangeRequest = Schemas["ChangeRequest"];
export type Rule = Schemas["Rule"];
export type RuleConfig = Schemas["Config"];

const rank: Record<Role, number> = { viewer: 0, editor: 1, approver: 2, admin: 3 };
export const allows = (role: Role | undefined, min: Role) => role !== undefined && rank[role] >= rank[min];

export const keys = {
  me: ["me"] as const,
  projects: ["projects"] as const,
  project: (p: string) => ["project", p] as const,
  flags: (p: string) => ["flags", p] as const,
  flag: (p: string, f: string) => ["flag", p, f] as const,
  changes: (p: string) => ["changes", p] as const,
  audit: (p: string) => ["audit", p] as const,
  users: ["users"] as const,
  sdkKeys: (p: string, e: string) => ["sdk-keys", p, e] as const,
  exposures: (p: string, f: string, e: string, h: number) => ["exposures", p, f, e, h] as const,
};

export const fetchMe = () => unwrap(api.GET("/api/v1/auth/me"));

export function useMe() {
  return useQuery({ queryKey: keys.me, queryFn: fetchMe, retry: false, staleTime: 60_000 });
}

export function useProjects() {
  return useQuery({ queryKey: keys.projects, queryFn: () => unwrap(api.GET("/api/v1/projects")) });
}

export function useProject(project: string) {
  return useQuery({
    queryKey: keys.project(project),
    queryFn: () => unwrap(api.GET("/api/v1/projects/{project}", { params: { path: { project } } })),
  });
}

export function useFlags(project: string, includeArchived = false) {
  return useQuery({
    queryKey: [...keys.flags(project), includeArchived],
    queryFn: () => unwrap(api.GET("/api/v1/projects/{project}/flags", { params: { path: { project }, query: { include_archived: includeArchived } } })),
  });
}

export function useFlag(project: string, flag: string) {
  return useQuery({
    queryKey: keys.flag(project, flag),
    queryFn: () => unwrap(api.GET("/api/v1/projects/{project}/flags/{flag}", { params: { path: { project, flag } } })),
  });
}

export function useChangeRequests(project: string) {
  return useQuery({
    queryKey: keys.changes(project),
    queryFn: () => unwrap(api.GET("/api/v1/projects/{project}/change-requests", { params: { path: { project } } })),
  });
}

export function useAudit(project: string) {
  return useQuery({
    queryKey: keys.audit(project),
    queryFn: () => unwrap(api.GET("/api/v1/projects/{project}/audit", { params: { path: { project }, query: { limit: 100 } } })),
  });
}

export function useUsers() {
  return useQuery({ queryKey: keys.users, queryFn: () => unwrap(api.GET("/api/v1/users")) });
}

export function useSDKKeys(project: string, env: string) {
  return useQuery({
    queryKey: keys.sdkKeys(project, env),
    queryFn: () => unwrap(api.GET("/api/v1/projects/{project}/environments/{env}/sdk-keys", { params: { path: { project, env } } })),
  });
}

export function useExposures(project: string, flag: string, env: string, hours: number) {
  return useQuery({
    queryKey: keys.exposures(project, flag, env, hours),
    queryFn: () =>
      unwrap(api.GET("/api/v1/projects/{project}/flags/{flag}/environments/{env}/exposures", { params: { path: { project, flag, env }, query: { hours } } })),
    refetchInterval: 15_000,
  });
}

/** A mutation that refreshes the given queries when it succeeds. */
export function useAction<TArgs, TResult>(fn: (args: TArgs) => Promise<TResult>, invalidate: readonly (readonly unknown[])[]) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => Promise.all(invalidate.map((queryKey) => qc.invalidateQueries({ queryKey }))),
  });
}
