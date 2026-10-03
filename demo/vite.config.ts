import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

const sdk = (file: string) => fileURLToPath(new URL(`../sdk/js/src/${file}`, import.meta.url));

// The shop talks to the API directly, cross-origin, as a real site would; the
// SDK routes allow any origin. The SDK is imported from source.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: [
      { find: "@flagpole/sdk/react", replacement: sdk("react.tsx") },
      { find: "@flagpole/sdk", replacement: sdk("index.ts") },
    ],
    dedupe: ["react", "react-dom"],
  },
  server: { port: 5174, fs: { allow: [".."] } },
});
