// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package actions

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

func (n *actionsNotifier) IssueChangeProjectColumn(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, project *project_model.Project, oldColumn, newColumn *project_model.Column) {
	ctx = withMethod(ctx, "IssueChangeProjectColumn")
	if err := issue.LoadRepo(ctx); err != nil {
		log.Error("LoadRepo: %v", err)
		return
	}
	permission, _ := access_model.GetDoerRepoPermission(ctx, issue.Repo, doer)
	// no WithPullRequest: like issue events, project_card runs on the default branch, also for pull requests
	newNotifyInput(issue.Repo, doer, webhook_module.HookEventProjectCard).
		WithPayload(convert.ToProjectCardPayload(ctx, doer, issue, project, oldColumn, newColumn, permission)).
		Notify(ctx)
}
