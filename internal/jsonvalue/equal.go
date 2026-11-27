// Package jsonvalue holds helpers for attributes whose values are JSON strings.
package jsonvalue

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Equal reports whether two JSON documents have the same value, ignoring formatting such as whitespace and
// the order of object keys. Numbers are compared exactly, so large integers aren't rounded before they're compared.
func Equal(left, right string) (bool, error) {
	leftValue, err := decode(left)
	if err != nil {
		return false, err
	}
	rightValue, err := decode(right)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(leftValue, rightValue), nil
}

func decode(value string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
