package decode

import (
	"encoding/json"
	"errors"
	"io"
)

var errTrailingData = errors.New("decode: data after the JSON value")

func JSON[T any](r io.Reader) (T, error) {
	var out T
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return out, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return out, errors.Join(errTrailingData, err)
	}
	return out, nil
}
