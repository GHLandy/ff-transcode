/// <reference types="vite/client" />

/** 应用版本号，来自 wails.json 的 version 字段（由 Vite define 注入）。 */
declare const __APP_VERSION__: string;

/** 构建时间，格式 YYYY-MM-DD HH:mm:ss（由 Vite 在构建时注入）。 */
declare const __BUILD_TIME__: string;
