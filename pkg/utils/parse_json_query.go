package utils

import (
	"fmt"
	"strings"
	"reflect"
)

func ParseJSONQuery(j map[string]interface{}, jq string, o interface{}) error {
	for _, k := range strings.Split(jq, ".") {
		// optional field
		opt := false
		sz := len(k)
		if sz > 0 && k[sz-1] == '?' {
			opt = true
			k = k[:sz-1]
		}

		if uv, ok := j[k]; ok {
			if v, ok := uv.(map[string]interface{}); ok {
				j = v
				continue
			}
			// ValueOf to enter reflect-land
			dstPtrValue := reflect.ValueOf(o)
			dstValue := reflect.Indirect(dstPtrValue)
			dstValue.Set(reflect.ValueOf(uv))

			return nil
		}
		if opt {
			return nil
		}
		// otherwise key was not found
		// keys are case sensitive, because maps are
		for ki := range j {
			if strings.EqualFold(k, ki) {
				return fmt.Errorf("key %s wasn't found, but %s was ", k, ki)
			}
		}
		return fmt.Errorf("key %s wasn't found in %v ", k, j)
	}
	return fmt.Errorf("query didn't match the structure")
}