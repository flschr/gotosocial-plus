// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package bundb_test

import (
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"github.com/stretchr/testify/suite"
)

type SearchTestSuite struct {
	BunDBStandardTestSuite
}

func (suite *SearchTestSuite) TestSearchAccountsTurtleAny() {
	testAccount := suite.testAccounts["local_account_1"]

	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "turtle", "", "", 10, false, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccounts1HappyWithPrefix() {
	testAccount := suite.testAccounts["local_account_1"]

	// Query will just look for usernames that start with "1happy".
	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "@1happy", "", "", 10, false, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccounts1HappyWithPrefixUpper() {
	testAccount := suite.testAccounts["local_account_1"]

	// Query will just look for usernames that start with "1HAPPY".
	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "@1HAPPY", "", "", 10, false, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccounts1HappyNoPrefix() {
	testAccount := suite.testAccounts["local_account_1"]

	// Query will do the full coalesce.
	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "1happy", "", "", 10, false, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccountsTurtleFollowing() {
	testAccount := suite.testAccounts["local_account_1"]

	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "turtle", "", "", 10, true, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccountsTurtleFollowingUpper() {
	testAccount := suite.testAccounts["local_account_1"]

	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "TURTLE", "", "", 10, true, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccountsPostFollowing() {
	testAccount := suite.testAccounts["local_account_1"]

	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "post", "", "", 10, true, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchAccountsPostAny() {
	testAccount := suite.testAccounts["local_account_1"]

	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "post", "", "", 10, false, 0)
	suite.NoError(err, db.ErrNoEntries)
	suite.Empty(accounts)
}

func (suite *SearchTestSuite) TestSearchAccountsFossAny() {
	testAccount := suite.testAccounts["local_account_1"]

	accounts, err := suite.db.SearchForAccounts(suite.T().Context(), testAccount.ID, "foss", "", "", 10, false, 0)
	suite.NoError(err)
	suite.Len(accounts, 1)
}

func (suite *SearchTestSuite) TestSearchStatuses() {
	testAccount := suite.testAccounts["local_account_1"]

	statuses, err := suite.db.SearchForStatuses(suite.T().Context(), testAccount.ID, "hello", "", "", "", 10, 0)
	suite.NoError(err)
	suite.Len(statuses, 2)
}

func (suite *SearchTestSuite) TestSearchStatusesFromAccount() {
	testAccount := suite.testAccounts["local_account_1"]
	fromAccount := suite.testAccounts["local_account_2"]

	statuses, err := suite.db.SearchForStatuses(suite.T().Context(), testAccount.ID, "hi", fromAccount.ID, "", "", 10, 0)
	suite.NoError(err)
	suite.Len(statuses, 5)
	for _, status := range statuses {
		suite.Equal(fromAccount.ID, status.AccountID)
	}
}

func (suite *SearchTestSuite) TestSearchTags() {
	// Search with full tag string.
	tags, err := suite.db.SearchForTags(suite.T().Context(), "welcome", "", "", 10, 0)
	suite.NoError(err)
	suite.Len(tags, 1)

	// Search with partial tag string.
	tags, err = suite.db.SearchForTags(suite.T().Context(), "wel", "", "", 10, 0)
	suite.NoError(err)
	suite.Len(tags, 1)

	// Search with end of tag string.
	tags, err = suite.db.SearchForTags(suite.T().Context(), "come", "", "", 10, 0)
	suite.NoError(err)
	suite.Len(tags, 0)
}

// The candidate query must add public remote posts without adding unrelated
// non-public posts. API tests separately verify authentication and visibility.
func (suite *SearchTestSuite) TestPublicStatusCandidates() {
	ctx := suite.T().Context()
	requester := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["remote_account_1_status_1"]
	status.Content = "uniquepubliccandidate"
	status.Visibility = gtsmodel.VisibilityPublic
	suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "content", "visibility"))
	statuses, err := suite.db.SearchForStatuses(ctx, requester.ID, "uniquepubliccandidate", "", "", "", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(statuses, 1)
	suite.Equal(status.ID, statuses[0].ID)
	for _, visibility := range []gtsmodel.Visibility{gtsmodel.VisibilityUnlocked, gtsmodel.VisibilityFollowersOnly, gtsmodel.VisibilityDirect} {
		status.Visibility = visibility
		suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "visibility"))
		statuses, err := suite.db.SearchForStatuses(ctx, requester.ID, "uniquepubliccandidate", "", "", "", 10, 0)
		suite.NoError(err)
		suite.Empty(statuses)
	}
}

func (suite *SearchTestSuite) TestPrivateRepliesRemainCandidates() {
	ctx := suite.T().Context()
	requester := suite.testAccounts["local_account_1"]
	status := suite.testStatuses["remote_account_1_status_1"]
	status.Content = "uniqueprivatereply"
	status.Visibility = gtsmodel.VisibilityDirect
	status.InReplyToAccountID = requester.ID
	suite.Require().NoError(suite.db.UpdateStatus(ctx, status, "content", "visibility", "in_reply_to_account_id"))
	statuses, err := suite.db.SearchForStatuses(ctx, requester.ID, "uniqueprivatereply", "", "", "", 10, 0)
	suite.Require().NoError(err)
	suite.Require().Len(statuses, 1)
	suite.Equal(status.ID, statuses[0].ID)
}

func TestSearchTestSuite(t *testing.T) {
	suite.Run(t, new(SearchTestSuite))
}
