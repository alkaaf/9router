import { defineConfig } from "vitest/config";
import { resolve } from "path";
import { fileURLToPath } from "url";

const __dirname = fileURLToPath(new URL(".", import.meta.url));

export default defineConfig({
  test: {
    environment: "node",
    globals: true,
    include: ["**/*.{test,spec}.{js,jsx,ts,tsx}"],
    silent: false,
    setupFiles: [resolve(__dirname, "./setup.js")],
  },
  resolve: {
    alias: {
      "open-sse": resolve(__dirname, "../open-sse"),
      "@": resolve(__dirname, "../src"),
    },
  },
});
