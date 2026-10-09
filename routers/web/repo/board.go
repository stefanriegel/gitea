// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"fmt"

	"gitea.dev/services/context"
	project_service "gitea.dev/services/projects"
)

// Board redirects to the repository's kanban board, or to its project list if it has none
func Board(ctx *context.Context) {
	board, err := project_service.FindBoard(ctx, ctx.Repo.Repository.ID)
	if err != nil {
		ctx.ServerError("FindBoard", err)
		return
	}
	if board == nil {
		ctx.Redirect(ctx.Repo.RepoLink + "/projects")
		return
	}
	ctx.Redirect(fmt.Sprintf("%s/projects/%d", ctx.Repo.RepoLink, board.ID))
}
