package config

import "gopkg.in/yaml.v3"

// SaveGeoIPArea writes GeoIP controls to their owning security.yml area file.
func SaveGeoIPArea(geoip GeoIPConfig) error {
	return updateSecurityArea(func(server map[string]interface{}) error {
		data, err := yaml.Marshal(geoip)
		if err != nil {
			return err
		}
		var values map[string]interface{}
		if err := yaml.Unmarshal(data, &values); err != nil {
			return err
		}
		server["geoip"] = values
		return nil
	})
}
