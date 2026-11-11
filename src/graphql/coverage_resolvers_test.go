package graphql

import (
	"context"
	"reflect"
	"time"

	"github.com/99designs/gqlgen/graphql"
	model "github.com/webappsgo/wthr/src/server/model"
)

type coverageResolver struct {
	*Resolver
	query    QueryResolver
	mutation MutationResolver
}

func (r *coverageResolver) Query() QueryResolver       { return r.query }
func (r *coverageResolver) Mutation() MutationResolver { return r.mutation }

type coverageQueryResolver struct{}
type coverageMutationResolver struct{}

func graphQLCoverageFixture[T any]() T {
	var value T
	fixtureGraphQLValue(reflect.ValueOf(&value).Elem(), 0)
	return value
}

func fixtureGraphQLValue(value reflect.Value, depth int) {
	if !value.IsValid() || !value.CanSet() || depth > 4 {
		return
	}
	switch value.Kind() {
	case reflect.Pointer:
		value.Set(reflect.New(value.Type().Elem()))
		fixtureGraphQLValue(value.Elem(), depth+1)
	case reflect.Interface:
		if value.NumMethod() == 0 {
			value.Set(reflect.ValueOf(map[string]interface{}{}))
		}
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			value.Set(reflect.ValueOf(time.Unix(1, 0).UTC()))
			return
		}
		for i := 0; i < value.NumField(); i++ {
			fixtureGraphQLValue(value.Field(i), depth+1)
		}
	case reflect.Slice:
		value.Set(reflect.MakeSlice(value.Type(), 1, 1))
		fixtureGraphQLValue(value.Index(0), depth+1)
	case reflect.Map:
		value.Set(reflect.MakeMap(value.Type()))
	case reflect.String:
		value.SetString("value")
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value.SetUint(1)
	case reflect.Float32, reflect.Float64:
		value.SetFloat(1)
	}
}

func (coverageQueryResolver) Health(ctx context.Context) (*HealthStatus, error) {
	return graphQLCoverageFixture[*HealthStatus](), nil
}

func (coverageQueryResolver) Weather(ctx context.Context, location *string, lat *float64, lon *float64) (*Weather, error) {
	return graphQLCoverageFixture[*Weather](), nil
}

func (coverageQueryResolver) Forecast(ctx context.Context, location *string, lat *float64, lon *float64, days *int) (*Weather, error) {
	return graphQLCoverageFixture[*Weather](), nil
}

func (coverageQueryResolver) SearchLocations(ctx context.Context, query string) ([]*LocationSearchResult, error) {
	return graphQLCoverageFixture[[]*LocationSearchResult](), nil
}

func (coverageQueryResolver) IPGeolocation(ctx context.Context, ip *string) (*IPGeolocation, error) {
	return graphQLCoverageFixture[*IPGeolocation](), nil
}

func (coverageQueryResolver) CurrentLocation(ctx context.Context) (*IPGeolocation, error) {
	return graphQLCoverageFixture[*IPGeolocation](), nil
}

func (coverageQueryResolver) HistoricalWeather(ctx context.Context, location string, date string) (*HistoricalWeather, error) {
	return graphQLCoverageFixture[*HistoricalWeather](), nil
}

func (coverageQueryResolver) LookupZipCode(ctx context.Context, code string) (*ZipCodeLookup, error) {
	return graphQLCoverageFixture[*ZipCodeLookup](), nil
}

func (coverageQueryResolver) LookupCoordinates(ctx context.Context, lat float64, lon float64) (*CoordinatesLookup, error) {
	return graphQLCoverageFixture[*CoordinatesLookup](), nil
}

func (coverageQueryResolver) Earthquakes(ctx context.Context, minMagnitude *float64, limit *int) ([]*Earthquake, error) {
	return graphQLCoverageFixture[[]*Earthquake](), nil
}

func (coverageQueryResolver) Hurricanes(ctx context.Context, active *bool) ([]*Hurricane, error) {
	return graphQLCoverageFixture[[]*Hurricane](), nil
}

func (coverageQueryResolver) SevereWeather(ctx context.Context, location *string) ([]*SevereWeather, error) {
	return graphQLCoverageFixture[[]*SevereWeather](), nil
}

func (coverageQueryResolver) MoonPhase(ctx context.Context, date *string) (*MoonPhase, error) {
	return graphQLCoverageFixture[*MoonPhase](), nil
}

func (coverageQueryResolver) PublicUserProfile(ctx context.Context, username string) (*PublicUserProfile, error) {
	return graphQLCoverageFixture[*PublicUserProfile](), nil
}

func (coverageQueryResolver) ValidateUserInvite(ctx context.Context, token string) (*UserInviteValidation, error) {
	return graphQLCoverageFixture[*UserInviteValidation](), nil
}

func (coverageQueryResolver) ValidateServerInvite(ctx context.Context, token string) (*ServerInviteValidation, error) {
	return graphQLCoverageFixture[*ServerInviteValidation](), nil
}

func (coverageQueryResolver) CurrentUser(ctx context.Context) (*model.User, error) {
	return graphQLCoverageFixture[*model.User](), nil
}

func (coverageQueryResolver) CurrentUserAvatar(ctx context.Context) (*PublicAvatar, error) {
	return graphQLCoverageFixture[*PublicAvatar](), nil
}

func (coverageQueryResolver) CurrentUserTwoFactorStatus(ctx context.Context) (*TOTPStatus, error) {
	return graphQLCoverageFixture[*TOTPStatus](), nil
}

func (coverageQueryResolver) CurrentUserTwoFactorSetup(ctx context.Context) (*TOTPSetup, error) {
	return graphQLCoverageFixture[*TOTPSetup](), nil
}

func (coverageQueryResolver) CurrentUserPasskeys(ctx context.Context) ([]*UserPasskey, error) {
	return graphQLCoverageFixture[[]*UserPasskey](), nil
}

func (coverageQueryResolver) UserSettings(ctx context.Context) (*UserSettings, error) {
	return graphQLCoverageFixture[*UserSettings](), nil
}

func (coverageQueryResolver) UserTokens(ctx context.Context) ([]*UserToken, error) {
	return graphQLCoverageFixture[[]*UserToken](), nil
}

func (coverageQueryResolver) SavedLocations(ctx context.Context) ([]*model.SavedLocation, error) {
	return graphQLCoverageFixture[[]*model.SavedLocation](), nil
}

func (coverageQueryResolver) SavedLocation(ctx context.Context, id string) (*model.SavedLocation, error) {
	return graphQLCoverageFixture[*model.SavedLocation](), nil
}

func (coverageQueryResolver) Notifications(ctx context.Context) ([]*model.Notification, error) {
	return graphQLCoverageFixture[[]*model.Notification](), nil
}

func (coverageQueryResolver) UnreadNotifications(ctx context.Context) (*UnreadCount, error) {
	return graphQLCoverageFixture[*UnreadCount](), nil
}

func (coverageQueryResolver) AdminUsers(ctx context.Context) ([]*model.User, error) {
	return graphQLCoverageFixture[[]*model.User](), nil
}

func (coverageQueryResolver) AdminServerAdmins(ctx context.Context) (*ServerAdminOverview, error) {
	return graphQLCoverageFixture[*ServerAdminOverview](), nil
}

func (coverageQueryResolver) AdminServerAdmin(ctx context.Context, id string) (*ServerAdmin, error) {
	return graphQLCoverageFixture[*ServerAdmin](), nil
}

func (coverageQueryResolver) AdminUserInvites(ctx context.Context) ([]*UserInvite, error) {
	return graphQLCoverageFixture[[]*UserInvite](), nil
}

func (coverageQueryResolver) AdminUserInvite(ctx context.Context, id string) (*UserInvite, error) {
	return graphQLCoverageFixture[*UserInvite](), nil
}

func (coverageQueryResolver) AdminSettings(ctx context.Context) ([]*model.Setting, error) {
	return graphQLCoverageFixture[[]*model.Setting](), nil
}

func (coverageQueryResolver) AdminSetting(ctx context.Context, key string) (*model.Setting, error) {
	return graphQLCoverageFixture[*model.Setting](), nil
}

func (coverageQueryResolver) AdminTokens(ctx context.Context) ([]*model.APIToken, error) {
	return graphQLCoverageFixture[[]*model.APIToken](), nil
}

func (coverageQueryResolver) AdminAuditLogs(ctx context.Context, limit *int, offset *int) ([]*AuditLog, error) {
	return graphQLCoverageFixture[[]*AuditLog](), nil
}

func (coverageQueryResolver) AdminStats(ctx context.Context) (*SystemStats, error) {
	return graphQLCoverageFixture[*SystemStats](), nil
}

func (coverageQueryResolver) AdminTasks(ctx context.Context) ([]*ScheduledTask, error) {
	return graphQLCoverageFixture[[]*ScheduledTask](), nil
}

func (coverageQueryResolver) AdminTaskHistory(ctx context.Context, name string, limit *int) ([]*TaskHistory, error) {
	return graphQLCoverageFixture[[]*TaskHistory](), nil
}

func (coverageQueryResolver) AdminChannels(ctx context.Context) ([]*NotificationChannel, error) {
	return graphQLCoverageFixture[[]*NotificationChannel](), nil
}

func (coverageQueryResolver) AdminChannel(ctx context.Context, typeArg string) (*NotificationChannel, error) {
	return graphQLCoverageFixture[*NotificationChannel](), nil
}

func (coverageQueryResolver) AdminChannelStats(ctx context.Context, typeArg string) (*ChannelStats, error) {
	return graphQLCoverageFixture[*ChannelStats](), nil
}

func (coverageQueryResolver) AdminQueueStats(ctx context.Context) (*QueueStats, error) {
	return graphQLCoverageFixture[*QueueStats](), nil
}

func (coverageQueryResolver) AdminSMTPProviders(ctx context.Context) ([]*SMTPProvider, error) {
	return graphQLCoverageFixture[[]*SMTPProvider](), nil
}

func (coverageQueryResolver) AdminPasskeys(ctx context.Context) ([]*AdminPasskey, error) {
	return graphQLCoverageFixture[[]*AdminPasskey](), nil
}

func (coverageMutationResolver) RegisterUser(ctx context.Context, username string, email string, password string) (*AuthResult, error) {
	return graphQLCoverageFixture[*AuthResult](), nil
}

func (coverageMutationResolver) LoginUser(ctx context.Context, identifier string, password string, twoFactorCode *string, recoveryKey *string) (*AuthResult, error) {
	return graphQLCoverageFixture[*AuthResult](), nil
}

func (coverageMutationResolver) CompleteUserTwoFactor(ctx context.Context, sessionToken string, twoFactorCode string) (*AuthResult, error) {
	return graphQLCoverageFixture[*AuthResult](), nil
}

func (coverageMutationResolver) UseUserRecoveryKey(ctx context.Context, sessionToken string, recoveryKey string) (*AuthResult, error) {
	return graphQLCoverageFixture[*AuthResult](), nil
}

func (coverageMutationResolver) LogoutUser(ctx context.Context) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) RefreshUserSession(ctx context.Context) (*AuthResult, error) {
	return graphQLCoverageFixture[*AuthResult](), nil
}

func (coverageMutationResolver) RequestPasswordReset(ctx context.Context, email string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) ResetUserPassword(ctx context.Context, token string, password string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) VerifyUserEmail(ctx context.Context, token string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) CompleteUserInvite(ctx context.Context, token string, username string, password string) (*UserInviteCompletion, error) {
	return graphQLCoverageFixture[*UserInviteCompletion](), nil
}

func (coverageMutationResolver) CompleteServerInvite(ctx context.Context, token string, username string, password string) (*ServerInviteCompletion, error) {
	return graphQLCoverageFixture[*ServerInviteCompletion](), nil
}

func (coverageMutationResolver) UpdateUserProfile(ctx context.Context, displayName *string, phone *string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) UploadUserAvatar(ctx context.Context, file graphql.Upload) (*PublicAvatar, error) {
	return graphQLCoverageFixture[*PublicAvatar](), nil
}

func (coverageMutationResolver) UpdateUserAvatar(ctx context.Context, typeArg string, url *string) (*PublicAvatar, error) {
	return graphQLCoverageFixture[*PublicAvatar](), nil
}

func (coverageMutationResolver) ResetUserAvatar(ctx context.Context) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) ChangeUserPassword(ctx context.Context, currentPassword string, newPassword string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) EnableUserTwoFactor(ctx context.Context, secret string, code string, password string) (*TOTPRecoveryKeys, error) {
	return graphQLCoverageFixture[*TOTPRecoveryKeys](), nil
}

func (coverageMutationResolver) DisableUserTwoFactor(ctx context.Context, password string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) VerifyUserTwoFactor(ctx context.Context, code string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) RegenerateUserRecoveryKeys(ctx context.Context, code string) (*TOTPRecoveryKeys, error) {
	return graphQLCoverageFixture[*TOTPRecoveryKeys](), nil
}

func (coverageMutationResolver) BeginUserPasskeyRegistration(ctx context.Context, name string, password string) (*PasskeyRegistrationOptions, error) {
	return graphQLCoverageFixture[*PasskeyRegistrationOptions](), nil
}

func (coverageMutationResolver) FinishUserPasskeyRegistration(ctx context.Context, ceremonyToken string, credential interface{}) (*PasskeyRegistrationResult, error) {
	return graphQLCoverageFixture[*PasskeyRegistrationResult](), nil
}

func (coverageMutationResolver) DeleteUserPasskey(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) BeginUserPasskeyChallenge(ctx context.Context, sessionToken *string) (*PasskeyChallengeOptions, error) {
	return graphQLCoverageFixture[*PasskeyChallengeOptions](), nil
}

func (coverageMutationResolver) FinishUserPasskeyChallenge(ctx context.Context, ceremonyToken string, credential interface{}) (*AuthResult, error) {
	return graphQLCoverageFixture[*AuthResult](), nil
}

func (coverageMutationResolver) UpdateUserSettings(ctx context.Context, account *AccountSettingsInput, privacy *PrivacySettingsInput, notifications *NotificationSettingsInput, appearance *AppearanceSettingsInput) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) CreateUserToken(ctx context.Context, name string, scopes *string, expiresIn *int) (*UserToken, error) {
	return graphQLCoverageFixture[*UserToken](), nil
}

func (coverageMutationResolver) RevokeUserToken(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) CreateSavedLocation(ctx context.Context, name string, lat float64, lon float64, country *string, region *string, alerts *bool) (*model.SavedLocation, error) {
	return graphQLCoverageFixture[*model.SavedLocation](), nil
}

func (coverageMutationResolver) UpdateSavedLocation(ctx context.Context, id string, name *string, alerts *bool) (*model.SavedLocation, error) {
	return graphQLCoverageFixture[*model.SavedLocation](), nil
}

func (coverageMutationResolver) DeleteSavedLocation(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) ToggleLocationAlerts(ctx context.Context, id string) (*model.SavedLocation, error) {
	return graphQLCoverageFixture[*model.SavedLocation](), nil
}

func (coverageMutationResolver) MarkNotificationRead(ctx context.Context, id string) (*model.Notification, error) {
	return graphQLCoverageFixture[*model.Notification](), nil
}

func (coverageMutationResolver) MarkAllNotificationsRead(ctx context.Context) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) DeleteNotification(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminUpdateUser(ctx context.Context, id string, username *string, email *string, role *string) (*model.User, error) {
	return graphQLCoverageFixture[*model.User](), nil
}

func (coverageMutationResolver) AdminDeleteUser(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminCreateUserInvite(ctx context.Context, username string, email string, role *string, expiresInDays *int) (*UserInvite, error) {
	return graphQLCoverageFixture[*UserInvite](), nil
}

func (coverageMutationResolver) AdminDeleteUserInvite(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminInviteServerAdmin(ctx context.Context, email string, expiresIn *string) (*ServerAdminInvite, error) {
	return graphQLCoverageFixture[*ServerAdminInvite](), nil
}

func (coverageMutationResolver) AdminDeleteServerAdmin(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminDisableServerAdmin(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminEnableServerAdmin(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminUpdateSetting(ctx context.Context, key string, value string) (*model.Setting, error) {
	return graphQLCoverageFixture[*model.Setting](), nil
}

func (coverageMutationResolver) AdminUpdateSettings(ctx context.Context, settings []*SettingInput) (*BulkResponse, error) {
	return graphQLCoverageFixture[*BulkResponse](), nil
}

func (coverageMutationResolver) AdminResetSettings(ctx context.Context) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminGenerateToken(ctx context.Context) (*model.APIToken, error) {
	return graphQLCoverageFixture[*model.APIToken](), nil
}

func (coverageMutationResolver) AdminRevokeToken(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminClearAuditLogs(ctx context.Context) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminUpdateTask(ctx context.Context, name string, enabled bool) (*ScheduledTask, error) {
	return graphQLCoverageFixture[*ScheduledTask](), nil
}

func (coverageMutationResolver) AdminEnableTask(ctx context.Context, name string) (*ScheduledTask, error) {
	return graphQLCoverageFixture[*ScheduledTask](), nil
}

func (coverageMutationResolver) AdminDisableTask(ctx context.Context, name string) (*ScheduledTask, error) {
	return graphQLCoverageFixture[*ScheduledTask](), nil
}

func (coverageMutationResolver) AdminTriggerTask(ctx context.Context, name string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminUpdateChannel(ctx context.Context, typeArg string, enabled *bool, config interface{}) (*NotificationChannel, error) {
	return graphQLCoverageFixture[*NotificationChannel](), nil
}

func (coverageMutationResolver) AdminEnableChannel(ctx context.Context, typeArg string) (*NotificationChannel, error) {
	return graphQLCoverageFixture[*NotificationChannel](), nil
}

func (coverageMutationResolver) AdminDisableChannel(ctx context.Context, typeArg string) (*NotificationChannel, error) {
	return graphQLCoverageFixture[*NotificationChannel](), nil
}

func (coverageMutationResolver) AdminTestChannel(ctx context.Context, typeArg string, recipient *string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminInitializeChannels(ctx context.Context) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) AdminAutoDetectSMTP(ctx context.Context) (*SMTPProvider, error) {
	return graphQLCoverageFixture[*SMTPProvider](), nil
}

func (coverageMutationResolver) BeginAdminPasskeyRegistration(ctx context.Context, name string, password string) (*PasskeyRegistrationOptions, error) {
	return graphQLCoverageFixture[*PasskeyRegistrationOptions](), nil
}

func (coverageMutationResolver) FinishAdminPasskeyRegistration(ctx context.Context, ceremonyToken string, credential interface{}) (*AdminPasskeyRegistrationResult, error) {
	return graphQLCoverageFixture[*AdminPasskeyRegistrationResult](), nil
}

func (coverageMutationResolver) DeleteAdminPasskey(ctx context.Context, id string) (*GenericResponse, error) {
	return graphQLCoverageFixture[*GenericResponse](), nil
}

func (coverageMutationResolver) BeginAdminPasskeyChallenge(ctx context.Context, sessionToken string) (*PasskeyChallengeOptions, error) {
	return graphQLCoverageFixture[*PasskeyChallengeOptions](), nil
}

func (coverageMutationResolver) FinishAdminPasskeyChallenge(ctx context.Context, ceremonyToken string, credential interface{}) (*AdminLoginResult, error) {
	return graphQLCoverageFixture[*AdminLoginResult](), nil
}

func (coverageMutationResolver) SubmitContactForm(ctx context.Context, name string, email string, subject string, message string) (*ContactSubmission, error) {
	return graphQLCoverageFixture[*ContactSubmission](), nil
}
