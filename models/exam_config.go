package models

import "encoding/json"

// ApplyExamConfigDefaults fills security-related defaults for legacy exams whose
// config JSON predates security_* keys (zero values would otherwise disable security).
func ApplyExamConfigDefaults(cfg *ExamConfig, rawJSON []byte) {
	var raw map[string]json.RawMessage
	if len(rawJSON) > 0 {
		_ = json.Unmarshal(rawJSON, &raw)
	}
	has := func(key string) bool {
		_, ok := raw[key]
		return ok
	}
	if !has("security_enabled") {
		cfg.SecurityEnabled = true
	}
	if !has("max_violations") {
		if cfg.SecurityEnabled {
			cfg.MaxViolations = 3
		}
	}
	if !has("require_fullscreen") {
		cfg.RequireFullscreen = true
	}
	if !has("block_copy_paste") {
		cfg.BlockCopyPaste = true
	}
	if !has("detect_focus_loss") {
		cfg.DetectFocusLoss = true
	}
}

// SecurityPayload returns client-facing proctoring settings for an exam session.
func (c ExamConfig) SecurityPayload() map[string]interface{} {
	enabled := c.SecurityEnabled
	return map[string]interface{}{
		"enabled":            enabled,
		"max_violations":     c.MaxViolations,
		"require_fullscreen": enabled && c.RequireFullscreen,
		"block_copy_paste":   enabled && c.BlockCopyPaste,
		"detect_focus_loss":  enabled && c.DetectFocusLoss,
	}
}
