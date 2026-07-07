package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/middleware"
)

// GetConfig handles GET /api/admin/config (requires admin JWT)
func (h *AdminHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, err := h.db.GetConfig(ctx, "global")
	if err != nil || cfg == nil {
		apierror.NotFound("Config not found").Write(w)
		return
	}

	var storedConfig struct {
		EmailEnabled  bool   `json:"email_enabled"`
		ResendApiKey  string `json:"resend_api_key"`
		EmailFrom     string `json:"email_from"`
		AdminPassword string `json:"admin_password"`
	}
	if err := json.Unmarshal([]byte(cfg.Config), &storedConfig); err != nil {
		apierror.InternalError("Internal server error").Write(w)
		return
	}

	resendApiKey := storedConfig.ResendApiKey
	if resendApiKey != "" {
		decrypted, err := crypto.Decrypt(resendApiKey)
		if err != nil {
			resendApiKey = storedConfig.ResendApiKey
		} else {
			resendApiKey = decrypted
		}
	}

	maskedApiKey := ""
	if resendApiKey != "" {
		maskedApiKey = maskedKey
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"emailEnabled": storedConfig.EmailEnabled,
		"resendApiKey": maskedApiKey,
		"emailFrom":    storedConfig.EmailFrom,
	})
}

// UpdateConfig handles PATCH /api/v1/admin/config (requires admin JWT)
func (h *AdminHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	updates, err := h.parseConfigUpdates(w, r)
	if err != nil {
		apierror.BadRequest("Invalid request body").Write(w)
		return
	}

	ctx := r.Context()
	cfg, err := h.db.GetConfig(ctx, "global")
	if err != nil || cfg == nil {
		apierror.NotFound("Config not found").Write(w)
		return
	}

	var storedConfig map[string]interface{}
	if err := json.Unmarshal([]byte(cfg.Config), &storedConfig); err != nil {
		storedConfig = make(map[string]interface{})
	}

	beforeConfig := maskSensitiveFields(storedConfig)

	if !h.applyConfigUpdates(ctx, w, r, storedConfig, updates) {
		return
	}

	if err := h.saveConfig(ctx, cfg, storedConfig); err != nil {
		apierror.InternalError("Failed to save config").Write(w)
		return
	}

	h.auditConfigChange(ctx, r, beforeConfig, storedConfig)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Config updated"})
}

// applyConfigUpdates applies the requested updates to storedConfig.
func (h *AdminHandler) applyConfigUpdates(ctx context.Context, w http.ResponseWriter, r *http.Request, storedConfig map[string]interface{}, updates *configUpdates) bool {
	if updates.EmailEnabled != nil {
		storedConfig["email_enabled"] = *updates.EmailEnabled
	}
	if updates.ResendApiKey != nil && *updates.ResendApiKey != maskedKey {
		encrypted, err := crypto.Encrypt(*updates.ResendApiKey)
		if err != nil {
			apierror.InternalError("Failed to encrypt API key").Write(w)
			return false
		}
		storedConfig["resend_api_key"] = encrypted
	}
	if updates.EmailFrom != nil {
		storedConfig["email_from"] = *updates.EmailFrom
	}
	if updates.AdminPassword != nil {
		if updates.OldPassword == nil {
			apierror.BadRequest("oldPassword required to change adminPassword").Write(w)
			return false
		}
		currentPwd, _ := storedConfig["admin_password"].(string)
		if !compareAdminPassword(*updates.OldPassword, currentPwd) {
			apierror.Unauthorized("wrong old password").Write(w)
			return false
		}
		hashed, err := hashAdminPassword(*updates.AdminPassword)
		if err != nil {
			apierror.InternalError("Failed to hash password").Write(w)
			return false
		}
		storedConfig["admin_password"] = hashed
		AuditPasswordChange(ctx, middleware.ExtractClientIP(r))

		// Revoke ALL admin sessions, not just the current one (H5).
		if h.redis != nil {
			h.revokeAllAdminSessions(ctx)
		}
	}
	return true
}

// saveConfig marshals storedConfig and persists it to the database.
func (h *AdminHandler) saveConfig(ctx context.Context, cfg *domain.AppConfig, storedConfig map[string]interface{}) error {
	configBytes, err := json.Marshal(storedConfig)
	if err != nil {
		return err
	}
	cfg.Config = string(configBytes)
	cfg.UpdatedAt = time.Now().UnixMilli()
	return h.db.SaveConfig(ctx, cfg)
}
