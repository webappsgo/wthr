// Server-rendered one-shot flash messages per AI.md PART 16
package handler

import (
	"net/http"
	"strings"

	"github.com/webappsgo/wthr/src/server/middleware"
	"github.com/webappsgo/wthr/src/util"
)

// FlashCookieName is the short-lived cookie carrying a one-shot flash message.
const FlashCookieName = "flash"

// Flash is a one-shot message rendered as a static dismissible alert on the
// page reached by a POST-redirect-GET, so form feedback works without JavaScript.
type Flash struct {
	Kind string
	Key  string
}

// flashKinds are the alert styles a flash may request.
var flashKinds = map[string]bool{
	"success": true,
	"error":   true,
	"info":    true,
	"warning": true,
}

// flashKeys is the allow-list of translation keys a flash may name. The cookie
// value is attacker-controllable, so only keys listed here are ever rendered.
var flashKeys = map[string]bool{
	"flash.settings_saved":                                                          true,
	"flash.settings_save_failed":                                                    true,
	"flash.admin_invite_create_failed":                                              true,
	"flash.admin_invite_created":                                                    true,
	"flash.backup_password_mismatch":                                                true,
	"flash.channel_updated":                                                         true,
	"flash.channel_update_failed":                                                   true,
	"flash.channel_enabled":                                                         true,
	"flash.channel_disabled":                                                        true,
	"flash.channel_test_sent":                                                       true,
	"flash.channel_test_failed":                                                     true,
	"flash.email_template_saved":                                                    true,
	"flash.email_template_save_failed":                                              true,
	"flash.test_email_sent":                                                         true,
	"flash.test_email_failed":                                                       true,
	"flash.contact_sent":                                                            true,
	"flash.contact_saved":                                                           true,
	"flash.contact_invalid":                                                         true,
	"flash.contact_failed":                                                          true,
	"flash.contact_captcha_failed":                                                  true,
	"flash.contact_captcha_unavailable":                                             true,
	"flash.scheduler_settings_saved":                                                true,
	"flash.scheduler_task_saved":                                                    true,
	"flash.scheduler_task_enabled":                                                  true,
	"flash.scheduler_task_disabled":                                                 true,
	"flash.scheduler_run_started":                                                   true,
	"flash.scheduler_unknown_task":                                                  true,
	"flash.scheduler_invalid":                                                       true,
	"flash.encryption_rotated":                                                      true,
	"flash.encryption_password_invalid":                                             true,
	"flash.encryption_rotation_pending":                                             true,
	"flash.encryption_rotation_failed":                                              true,
	"flash.cors_invalid":                                                            true,
	"flash.operator_token_password_required":                                        true,
	"flash.legacy.errors.admin.admins.cannot_delete_your_own_account":               true,
	"flash.legacy.errors.admin.admins.current_password_is_incorrect":                true,
	"flash.legacy.errors.admin.admins.email_in_use":                                 true,
	"flash.legacy.errors.admin.admins.failed_to_accept_invite":                      true,
	"flash.legacy.errors.admin.admins.failed_to_count_admins":                       true,
	"flash.legacy.errors.admin.admins.failed_to_create_invite":                      true,
	"flash.legacy.errors.admin.admins.failed_to_delete_admin":                       true,
	"flash.legacy.errors.admin.admins.failed_to_fetch_admins":                       true,
	"flash.legacy.errors.admin.admins.failed_to_fetch_invites":                      true,
	"flash.legacy.errors.admin.admins.failed_to_fetch_preferences":                  true,
	"flash.legacy.errors.admin.admins.failed_to_fetch_sessions":                     true,
	"flash.legacy.errors.admin.admins.failed_to_load_profile":                       true,
	"flash.legacy.errors.admin.admins.failed_to_logout_sessions":                    true,
	"flash.legacy.errors.admin.admins.failed_to_regenerate_token":                   true,
	"flash.legacy.errors.admin.admins.failed_to_revoke_invite":                      true,
	"flash.legacy.errors.admin.admins.failed_to_update_admin":                       true,
	"flash.legacy.errors.admin.admins.failed_to_update_password":                    true,
	"flash.legacy.errors.admin.admins.failed_to_update_preferences":                 true,
	"flash.legacy.errors.admin.admins.failed_to_update_profile":                     true,
	"flash.legacy.errors.admin.admins.failed_to_verify_password":                    true,
	"flash.legacy.errors.admin.admins.invalid_admin_id":                             true,
	"flash.legacy.errors.admin.admins.invalid_email":                                true,
	"flash.legacy.errors.admin.admins.invalid_invite":                               true,
	"flash.legacy.errors.admin.admins.invalid_request_body":                         true,
	"flash.legacy.errors.admin.admins.invalid_theme":                                true,
	"flash.legacy.errors.admin.admins.invalid_username":                             true,
	"flash.legacy.errors.admin.admins.invite_not_found":                             true,
	"flash.legacy.errors.admin.admins.language_required":                            true,
	"flash.legacy.errors.admin.admins.no_invite_token_provided":                     true,
	"flash.legacy.errors.admin.admins.no_profile_fields":                            true,
	"flash.legacy.errors.admin.admins.not_authenticated":                            true,
	"flash.legacy.errors.admin.admins.other_admin_details_private":                  true,
	"flash.legacy.errors.admin.admins.passwords_do_not_match":                       true,
	"flash.legacy.errors.admin.admins.timezone_required":                            true,
	"flash.legacy.errors.admin.admins.token_is_required":                            true,
	"flash.legacy.errors.admin.admins.username_in_use":                              true,
	"flash.legacy.errors.admin.auth_settings.failed_to_update_settings":             true,
	"flash.legacy.errors.admin.auth_settings.invalid_request":                       true,
	"flash.legacy.errors.admin.core.cannot_modify_your_own_account_credentials":     true,
	"flash.legacy.errors.admin.core.failed_to_clear_logs":                           true,
	"flash.legacy.errors.admin.core.failed_to_create_user":                          true,
	"flash.legacy.errors.admin.core.failed_to_delete_user":                          true,
	"flash.legacy.errors.admin.core.failed_to_fetch_audit_logs":                     true,
	"flash.legacy.errors.admin.core.failed_to_fetch_scheduled_tasks":                true,
	"flash.legacy.errors.admin.core.failed_to_fetch_setting":                        true,
	"flash.legacy.errors.admin.core.failed_to_fetch_settings":                       true,
	"flash.legacy.errors.admin.core.failed_to_fetch_tokens":                         true,
	"flash.legacy.errors.admin.core.failed_to_fetch_users":                          true,
	"flash.legacy.errors.admin.core.failed_to_generate_token":                       true,
	"flash.legacy.errors.admin.core.failed_to_revoke_token":                         true,
	"flash.legacy.errors.admin.core.failed_to_update_password":                      true,
	"flash.legacy.errors.admin.core.failed_to_update_setting":                       true,
	"flash.legacy.errors.admin.core.failed_to_update_user":                          true,
	"flash.legacy.errors.admin.core.invalid_token_id":                               true,
	"flash.legacy.errors.admin.core.invalid_user_id":                                true,
	"flash.legacy.errors.admin.core.password_must_be_at_least_8_characters":         true,
	"flash.legacy.errors.admin.core.setting_not_found":                              true,
	"flash.legacy.errors.admin.core.user_id_and_name_are_required":                  true,
	"flash.legacy.errors.admin.core.value_is_required":                              true,
	"flash.legacy.errors.admin.email_templates.failed_to_import_template":           true,
	"flash.legacy.errors.admin.email_templates.failed_to_read_templates_directory":  true,
	"flash.legacy.errors.admin.email_templates.failed_to_save_template":             true,
	"flash.legacy.errors.admin.email_templates.invalid_template_name":               true,
	"flash.legacy.errors.admin.email_templates.subject_and_body_are_required":       true,
	"flash.legacy.errors.admin.email_templates.template_not_found":                  true,
	"flash.legacy.errors.admin.logging.invalid_format":                              true,
	"flash.legacy.errors.admin.logs.audit_log_file_not_found":                       true,
	"flash.legacy.errors.admin.logs.failed_to_archive_logs":                         true,
	"flash.legacy.errors.admin.logs.failed_to_read_audit_logs":                      true,
	"flash.legacy.errors.admin.logs.failed_to_read_logs":                            true,
	"flash.legacy.errors.admin.logs.failed_to_read_logs_directory":                  true,
	"flash.legacy.errors.admin.logs.failed_to_search_audit_logs":                    true,
	"flash.legacy.errors.admin.logs.failed_to_stat_log_file":                        true,
	"flash.legacy.errors.admin.logs.failed_to_truncate_log_file":                    true,
	"flash.legacy.errors.admin.logs.invalid_search_parameters":                      true,
	"flash.legacy.errors.admin.logs.log_file_not_found":                             true,
	"flash.legacy.errors.admin.logs.streaming_unsupported":                          true,
	"flash.legacy.errors.admin.metrics.failed_to_read_metrics":                      true,
	"flash.legacy.errors.admin.profile.invalid_pgp_key":                             true,
	"flash.legacy.errors.admin.scheduler.invalid_request_body":                      true,
	"flash.legacy.errors.admin.security.operator_token_failed":                      true,
	"flash.legacy.errors.admin.settings.failed_to_load_settings":                    true,
	"flash.legacy.errors.admin.settings.load_failed":                                true,
	"flash.legacy.errors.admin.ssl.certificate_has_expired":                         true,
	"flash.legacy.errors.admin.ssl.certificate_is_not_yet_valid":                    true,
	"flash.legacy.errors.admin.ssl.dns_credential_name_invalid":                     true,
	"flash.legacy.errors.admin.ssl.dns_encryption_unavailable":                      true,
	"flash.legacy.errors.admin.ssl.dns_provider_check_failed":                       true,
	"flash.legacy.errors.admin.ssl.dns_provider_unsupported":                        true,
	"flash.legacy.errors.admin.ssl.dns_save_failed":                                 true,
	"flash.legacy.errors.admin.ssl.failed_to_check_renewal_status":                  true,
	"flash.legacy.errors.admin.ssl.failed_to_initialize_lets_encrypt_service":       true,
	"flash.legacy.errors.admin.ssl.failed_to_obtain_certificate":                    true,
	"flash.legacy.errors.admin.ssl.failed_to_renew_certificate":                     true,
	"flash.legacy.errors.admin.ssl.failed_to_revoke_certificate":                    true,
	"flash.legacy.errors.admin.ssl.invalid_challenge_type":                          true,
	"flash.legacy.errors.admin.ssl.let_s_encrypt_service_not_initialized":           true,
	"flash.legacy.errors.admin.ssl.no_certificate_found_or_unable_to_load":          true,
	"flash.legacy.errors.admin.totp.already_enabled":                                true,
	"flash.legacy.errors.admin.totp.code_invalid":                                   true,
	"flash.legacy.errors.admin.totp.disable_failed":                                 true,
	"flash.legacy.errors.admin.totp.enable_failed":                                  true,
	"flash.legacy.errors.admin.totp.invalid_request":                                true,
	"flash.legacy.errors.admin.totp.password_invalid":                               true,
	"flash.legacy.errors.admin.totp.recovery_ack_required":                          true,
	"flash.legacy.errors.admin.totp.recovery_failed":                                true,
	"flash.legacy.errors.admin.totp.setup_failed":                                   true,
	"flash.legacy.errors.admin.totp.setup_missing":                                  true,
	"flash.legacy.errors.admin.totp.status_failed":                                  true,
	"flash.legacy.errors.admin.users.invalid_registration_mode":                     true,
	"flash.legacy.errors.admin.web.feed_update_failed":                              true,
	"flash.legacy.errors.admin.web.security_update_failed":                          true,
	"flash.legacy.success.admin.admins.admin_account_created_successfully":          true,
	"flash.legacy.success.admin.admins.admin_deleted_successfully":                  true,
	"flash.legacy.success.admin.admins.admin_updated_successfully":                  true,
	"flash.legacy.success.admin.admins.invitation_created_successfully":             true,
	"flash.legacy.success.admin.admins.invite_revoked_successfully":                 true,
	"flash.legacy.success.admin.admins.password_updated_successfully":               true,
	"flash.legacy.success.admin.core.admin_deleted":                                 true,
	"flash.legacy.success.admin.core.admin_disabled":                                true,
	"flash.legacy.success.admin.core.admin_enabled":                                 true,
	"flash.legacy.success.admin.core.admin_invite_created":                          true,
	"flash.legacy.success.admin.core.audit_logs_cleared_successfully":               true,
	"flash.legacy.success.admin.core.password_updated_successfully":                 true,
	"flash.legacy.success.admin.core.setting_updated_successfully":                  true,
	"flash.legacy.success.admin.core.token_generated_successfully":                  true,
	"flash.legacy.success.admin.core.token_revoked_successfully":                    true,
	"flash.legacy.success.admin.core.user_deleted_successfully":                     true,
	"flash.legacy.success.admin.core.user_updated_successfully":                     true,
	"flash.legacy.success.admin.debug.configuration_reload_triggered":               true,
	"flash.legacy.success.admin.debug.garbage_collection_triggered":                 true,
	"flash.legacy.success.admin.email_templates.template_imported_successfully":     true,
	"flash.legacy.success.admin.email_templates.template_updated_successfully":      true,
	"flash.legacy.success.admin.email_templates.test_email_sent_using_template":     true,
	"flash.legacy.success.admin.invites.created":                                    true,
	"flash.legacy.success.admin.invites.revoked":                                    true,
	"flash.legacy.success.admin.logging.fail2ban_configuration_saved":               true,
	"flash.legacy.success.admin.logging.log_format_updated":                         true,
	"flash.legacy.success.admin.logging.logging_formats_updated_successfully":       true,
	"flash.legacy.success.admin.logging.syslog_configuration_saved":                 true,
	"flash.legacy.success.admin.logs.logs_cleared_successfully":                     true,
	"flash.legacy.success.admin.logs.logs_rotated_successfully":                     true,
	"flash.legacy.success.admin.metrics.metrics_configuration_updated_successfully": true,
	"flash.legacy.success.admin.setup.admin_account_created":                        true,
	"flash.legacy.success.admin.setup.complete":                                     true,
	"flash.legacy.success.admin.setup.email_settings_updated":                       true,
	"flash.legacy.success.admin.setup.restart_initiated":                            true,
	"flash.legacy.success.admin.setup.security_settings_saved":                      true,
	"flash.legacy.success.admin.setup.server_config_saved":                          true,
	"flash.legacy.success.admin.setup.services_configured":                          true,
	"flash.legacy.success.admin.setup.test_email_unavailable":                       true,
	"flash.legacy.success.admin.setup.web_settings_updated":                         true,
	"flash.legacy.success.admin.ssl.auto_renewal_started":                           true,
	"flash.legacy.success.admin.ssl.certificate_does_not_need_renewal_yet":          true,
	"flash.legacy.success.admin.ssl.certificate_is_valid":                           true,
	"flash.legacy.success.admin.ssl.certificate_obtained_successfully":              true,
	"flash.legacy.success.admin.ssl.certificate_renewed_successfully":               true,
	"flash.legacy.success.admin.ssl.certificate_revoked_successfully":               true,
	"flash.legacy.success.admin.ssl.dns_checked":                                    true,
	"flash.legacy.success.admin.ssl.dns_records_for_manual_configuration":           true,
	"flash.legacy.success.admin.ssl.dns_saved":                                      true,
	"flash.legacy.success.admin.totp.disabled":                                      true,
	"flash.legacy.success.admin.totp.enabled":                                       true,
	"flash.legacy.success.admin.totp.recovery_regenerated":                          true,
}

// SetFlash stores a one-shot flash message for the next rendered page.
func SetFlash(w http.ResponseWriter, r *http.Request, kind, key string) {
	if middleware.GPCOptOut(r) {
		return
	}
	if !flashKinds[kind] {
		return
	}
	if !flashKeys[key] {
		if !isLegacyFlashTranslation(key) {
			return
		}
		key = "flash.legacy." + key
	}

	secure := util.TrustedIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     FlashCookieName,
		Value:    kind + ":" + key,
		Path:     "/",
		MaxAge:   120,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// TakeFlash returns the pending flash message, if any, and clears the cookie so
// the message is displayed exactly once.
func TakeFlash(w http.ResponseWriter, r *http.Request) *Flash {
	if middleware.GPCOptOut(r) {
		return nil
	}
	cookie, err := r.Cookie(FlashCookieName)
	if err != nil || cookie.Value == "" {
		return nil
	}

	secure := util.TrustedIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     FlashCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	kind, key, found := strings.Cut(cookie.Value, ":")
	if !found || !flashKinds[kind] {
		return nil
	}
	if strings.HasPrefix(key, "flash.legacy.") {
		key = strings.TrimPrefix(key, "flash.legacy.")
		if !isLegacyFlashTranslation(key) {
			return nil
		}
	} else if !flashKeys[key] {
		return nil
	}

	return &Flash{Kind: kind, Key: key}
}

// isLegacyFlashTranslation accepts existing server-owned success and error
// message IDs so form handlers can share wording with their API responses.
func isLegacyFlashTranslation(key string) bool {
	return flashKeys["flash.legacy."+key]
}
