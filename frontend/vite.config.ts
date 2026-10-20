import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import wailsConfig from '../wails.json';

/** 两位补零，用于构建时间格式化。 */
const pad = (n: number) => String(n).padStart(2, '0');

// 构建时间：Vite 执行构建时取当前时间，格式 YYYY-MM-DD HH:mm:ss。
const now = new Date();
const buildTime = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())} ${pad(
  now.getHours(),
)}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`;

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    // 版本号取自 wails.json 的 version 字段。
    __APP_VERSION__: JSON.stringify(wailsConfig.info.productVersion ?? '0.0.0'),
    __BUILD_TIME__: JSON.stringify(buildTime),
  },
});
