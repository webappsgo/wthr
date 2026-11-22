package handler

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var errInvalidAdminForm = errors.New("invalid admin settings form")

func readAdminConfig(path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{}
	}
	var config map[string]interface{}
	if yaml.Unmarshal(data, &config) != nil || config == nil {
		return map[string]interface{}{}
	}
	return config
}

func adminConfigValue(config map[string]interface{}, path string) interface{} {
	var current interface{} = config
	for _, part := range strings.Split(path, ".") {
		values, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current = values[part]
	}
	return current
}

func adminConfigBool(config map[string]interface{}, path string) bool {
	value, _ := adminConfigValue(config, path).(bool)
	return value
}

func adminConfigString(config map[string]interface{}, path string) string {
	value, _ := adminConfigValue(config, path).(string)
	return value
}
