package enforcement

import "encoding/json"

func MapToJson(m map[string]interface{}) string {
	json, _ := json.Marshal(m)
	return string(json)
}

func mapToStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v.(string)
	}
	return result
}
