// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package user_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	userapi "code.superseriousbusiness.org/gotosocial/internal/api/client/user"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/oauth"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type BlueskyHandlerTestSuite struct {
	UserStandardTestSuite
}

func (suite *BlueskyHandlerTestSuite) TestMetadataIsPublic() {
	recorder := httptest.NewRecorder()
	ctx, _ := testrig.CreateGinTestContext(recorder, nil)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/user/bluesky/client-metadata.json", nil)
	suite.userModule.BlueskyMetadataGETHandler(ctx)
	suite.Equal(http.StatusOK, recorder.Code)
	suite.Contains(recorder.Body.String(), `"dpop_bound_access_tokens":true`)
}

func (suite *BlueskyHandlerTestSuite) TestInvalidCallbackRedirectsSafely() {
	recorder := httptest.NewRecorder()
	ctx, _ := testrig.CreateGinTestContext(recorder, nil)
	ctx.Request = httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/user/bluesky/callback?state=missing", nil)
	suite.userModule.BlueskyCallbackGETHandler(ctx)
	suite.Equal(http.StatusFound, recorder.Code)
	suite.Equal("/settings/user/bluesky?error=connection_failed", recorder.Header().Get("Location"))
}

func (suite *BlueskyHandlerTestSuite) TestAppPasswordRequiresBoundedSecret() {
	_, code := suite.POST(userapi.BlueskyAppPasswordPath, map[string][]string{
		"identifier": {"fischr.org"},
	}, suite.userModule.BlueskyAppPasswordPOSTHandler)
	suite.Equal(http.StatusBadRequest, code)

	_, code = suite.POST(userapi.BlueskyAppPasswordPath, map[string][]string{
		"identifier":   {"fischr.org"},
		"app_password": {strings.Repeat("x", 257)},
	}, suite.userModule.BlueskyAppPasswordPOSTHandler)
	suite.Equal(http.StatusBadRequest, code)
}

func TestBlueskyHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(BlueskyHandlerTestSuite))
}

func (suite *BlueskyHandlerTestSuite) TestApplicationSelectionJSONPatch() {
	accountID := suite.testAccounts["local_account_1"].ID
	suite.Require().NoError(suite.db.PutBlueskyConnection(suite.T().Context(), &gtsmodel.BlueskyConnection{
		ID: id.NewULID(), AccountID: accountID, DID: "did:plc:settings-api", Handle: "publisher.example", PDSURL: "https://pds.example", CrosspostPublic: true,
	}))
	appID := suite.testApplications["application_1"].ID
	patch := func(body string, expectedCode int) apimodel.BlueskyConnection {
		recorder := httptest.NewRecorder()
		ctx, _ := testrig.CreateGinTestContext(recorder, nil)
		ctx.Set(oauth.SessionAuthorizedApplication, suite.testApplications["application_1"])
		ctx.Set(oauth.SessionAuthorizedToken, oauth.DBTokenToToken(suite.testTokens["local_account_1"]))
		ctx.Set(oauth.SessionAuthorizedUser, suite.testUsers["local_account_1"])
		ctx.Set(oauth.SessionAuthorizedAccount, suite.testAccounts["local_account_1"])
		ctx.Request = httptest.NewRequest(http.MethodPatch, "http://localhost:8080/api/v1/user/bluesky", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Request.Header.Set("Accept", "application/json")
		suite.userModule.BlueskyPATCHHandler(ctx)
		suite.Require().Equal(expectedCode, recorder.Code, recorder.Body.String())
		var result apimodel.BlueskyConnection
		if expectedCode == http.StatusOK {
			suite.Require().NoError(json.Unmarshal(recorder.Body.Bytes(), &result))
		}
		return result
	}
	result := patch(`{"crosspost_application_filter":true,"crosspost_application_ids":["`+appID+`"]}`, http.StatusOK)
	suite.True(result.CrosspostApplicationFilter)
	suite.Equal([]string{appID}, result.CrosspostApplicationIDs)
	result = patch(`{"show_profile_follow":true}`, http.StatusOK)
	suite.True(result.CrosspostApplicationFilter)
	suite.Equal([]string{appID}, result.CrosspostApplicationIDs)
	result = patch(`{"crosspost_application_ids":[]}`, http.StatusOK)
	suite.True(result.CrosspostApplicationFilter)
	suite.Empty(result.CrosspostApplicationIDs)
	patch(`{"crosspost_public":false,"crosspost_application_ids":["`+suite.testApplications["application_2"].ID+`"]}`, http.StatusBadRequest)
	stored, err := suite.db.GetBlueskyConnectionByAccountID(suite.T().Context(), accountID)
	suite.Require().NoError(err)
	suite.True(stored.CrosspostPublic)
	suite.True(stored.CrosspostApplicationFilter)
}
