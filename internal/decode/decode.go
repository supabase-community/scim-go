package decode

import (
	"encoding/json"
	"io"
)

func JSON[T any](r io.Reader) (T, error) {
	var out T
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	err := decoder.Decode(&out)
	return out, err
}
