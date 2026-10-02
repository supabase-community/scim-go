package decode

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

var (
	errTrailingData = errors.New("decode: data after the JSON value")
	errNotObject    = errors.New("decode: JSON value is not an object")
)

func JSON[T any](raw []byte) (T, error) {
	var out T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return out, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return out, errors.Join(errTrailingData, err)
	}
	return out, nil
}

func Value(raw []byte) (any, error) {
	return JSON[any](raw)
}

func Object(raw []byte) (map[string]any, error) {
	out, err := Value(raw)
	if err != nil {
		return nil, err
	}
	object, ok := out.(map[string]any)
	if !ok {
		return nil, errNotObject
	}
	return object, nil
}
