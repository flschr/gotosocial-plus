// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryAfterDuration(t *testing.T) {
	now := time.Date(2026, time.November, 14, 19, 47, 16, 0, time.UTC)

	after, ok := retryAfterDuration("6034", now)
	require.True(t, ok)
	require.Equal(t, 6034*time.Second, after)

	after, ok = retryAfterDuration(now.Add(90*time.Minute).Format(http.TimeFormat), now)
	require.True(t, ok)
	require.Equal(t, 90*time.Minute, after)

	for _, value := range []string{"", "0", "later", now.Add(-time.Minute).Format(http.TimeFormat)} {
		_, ok := retryAfterDuration(value, now)
		require.False(t, ok, value)
	}
}

func TestRetryAfterError(t *testing.T) {
	original := errors.New("http response: 429 Too Many Requests")
	err := fmt.Errorf("preview failed: %w", &retryAfterError{err: original, after: 100 * time.Minute})
	after, ok := RetryAfter(err)
	require.True(t, ok)
	require.Equal(t, 100*time.Minute, after)
	require.ErrorIs(t, err, original)
}
