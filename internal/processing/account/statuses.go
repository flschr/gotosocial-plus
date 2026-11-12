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

package account

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"code.superseriousbusiness.org/gopkg/log"
	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	apiutil "code.superseriousbusiness.org/gotosocial/internal/api/util"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/paging"
	"code.superseriousbusiness.org/gotosocial/internal/util"
)

// StatusesGet fetches a number of statuses (in time descending order) from the
// target account, filtered by visibility according to the requesting account.
func (p *Processor) StatusesGet(
	ctx context.Context,
	requester *gtsmodel.Account,
	targetAccountID string,
	limit int,
	excludeReplies bool,
	excludeReblogs bool,
	maxID string,
	minID string,
	pinned bool,
	mediaOnly bool,
	publicOnly bool,
) (*apimodel.PageableResponse, gtserror.WithCode) {
	if requester != nil {
		blocked, err := p.state.DB.IsEitherBlocked(ctx, requester.ID, targetAccountID)
		if err != nil {
			return nil, gtserror.NewErrorInternalError(err)
		}

		if blocked {
			// Block exists between accounts.
			// Just return empty statuses.
			return util.EmptyPageableResponse(), nil
		}
	}

	// Only the authenticated, unpaged profile view initiates a remote fetch.
	// Pagination and pinned tabs continue to use the local snapshot.
	if requester != nil && maxID == "" && minID == "" && !pinned {
		account, err := p.state.DB.GetAccountByID(ctx, targetAccountID)
		if err != nil && !errors.Is(err, db.ErrNoEntries) {
			return nil, gtserror.NewErrorInternalError(err)
		}
		if account != nil && account.Domain != "" {
			if err := p.federator.RefreshAccountOutbox(ctx, account); err != nil {
				log.Debugf(ctx, "remote profile refresh failed for %s: %v", account.URI, err)
			}
		}
	}

	var (
		statuses []*gtsmodel.Status
		err      error
	)

	if pinned {
		// Get *ONLY* pinned statuses.
		statuses, err = p.state.DB.GetAccountPinnedStatuses(ctx, targetAccountID)
	} else {
		// Get account statuses which *may* include pinned ones.
		statuses, err = p.state.DB.GetAccountStatuses(ctx,
			targetAccountID,
			limit,
			excludeReplies,
			excludeReblogs,
			maxID,
			minID,
			mediaOnly,
			publicOnly,
		)
	}

	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, gtserror.NewErrorInternalError(err)
	}

	count := len(statuses)
	if count == 0 {
		return util.EmptyPageableResponse(), nil
	}

	var (
		items = make([]interface{}, 0, count)

		// Set next + prev values before filtering and API
		// converting, so caller can still page properly.
		nextMaxIDValue = statuses[count-1].ID
		prevMinIDValue = statuses[0].ID
	)

	// Filtering + serialization process is same for
	// both pinned status queries and 'normal' ones.
	filtered, err := p.visFilter.StatusesVisible(ctx,
		requester,
		statuses,
	)
	if err != nil {
		return nil, gtserror.NewErrorInternalError(err)
	}

	for _, status := range filtered {
		// Apply status filtering in account context to each of the statuses.
		filtered, hide, err := p.statusFilter.StatusFilterResultsInContext(ctx,
			requester,
			status,
			gtsmodel.FilterContextAccount,
		)
		if err != nil {
			log.Errorf(ctx, "error filtering status: %v", err)
			continue
		}

		if hide {
			// Don't show.
			continue
		}

		// Convert filtered statuses to API statuses.
		item, err := p.converter.StatusToAPIStatus(ctx,
			status,
			requester,
		)
		if err != nil {
			log.Errorf(ctx, "error convering to api status: %v", err)
			continue
		}

		// Set any filter results.
		item.Filtered = filtered

		// Append item to ret slice.
		items = append(items, item)
	}

	if pinned {
		// We don't page on pinned status responses,
		// so we can save some work + just return items.
		return &apimodel.PageableResponse{
			Items: items,
		}, nil
	}

	return util.PackagePageableResponse(util.PageableResponseParams{
		Items:          items,
		Path:           "/api/v1/accounts/" + targetAccountID + "/statuses",
		NextMaxIDValue: nextMaxIDValue,
		PrevMinIDValue: prevMinIDValue,
		Limit:          limit,
		ExtraQueryParams: []string{
			fmt.Sprintf("exclude_replies=%t", excludeReplies),
			fmt.Sprintf("exclude_reblogs=%t", excludeReblogs),
			fmt.Sprintf("pinned=%t", pinned),
			fmt.Sprintf("only_media=%t", mediaOnly),
			fmt.Sprintf("only_public=%t", publicOnly),
		},
	})
}

type WebStatusesGetResp struct {
	*apimodel.PageableResponse
	AllowsIncludingBoosts bool
	IncludedBoosts        bool
	ExcludedBoosts        bool
}

// WebStatusesGet fetches a number of statuses (in descending order)
// from the given account. It selects only statuses which are suitable
// for showing on the public web profile of an account.
//
// The returned boolean indicates whether boosts were included in the query.
func (p *Processor) WebStatusesGet(
	ctx context.Context,
	targetAccountID string,
	page *paging.Page,
	mediaOnly bool,
	includeBoosts *bool,
) (*WebStatusesGetResp, gtserror.WithCode) {
	account, err := p.state.DB.GetAccountByID(ctx, targetAccountID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("db error getting account: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	if account == nil {
		err := gtserror.Newf("account %s not found", targetAccountID)
		return nil, gtserror.NewErrorNotFound(err)
	}

	if account.Domain != "" {
		err := gtserror.Newf("account %s not local", targetAccountID)
		return nil, gtserror.NewErrorNotFound(err)
	}

	// Consider account's preference for including boosts,
	// as well as provided includeBoosts param, to determine
	// if we should include boosts when fetching statuses.
	var (
		allowsIncludingBoosts = *account.Settings.WebIncludeBoosts
		includingBoosts       bool
		excludingBoosts       bool
	)
	switch {
	case !allowsIncludingBoosts:
		// Never include boosts as
		// this account doesn't allow it
		// (leave includingBoosts false).
	case includeBoosts != nil:
		// Account allows including
		// boosts, but includeBoosts
		// param was explicitly provided,
		// so use this instead.
		includingBoosts = *includeBoosts

		// If includingBoosts was (*bool)(false),
		// boosts are being explicitly excluded.
		if !includingBoosts {
			excludingBoosts = true
		}
	default:
		// Account allows including boosts,
		// caller hasn't expressed a preference
		// for with or without, so include.
		includingBoosts = true
	}

	statuses, err := p.state.DB.GetAccountWebStatuses(ctx,
		account,
		page,
		mediaOnly,
		includingBoosts,
	)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		err := gtserror.Newf("db error getting statuses: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	count := len(statuses)
	if count == 0 {
		return &WebStatusesGetResp{
			PageableResponse:      util.EmptyPageableResponse(),
			AllowsIncludingBoosts: allowsIncludingBoosts,
			IncludedBoosts:        includingBoosts,
			ExcludedBoosts:        excludingBoosts,
		}, nil
	}

	var (
		// Preallocate expected frontend items.
		items = make([]any, 0, count)

		// Set paging low / high IDs.
		lo = statuses[count-1].ID
		hi = statuses[0].ID
	)

	for _, s := range statuses {
		// Convert fetched statuses to web view statuses.
		item, err := p.converter.StatusToWebStatus(ctx, s)
		if err != nil {
			log.Errorf(ctx, "error convering to web status: %v", err)
			continue
		}
		items = append(items, item)
	}

	// If explicitly excluding boosts,
	// this should be reflected in the
	// next page query params.
	var query url.Values
	if excludingBoosts {
		query = make(map[string][]string, 1)
		query.Set(apiutil.WebIncludeBoostsKey, "false")
	}

	return &WebStatusesGetResp{
		// Package the response.
		PageableResponse: paging.PackageResponse(
			paging.ResponseParams{
				Items: items,
				Path:  "/@" + account.Username,
				Next:  page.Next(lo, hi),
				Prev:  page.Prev(lo, hi),
				Query: query,
			},
		),
		// Indicate to caller whether boosts
		// were included, etc, so they can
		// provide paging options.
		AllowsIncludingBoosts: allowsIncludingBoosts,
		IncludedBoosts:        includingBoosts,
		ExcludedBoosts:        excludingBoosts,
	}, nil
}

// WebStatusesGetPinned returns web versions of pinned statuses.
func (p *Processor) WebStatusesGetPinned(
	ctx context.Context,
	targetAccountID string,
	mediaOnly bool,
) ([]*apimodel.WebStatus, gtserror.WithCode) {
	statuses, err := p.state.DB.GetAccountPinnedStatuses(ctx, targetAccountID)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		return nil, gtserror.NewErrorInternalError(err)
	}

	webStatuses := make([]*apimodel.WebStatus, 0, len(statuses))
	for _, status := range statuses {
		if mediaOnly && len(status.Attachments) == 0 {
			// No media, skip.
			continue
		}

		// Ensure visible via the web.
		visible, err := p.visFilter.StatusVisible(ctx, nil, status)
		if err != nil {
			log.Errorf(ctx, "error checking status visibility: %v", err)
			continue
		}

		if !visible {
			// Don't serve.
			continue
		}

		webStatus, err := p.converter.StatusToWebStatus(ctx, status)
		if err != nil {
			log.Errorf(ctx, "error convering to web status: %v", err)
			continue
		}

		// Normally when viewed via the API, 'pinned' is
		// only true if the *viewing account* has pinned
		// the status being viewed. For web statuses,
		// however, we still want to be able to indicate
		// a pinned status, so bodge this in here.
		webStatus.Pinned = true

		webStatuses = append(webStatuses, webStatus)
	}

	return webStatuses, nil
}
