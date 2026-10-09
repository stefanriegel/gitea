// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/url"
	"strings"
	"testing"

	actions_model "gitea.dev/models/actions"
	issues_model "gitea.dev/models/issues"
	project_model "gitea.dev/models/project"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/optional"
	webhook_module "gitea.dev/modules/webhook"
	issue_service "gitea.dev/services/issue"
	project_service "gitea.dev/services/projects"
	repo_service "gitea.dev/services/repository"
	files_service "gitea.dev/services/repository/files"

	"github.com/stretchr/testify/require"
)

func TestActionsProjectCardEvent(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo, err := repo_service.CreateRepository(t.Context(), user2, user2, repo_service.CreateRepoOptions{Name: "actions-project-card", AutoInit: true, Readme: "Default", DefaultBranch: "main"})
		require.NoError(t, err)
		_, err = files_service.ChangeRepoFiles(t.Context(), repo, user2, &files_service.ChangeRepoFilesOptions{
			Files: []*files_service.ChangeRepoFile{{
				Operation: "create",
				TreePath:  ".gitea/workflows/agent.yml",
				ContentReader: strings.NewReader(`on:
  project_card:
    types: [moved]
run-name: card moved to ${{ github.event.column.title }} from ${{ github.event.from_column.title }}
jobs:
  agent:
    if: github.event.column.title == 'Agent'
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ github.event.issue.number }}
`),
			}},
			OldBranch: "main",
			NewBranch: "main",
			Message:   "add workflow",
		})
		require.NoError(t, err)

		project := &project_model.Project{Title: "Board", RepoID: repo.ID, Type: project_model.TypeRepository, TemplateType: project_model.TemplateTypeBasicKanban, CreatorID: user2.ID}
		require.NoError(t, project_model.NewProject(t.Context(), project))
		defaultColumn, err := project.MustDefaultColumn(t.Context())
		require.NoError(t, err)
		agent := &project_model.Column{Title: "Agent", ProjectID: project.ID, CreatorID: user2.ID}
		require.NoError(t, project_model.NewColumn(t.Context(), agent))

		issue := &issues_model.Issue{RepoID: repo.ID, Repo: repo, Title: "do it", PosterID: user2.ID, Poster: user2}
		require.NoError(t, issue_service.NewIssue(t.Context(), repo, issue, nil, nil, nil, []int64{project.ID}))
		unittest.AssertCount(t, &actions_model.ActionRun{RepoID: repo.ID}, 0) // "created" is filtered out by types

		require.NoError(t, project_service.MoveIssueToColumn(t.Context(), user2, issue, agent, optional.None[int64]()))
		unittest.AssertExistsAndLoadBean(t, &actions_model.ActionRun{
			RepoID:       repo.ID,
			Event:        webhook_module.HookEventProjectCard,
			TriggerEvent: "project_card",
			Title:        "card moved to Agent from " + defaultColumn.Title,
			Ref:          "refs/heads/main",
		})
	})
}
