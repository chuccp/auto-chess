package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chuccp/auto-chess/internal/feedback"
	"github.com/chuccp/auto-chess/internal/webui"
	"github.com/chuccp/go-web-frame/log"
	"github.com/chuccp/go-web-frame/web"
	"go.uber.org/zap"
)

type feedbackRequest struct {
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Message       string `json:"message"`
	CaptchaID     string `json:"captchaId"`
	CaptchaAnswer []int  `json:"captchaAnswer"`
}

func (c *Controller) feedbackPage(req *web.Request) (any, error) {
	return c.renderPage(req, http.StatusOK, "feedback", webui.PageData{
		Title:       "意见反馈 · auto-chess",
		Description: "提交使用问题或改进建议；手机号和邮箱选填，提交前需按图片提示依次选字验证。",
		Active:      "feedback",
		Page:        "feedback",
	})
}

func (c *Controller) captcha(req *web.Request) (any, error) {
	if c.captchas == nil {
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "验证服务未初始化")
	}
	lang := req.Query("lang")
	if lang == "" {
		lang = req.Cookie().Get("auto-chess-lang")
	}
	view, err := c.captchas.Issue(webui.NormalizeLang(lang))
	if errors.Is(err, feedback.ErrCaptchaBusy) {
		return fail(req, http.StatusTooManyRequests, "CAPTCHA_BUSY", "验证码请求过多，请稍后再试")
	}
	if err != nil {
		log.Error("issue captcha", zap.Error(err))
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "验证码生成失败")
	}
	req.Response().Header().Set("Cache-Control", "no-store")
	return ok(req, view)
}

func (c *Controller) captchaImage(req *web.Request) (any, error) {
	if c.captchas == nil {
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "验证服务未初始化")
	}
	svg, err := c.captchas.Render(req.Param("id"))
	if err != nil {
		return fail(req, http.StatusNotFound, "CAPTCHA_EXPIRED", "验证码已过期，请刷新")
	}
	header := req.Response().Header()
	header.Set("Content-Type", "image/svg+xml; charset=utf-8")
	header.Set("Cache-Control", "no-store, max-age=0")
	header.Set("X-Content-Type-Options", "nosniff")
	_, _ = req.Response().Write(svg)
	return nil, nil
}

func (c *Controller) submitFeedback(req *web.Request) (any, error) {
	if c.captchas == nil || c.feedbackStore == nil {
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "反馈服务未初始化")
	}
	var body feedbackRequest
	if err := bindJSON(req, &body); err != nil {
		return fail(req, http.StatusBadRequest, "BAD_REQUEST", "请求体不是合法 JSON："+err.Error())
	}
	phone := strings.TrimSpace(body.Phone)
	email := strings.TrimSpace(body.Email)
	message := strings.TrimSpace(body.Message)
	if err := feedback.Validate(phone, email, message); err != nil {
		return fail(req, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	if body.CaptchaID == "" || len(body.CaptchaAnswer) == 0 {
		return fail(req, http.StatusBadRequest, "CAPTCHA_REQUIRED", "请先完成按顺序选字验证")
	}
	if err := c.captchas.Verify(body.CaptchaID, body.CaptchaAnswer); err != nil {
		if errors.Is(err, feedback.ErrCaptchaExpired) {
			return fail(req, http.StatusBadRequest, "CAPTCHA_EXPIRED", "验证已过期，请重新验证")
		}
		return fail(req, http.StatusBadRequest, "CAPTCHA_INVALID", "选字顺序不正确，请重新验证")
	}
	entry := feedback.Entry{
		Time:    time.Now(),
		Phone:   phone,
		Email:   email,
		Message: message,
	}
	if err := c.feedbackStore.Append(entry); err != nil {
		log.Error("save feedback", zap.Error(err))
		return fail(req, http.StatusInternalServerError, "INTERNAL_ERROR", "反馈保存失败，请稍后重试")
	}
	return ok(req, map[string]any{"received": true})
}
