package store

import (
	"encoding/json"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Record struct {
	ID      string
	Body    json.RawMessage
	Created time.Time
	Updated time.Time
	Version string
}

func (r Record) Into[T core.Resource]() (T, error) {
	var item T
	if err := json.Unmarshal(r.Body, &item); err != nil {
		return item, err
	}
	common := item.Common()
	common.ID = r.ID
	common.Meta = r.meta()
	return item, nil
}

func (r Record) meta() core.Meta {
	return core.Meta{Created: r.Created.UTC(), LastModified: r.Updated.UTC(), Version: r.Version}
}

func Decode[T core.Resource](records []Record) ([]T, error) {
	items := make([]T, len(records))
	for i, record := range records {
		item, err := record.Into[T]()
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}
