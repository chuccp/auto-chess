(function () {
  "use strict";

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
  const BASE = location.origin;
  const isEN = () => window.__LANG === "en" || document.documentElement.lang === "en";
  const T = (zh, en) => (isEN() ? en : zh);

  function showToast(message) {
    const el = $("#toast");
    if (!el) return;
    el.textContent = message;
    el.classList.add("show");
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => el.classList.remove("show"), 1900);
  }

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      showToast(T("已复制到剪贴板", "Copied to clipboard"));
    } catch (_) {
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy");
        showToast(T("已复制到剪贴板", "Copied to clipboard"));
      } catch (e) {
        showToast(T("复制失败，请手动选择", "Copy failed - select and copy manually"));
      }
      area.remove();
    }
  }

  function setupNav() {
    const toggle = $("#nav-toggle");
    const nav = $("#site-nav");
    if (!toggle || !nav) return;
    toggle.addEventListener("click", () => {
      const open = nav.classList.toggle("open");
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
    nav.addEventListener("click", () => {
      nav.classList.remove("open");
      toggle.setAttribute("aria-expanded", "false");
    });
  }

  function setupCopy() {
    $$("[data-copy-target]").forEach((button) => {
      button.addEventListener("click", () => {
        const target = document.getElementById(button.dataset.copyTarget);
        if (target) copyText(target.innerText);
      });
    });
  }

  async function loadHealth() {
    const status = $("#service-status");
    if (!status) return;
    try {
      const res = await fetch(BASE + "/api/v1/health", { cache: "no-store" });
      if (!res.ok) throw new Error("HTTP " + res.status);
      const payload = await res.json();
      const games = payload?.data?.games ?? 7;
      status.classList.add("online");
      status.classList.remove("offline");
      $(".status-text", status).textContent = T(`在线 · ${games} 棋种`, `Online · ${games} games`);
    } catch (_) {
      status.classList.add("offline");
      $(".status-text", status).textContent = T("服务未连接", "Service unavailable");
    }
  }

  function setupTheme() {
    const root = document.documentElement;
    const toggle = $("#theme-toggle");
    const icon = $("#theme-icon");
    const apply = (theme) => {
      root.setAttribute("data-theme", theme);
      if (icon) icon.textContent = theme === "dark" ? "☀" : "☾";
      if (toggle) {
        const label = theme === "dark"
          ? T("切换到亮色主题", "Switch to light theme")
          : T("切换到深色主题", "Switch to dark theme");
        toggle.setAttribute("aria-label", label);
        toggle.setAttribute("title", label);
        toggle.setAttribute("aria-pressed", theme === "dark" ? "true" : "false");
      }
    };
    apply(root.getAttribute("data-theme") === "dark" ? "dark" : "light");
    if (!toggle) return;
    toggle.addEventListener("click", () => {
      const next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
      apply(next);
      try {
        localStorage.setItem("auto-chess-theme", next);
      } catch (_) {
        // ignore storage failures
      }
    });
  }

  function init() {
    setupTheme();
    setupNav();
    setupCopy();
    loadHealth();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
