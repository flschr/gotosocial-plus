// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"net/url"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/stretchr/testify/require"
)

func TestPreviewCardSkippedForBlueskyInteraction(t *testing.T) {
	config.SetStatusesPreviewCards(true)
	converter := NewConverter(&state.State{})
	card := converter.previewCardForStatus(t.Context(), &gtsmodel.Status{
		Content:              `<a href="https://bsky.app/profile/example/post/1">View reply on Bluesky</a>`,
		BlueskyInteractionID: "01K00000000000000000000000",
	}, false)
	require.Nil(t, card)
}

func TestFirstPreviewCardURLSkipsMentionsAndTags(t *testing.T) {
	content := `<p><a class="mention" href="https://social.example/@alice">@alice</a> ` +
		`<a rel="tag" href="https://social.example/tags/test">#test</a> ` +
		`<a href="https://example.org/article">article</a></p>`

	found := firstPreviewCardURL(content, nil)
	require.NotNil(t, found)
	require.Equal(t, "https://example.org/article", found.String())
}

func TestFirstPreviewCardURLSkipsStructuredMentionProfile(t *testing.T) {
	status := &gtsmodel.Status{
		Content: `<p><a href="https://micro.blog/activitypub/iChris">@iChris</a> ` +
			`<a href="https://micro.blog/manton/94592960">post</a></p>`,
		Mentions: []*gtsmodel.Mention{{
			TargetAccountURI: "https://micro.blog/activitypub/iChris/",
			TargetAccountURL: "https://micro.blog/iChris",
		}},
	}

	found := firstPreviewCardURL(status.Content, previewCardMentionURLs(status))
	require.NotNil(t, found)
	require.Equal(t, "https://micro.blog/manton/94592960", found.String())
}

func TestFirstPreviewCardURLReturnsNilForMentionProfileOnly(t *testing.T) {
	status := &gtsmodel.Status{
		Content: `<p><a href="https://micro.blog/activitypub/iChris">@iChris</a></p>`,
		Mentions: []*gtsmodel.Mention{{
			TargetAccountURI: "https://micro.blog/activitypub/iChris",
		}},
	}

	require.Nil(t, firstPreviewCardURL(status.Content, previewCardMentionURLs(status)))
}

func TestParsePreviewCard(t *testing.T) {
	target, err := url.Parse("https://example.org/articles/one")
	require.NoError(t, err)

	body := []byte(`<!doctype html><html><head>
		<title>Fallback title</title>
		<meta property="og:title" content="A useful article">
		<meta property="og:description" content="Description here">
		<meta property="og:site_name" content="Example">
		<meta property="og:image" content="/preview.jpg">
		<meta property="og:image:width" content="1200">
		<meta property="og:image:height" content="630">
	</head></html>`)

	card, err := parsePreviewCard(target, body, "")
	require.NoError(t, err)
	require.Equal(t, "https://example.org/articles/one", card.URL)
	require.Equal(t, "A useful article", card.Title)
	require.Equal(t, "Description here", card.Description)
	require.Equal(t, "Example", card.ProviderName)
	require.Equal(t, "https://example.org/preview.jpg", card.Image)
	require.Equal(t, 1200, card.Width)
	require.Equal(t, 630, card.Height)
}

func TestParsePreviewCardLatin1(t *testing.T) {
	target, err := url.Parse("https://example.org/articles/one")
	require.NoError(t, err)

	// "Fake-Date-Überfall" / "19-Jähriger über" encoded as ISO-8859-1.
	title := "Fake-Date-\xdcberfall"
	description := "19-J\xe4hriger \xfcber"

	for name, tc := range map[string]struct {
		contentType string
		head        string
	}{
		"header":                {contentType: "text/html; charset=iso-8859-1"},
		"meta":                  {contentType: "text/html", head: `<meta charset="iso-8859-1">`},
		"http-equiv":            {head: `<meta http-equiv="Content-Type" content="text/html; charset=iso-8859-1">`},
		"header overrides meta": {contentType: "text/html; charset=iso-8859-1", head: `<meta charset="utf-8">`},
	} {
		t.Run(name, func(t *testing.T) {
			body := []byte(`<!doctype html><html><head>` + tc.head +
				`<meta property="og:title" content="` + title + `">` +
				`<meta property="og:description" content="` + description + `">` +
				`</head></html>`)

			card, err := parsePreviewCard(target, body, tc.contentType)
			require.NoError(t, err)
			require.Equal(t, "Fake-Date-Überfall", card.Title)
			require.Equal(t, "19-Jähriger über", card.Description)
		})
	}
}

func TestParsePreviewCardYouTubePlayer(t *testing.T) {
	target, err := url.Parse("https://youtu.be/Gg-SCpXba64")
	require.NoError(t, err)

	body := []byte(`<!doctype html><html><head>
		<meta property="og:title" content="A useful video">
		<meta property="og:image" content="https://i.ytimg.com/vi/Gg-SCpXba64/maxresdefault.jpg">
	</head></html>`)

	card, err := parsePreviewCard(target, body, "")
	require.NoError(t, err)
	require.Equal(t, "video", card.Type)
	require.Equal(t, 480, card.Width)
	require.Equal(t, 270, card.Height)
	require.Contains(t, card.HTML, `src="https://www.youtube-nocookie.com/embed/Gg-SCpXba64"`)
}

func TestYouTubeEmbedURLRejectsUntrustedAndInvalidURLs(t *testing.T) {
	untrusted, err := url.Parse("https://example.org/watch?v=Gg-SCpXba64")
	require.NoError(t, err)
	require.Empty(t, youtubeEmbedURL(untrusted))

	invalid, err := url.Parse("https://youtu.be/not-valid")
	require.NoError(t, err)
	require.Empty(t, youtubeEmbedURL(invalid))
}

func TestYouTubePreviewCardFromOEmbed(t *testing.T) {
	target, err := url.Parse("https://youtu.be/Gg-SCpXba64")
	require.NoError(t, err)

	card, err := youtubePreviewCardFromOEmbed(target, &youtubeOEmbed{
		Title:           "Danger Dan - Keine Angst",
		AuthorName:      "Danger Dan",
		AuthorURL:       "https://www.youtube.com/@DangerDan",
		ThumbnailURL:    "https://i.ytimg.com/vi/Gg-SCpXba64/hqdefault.jpg",
		ThumbnailWidth:  480,
		ThumbnailHeight: 360,
	})
	require.NoError(t, err)
	require.Equal(t, "video", card.Type)
	require.Equal(t, "Danger Dan - Keine Angst", card.Title)
	require.Equal(t, "https://www.youtube-nocookie.com/embed/Gg-SCpXba64", card.EmbedURL)
	require.Equal(t, "https://i.ytimg.com/vi/Gg-SCpXba64/hqdefault.jpg", card.Image)
}

func TestParsePreviewCardUTF8(t *testing.T) {
	target, err := url.Parse("https://example.org/article")
	require.NoError(t, err)
	for name, tc := range map[string]struct{ prefix, contentType string }{
		"header":               {contentType: "text/html; charset=utf-8"},
		"sniffed":              {},
		"BOM overrides header": {prefix: "\xef\xbb\xbf", contentType: "text/html; charset=iso-8859-1"},
	} {
		t.Run(name, func(t *testing.T) {
			card, err := parsePreviewCard(target, []byte(tc.prefix+`<html><head><title>Grüße 日本語 🦋</title><meta name="description" content="Äpfel &amp; Öl"></head></html>`), tc.contentType)
			require.NoError(t, err)
			require.Equal(t, "Grüße 日本語 🦋", card.Title)
			require.Equal(t, "Äpfel & Öl", card.Description)
		})
	}
}
