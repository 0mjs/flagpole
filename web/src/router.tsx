import { createRootRouteWithContext, createRoute, createRouter, Outlet, redirect } from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";
import { fetchMe, keys } from "./api/queries";
import { Shell } from "./components/shell";
import { AcceptInvitePage, ForgotPasswordPage, LoginPage, ResetPasswordPage } from "./pages/auth";
import { ProjectsPage } from "./pages/projects";
import { FlagsPage } from "./pages/flags";
import { FlagPage } from "./pages/flag";
import { ChangesPage } from "./pages/changes";
import { SettingsPage } from "./pages/settings";
import { AuditPage } from "./pages/audit";
import { UsersPage } from "./pages/users";

const root = createRootRouteWithContext<{ queryClient: QueryClient }>()({ component: Outlet });

const tokenSearch = (s: Record<string, unknown>) => ({ token: typeof s.token === "string" ? s.token : "" });
const login = createRoute({
  getParentRoute: () => root,
  path: "/login",
  validateSearch: (s: Record<string, unknown>) => ({ next: typeof s.next === "string" ? s.next : undefined }),
  component: LoginPage,
});
const acceptInvite = createRoute({ getParentRoute: () => root, path: "/accept-invite", validateSearch: tokenSearch, component: AcceptInvitePage });
const resetPassword = createRoute({ getParentRoute: () => root, path: "/reset-password", validateSearch: tokenSearch, component: ResetPasswordPage });
const forgotPassword = createRoute({ getParentRoute: () => root, path: "/forgot-password", component: ForgotPasswordPage });

// Everything else needs a signed-in user.
const app = createRoute({
  getParentRoute: () => root,
  id: "app",
  beforeLoad: async ({ context, location }) => {
    try {
      return { me: await context.queryClient.ensureQueryData({ queryKey: keys.me, queryFn: fetchMe }) };
    } catch {
      throw redirect({ to: "/login", search: { next: location.href } });
    }
  },
  component: Shell,
});

const projects = createRoute({ getParentRoute: () => app, path: "/", component: ProjectsPage });
const users = createRoute({ getParentRoute: () => app, path: "/users", component: UsersPage });
const project = createRoute({ getParentRoute: () => app, path: "/projects/$project" });
const flags = createRoute({ getParentRoute: () => project, path: "/", component: FlagsPage });
const flag = createRoute({ getParentRoute: () => project, path: "/flags/$flag", component: FlagPage });
const changes = createRoute({ getParentRoute: () => project, path: "/changes", component: ChangesPage });
const settings = createRoute({ getParentRoute: () => project, path: "/settings", component: SettingsPage });
const audit = createRoute({ getParentRoute: () => project, path: "/audit", component: AuditPage });

const routeTree = root.addChildren([
  login,
  acceptInvite,
  resetPassword,
  forgotPassword,
  app.addChildren([projects, users, project.addChildren([flags, flag, changes, settings, audit])]),
]);

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({ routeTree, context: { queryClient }, defaultPreload: "intent" });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
