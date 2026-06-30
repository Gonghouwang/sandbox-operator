package controller

import (
	"encoding/json"
	"reflect"
)

func cloneForCompare[T any](in T) T {
	var out T
	data, err := json.Marshal(in)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func hasChanged[T any](before, after T) bool {
	return !reflect.DeepEqual(before, after)
}
