package graphql

import (
	"context"
	"reflect"
	"time"

	model "github.com/webappsgo/wthr/src/server/model"
)

type coverageResolver struct {
	*Resolver
	query QueryResolver
}

func (r *coverageResolver) Query() QueryResolver { return r.query }

type coverageQueryResolver struct{}

func graphQLCoverageFixture[T any]() T {
	var value T
	fixtureGraphQLValue(reflect.ValueOf(&value).Elem(), 0)
	return value
}

func fixtureGraphQLValue(value reflect.Value, depth int) {
	if !value.IsValid() || !value.CanSet() || depth > 8 {
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
