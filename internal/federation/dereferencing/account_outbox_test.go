// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package dereferencing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/ap"
	"code.superseriousbusiness.org/gotosocial/internal/federation/dereferencing"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/testrig"
	"github.com/stretchr/testify/suite"
)

type ProfileHistoryTestSuite struct{ DereferencerStandardTestSuite }

// All fixture URLs are served in memory; no actual federation is performed.
func (s *ProfileHistoryTestSuite) historyClient(documents map[string]any) *atomic.Int32 {
	fallback := s.client
	calls := new(atomic.Int32)
	client := testrig.NewMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if document, ok := documents[req.URL.String()]; ok {
			calls.Add(1)
			body, err := json.Marshal(document)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/activity+json"}}, Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: req}, nil
		}
		return fallback.Do(req)
	}, "")
	s.dereferencer = dereferencing.NewDereferencer(&s.state, s.converter, testrig.NewTestTransportController(&s.state, client), s.visFilter, s.intFilter, s.relayFilter, s.media)
	return calls
}

func historyNote(account *gtsmodel.Account, name string) map[string]any {
	return map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "Note", "id": account.URI + "/statuses/" + name, "attributedTo": account.URI, "to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "content": "A public history entry", "published": "2026-10-01T10:00:00Z"}
}
func historyCollection(items ...any) map[string]any {
	return map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollection", "orderedItems": items}
}
func (s *ProfileHistoryTestSuite) TestImportAndConcurrentCache() {
	a := s.testAccounts["remote_account_1"]
	n1, n2, n3 := historyNote(a, "history1"), historyNote(a, "history2"), historyNote(a, "history3")
	page := a.OutboxURI + "?page=true"
	calls := s.historyClient(map[string]any{
		a.OutboxURI:       map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollection", "first": page},
		page:              map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollectionPage", "orderedItems": []any{n1["id"], n2, map[string]any{"type": "Create", "object": n3}}},
		n1["id"].(string): n1, n2["id"].(string): n2, n3["id"].(string): n3,
	})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.dereferencer.RefreshAccountOutbox(s.T().Context(), a) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		s.Require().NoError(err)
	}
	for _, n := range []map[string]any{n1, n2, n3} {
		status, err := s.db.GetStatusByURI(s.T().Context(), n["id"].(string))
		s.Require().NoError(err)
		s.Equal(gtsmodel.VisibilityPublic, status.Visibility)
		s.Equal(a.ID, status.AccountID)
	}
	s.Equal(int32(5), calls.Load(), "one collection, one page, three canonical status fetches")
}
func (s *ProfileHistoryTestSuite) TestRejectNonPublicAndWrongAuthor() {
	a := s.testAccounts["remote_account_1"]
	private, unlisted, foreign := historyNote(a, "private"), historyNote(a, "unlisted"), historyNote(a, "foreign")
	private["to"] = []string{a.FollowersURI}
	unlisted["to"] = []string{a.FollowersURI}
	unlisted["cc"] = []string{"https://www.w3.org/ns/activitystreams#Public"}
	foreign["attributedTo"] = a.URI + "-someone-else"
	s.historyClient(map[string]any{a.OutboxURI: historyCollection(private, unlisted, foreign), private["id"].(string): private, unlisted["id"].(string): unlisted, foreign["id"].(string): foreign})
	s.Require().NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	for _, n := range []map[string]any{private, unlisted, foreign} {
		status, _ := s.db.GetStatusByURI(s.T().Context(), n["id"].(string))
		s.Nil(status)
	}
}
func (s *ProfileHistoryTestSuite) TestFortyEntryBound() {
	a := s.testAccounts["remote_account_1"]
	n := historyNote(a, "beyond-limit")
	items := make([]any, 40)
	// Invalid entries count towards the bound, too.
	for i := range items {
		items[i] = "https://other.example/status"
	}
	items = append(items, n)
	calls := s.historyClient(map[string]any{a.OutboxURI: historyCollection(items...), n["id"].(string): n})
	s.Require().NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Equal(int32(1), calls.Load())
	status, _ := s.db.GetStatusByURI(s.T().Context(), n["id"].(string))
	s.Nil(status)
}
func (s *ProfileHistoryTestSuite) TestPageBound() {
	a := s.testAccounts["remote_account_1"]
	docs := map[string]any{a.OutboxURI: map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollection", "first": a.OutboxURI + "?page=1"}}
	for i := 1; i <= 5; i++ {
		docs[fmt.Sprintf("%s?page=%d", a.OutboxURI, i)] = map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollectionPage", "orderedItems": []any{}, "next": fmt.Sprintf("%s?page=%d", a.OutboxURI, i+1)}
	}
	calls := s.historyClient(docs)
	s.Require().NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Equal(int32(5), calls.Load())
}
func (s *ProfileHistoryTestSuite) TestFailureBackoff() {
	a := s.testAccounts["remote_account_1"]
	calls := s.historyClient(map[string]any{a.OutboxURI: map[string]any{"type": "NotACollection"}})
	s.Error(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Equal(int32(1), calls.Load())
}
func (s *ProfileHistoryTestSuite) TestBlockedDomainDoesNotFetch() {
	a := s.testAccounts["remote_account_1"]
	s.Require().NoError(s.db.PutDomainBlock(s.T().Context(), &gtsmodel.DomainBlock{ID: id.NewULID(), Domain: a.Domain, CreatedByAccountID: s.testAccounts["admin_account"].ID}))
	calls := s.historyClient(map[string]any{a.OutboxURI: historyCollection()})
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Zero(calls.Load())
}
func (s *ProfileHistoryTestSuite) TestEmbeddedFirstPageAndCycle() {
	a := s.testAccounts["remote_account_1"]
	page := a.OutboxURI + "?page=1"
	calls := s.historyClient(map[string]any{
		a.OutboxURI: map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollection", "first": map[string]any{"type": "OrderedCollectionPage", "orderedItems": []any{}, "next": page}},
		page:        map[string]any{"@context": "https://www.w3.org/ns/activitystreams", "type": "OrderedCollectionPage", "orderedItems": []any{}, "next": page},
	})
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Equal(int32(2), calls.Load(), "a repeated page must stop traversal")
}

func (s *ProfileHistoryTestSuite) TestCancellationAndBackoff() {
	a := s.testAccounts["remote_account_1"]
	calls := new(atomic.Int32)
	client := testrig.NewMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}, "")
	s.dereferencer = dereferencing.NewDereferencer(&s.state, s.converter, testrig.NewTestTransportController(&s.state, client), s.visFilter, s.intFilter, s.relayFilter, s.media)
	ctx, cancel := context.WithTimeout(s.T().Context(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	s.Error(s.dereferencer.RefreshAccountOutbox(ctx, a))
	s.Less(time.Since(start), time.Second)
	before := calls.Load()
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Equal(before, calls.Load(), "a timed out refresh must be backed off")
}

func (s *ProfileHistoryTestSuite) TestCancelledWaiterDoesNotWaitForOwner() {
	a := s.testAccounts["remote_account_1"]
	started, release := make(chan struct{}), make(chan struct{})
	fallback := s.client
	client := testrig.NewMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != a.OutboxURI {
			return fallback.Do(req)
		}
		close(started)
		<-release
		body, _ := json.Marshal(historyCollection())
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/activity+json"}}, Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: req}, nil
	}, "")
	s.dereferencer = dereferencing.NewDereferencer(&s.state, s.converter, testrig.NewTestTransportController(&s.state, client), s.visFilter, s.intFilter, s.relayFilter, s.media)
	owner := make(chan error, 1)
	go func() { owner <- s.dereferencer.RefreshAccountOutbox(s.T().Context(), a) }()
	<-started
	ctx, cancel := context.WithCancel(s.T().Context())
	waiter := make(chan error, 1)
	go func() { waiter <- s.dereferencer.RefreshAccountOutbox(ctx, a) }()
	cancel()
	select {
	case err := <-waiter:
		s.ErrorIs(err, context.Canceled)
	case <-time.After(time.Second):
		s.Fail("cancelled profile waiter remained blocked on another request")
	}
	close(release)
	s.NoError(<-owner)
}

func (s *ProfileHistoryTestSuite) TestRedirectCannotResurrectDeletedStatus() {
	a := s.testAccounts["remote_account_1"]
	old, err := s.db.GetStatusByID(s.T().Context(), testrig.NewTestStatuses()["remote_account_1_status_1"].ID)
	s.Require().NoError(err)
	s.Require().NoError(s.db.StubStatus(s.T().Context(), old, true))
	alias := old.URI + "?history=alias"
	note := historyNote(a, "unused")
	note["id"], note["published"] = old.URI, old.CreatedAt.UTC().Format(time.RFC3339)
	fallback := s.client
	client := testrig.NewMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		var document any
		final := req.URL
		switch req.URL.String() {
		case a.OutboxURI:
			document = historyCollection(alias)
		case alias:
			document = note
			final = testrig.URLMustParse(old.URI)
		default:
			return fallback.Do(req)
		}
		body, _ := json.Marshal(document)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/activity+json"}}, Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Request: &http.Request{URL: final}}, nil
	}, "")
	s.dereferencer = dereferencing.NewDereferencer(&s.state, s.converter, testrig.NewTestTransportController(&s.state, client), s.visFilter, s.intFilter, s.relayFilter, s.media)
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	stored, err := s.db.GetStatusByID(s.T().Context(), old.ID)
	s.Require().NoError(err)
	s.True(stored.Flags.Deleted(), "historical content must never resurrect a locally deleted status")
	s.Empty(stored.Content)
	// Direct federation lookup through the same redirected alias must also
	// preserve the canonical deletion, not just the profile-import shortcut.
	_, _, err = s.dereferencer.GetStatusByURI(s.T().Context(), "", testrig.URLMustParse(alias))
	s.NoError(err)
	stored, err = s.db.GetStatusByID(s.T().Context(), old.ID)
	s.Require().NoError(err)
	s.True(stored.Flags.Deleted())
	s.Empty(stored.Content)
}

func (s *ProfileHistoryTestSuite) TestFetchedModelCannotUndoConcurrentDelete() {
	a := s.testAccounts["remote_account_1"]
	stale, err := s.db.GetStatusByID(s.T().Context(), testrig.NewTestStatuses()["remote_account_1_status_1"].ID)
	s.Require().NoError(err)
	note := historyNote(a, "unused")
	note["id"], note["published"] = stale.URI, stale.CreatedAt.UTC().Format(time.RFC3339)
	body, _ := json.Marshal(note)
	statusable, err := ap.ResolveStatusable(s.T().Context(), io.NopCloser(strings.NewReader(string(body))))
	s.Require().NoError(err)
	s.Require().NoError(s.db.StubStatus(s.T().Context(), stale, true))
	var surfaced atomic.Int32
	s.dereferencer.OnStatusDereference = func(context.Context, *gtsmodel.Status, bool) error { surfaced.Add(1); return nil }
	_, _, err = s.dereferencer.RefreshStatus(s.T().Context(), "", stale, statusable, nil)
	s.Zero(surfaced.Load(), "deleted models must not emit a status update")
	s.NoError(err)
	stored, err := s.db.GetStatusByID(s.T().Context(), stale.ID)
	s.Require().NoError(err)
	s.True(stored.Flags.Deleted())
	s.Empty(stored.Content)
}

func (s *ProfileHistoryTestSuite) TestStatusLockHonorsCancellation() {
	stale, err := s.db.GetStatusByID(s.T().Context(), testrig.NewTestStatuses()["remote_account_1_status_1"].ID)
	s.Require().NoError(err)
	body, _ := json.Marshal(historyNote(s.testAccounts["remote_account_1"], "unused"))
	statusable, err := ap.ResolveStatusable(s.T().Context(), io.NopCloser(strings.NewReader(string(body))))
	s.Require().NoError(err)
	unlock := s.state.FedLocks.Lock(stale.URI)
	ctx, cancel := context.WithTimeout(s.T().Context(), 30*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, _, err := s.dereferencer.RefreshStatus(ctx, "", stale, statusable, nil); result <- err }()
	select {
	case err := <-result:
		s.ErrorIs(err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		s.Fail("status storage lock ignored request cancellation")
	}
	unlock()
}

func (s *ProfileHistoryTestSuite) TestSuspendedAccountDoesNotFetch() {
	a := s.testAccounts["remote_account_1"]
	a.SuspendedAt = time.Now()
	calls := s.historyClient(map[string]any{a.OutboxURI: historyCollection()})
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Zero(calls.Load())
}
func (s *ProfileHistoryTestSuite) TestBlockedAccountDomainDoesNotFetch() {
	a := s.testAccounts["remote_account_1"]
	a.Domain = "account-alias.example"
	s.Require().NoError(s.db.PutDomainBlock(s.T().Context(), &gtsmodel.DomainBlock{ID: id.NewULID(), Domain: a.Domain, CreatedByAccountID: s.testAccounts["admin_account"].ID}))
	calls := s.historyClient(map[string]any{a.OutboxURI: historyCollection()})
	s.NoError(s.dereferencer.RefreshAccountOutbox(s.T().Context(), a))
	s.Zero(calls.Load())
}

func (s *ProfileHistoryTestSuite) TestAccountLockHonorsCancellation() {
	account := s.testAccounts["remote_account_1"]
	unlock := s.state.FedLocks.Lock(account.URI)
	ctx, cancel := context.WithTimeout(s.T().Context(), 30*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := s.dereferencer.RefreshAccount(ctx, "", account, nil, &dereferencing.Freshest)
		result <- err
	}()
	select {
	case err := <-result:
		s.ErrorIs(err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		s.Fail("account refresh lock ignored request cancellation")
	}
	unlock()
}

func TestProfileHistory(t *testing.T) { suite.Run(t, new(ProfileHistoryTestSuite)) }
