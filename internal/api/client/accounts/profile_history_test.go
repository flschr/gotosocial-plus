// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package accounts_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/api/client/accounts"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/oauth"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type RemoteProfileAPITestSuite struct {
	AccountStandardTestSuite
	calls     atomic.Int32
	statusURI string
}

func (s *RemoteProfileAPITestSuite) SetupTest() {
	s.AccountStandardTestSuite.SetupTest()
	s.calls.Store(0)
	a := s.testAccounts["remote_account_1"]
	s.statusURI = a.URI + "/statuses/profile-history"
	fallback := testrig.NewMockHTTPClient(nil, "../../../../testrig/media")
	client := testrig.NewMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var document any
		switch req.URL.String() {
		case a.OutboxURI:
			s.calls.Add(1)
			document = map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollection", "orderedItems": []string{s.statusURI}}
		case s.statusURI:
			document = map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "Note", "id": s.statusURI, "attributedTo": a.URI, "to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "content": "Imported remote profile post", "published": "2026-10-01T10:00:00Z"}
		default:
			return fallback.Do(req)
		}
		body, err := json.Marshal(document)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/activity+json"}}, Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: req}, nil
	}, "")
	s.federator = testrig.NewTestFederator(&s.state, testrig.NewTestTransportController(&s.state, client), s.mediaManager)
	s.processor = testrig.NewTestProcessor(&s.state, s.federator, s.emailSender, testrig.NewNoopWebPushSender(), s.mediaManager)
	s.accountsModule = accounts.New(s.processor)
}
func (s *RemoteProfileAPITestSuite) profile(query string, authenticated bool) *httptest.ResponseRecorder {
	a := s.testAccounts["remote_account_1"]
	r := httptest.NewRecorder()
	c := s.newContext(r, http.MethodGet, nil, "/api/v1/accounts/"+a.ID+"/statuses"+query, "")
	c.Request.Method = http.MethodGet
	c.Params = gin.Params{{Key: accounts.IDKey, Value: a.ID}}
	if !authenticated {
		c.Set(oauth.SessionAuthorizedAccount, nil)
		c.Set(oauth.SessionAuthorizedToken, nil)
		c.Set(oauth.SessionAuthorizedUser, nil)
		c.Set(oauth.SessionAuthorizedApplication, nil)
	}
	s.accountsModule.AccountStatusesGETHandler(c)
	return r
}
func (s *RemoteProfileAPITestSuite) TestFirstResponseContainsImportedHistory() {
	r := s.profile("?limit=40", true)
	s.Require().Equal(http.StatusOK, r.Code, r.Body.String())
	var statuses []*apimodel.Status
	s.Require().NoError(json.Unmarshal(r.Body.Bytes(), &statuses))
	found := false
	for _, status := range statuses {
		if status.URI == s.statusURI {
			found = true
		}
	}
	s.True(found, "the first API response must contain the imported post")
	s.Equal(int32(1), s.calls.Load())
	s.Equal(http.StatusOK, s.profile("?limit=40", true).Code)
	s.Equal(int32(1), s.calls.Load())
}
func (s *RemoteProfileAPITestSuite) TestPaginationDoesNotFetch() {
	s.Equal(http.StatusOK, s.profile("?max_id=01ZZZZZZZZZZZZZZZZZZZZZZZZ", true).Code)
	s.Equal(http.StatusOK, s.profile("?min_id=01F8MH75CBF9JFX4ZAD54N0W0R", true).Code)
	s.Zero(s.calls.Load())
}
func (s *RemoteProfileAPITestSuite) TestPinnedDoesNotFetch() {
	s.Equal(http.StatusOK, s.profile("?pinned=true", true).Code)
	s.Zero(s.calls.Load())
}
func (s *RemoteProfileAPITestSuite) TestUnauthenticatedDoesNotFetch() {
	s.Equal(http.StatusUnauthorized, s.profile("", false).Code)
	s.Zero(s.calls.Load())
}
func (s *RemoteProfileAPITestSuite) TestBlockedAccountDoesNotFetch() {
	a := s.testAccounts["local_account_1"]
	target := s.testAccounts["remote_account_1"]
	s.Require().NoError(s.db.PutBlock(s.T().Context(), &gtsmodel.Block{ID: id.NewULID(), URI: a.URI + "/blocks/history", AccountID: a.ID, TargetAccountID: target.ID}))
	r := s.profile("", true)
	s.Equal(http.StatusOK, r.Code)
	s.Equal("[]", r.Body.String())
	s.Zero(s.calls.Load())
}
func (s *RemoteProfileAPITestSuite) TestRemoteFailureReturnsExistingHistory() {
	a := s.testAccounts["remote_account_1"]
	// An inaccessible outbox must not turn an otherwise valid profile into an error.
	a.OutboxURI = a.URI + "/missing-outbox"
	s.Require().NoError(s.db.UpdateAccount(s.T().Context(), a, "outbox_uri"))
	r := s.profile("", true)
	s.Require().Equal(http.StatusOK, r.Code, r.Body.String())
	var statuses []*apimodel.Status
	s.Require().NoError(json.Unmarshal(r.Body.Bytes(), &statuses))
	s.NotEmpty(statuses)
}
func TestRemoteProfileAPI(t *testing.T) { suite.Run(t, new(RemoteProfileAPITestSuite)) }
