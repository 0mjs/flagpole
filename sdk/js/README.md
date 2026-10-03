# @flagpole/sdk

Flagpole's JavaScript SDK. It asks the API to evaluate every flag for the current user, keeps the results, and re-evaluates when the live stream says something changed. Reading a flag is synchronous and records an exposure; exposures go to the API in batches, one per user, flag and variant.

```ts
import { FlagpoleClient } from "@flagpole/sdk";

const client = new FlagpoleClient({
  url: "http://localhost:8080",
  key: "fp_dev_…", // an SDK key for one environment
  context: { key: "user-42", attributes: { country: "uk", plan: "pro" } },
});
await client.start();

client.variation("new-checkout", false); // true or false
client.on("change", ({ flags }) => console.log("changed:", flags));
await client.identify({ key: "user-43" }); // another user
```

With React:

```tsx
import { FlagpoleProvider, useFlag } from "@flagpole/sdk/react";

<FlagpoleProvider client={client}><App /></FlagpoleProvider>;

function BuyButton() {
  const color = useFlag("button-color", "blue");
  return <button className={color}>Buy</button>;
}
```

The package ships TypeScript source; the demo shop (`../../demo`) imports it directly.

An SDK key can read its environment's flags and rules, so a key used in a browser shows those rules to anyone who looks.
