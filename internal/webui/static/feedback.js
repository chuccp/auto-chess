(function () {
  "use strict";

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
  const BASE = window.__REL_BASE__ || "./";
  const isEN = () => window.__LANG === "en" || document.documentElement.lang === "en";
  const T = (zh, en) => (isEN() ? en : zh);

  const form = $("#feedback-form");
  const modal = $("#captcha-modal");
  if (!form || !modal) return;

  const phoneInput = $("#fb-phone");
  const emailInput = $("#fb-email");
  const messageInput = $("#fb-message");
  const counter = $("#fb-counter");
  const formError = $("#fb-error");
  const submitButton = $("#fb-submit");
  const image = $("#captcha-image");
  const hits = $("#captcha-hits");
  const promptEl = $("#captcha-prompt");
  const statusEl = $("#captcha-status");
  const refreshButton = $("#captcha-refresh");
  const resetButton = $("#captcha-reset");

  const PHONE_RE = /^[0-9+\-() ]{5,40}$/;
  const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;

  let challenge = null;
  let picks = [];
  let pending = null;
  let verifying = false;
  let ready = false;
  let loadSeq = 0;

  function showToast(message) {
    const el = $("#toast");
    if (!el) return;
    el.textContent = message;
    el.classList.add("show");
    clearTimeout(showToast.timer);
    showToast.timer = setTimeout(() => el.classList.remove("show"), 2200);
  }

  function runeLength(value) {
    return Array.from(value).length;
  }

  function updateCounter() {
    if (!counter) return;
    counter.textContent = runeLength(messageInput.value) + " / 2000";
  }

  function setFormError(text) {
    if (!formError) return;
    formError.textContent = text || "";
    formError.hidden = !text;
  }

  function setStatus(text, kind) {
    if (!statusEl) return;
    statusEl.textContent = text || "";
    statusEl.className = "captcha-status" + (kind ? " " + kind : "");
  }

  function selectionText() {
    if (!challenge) return "";
    return T(
      `已选择 ${picks.length} / ${challenge.count}`,
      `Selected ${picks.length} / ${challenge.count}`
    );
  }

  function readForm() {
    const phone = phoneInput.value.trim();
    const email = emailInput.value.trim();
    const message = messageInput.value.trim();
    if (!message) {
      setFormError(T("请先填写意见或建议。", "Please enter your feedback first."));
      messageInput.focus();
      return null;
    }
    if (runeLength(message) > 2000) {
      setFormError(T("意见最多 2000 个字。", "Feedback is limited to 2000 characters."));
      messageInput.focus();
      return null;
    }
    if (phone && !PHONE_RE.test(phone)) {
      setFormError(T("手机号 / 联系方式格式不正确。", "That phone or contact number looks invalid."));
      phoneInput.focus();
      return null;
    }
    if (email && !EMAIL_RE.test(email)) {
      setFormError(T("邮箱格式不正确。", "That email address looks invalid."));
      emailInput.focus();
      return null;
    }
    setFormError("");
    return { phone: phone, email: email, message: message };
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    if (verifying) return;
    const payload = readForm();
    if (!payload) return;
    pending = payload;
    openCaptcha();
  });

  [phoneInput, emailInput, messageInput].forEach((input) => {
    input.addEventListener("input", () => {
      setFormError("");
      if (input === messageInput) updateCounter();
    });
  });

  function openCaptcha() {
    picks = [];
    verifying = false;
    modal.hidden = false;
    modal.setAttribute("aria-hidden", "false");
    document.body.classList.add("captcha-open");
    promptEl.textContent = T(
      "请按图片中的提示，依次点击正确的字",
      "Click the characters in the image in the order shown"
    );
    setStatus(T("正在加载验证…", "Loading verification…"));
    loadChallenge().then(() => {
      const first = hits.firstElementChild;
      if (first) first.focus();
    });
  }

  function closeCaptcha() {
    if (verifying) return;
    loadSeq++;
    ready = false;
    challenge = null;
    picks = [];
    pending = null;
    hits.innerHTML = "";
    image.removeAttribute("src");
    modal.hidden = true;
    modal.setAttribute("aria-hidden", "true");
    document.body.classList.remove("captcha-open");
    if (submitButton) submitButton.focus();
  }

  $$("[data-captcha-close]", modal).forEach((el) => el.addEventListener("click", closeCaptcha));

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !modal.hidden) closeCaptcha();
  });

  refreshButton.addEventListener("click", () => {
    if (verifying) return;
    picks = [];
    loadChallenge();
  });

  resetButton.addEventListener("click", () => {
    if (verifying) return;
    picks = [];
    updateMarks();
    setStatus(selectionText());
  });

  async function loadChallenge(statusText, statusKind) {
    const seq = ++loadSeq;
    ready = false;
    picks = [];
    hits.innerHTML = "";
    challenge = null;
    setStatus(T("正在加载验证…", "Loading verification…"));
    try {
      const lang = isEN() ? "en" : "zh";
      const res = await fetch(BASE + "api/v1/captcha?lang=" + lang, { cache: "no-store" });
      let payload = null;
      try {
        payload = await res.json();
      } catch (_) {
        // fall through to the HTTP status error
      }
      if (seq !== loadSeq) return;
      if (!res.ok || !payload || !payload.ok) {
        const message = payload && payload.error
          ? captchaErrorText(payload.error.code, payload.error.message)
          : "HTTP " + res.status;
        throw new Error(message);
      }
      challenge = payload.data;
      promptEl.textContent = challenge.prompt || promptEl.textContent;
      const stage = $("#captcha-stage");
      if (stage) stage.style.aspectRatio = challenge.width + " / " + challenge.height;
      renderHits();

      image.onload = () => {
        if (seq !== loadSeq) return;
        ready = true;
        // Keep an error from the previous attempt visible until the visitor
        // starts selecting again.
        setStatus(statusText || selectionText(), statusKind);
      };
      image.onerror = () => {
        if (seq !== loadSeq) return;
        ready = false;
        setStatus(T("图片加载失败，请点击“换一张”。", "Image failed to load. Click “New image”."), "error");
      };
      image.src = BASE + "api/v1/captcha/" + encodeURIComponent(challenge.id) + "/image";
      if (statusText) setStatus(statusText, statusKind);
    } catch (err) {
      if (seq !== loadSeq) return;
      challenge = null;
      hits.innerHTML = "";
      setStatus(err && err.message ? err.message : T("验证加载失败，请重试。", "Could not load verification. Try again."), "error");
    }
  }

  function renderHits() {
    hits.innerHTML = "";
    if (!challenge) return;
    challenge.tiles.forEach((tile, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "captcha-hit";
      button.style.left = (tile.x / challenge.width) * 100 + "%";
      button.style.top = (tile.y / challenge.height) * 100 + "%";
      button.style.width = (tile.w / challenge.width) * 100 + "%";
      button.style.height = (tile.h / challenge.height) * 100 + "%";
      button.setAttribute("aria-label", T(`第 ${index + 1} 个位置`, `Position ${index + 1}`));
      button.addEventListener("click", () => pick(index));
      hits.appendChild(button);
    });
  }

  function pick(index) {
    if (verifying || !challenge || !ready) return;
    const at = picks.indexOf(index);
    if (at >= 0) {
      picks.splice(at, 1);
    } else if (picks.length < challenge.count) {
      picks.push(index);
    } else {
      return;
    }
    updateMarks();
    setStatus(selectionText());
    if (picks.length === challenge.count) verify();
  }

  function updateMarks() {
    Array.from(hits.children).forEach((button, index) => {
      const order = picks.indexOf(index);
      if (order >= 0) {
        button.dataset.order = String(order + 1);
      } else {
        delete button.dataset.order;
      }
    });
  }

  async function verify() {
    if (verifying || !challenge || !pending) return;
    verifying = true;
    refreshButton.disabled = true;
    resetButton.disabled = true;
    setStatus(T("正在验证…", "Verifying…"));
    const sentId = challenge.id;
    const answer = picks.slice();
    try {
      const res = await fetch(BASE + "api/v1/feedback", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          phone: pending.phone,
          email: pending.email,
          message: pending.message,
          captchaId: sentId,
          captchaAnswer: answer,
        }),
      });
      let payload = null;
      try {
        payload = await res.json();
      } catch (_) {
        // treat an unreadable body as a failed request
      }
      if (res.ok && payload && payload.ok) {
        setStatus(T("验证成功，反馈已提交。", "Verified — feedback submitted."), "ok");
        showToast(T("感谢你的反馈！", "Thanks for your feedback!"));
        form.reset();
        updateCounter();
        pending = null;
        challenge = null;
        picks = [];
        hits.innerHTML = "";
        setTimeout(() => {
          verifying = false;
          closeCaptcha();
        }, 1200);
        return;
      }
      const code = payload && payload.error ? payload.error.code : "";
      const fallback = payload && payload.error ? payload.error.message : "";
      const message = captchaErrorText(code, fallback);
      challenge = null;
      picks = [];
      hits.innerHTML = "";
      await loadChallenge(message, "error");
    } catch (_) {
      challenge = null;
      picks = [];
      hits.innerHTML = "";
      await loadChallenge(
        T("网络异常，请稍后重试。", "Network error. Please try again."),
        "error"
      );
    } finally {
      refreshButton.disabled = false;
      resetButton.disabled = false;
      if (pending) verifying = false;
    }
  }

  function captchaErrorText(code, fallback) {
    switch (code) {
      case "CAPTCHA_INVALID":
        return T("选字顺序不对，请重新验证。", "Wrong character order. Please try again.");
      case "CAPTCHA_EXPIRED":
      case "CAPTCHA_NOT_FOUND":
        return T("验证已过期，请重新验证。", "Verification expired. Please try again.");
      case "CAPTCHA_BUSY":
        return T("验证请求过多，请稍后再试。", "Too many verification requests. Try again later.");
      default:
        return fallback || T("验证失败，请重试。", "Verification failed. Please try again.");
    }
  }

  updateCounter();
})();
