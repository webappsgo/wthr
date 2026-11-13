package server

import "fmt"

// TemplateDict builds a map from alternating key/value arguments so a partial
// can receive its data together with the request language.
func TemplateDict(pairs ...interface{}) (map[string]interface{}, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict needs an even number of arguments, got %d", len(pairs))
	}
	out := make(map[string]interface{}, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key %d is not a string", i/2)
		}
		out[key] = pairs[i+1]
	}

	return out, nil
}
