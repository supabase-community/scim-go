package server

import "net/http"

type muxStatus struct {
	header http.Header
	code   int
}

func (m *muxStatus) Header() http.Header { return m.header }

func (m *muxStatus) Write(b []byte) (int, error) { return len(b), nil }

func (m *muxStatus) WriteHeader(code int) { m.code = code }
