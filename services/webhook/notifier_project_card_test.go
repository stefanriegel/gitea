// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package webhook

import (
	"testing"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	project_model "gitea.dev/models/project"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	webhook_model "gitea.dev/models/webhook"
	"gitea.dev/modules/json"
	api "gitea.dev/modules/structs"
	webhook_module "gitea.dev/modules/webhook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueChangeProjectColumnWebhook(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	issue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: 1})
	project := unittest.AssertExistsAndLoadBean(t, &project_model.Project{ID: 1})
	from := unittest.AssertExistsAndLoadBean(t, &project_model.Column{ID: 1})
	to := unittest.AssertExistsAndLoadBean(t, &project_model.Column{ID: 2})

	giteaHook := &webhook_model.Webhook{RepoID: issue.RepoID, Type: webhook_module.GITEA, URL: "http://localhost/project-card", ContentType: webhook_model.ContentTypeJSON, Events: `{"choose_events":true,"events":{"project_card":true}}`, IsActive: true}
	slackHook := &webhook_model.Webhook{RepoID: issue.RepoID, Type: webhook_module.SLACK, URL: "http://localhost/project-card-slack", ContentType: webhook_model.ContentTypeJSON, Events: `{"send_everything":true}`, IsActive: true}
	require.NoError(t, db.Insert(t.Context(), giteaHook, slackHook))

	(&webhookNotifier{}).IssueChangeProjectColumn(t.Context(), doer, issue, project, from, to)

	task := unittest.AssertExistsAndLoadBean(t, &webhook_model.HookTask{HookID: giteaHook.ID, EventType: webhook_module.HookEventProjectCard})
	var payload api.ProjectCardPayload
	require.NoError(t, json.Unmarshal([]byte(task.PayloadContent), &payload))
	assert.Equal(t, api.HookProjectCardMoved, payload.Action)
	assert.Equal(t, issue.ID, payload.Issue.ID)
	assert.Equal(t, &api.ProjectCardRef{ID: 1, Title: project.Title}, payload.Project)
	assert.Equal(t, &api.ProjectCardRef{ID: 2, Title: to.Title}, payload.Column)
	assert.Equal(t, &api.ProjectCardRef{ID: 1, Title: from.Title}, payload.FromColumn)
	assert.Equal(t, doer.ID, payload.Sender.ID)
	unittest.AssertNotExistsBean(t, &webhook_model.HookTask{HookID: slackHook.ID})
}
