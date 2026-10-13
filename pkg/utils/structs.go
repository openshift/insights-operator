package utils

import "encoding/json"

func StructToMap(obj interface{}) (map[string]interface{}, error) {
	objBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(objBytes, &result)

	return result, err
}
