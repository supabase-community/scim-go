package scim

import (
	"context"
	"errors"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

var ErrNotFound = errors.New("scim: resource not found")

type Repository[T core.Resource] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, id string, item T) (T, error)
	Delete(ctx context.Context, id string) error
}

type resourceServer[T core.Resource] struct {
	limits   protocol.Limits
	repo     Repository[T]
	validate func(T) *protocol.Error
	decode   func(*http.Request) (T, error)
}

func newResourceServer[T core.Resource](
	limits protocol.Limits,
	repo Repository[T],
	validate func(T) *protocol.Error,
	decode func(*http.Request) (T, error),
) *resourceServer[T] {
	return &resourceServer[T]{
		limits:   limits,
		repo:     repo,
		validate: validate,
		decode:   decode,
	}
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
