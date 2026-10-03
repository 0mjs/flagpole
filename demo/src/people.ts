import type { Context } from "@flagpole/sdk";

// Shoppers to browse as. Rules in the dashboard can target their attributes:
// try "plan is one of pro, team" or "country is one of uk".
export interface Person {
  id: string;
  name: string;
  note: string;
  context: () => Context;
}

function visitorKey(fresh = false): string {
  try {
    let key = fresh ? null : localStorage.getItem("harbour-visitor");
    if (!key) {
      key = `visitor-${Math.random().toString(36).slice(2, 8)}`;
      localStorage.setItem("harbour-visitor", key);
    }
    return key;
  } catch {
    return `visitor-${Math.random().toString(36).slice(2, 8)}`;
  }
}

export const newVisitor = () => visitorKey(true);

export const people: Person[] = [
  { id: "ada", name: "Ada", note: "UK · pro plan", context: () => ({ key: "user-ada", attributes: { country: "uk", plan: "pro", email: "ada@harbour.test" } }) },
  { id: "bram", name: "Bram", note: "Netherlands · free plan", context: () => ({ key: "user-bram", attributes: { country: "nl", plan: "free", email: "bram@harbour.test" } }) },
  { id: "chen", name: "Chen", note: "US · team plan", context: () => ({ key: "user-chen", attributes: { country: "us", plan: "team", email: "chen@harbour.test" } }) },
  { id: "visitor", name: "A visitor", note: "Not signed in", context: () => ({ key: visitorKey() }) },
];
