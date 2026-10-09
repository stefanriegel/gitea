// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package project

import (
	"context"

	"gitea.dev/models/db"
	project_model "gitea.dev/models/project"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/optional"
	notify_service "gitea.dev/services/notify"
)

// BoardTitle is the title of the kanban project every new repository gets
const BoardTitle = "Board"

type boardNotifier struct {
	notify_service.NullNotifier
}

// Init registers the notifier that gives new repositories a board
func Init() error {
	notify_service.RegisterNotifier(&boardNotifier{})
	return nil
}

func (*boardNotifier) CreateRepository(ctx context.Context, doer, _ *user_model.User, repo *repo_model.Repository) {
	createBoard(ctx, doer, repo)
}

func (*boardNotifier) MigrateRepository(ctx context.Context, doer, _ *user_model.User, repo *repo_model.Repository) {
	createBoard(ctx, doer, repo)
}

func (*boardNotifier) ForkRepository(ctx context.Context, doer *user_model.User, _, repo *repo_model.Repository) {
	createBoard(ctx, doer, repo)
}

// createBoard adds a "Basic Kanban" project, whose columns come from [project] PROJECT_BOARD_BASIC_KANBAN_TYPE
func createBoard(ctx context.Context, doer *user_model.User, repo *repo_model.Repository) {
	if doer == nil || !repo.UnitEnabled(ctx, unit.TypeProjects) {
		return
	}
	if err := project_model.NewProject(ctx, &project_model.Project{
		Title:        BoardTitle,
		RepoID:       repo.ID,
		CreatorID:    doer.ID,
		TemplateType: project_model.TemplateTypeBasicKanban,
		CardType:     project_model.CardTypeTextOnly,
		Type:         project_model.TypeRepository,
	}); err != nil {
		log.Error("create board for repo %d: %v", repo.ID, err)
	}
}

// FindBoard returns the repository's open project titled BoardTitle, else its oldest open project, else nil
func FindBoard(ctx context.Context, repoID int64) (*project_model.Project, error) {
	projects, err := db.Find[project_model.Project](ctx, project_model.SearchOptions{
		RepoID:   repoID,
		IsClosed: optional.Some(false),
		OrderBy:  db.SearchOrderByOldest,
	})
	if err != nil || len(projects) == 0 {
		return nil, err
	}
	for _, p := range projects {
		if p.Title == BoardTitle {
			return p, nil
		}
	}
	return projects[0], nil
}
