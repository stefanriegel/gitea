// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package notify

import (
	"context"

	issues_model "gitea.dev/models/issues"
	project_model "gitea.dev/models/project"
	user_model "gitea.dev/models/user"
)

// ProjectCardNotifier is notified when an issue is added to (oldColumn nil), moved within,
// or removed from (newColumn nil) a project
type ProjectCardNotifier interface {
	IssueChangeProjectColumn(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, project *project_model.Project, oldColumn, newColumn *project_model.Column)
}

func (*NullNotifier) IssueChangeProjectColumn(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, project *project_model.Project, oldColumn, newColumn *project_model.Column) {
}

// IssueChangeProjectColumn notifies a project card change to notifiers
func IssueChangeProjectColumn(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, project *project_model.Project, oldColumn, newColumn *project_model.Column) {
	for _, notifier := range notifiers {
		notifier.IssueChangeProjectColumn(ctx, doer, issue, project, oldColumn, newColumn)
	}
}
