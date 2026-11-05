import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  // base 必须是 "/"。
  //
  // 上游写的是 "/ai/svg-ora-studio"（它的线上部署子路径），照搬会让构建
  // 产物里的资源引用全部指向不存在的子路径 → 白屏 + 一堆 404。
  base: "/",
  server: {
    host: "127.0.0.1",
    // 官方模板写死 9245 + strictPort: true，同时开第二个 Wails 项目直接
    // 起不来（"Port 9245 is already in use"）。
    port: Number(process.env.WAILS_VITE_PORT) || 0,
    strictPort: false,
  },
  plugins: [react(), wails("./bindings")],
});
