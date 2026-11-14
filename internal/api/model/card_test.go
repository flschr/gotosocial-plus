// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCardMarshalEmptyImageAsNull(t *testing.T) {
	b, err := json.Marshal(&Card{URL: "https://www.pastpuzzle.de", Title: "Past Puzzle"})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Contains(t, got, "image")
	require.Nil(t, got["image"])
	require.Contains(t, got, "blurhash")
	require.Nil(t, got["blurhash"])
	require.Equal(t, "Past Puzzle", got["title"])
}

func TestCardMarshalImageSet(t *testing.T) {
	b, err := json.Marshal(Card{Image: "https://example.org/preview.jpg", Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Type: "link"})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "https://example.org/preview.jpg", got["image"])
	require.Equal(t, "LEHV6nWB2yk8pyo0adR*.7kCMdnj", got["blurhash"])
	require.Equal(t, "link", got["type"])
}
