import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    // Do not publish source maps to the public CDN.
    // Upload private source maps separately to the error-monitoring service.
    sourcemap: false,
    minify: "esbuild",
    target: "es2022",
    cssCodeSplit: true,
    rollupOptions: {
      output: {
        manualChunks: {
          react: ["react", "react-dom"],
          editor: ["zustand"]
        }
      }
    }
  },
  esbuild: {
    drop: ["debugger"],
    pure: ["console.debug"]
  }
});
