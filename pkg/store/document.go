package store

import "encoding/json"

type Document struct {
	ID         string
	Body       json.RawMessage
	Links      []Link
	References []Reference
}
