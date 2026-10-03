// React bindings: put a started client in <FlagpoleProvider>, then read flags
// with useFlag. Components re-render when their flag changes. Whoever creates
// the client closes it.

import { createContext, useContext, useSyncExternalStore, type ReactNode } from "react";
import type { FlagpoleClient, Result, Status } from "./index";

const ClientContext = createContext<FlagpoleClient | null>(null);

export function FlagpoleProvider({ client, children }: { client: FlagpoleClient; children: ReactNode }) {
  return <ClientContext.Provider value={client}>{children}</ClientContext.Provider>;
}

export function useFlagpole(): FlagpoleClient {
  const client = useContext(ClientContext);
  if (!client) throw new Error("useFlagpole needs a <FlagpoleProvider>");
  return client;
}

function useResults(client: FlagpoleClient) {
  return useSyncExternalStore(
    (notify) => client.on("change", notify),
    () => client.all(),
  );
}

/** A flag's value for the current user, recording an exposure. */
export function useFlag<T>(flag: string, fallback: T): T {
  const client = useFlagpole();
  useResults(client);
  return client.variation(flag, fallback);
}

/** Every flag's result, for debugging tools. Doesn't record exposures. */
export function useAllFlags(): Readonly<Record<string, Result>> {
  return useResults(useFlagpole());
}

/** The live stream's status. */
export function useFlagpoleStatus(): Status {
  const client = useFlagpole();
  return useSyncExternalStore(
    (notify) => client.on("status", notify),
    () => client.status,
  );
}
