// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package dereferencing

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"code.superseriousbusiness.org/activity/pub"
	"code.superseriousbusiness.org/activity/streams/vocab"
	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

type profileRefresh struct {
	at      time.Time
	success bool
}

const (
	profileItems   = 40
	profilePages   = 4
	profileTimeout = 5 * time.Second
)

// RefreshAccountOutbox opportunistically imports a bounded public profile
// history. It deliberately uses the instance identity: a profile refresh must
// never import a personalized follower-only outbox into the shared cache.
// Callers must authenticate and check account blocks before calling this.
func (d *Dereferencer) RefreshAccountOutbox(ctx context.Context, account *gtsmodel.Account) error {
	if account.Domain == "" || account.OutboxURI == "" {
		return nil
	}
	actor, err := url.Parse(account.URI)
	if err != nil {
		return err
	}
	outbox, err := url.Parse(account.OutboxURI)
	if err != nil || !profileURIAllowed(outbox, actor) {
		return fmt.Errorf("invalid outbox URI")
	}
	if blocked, err := d.state.DB.IsDomainBlocked(ctx, actor.Hostname()); err != nil {
		return err
	} else if blocked {
		return nil
	}

	// The same lock also coalesces requests from simultaneous profile tabs.
	unlock := d.state.FedLocks.Lock("profile-outbox:" + account.URI)
	defer unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	if refresh, ok := d.profileRefreshes.Get(account.URI); ok {
		ttl := time.Minute
		if refresh.success {
			ttl = 10 * time.Minute
		}
		if now.Sub(refresh.at) < ttl {
			return nil
		}
	}
	// Bound total concurrent history work as well as each individual traversal.
	select {
	case d.profileRefreshSlots <- struct{}{}:
	default:
		return fmt.Errorf("profile refresh capacity reached")
	}
	defer func() { <-d.profileRefreshSlots }()
	ctx, cancel := context.WithTimeout(ctx, profileTimeout)
	defer cancel()
	err = d.fetchAccountOutbox(ctx, account, actor, outbox)
	d.profileRefreshes.Set(account.URI, profileRefresh{at: time.Now(), success: err == nil})
	return err
}

func profileURIAllowed(uri, actor *url.URL) bool {
	return uri != nil && actor != nil && (uri.Scheme == "https" || uri.Scheme == "http") &&
		uri.Scheme == actor.Scheme && uri.Host == actor.Host && uri.Host != "" && uri.User == nil && uri.Fragment == ""
}

// profileItemURI supports outboxes containing status IRIs, embedded Notes, and
// Create activities with either embedded objects or object IRIs. Announce and
// other activities are intentionally excluded from this public history import.
func profileItemURI(item ap.TypeOrIRI) *url.URL {
	if t := item.GetType(); t != nil {
		if t.GetTypeName() == ap.ActivityCreate {
			if activity, ok := t.(ap.WithObject); ok {
				uri, _ := ap.GetOneObjectIRI(activity)
				return uri
			}
			return nil
		}
		if !ap.IsStatusable(t.GetTypeName()) {
			return nil
		}
	}
	uri, _ := pub.ToId(item)
	return uri
}

func (d *Dereferencer) fetchAccountOutbox(ctx context.Context, account *gtsmodel.Account, actor, outbox *url.URL) error {
	collection, err := d.dereferenceCollection(ctx, "", outbox)
	if err != nil {
		return err
	}
	var items interface{ NextItem() ap.TypeOrIRI } = collection
	pages := 0
	var next ap.WithIRI
	if first, ok := collection.(interface {
		GetActivityStreamsFirst() vocab.ActivityStreamsFirstProperty
	}); ok {
		if property := first.GetActivityStreamsFirst(); property != nil {
			if t := property.GetType(); t != nil {
				page, err := ap.ToCollectionPageIterator(t)
				if err != nil {
					return err
				}
				items, next = page, page.NextPage()
				pages = 1
			} else {
				next = property
			}
		}
	}
	seen := map[string]bool{outbox.String(): true}
	remaining := profileItems
	var partialErr error
	for {
		for remaining > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			item := items.NextItem()
			if item == nil {
				break
			}
			remaining-- // Count examined entries, including invalid/private entries.
			uri := profileItemURI(item)
			if !profileURIAllowed(uri, actor) {
				continue
			}
			if err := d.importProfileStatus(ctx, account, actor, uri); err != nil {
				partialErr = err
			}
		}
		if remaining == 0 || next == nil || next.GetIRI() == nil || pages == profilePages {
			return partialErr
		}
		uri := next.GetIRI()
		if !profileURIAllowed(uri, actor) || seen[uri.String()] {
			return partialErr
		}
		seen[uri.String()] = true
		page, err := d.dereferenceCollectionPage(ctx, "", uri)
		if err != nil {
			return err
		}
		pages++
		items, next = page, page.NextPage()
	}
}

func (d *Dereferencer) importProfileStatus(ctx context.Context, account *gtsmodel.Account, actor, uri *url.URL) error {
	// Existing statuses need no history import; normal federation refreshes them.
	existing, err := d.getStatusFromDB(ctx, uri.String())
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	transport, err := d.transportController.NewTransportForUsername(ctx, "")
	if err != nil {
		return err
	}
	statusable, existing, canonical, err := d.retrieveStatusable(ctx, transport, uri)
	if err != nil {
		return err
	}
	author, err := ap.GetOneAttributedTo(statusable)
	if err != nil || author.String() != account.URI || !profileURIAllowed(canonical, actor) {
		return nil
	}
	visibility, err := ap.ExtractVisibility(statusable, account.FollowersURI)
	if err != nil {
		return err
	}
	if visibility != gtsmodel.VisibilityPublic {
		return nil
	}
	if existing == nil {
		existing = &gtsmodel.Status{URI: canonical.String()}
	}
	// Reuse normal validation, locking, storage and surfacing; do not recursively
	// crawl threads from historical entries, which would defeat the import bound.
	_, _, _, err = d.enrichAndStoreStatusSafely(ctx, "", canonical, existing, statusable)
	return err
}
