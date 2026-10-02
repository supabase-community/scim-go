package decode

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"io"
)

var (
	errTrailingData = errors.New("decode: data after the JSON value")
	ErrNotObject    = errors.New("decode: JSON value is not an object")
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
	decoder := jsontext.NewDecoder(
		bytes.NewBuffer(raw),
		jsontext.AllowDuplicateNames(true),
		jsontext.AllowInvalidUTF8(true),
	)
	out, err := read(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.ReadToken(); !errors.Is(err, io.EOF) {
		return nil, errors.Join(errTrailingData, err)
	}
	return out, nil
}

func Object(raw []byte) (map[string]any, error) {
	out, err := Value(raw)
	if err != nil {
		return nil, err
	}
	object, ok := out.(map[string]any)
	if !ok {
		return nil, ErrNotObject
	}
	return object, nil
}

func read(decoder *jsontext.Decoder) (any, error) {
	token, err := decoder.ReadToken()
	if err != nil {
		return nil, err
	}
	switch token.Kind() {
	case '{':
		return readObject(decoder)
	case '[':
		return readArray(decoder)
	case '"':
		return token.String(), nil
	case '0':
		return json.Number(token.String()), nil
	case 't', 'f':
		return token.Bool(), nil
	}
	return nil, nil
}

func readObject(decoder *jsontext.Decoder) (map[string]any, error) {
	out := map[string]any{}
	for decoder.PeekKind() != '}' {
		token, err := decoder.ReadToken()
		if err != nil {
			return nil, err
		}
		name := token.String()
		item, err := read(decoder)
		if err != nil {
			return nil, err
		}
		out[name] = item
	}
	_, err := decoder.ReadToken()
	return out, err
}

func readArray(decoder *jsontext.Decoder) ([]any, error) {
	out := []any{}
	for decoder.PeekKind() != ']' {
		item, err := read(decoder)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	_, err := decoder.ReadToken()
	return out, err
}
