package scim

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func validateUser(user *core.User) *protocol.Error {
	if user.UserName == "" {
		return protocol.ErrInvalidValue(`"userName" is required`)
	}
	if !slices.Contains(user.Schemas, core.SchemaUser) {
		return protocol.ErrInvalidValue(`"schemas" must include the User schema URN`)
	}
	return nil
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, protocol.ErrTooLarge("the request body is too large")
		}
		return nil, protocol.ErrInvalidSyntax("could not read the request body")
	}
	return body, nil
}

func decodeUser(r *http.Request) (*core.User, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}

	user := new(core.User)
	if err := json.Unmarshal(body, user); err != nil {
		return nil, protocol.ErrInvalidSyntax("request body is not a valid User")
	}
	return user, nil
}

type UserRepository struct {
	mu      sync.RWMutex
	baseURL string
	items   map[string]*core.User
}

func NewMemoryUserRepository(baseURL string) Repository[*core.User] {
	return &UserRepository{baseURL: baseURL, items: make(map[string]*core.User)}
}

func (r *UserRepository) Get(ctx context.Context, id string) (*core.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *UserRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*core.User, 0, len(r.items))
	for _, item := range r.items {
		all = append(all, item)
	}

	if query.Filter != "" {
		matched, err := filterResources(all, query.Filter)
		if err != nil {
			return nil, 0, err
		}
		all = matched
	}

	slices.SortFunc(all, func(a, b *core.User) int { return strings.Compare(a.ID, b.ID) })

	total := len(all)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	return all[start:end], total, nil
}

func (r *UserRepository) Create(ctx context.Context, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user.ID = uuid.NewV7().String()
	r.items[user.ID] = user
	return user, nil
}

func (r *UserRepository) Replace(ctx context.Context, id string, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return nil, ErrNotFound
	}
	user.ID = id
	r.items[id] = user
	return user, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func NewUserSchema(baseURL string) *core.Schema {
	schema := &core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      core.SchemaUser,
		Name:    "User",
		Meta: core.Meta{
			ResourceType: "User",
			Location:     Join(baseURL, "/Users"),
		},
	}

	return schema.
		Describe("User Account").
		With(
			core.NewAttribute("userName", core.TypeString, "Unique identifier for the User").
				AsRequired().
				UniqueOn(core.UniquenessServer),
			core.NewAttribute("name", core.TypeComplex, "The components of the user's name.").
				With(
					core.NewAttribute("formatted", core.TypeString, "The name formatted for display."),
					core.NewAttribute("familyName", core.TypeString, "The family name of the User."),
					core.NewAttribute("givenName", core.TypeString, "The given name of the User."),
					core.NewAttribute("middleName", core.TypeString, "The middle name(s) of the User."),
				),
			core.NewAttribute("emails", core.TypeComplex, "Email addresses for the user.").
				AsMultiValued().
				With(
					core.NewAttribute("value", core.TypeString, "An email address for the user."),
					core.NewAttribute("primary", core.TypeBoolean, "The 'primary' email address"),
				),
			core.NewAttribute("active", core.TypeBoolean, ""),
		)
}
