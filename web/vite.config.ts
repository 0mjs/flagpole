import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The API runs on :8080. Proxying it makes the dashboard and the API one
// origin, so the session and CSRF cookies just work.
const api = { target: "http://localhost:8080", changeOrigin: false };

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    proxy: { "/api": api, "/sdk": api, "/openapi.json": api, "/docs": api },
  },
});
