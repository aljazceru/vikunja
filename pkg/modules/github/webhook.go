// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"

	"xorm.io/xorm"
)

// deliveryTTL bounds how long a delivery id is remembered. GitHub re-delivers
// webhooks it deems failed, so ids must be deduped long enough that a retry of
// an already-processed delivery is recognized.
const deliveryTTL = 24 * time.Hour

// seenDeliveries remembers processed delivery ids. In-memory is enough:
// Vikunja is a single process, and a duplicate after a restart only causes one
// idempotent re-sync.
var seenDeliveries = struct {
	sync.Mutex
	ids map[string]time.Time
}{ids: map[string]time.Time{}}

func deliverySeen(id string) bool {
	if id == "" {
		return false
	}
	seenDeliveries.Lock()
	defer seenDeliveries.Unlock()
	if _, ok := seenDeliveries.ids[id]; ok {
		return true
	}
	now := time.Now()
	// Prune opportunistically; map stays tiny either way.
	for k, ts := range seenDeliveries.ids {
		if now.Sub(ts) > deliveryTTL {
			delete(seenDeliveries.ids, k)
		}
	}
	seenDeliveries.ids[id] = now
	return false
}

// VerifySignature checks GitHub's X-Hub-Signature-256 header: an HMAC-SHA256
// of the raw request body, hex-encoded with a "sha256=" prefix.
func VerifySignature(secret, body []byte, signatureHeader string) bool {
	if secret == nil || signatureHeader == "" {
		return false
	}
	signatureHeader = strings.TrimPrefix(signatureHeader, "sha256=")
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(signatureHeader)
	if err != nil {
		return false
	}
	return hmac.Equal(got, expected)
}

// ignoredActions lists delivery actions that never change the mirrored state.
var ignoredActions = map[string]bool{
	"assigned": true, "unassigned": true,
	"labeled": true, "unlabeled": true, // labels arrive via the issue body
	"milestoned": true, "demilestoned": true,
	"review_requested": true, "review_request_removed": true,
	"auto_merge_enabled": true, "auto_merge_disabled": true,
	"queued": true, "converted_to_draft": true, "ready_for_review": true,
	"synchronize": true, // code pushes; the PR metadata we mirror is unchanged
	"enqueued":    true, "dequeued": true,
}

// HandleWebhook processes an incoming GitHub webhook delivery: it verifies the
// signature, finds the connection the delivery belongs to and re-syncs the
// affected item through the regular API path.
//
// The whole handling is idempotent — content-hash comparisons make replays
// and re-deliveries no-ops.
func HandleWebhook(ctx context.Context, event string, deliveryID, signature string, body []byte) error {
	// issue_comment deliveries carry no number we could sync (comments aren't
	// mirrored); accept them so GitHub stops retrying, but do nothing.
	if event == "issue_comment" {
		return nil
	}
	if event != "issues" && event != "pull_request" {
		return nil
	}

	payload := &webhookPayload{}
	if err := json.Unmarshal(body, payload); err != nil {
		return err
	}
	if payload.Repository == nil || payload.Repository.Owner == nil || ignoredActions[payload.Action] {
		return nil
	}

	if deliverySeen(deliveryID) {
		return nil
	}

	conn, err := func() (*models.GitHubConnection, error) {
		s := db.NewSession()
		defer func() {
			_ = s.Close()
		}()
		return findConnectionForDelivery(s, payload)
	}()
	if err != nil {
		return err
	}
	if conn == nil {
		// A delivery for a repo we don't mirror (e.g. webhook removal raced a
		// pending delivery). Acknowledge so GitHub stops retrying.
		log.Debugf("github: webhook delivery for unconnected repo, ignoring")
		return nil
	}

	if !signatureValid(conn, body, signature) {
		return ErrInvalidSignature{}
	}

	stats, syncErr := func() (*SyncStats, error) {
		s := db.NewSession()
		defer func() {
			_ = s.Close()
		}()
		stats, err := SyncIssue(ctx, s, NewClientForConnection(conn), conn, payload.Number)
		if err != nil {
			// Roll back so a half-synced delivery doesn't partially apply.
			_ = s.Rollback()
			return stats, err
		}
		return stats, s.Commit()
	}()
	if syncErr != nil {
		// The sync session is closed by now, so recording the error on a
		// fresh session can't contend for SQLite's write lock.
		RecordSyncError(conn, syncErr)
		log.Errorf("github: webhook sync for %s#%d failed: %s", conn.RepoFullName(), payload.Number, syncErr)
		return nil // 200 so GitHub doesn't retry; the error is recorded on the connection
	}
	_ = stats
	return nil
}

func findConnectionForDelivery(s *xorm.Session, payload *webhookPayload) (*models.GitHubConnection, error) {
	conn := &models.GitHubConnection{}
	var has bool
	var err error
	if payload.Installation != nil && payload.Installation.ID > 0 {
		if payload.Repository == nil {
			return nil, errors.New("github: delivery without repository")
		}
		has, err = s.
			Where("installation_id = ? AND repo_owner = ? AND repo_name = ?",
				payload.Installation.ID, payload.Repository.Owner.Login, payload.Repository.Name).
			Get(conn)
	} else {
		has, err = s.
			Where("repo_owner = ? AND repo_name = ?",
				payload.Repository.Owner.Login, payload.Repository.Name).
			Get(conn)
	}
	if err != nil || !has {
		return nil, err
	}
	return conn, nil
}

// signatureValid checks the delivery against the connection's secret; GitHub
// App connections additionally accept the instance's current app secret, so a
// rotated config secret keeps existing installations working.
func signatureValid(conn *models.GitHubConnection, body []byte, signature string) bool {
	if VerifySignature([]byte(conn.WebhookSecret), body, signature) {
		return true
	}
	return conn.InstallationID > 0 &&
		VerifySignature([]byte(config.GithubWebhookSecret.GetString()), body, signature)
}

// ErrInvalidSignature is returned when a webhook delivery fails verification.
type ErrInvalidSignature struct{}

func (ErrInvalidSignature) Error() string {
	return "github: invalid webhook signature"
}

// IsErrInvalidSignature reports whether err is a signature-verification failure.
func IsErrInvalidSignature(err error) bool {
	var target ErrInvalidSignature
	return errors.As(err, &target)
}
