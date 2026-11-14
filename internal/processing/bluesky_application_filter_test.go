// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing_test

import (
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/stretchr/testify/suite"
)

type BlueskyApplicationFilterSuite struct{ ProcessingStandardTestSuite }

func (s *BlueskyApplicationFilterSuite) TestSettings() {
	ctx := s.T().Context()
	accountID := s.testAccounts["local_account_1"].ID
	appID := s.testApplications["application_1"].ID
	connection := &gtsmodel.BlueskyConnection{ID: id.NewULID(), AccountID: accountID, DID: "did:plc:filter", Handle: "filter.example", PDSURL: "https://pds.example", CrosspostPublic: true}
	s.Require().NoError(s.db.PutBlueskyConnection(ctx, connection))
	enabled := true
	ids := []string{appID, appID}
	result, err := s.processor.BlueskySettingsUpdate(ctx, accountID, &apimodel.BlueskySettingsUpdateRequest{CrosspostApplicationFilter: &enabled, CrosspostApplicationIDs: &ids})
	s.Require().Nil(err)
	s.True(result.CrosspostApplicationFilter)
	s.Equal([]string{appID}, result.CrosspostApplicationIDs)
	s.Contains(result.Applications, apimodel.BlueskyApplication{ID: appID, Name: s.testApplications["application_1"].Name})
	// Invalid selection cannot partially disable publishing.
	s.NotContains(result.Applications, apimodel.BlueskyApplication{ID: s.testApplications["application_2"].ID, Name: s.testApplications["application_2"].Name})
	foreignIDs := []string{s.testApplications["application_2"].ID}
	disabled := false
	_, err = s.processor.BlueskySettingsUpdate(ctx, accountID, &apimodel.BlueskySettingsUpdateRequest{CrosspostPublic: &disabled, CrosspostApplicationIDs: &foreignIDs})
	s.Require().NotNil(err)
	s.Equal(400, err.Code())
	result, err = s.processor.BlueskyConnectionGet(ctx, accountID)
	s.Require().Nil(err)
	s.True(result.CrosspostPublic)
	s.Equal([]string{appID}, result.CrosspostApplicationIDs)
	// Omitting the selection preserves it, including when older clients save settings.
	result, err = s.processor.BlueskySettingsUpdate(ctx, accountID, &apimodel.BlueskySettingsUpdateRequest{ShowProfileFollow: &enabled})
	s.Require().Nil(err)
	s.Equal([]string{appID}, result.CrosspostApplicationIDs)
	// Explicit empty arrays clear the selection without falling back to all apps.
	empty := []string{}
	result, err = s.processor.BlueskySettingsUpdate(ctx, accountID, &apimodel.BlueskySettingsUpdateRequest{CrosspostApplicationIDs: &empty})
	s.Require().Nil(err)
	s.True(result.CrosspostApplicationFilter)
	s.Empty(result.CrosspostApplicationIDs)
}

func TestBlueskyApplicationFilterSuite(t *testing.T) {
	suite.Run(t, new(BlueskyApplicationFilterSuite))
}
