package server_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestWeakETag(t *testing.T) {
	t.Run("round trips the microsecond precision of PostgreSQL", func(t *testing.T) {
		modified := time.Date(2026, 7, 22, 8, 12, 3, 123456000, time.UTC)

		parsed, ok := server.ParseWeakETag(server.WeakETag(modified))

		require.True(t, ok)
		require.True(t, modified.Equal(parsed))
	})

	t.Run("is a weak entity tag", func(t *testing.T) {
		require.Regexp(t, `^W/"\d+"$`, server.WeakETag(time.Now()))
	})

	t.Run("rejects what it did not produce", func(t *testing.T) {
		for _, version := range []string{"", `"123"`, `W/"abc"`, `W/"-1"`, `W/"123`, "123"} {
			_, ok := server.ParseWeakETag(version)

			require.False(t, ok, version)
		}
	})
}
