// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package convert

import (
	"context"

	issues_model "gitea.dev/models/issues"
	access_model "gitea.dev/models/perm/access"
	project_model "gitea.dev/models/project"
	user_model "gitea.dev/models/user"
	api "gitea.dev/modules/structs"
)

func toProjectCardColumn(c *project_model.Column) *api.ProjectCardRef {
	if c == nil {
		return nil
	}
	return &api.ProjectCardRef{ID: c.ID, Title: c.Title}
}

// ToProjectCardPayload builds the project_card payload, issue.Repo must be loaded
func ToProjectCardPayload(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, project *project_model.Project, oldColumn, newColumn *project_model.Column, perm access_model.Permission) *api.ProjectCardPayload {
	p := &api.ProjectCardPayload{
		Issue:      ToAPIIssue(ctx, doer, issue),
		Project:    &api.ProjectCardRef{ID: project.ID, Title: project.Title},
		Repository: ToRepo(ctx, issue.Repo, perm),
		Sender:     ToUser(ctx, doer, nil),
	}
	switch {
	case oldColumn == nil:
		p.Action, p.Column = api.HookProjectCardCreated, toProjectCardColumn(newColumn)
	case newColumn == nil:
		p.Action, p.Column = api.HookProjectCardDeleted, toProjectCardColumn(oldColumn)
	default:
		p.Action, p.Column, p.FromColumn = api.HookProjectCardMoved, toProjectCardColumn(newColumn), toProjectCardColumn(oldColumn)
	}
	return p
}
