import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"

// The page runs in the system's web view: WebKitGTK (Linux), WebView2
// (Windows) and WKWebView (macOS 12 and newer, Safari 15's engine).
export default defineConfig({
    plugins: [react()],
    build: {
        target: ["es2021", "safari15", "chrome100"],
        chunkSizeWarningLimit: 2000,
    },
})
