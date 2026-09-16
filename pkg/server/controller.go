package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Controller handles the HTTP requests for one SCIM resource type, per RFC 7644, Section 3.
type Controller[T core.Resource] interface {
	List(http.ResponseWriter, *http.Request) error
	ByID(http.ResponseWriter, *http.Request) error
	Create(http.ResponseWriter, *http.Request) error
	Replace(http.ResponseWriter, *http.Request) error
	Patch(http.ResponseWriter, *http.Request) error
	Delete(http.ResponseWriter, *http.Request) error
}
