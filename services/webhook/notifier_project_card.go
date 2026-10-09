// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package webhook

import (
	"context"

	issues_model "gitea.dev/models/issues"
	access_model "gitea.dev/models/perm/access"
	project_model "gitea.dev/models/project"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	webhook_module "gitea.dev/modules/webhook"
	"gitea.dev/services/convert"
)

func (m *webhookNotifier) IssueChangeProjectColumn(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, project *project_model.Project, oldColumn, newColumn *project_model.Column) {
	if err := issue.LoadRepo(ctx); err != nil {
		log.Error("LoadRepo: %v", err)
		return
	}
	permission, _ := access_model.GetDoerRepoPermission(ctx, issue.Repo, doer)
	payload := convert.ToProjectCardPayload(ctx, doer, issue, project, oldColumn, newColumn, permission)
	if err := PrepareWebhooks(ctx, EventSource{Repository: issue.Repo}, webhook_module.HookEventProjectCard, payload); err != nil {
		log.Error("PrepareWebhooks: %v", err)
	}
}
