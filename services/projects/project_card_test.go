// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package project

import (
	"context"
	"testing"

	issues_model "gitea.dev/models/issues"
	project_model "gitea.dev/models/project"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/optional"
	notify_service "gitea.dev/services/notify"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type projectCardRecorder struct {
	notify_service.NullNotifier
	events []string
}

func (r *projectCardRecorder) IssueChangeProjectColumn(_ context.Context, _ *user_model.User, issue *issues_model.Issue, _ *project_model.Project, oldColumn, newColumn *project_model.Column) {
	title := func(c *project_model.Column) string {
		if c == nil {
			return "-"
		}
		return c.Title
	}
	r.events = append(r.events, title(oldColumn)+">"+title(newColumn))
}

func TestProjectCardNotifications(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	recorder := &projectCardRecorder{}
	notify_service.RegisterNotifier(recorder)

	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	issue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: 1}) // in project 1, column 1 "To Do"
	toDo := unittest.AssertExistsAndLoadBean(t, &project_model.Column{ID: 1})
	inProgress := unittest.AssertExistsAndLoadBean(t, &project_model.Column{ID: 2})

	expect := func(events ...string) {
		t.Helper()
		assert.Equal(t, events, recorder.events)
		recorder.events = nil
	}

	require.NoError(t, MoveIssueToColumn(t.Context(), doer, issue, inProgress, optional.None[int64]()))
	expect("To Do>In Progress")

	require.NoError(t, MoveIssuesOnProjectColumn(t.Context(), doer, inProgress, map[int64]int64{0: issue.ID}))
	expect() // same column, only sorting

	require.NoError(t, RemoveIssueFromColumn(t.Context(), doer, issue, inProgress))
	expect("In Progress>-")

	require.NoError(t, AddIssueToColumn(t.Context(), doer, issue, inProgress))
	expect("->In Progress") // one event with the final column, not the default column it passed through

	require.NoError(t, AssignOrRemoveIssueProjects(t.Context(), issue, doer, nil))
	expect("In Progress>-")

	require.NoError(t, AssignOrRemoveIssueProjects(t.Context(), issue, doer, []int64{toDo.ProjectID}))
	expect("->To Do")

	require.NoError(t, MoveIssuesOnProjectColumn(t.Context(), doer, toDo, map[int64]int64{0: 2}))
	expect() // issue 2 has the legacy "no column" row, which already means the default column

	assert.ErrorIs(t, MoveIssuesOnProjectColumn(t.Context(), doer, inProgress, map[int64]int64{0: 2, 1: 4}), ErrIssueNotInProject)
	expect() // failed, issue 4 isn't in the project
}
