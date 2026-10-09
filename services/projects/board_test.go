// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package project

import (
	"testing"

	"gitea.dev/models/db"
	project_model "gitea.dev/models/project"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAndFindBoard(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	defer test.MockVariableValue(&setting.Project.ProjectBoardBasicKanbanType, []string{"Agent", "Done"})()

	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	before, err := FindBoard(t.Context(), repo.ID)
	require.NoError(t, err)
	require.NotNil(t, before) // fixtures already have open projects
	assert.NotEqual(t, BoardTitle, before.Title)

	createBoard(t.Context(), doer, repo)

	board, err := FindBoard(t.Context(), repo.ID)
	require.NoError(t, err)
	assert.Equal(t, BoardTitle, board.Title)
	columns, err := project_model.GetColumns(t.Context(), board.ID, db.ListOptionsAll)
	require.NoError(t, err)
	titles := make([]string, 0, len(columns))
	for _, c := range columns {
		titles = append(titles, c.Title)
	}
	assert.Equal(t, []string{"Backlog", "Agent", "Done"}, titles)
	assert.True(t, columns[0].Default)

	none, err := FindBoard(t.Context(), 999999)
	require.NoError(t, err)
	assert.Nil(t, none)
}
