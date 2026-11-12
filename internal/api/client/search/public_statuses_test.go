// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package search_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"
	"time"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/oauth"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type PublicStatusSearchTestSuite struct {
	SearchStandardTestSuite
	sequence int
}

const publicSearchTerm = "publicsearchregression"

func (s *PublicStatusSearchTestSuite) addSearchStatus(accountKey string, visibility gtsmodel.Visibility) string {
	a := s.testAccounts[accountKey]
	status := *testrig.NewTestStatuses()["remote_account_1_status_1"]
	s.sequence++
	status.CreatedAt = time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC).Add(time.Duration(s.sequence) * time.Second)
	status.ID = id.NewULIDFromTime(status.CreatedAt)
	status.URI = a.URI + "/statuses/" + status.ID
	status.URL = status.URI
	status.AccountID, status.AccountURI = a.ID, a.URI
	status.Visibility = visibility
	status.Content = "<p>" + publicSearchTerm + "</p>"
	status.AttachmentIDs = nil
	status.Flags.SetLocal(a.Domain == "")
	s.Require().NoError(s.db.PutStatus(s.T().Context(), &status))
	return status.ID
}
func (s *PublicStatusSearchTestSuite) searchPublic(params url.Values, authenticated bool) (*apimodel.SearchResult, int) {
	if params == nil {
		params = url.Values{}
	}
	if !params.Has("q") {
		params.Set("q", publicSearchTerm)
	}
	params.Set("type", "statuses")
	params.Set("resolve", "false")
	recorder := httptest.NewRecorder()
	c := s.newContext(recorder, "api/v2/search?"+params.Encode())
	c.AddParam(apiutil.APIVersionKey, "v2")
	if !authenticated {
		c.Set(oauth.SessionAuthorizedAccount, nil)
		c.Set(oauth.SessionAuthorizedToken, nil)
		c.Set(oauth.SessionAuthorizedUser, nil)
		c.Set(oauth.SessionAuthorizedApplication, nil)
	}
	s.searchModule.SearchGETHandler(c)
	if recorder.Code != http.StatusOK {
		return nil, recorder.Code
	}
	var result apimodel.SearchResult
	s.Require().NoError(json.Unmarshal(recorder.Body.Bytes(), &result))
	return &result, recorder.Code
}
func (s *PublicStatusSearchTestSuite) TestPublicRemoteWithoutIndexPermission() {
	a := s.testAccounts["remote_account_1"]
	a.Indexable = util.Ptr(false)
	s.Require().NoError(s.db.UpdateAccount(s.T().Context(), a, "indexable"))
	expected := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	result, code := s.searchPublic(nil, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(expected, result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestForeignNonPublicExcluded() {
	for _, v := range []gtsmodel.Visibility{gtsmodel.VisibilityUnlocked, gtsmodel.VisibilityFollowersOnly, gtsmodel.VisibilityDirect} {
		s.addSearchStatus("remote_account_1", v)
	}
	result, code := s.searchPublic(nil, true)
	s.Require().Equal(http.StatusOK, code)
	s.Empty(result.Statuses)
}
func (s *PublicStatusSearchTestSuite) TestOwnPrivateStillSearchable() {
	expected := s.addSearchStatus("local_account_1", gtsmodel.VisibilityDirect)
	result, code := s.searchPublic(nil, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(expected, result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestBlockedAuthorExcluded() {
	s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	local, remote := s.testAccounts["local_account_1"], s.testAccounts["remote_account_1"]
	s.Require().NoError(s.db.PutBlock(s.T().Context(), &gtsmodel.Block{ID: id.NewULID(), URI: local.URI + "/blocks/search", AccountID: local.ID, TargetAccountID: remote.ID}))
	result, code := s.searchPublic(nil, true)
	s.Require().Equal(http.StatusOK, code)
	s.Empty(result.Statuses)
}
func (s *PublicStatusSearchTestSuite) TestUnauthenticatedExcluded() {
	s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	_, code := s.searchPublic(nil, false)
	s.Equal(http.StatusUnauthorized, code)
}
func (s *PublicStatusSearchTestSuite) TestAuthorFilter() {
	s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	expected := s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	params := url.Values{"account_id": {s.testAccounts["remote_account_2"].ID}}
	result, code := s.searchPublic(params, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(expected, result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestPaging() {
	ids := []string{s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic), s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic), s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	result, code := s.searchPublic(url.Values{"limit": {"1"}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(ids[0], result.Statuses[0].ID)
	result, code = s.searchPublic(url.Values{"limit": {"1"}, "max_id": {ids[0]}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(ids[1], result.Statuses[0].ID)
	result, code = s.searchPublic(url.Values{"limit": {"1"}, "min_id": {ids[2]}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(ids[1], result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestBoostWrappersExcluded() {
	statusID := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	status, err := s.db.GetStatusByID(s.T().Context(), statusID)
	s.Require().NoError(err)
	status.BoostOfID = testrig.NewTestStatuses()["remote_account_1_status_1"].ID
	s.Require().NoError(s.db.UpdateStatus(s.T().Context(), status, "boost_of_id"))
	result, code := s.searchPublic(nil, true)
	s.Require().Equal(http.StatusOK, code)
	s.Empty(result.Statuses)
}

func (s *PublicStatusSearchTestSuite) TestBlockedCandidatesDoNotConsumePage() {
	visible := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	local, blocked := s.testAccounts["local_account_1"], s.testAccounts["remote_account_2"]
	s.Require().NoError(s.db.PutBlock(s.T().Context(), &gtsmodel.Block{ID: id.NewULID(), URI: local.URI + "/blocks/page", AccountID: local.ID, TargetAccountID: blocked.ID}))
	result, code := s.searchPublic(url.Values{"limit": {"1"}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1, "a newer blocked post must not hide the visible result")
	s.Equal(visible, result.Statuses[0].ID)
}

func (s *PublicStatusSearchTestSuite) TestSuspendedCandidatesDoNotConsumePage() {
	visible := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	suspended := s.testAccounts["remote_account_2"]
	suspended.SuspendedAt = time.Now()
	s.Require().NoError(s.db.UpdateAccount(s.T().Context(), suspended, "suspended_at"))
	result, code := s.searchPublic(url.Values{"limit": {"1"}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(visible, result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestPendingCandidatesDoNotConsumePage() {
	visible := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	pendingID := s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	pending, err := s.db.GetStatusByID(s.T().Context(), pendingID)
	s.Require().NoError(err)
	pending.Flags.SetPendingApproval(true)
	s.Require().NoError(s.db.UpdateStatus(s.T().Context(), pending, "flags"))
	result, code := s.searchPublic(url.Values{"limit": {"1"}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(visible, result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestDeletedCandidatesDoNotConsumeAuthorPage() {
	visible := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	deletedID := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	deleted, err := s.db.GetStatusByID(s.T().Context(), deletedID)
	s.Require().NoError(err)
	s.Require().NoError(s.db.StubStatus(s.T().Context(), deleted, true))
	author := s.testAccounts["remote_account_1"]
	result, code := s.searchPublic(url.Values{"limit": {"1"}, "q": {"from:" + author.Username + "@" + author.Domain}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(visible, result.Statuses[0].ID)
}

func (s *PublicStatusSearchTestSuite) TestDomainBlockedCandidatesDoNotConsumePage() {
	visible := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	blocked := s.testAccounts["remote_account_2"]
	s.Require().NoError(s.db.PutDomainBlock(s.T().Context(), &gtsmodel.DomainBlock{ID: id.NewULID(), Domain: blocked.Domain, CreatedByAccountID: s.testAccounts["admin_account"].ID}))
	result, code := s.searchPublic(url.Values{"limit": {"1"}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(visible, result.Statuses[0].ID)
}

func (s *PublicStatusSearchTestSuite) TestReverseBlockDoesNotConsumePage() {
	visible := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	local, remote := s.testAccounts["local_account_1"], s.testAccounts["remote_account_2"]
	s.Require().NoError(s.db.PutBlock(s.T().Context(), &gtsmodel.Block{ID: id.NewULID(), URI: remote.URI + "/blocks/page", AccountID: remote.ID, TargetAccountID: local.ID}))
	result, code := s.searchPublic(url.Values{"limit": {"1"}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 1)
	s.Equal(visible, result.Statuses[0].ID)
}
func (s *PublicStatusSearchTestSuite) TestUpwardPageRefillsAndPreservesOrder() {
	oldest := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	first := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	s.addSearchStatus("remote_account_2", gtsmodel.VisibilityPublic)
	second := s.addSearchStatus("remote_account_1", gtsmodel.VisibilityPublic)
	blocked := s.testAccounts["remote_account_2"]
	s.Require().NoError(s.db.PutDomainBlock(s.T().Context(), &gtsmodel.DomainBlock{ID: id.NewULID(), Domain: blocked.Domain, CreatedByAccountID: s.testAccounts["admin_account"].ID}))
	result, code := s.searchPublic(url.Values{"limit": {"2"}, "min_id": {oldest}}, true)
	s.Require().Equal(http.StatusOK, code)
	s.Require().Len(result.Statuses, 2)
	s.Equal(second, result.Statuses[0].ID)
	s.Equal(first, result.Statuses[1].ID)
}

func TestPublicStatusSearch(t *testing.T) { suite.Run(t, new(PublicStatusSearchTestSuite)) }
