/**
 * 轻量中英双语。
 *
 * 设计要点：**词条的 key 就是英文原文**。
 *
 *   t("Generate SVG")   中文模式 → "生成 SVG"
 *                       英文模式 → "Generate SVG"（key 自己）
 *
 * 好处是英文表根本不用写 —— 查不到就返回 key 本身，而上游本来全是英文，
 * 所以英文模式天然完整。只需要维护一张中文对照表，加语言也只是多一张表。
 *
 * 为什么不用 react-i18next：模板只需要双语、词条 126 条，i18next 打包要
 * 40KB+ 还要配插件。这里查表 + 一个 React state 就够了。
 */
import { useSyncExternalStore } from "react";
import { zh } from "./zh";

export type Locale = "zh-CN" | "en-US";

const STORAGE_KEY = "svgstudio-locale";

/** 当前语言。用模块级状态 + 订阅，任何组件都能读到并响应变化。 */
let current: Locale = "zh-CN";
const listeners = new Set<() => void>();

function emit() {
  listeners.forEach((l) => l());
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => listeners.delete(cb);
}

function getSnapshot(): Locale {
  return current;
}

/** 初始化。读 localStorage，没存过就用中文。 */
export function initLocale(): void {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === "zh-CN" || saved === "en-US") {
    current = saved;
  }
  document.documentElement.lang = current;
}

export function setLocale(l: Locale): void {
  current = l;
  localStorage.setItem(STORAGE_KEY, l);
  document.documentElement.lang = l;
  emit();
}

/**
 * 取词条。
 *
 * 中文模式查 zh 表，查不到返回 key（也就是英文原文）。
 * 英文模式直接返回 key —— 上游文案本来就是英文，不需要再维护一份。
 */
export function translate(key: string): string {
  if (current === "en-US") return key;
  return zh[key] ?? key;
}

/** 组件里用这个拿到响应式的 t。语言切换时自动重渲染。 */
export function useT(): (key: string) => string {
  const locale = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  // locale 参与闭包只是为了触发重渲染，实际查表看模块级 current
  void locale;
  return translate;
}

/** 给设置界面用的语言切换。 */
export function useLocale() {
  const locale = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return { locale, setLocale };
}
