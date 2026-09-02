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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/cron"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/utils"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

// backfills tracks in-flight initial backfills so tests can wait for them
// (production code never needs to — reconciliation catches up).
var backfills sync.WaitGroup

// WaitForBackfills blocks until every in-flight initial backfill has settled.
// Exclusively for tests: the HTTP request that starts a backfill returns
// before it finishes, and a test carrying on while one is mid-write races the
// next subtest's reads on SQLite.
func WaitForBackfills() {
	backfills.Wait()
}

// Connect validates the credentials against the repo, registers a webhook
// where possible and persists the connection. The initial backfill runs in the
// background so the request returns fast; its progress/failure is visible via
// the connection's sync fields.
func Connect(ctx context.Context, s *xorm.Session, a web.Auth, projectID int64, installationID int64, token, repoOwner, repoName string) (*models.GitHubConnection, error) {
	if (installationID > 0) == (token != "") {
		return nil, models.ErrGitHubInvalidCredentials{}
	}

	repoOwner = strings.TrimSpace(repoOwner)
	repoName = strings.TrimSpace(repoName)
	if strings.Contains(repoOwner, "/") || strings.Contains(repoName, "/") {
		return nil, models.ErrGitHubInvalidCredentials{}
	}

	client := NewClientForConnection(&models.GitHubConnection{
		InstallationID: installationID,
		Token:          token,
	})
	repo, err := client.GetRepo(ctx, repoOwner, repoName)
	if err != nil {
		return nil, translateClientError(err, repoOwner+"/"+repoName)
	}

	// Repo casing on GitHub is canonical; links and connection rows compare
	// owner/name strings, so store the canonical casing.
	repoOwner, repoName = splitFullName(repo.FullName)

	exists, err := s.
		Where("project_id = ? AND repo_owner = ? AND repo_name = ?", projectID, repoOwner, repoName).
		Exist(&models.GitHubConnection{})
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, models.ErrGitHubRepoAlreadyConnected{Repo: repo.FullName, ProjectID: projectID}
	}

	conn := &models.GitHubConnection{
		ProjectID:      projectID,
		RepoOwner:      repoOwner,
		RepoName:       repoName,
		InstallationID: installationID,
		Token:          token,
		Enabled:        true,
	}

	if installationID > 0 {
		// App deliveries are signed with the App's configured secret.
		conn.WebhookSecret = config.GithubWebhookSecret.GetString()
	} else {
		secret, err := utils.CryptoRandomString(32)
		if err != nil {
			return nil, err
		}
		conn.WebhookSecret = secret
	}

	if err := conn.Create(s, a); err != nil {
		return nil, err
	}

	if installationID == 0 {
		// PAT connections get a repo webhook pushed to GitHub so updates
		// arrive immediately instead of waiting for reconciliation. Best
		// effort: tokens without repo-admin rights can't create hooks, and
		// instances without a public URL can't receive any — the connection
		// still works, just reconcile-only.
		if hookURL := webhookTargetURL(); hookURL != "" {
			hookID, err := client.CreateRepoWebhook(ctx, repoOwner, repoName, hookURL, conn.WebhookSecret)
			if err != nil {
				log.Infof("github: could not create repo webhook for %s (%s); falling back to periodic reconciliation only", repo.FullName, err)
			} else {
				if _, err := s.ID(conn.ID).Cols("repo_webhook_id").Update(&models.GitHubConnection{RepoWebhookID: hookID}); err != nil {
					return nil, err
				}
			}
		} else {
			log.Infof("github: no public URL configured, %s will sync periodically only", repo.FullName)
		}
	}

	if err := s.Commit(); err != nil {
		return nil, err
	}

	// The struct handed to the caller lost its credentials in Create; the
	// backfill goroutine reloads the row WITH credentials (masking there would
	// authenticate against GitHub with an empty token).
	backfills.Add(1)
	go func() {
		defer backfills.Done()
		// Detached from the request: the response has long returned by the
		// time the backfill finishes, and request cancellation must not kill
		// it half-way.
		backfillCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
		defer cancel()
		bs := db.NewSession()
		full, err := models.GetGitHubConnectionByIDWithCredentials(bs, conn.ID)
		if err != nil {
			_ = bs.Close()
			log.Errorf("github: reloading connection %d for backfill: %s", conn.ID, err)
			return
		}
		if _, err := SyncConnection(backfillCtx, bs, full); err != nil {
			// Roll back and close before recording: RecordSyncError opens its
			// own session, and two open writers deadlock SQLite.
			_ = bs.Rollback()
			_ = bs.Close()
			RecordSyncError(full, err)
			log.Errorf("github: initial backfill of %s failed: %s", full.RepoFullName(), err)
			return
		}
		if err := bs.Commit(); err != nil {
			log.Errorf("github: committing backfill of %s: %s", full.RepoFullName(), err)
		}
		_ = bs.Close()
	}()

	return conn, nil
}

// Disconnect removes a connection. Tasks and their links are deliberately
// kept: disconnecting pauses mirroring, it doesn't rewrite history, and
// keeping links lets a future re-connection resume without duplicates.
func Disconnect(ctx context.Context, s *xorm.Session, conn *models.GitHubConnection) error {
	if conn.RepoWebhookID > 0 {
		client := NewClientForConnection(conn)
		if err := client.DeleteRepoWebhook(ctx, conn.RepoOwner, conn.RepoName, conn.RepoWebhookID); err != nil {
			// The hook may already be gone (deleted repo, revoked token) —
			// removing the connection must not fail because of it.
			log.Infof("github: could not delete repo webhook %d of %s: %s", conn.RepoWebhookID, conn.RepoFullName(), err)
		}
	}
	return conn.Delete(s, nil)
}

// SyncNow runs a full reconciliation of one connection within the given
// session and commits it.
func SyncNow(ctx context.Context, s *xorm.Session, conn *models.GitHubConnection) (*SyncStats, error) {
	stats, err := SyncConnection(ctx, s, conn)
	if err != nil {
		RecordSyncError(conn, err)
		_ = s.Rollback()
		return stats, err
	}
	return stats, s.Commit()
}

func splitFullName(fullName string) (owner, name string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return fullName, ""
	}
	return parts[0], parts[1]
}

// webhookTargetURL returns the URL GitHub should deliver webhooks to, empty
// when the instance has no externally reachable address.
func webhookTargetURL() string {
	if u := config.GithubWebhookURL.GetString(); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	base := strings.TrimSuffix(config.ServicePublicURL.GetString(), "/")
	if base == "" {
		return ""
	}
	return base + "/api/v2/integrations/github/webhook"
}

func translateClientError(err error, repo string) error {
	switch {
	case errors.Is(err, errNotFound):
		return models.ErrGitHubRepoNotFound{Repo: repo}
	case errors.Is(err, errUnauthorized):
		return models.ErrGitHubInvalidCredentials{}
	default:
		return fmt.Errorf("github: checking repository %s: %w", repo, err)
	}
}

// RegisterReconcileCron registers the periodic reconciliation that repairs
// anything webhooks missed (downtime, dropped deliveries, rate limits).
func RegisterReconcileCron() {
	if !config.GithubEnabled.GetBool() {
		return
	}
	err := cron.Schedule("*/10 * * * *", reconcileAllConnections)
	if err != nil {
		log.Fatalf("Could not register GitHub reconciliation cron: %s", err)
	}
}

func reconcileAllConnections() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	s := db.NewSession()
	defer func() {
		_ = s.Close()
	}()
	conns, err := models.GetGitHubConnectionsForSync(s)
	if err != nil {
		log.Errorf("github: loading connections for reconciliation: %s", err)
		return
	}
	for _, conn := range conns {
		stats, err := SyncConnection(ctx, s, conn)
		if err != nil {
			RecordSyncError(conn, err)
			log.Errorf("github: reconciling %s: %s", conn.RepoFullName(), err)
			_ = s.Rollback()
			continue
		}
		if err := s.Commit(); err != nil {
			log.Errorf("github: committing reconciliation of %s: %s", conn.RepoFullName(), err)
		}
		if stats != nil && (stats.Created > 0 || stats.Updated > 0) {
			log.Debugf("github: reconciled %s: %d created, %d updated", conn.RepoFullName(), stats.Created, stats.Updated)
		}
	}
}
