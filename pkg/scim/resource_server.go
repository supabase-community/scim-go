package scim

import (
	"errors"
	"net/http"
	"net/url"

	"mokhan.ca/go/scim/pkg/core"
	"mokhan.ca/go/scim/pkg/protocol"
)

type resourceServer[T core.Resource] struct {
	limits   protocol.Limits
	repo     Repository[T]
	validate func(T) *protocol.Error
	decode   func(*http.Request) (T, error)
}

func (s *resourceServer[T]) list(w http.ResponseWriter, r *http.Request) error {
	query, err := s.limits.ParseSearchRequest(r.URL.Query())
	if err != nil {
		return protocol.SendError(w, err)
	}

	items, total, err := s.repo.List(r.Context(), query)
	if err != nil {
		return protocol.SendError(w, err)
	}

	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(query.StartIndex, total, items))
}

func (s *resourceServer[T]) byID(w http.ResponseWriter, r *http.Request) error {
	item, err := s.repo.Get(r.Context(), urlParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFound(w)
		}
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, item)
}

func (s *resourceServer[T]) create(w http.ResponseWriter, r *http.Request) error {
	item, err := s.decode(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := s.validate(item); err != nil {
		return protocol.SendError(w, err)
	}

	created, err := s.repo.Create(r.Context(), item)
	if err != nil {
		return protocol.SendError(w, err)
	}

	w.Header().Set("Location", locationOf(created))
	return protocol.Send(w, http.StatusCreated, created)
}

func (s *resourceServer[T]) replace(w http.ResponseWriter, r *http.Request) error {
	id := urlParam(r, "id")

	item, err := s.decode(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := s.validate(item); err != nil {
		return protocol.SendError(w, err)
	}

	replaced, err := s.repo.Replace(r.Context(), id, item)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFound(w)
		}
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, replaced)
}

func (s *resourceServer[T]) delete(w http.ResponseWriter, r *http.Request) error {
	if err := s.repo.Delete(r.Context(), urlParam(r, "id")); err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFound(w)
		}
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}

type located interface{ Location() string }

func locationOf(v any) string {
	if l, ok := v.(located); ok {
		return l.Location()
	}
	return ""
}

func notFound(w http.ResponseWriter) error {
	return protocol.SendError(w, protocol.ErrNotFound("Endpoint or resource does not exist"))
}

func rejectFilter(w http.ResponseWriter, r *http.Request) (bool, error) {
	if !r.URL.Query().Has("filter") {
		return false, nil
	}
	return true, protocol.SendError(w, protocol.ErrForbidden("Filtering is not supported on this endpoint"))
}

func urlParam(r *http.Request, key string) string {
	if decoded, err := url.PathUnescape(r.PathValue(key)); err == nil {
		return decoded
	}
	return r.PathValue(key)
}
