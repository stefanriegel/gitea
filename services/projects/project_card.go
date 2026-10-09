// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package project

import (
	"context"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	project_model "gitea.dev/models/project"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/log"
	notify_service "gitea.dev/services/notify"
)

// issue ID -> project ID -> column ID (0 means the default column)
type issueProjectColumns map[int64]map[int64]int64

func loadIssueProjectColumns(ctx context.Context, issueIDs []int64) (issueProjectColumns, error) {
	var rows []*project_model.ProjectIssue
	if err := db.GetEngine(ctx).In("issue_id", issueIDs).Find(&rows); err != nil {
		return nil, err
	}
	res := make(issueProjectColumns, len(issueIDs))
	for _, row := range rows {
		if res[row.IssueID] == nil {
			res[row.IssueID] = map[int64]int64{}
		}
		res[row.IssueID][row.ProjectID] = row.ProjectColumnID
	}
	return res, nil
}

// notifyProjectCardAfter snapshots the issues' project columns and returns a func to defer with the
// caller's named error, which notifies the changes once the caller's transaction has committed.
// Inside a transaction it does nothing: the outermost caller notifies after its commit.
func notifyProjectCardAfter(ctx context.Context, doer *user_model.User, issueIDs ...int64) func(*error) {
	if db.InTransaction(ctx) {
		return func(*error) {}
	}
	before, err := loadIssueProjectColumns(ctx, issueIDs)
	if err != nil {
		log.Error("loadIssueProjectColumns: %v", err)
		return func(*error) {}
	}
	return func(errPtr *error) {
		if *errPtr == nil {
			notifyProjectCardChanges(ctx, doer, issueIDs, before)
		}
	}
}

// AssignOrRemoveIssueProjects is issues_model.IssueAssignOrRemoveProject with project_card notifications
func AssignOrRemoveIssueProjects(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, newProjectIDs []int64) (err error) {
	defer notifyProjectCardAfter(ctx, doer, issue.ID)(&err)
	return issues_model.IssueAssignOrRemoveProject(ctx, issue, doer, newProjectIDs)
}

// NotifyNewIssueProjects notifies the projects a newly created issue was added to, after its transaction committed
func NotifyNewIssueProjects(ctx context.Context, doer *user_model.User, issue *issues_model.Issue) {
	notifyProjectCardChanges(ctx, doer, []int64{issue.ID}, nil)
}

func notifyProjectCardChanges(ctx context.Context, doer *user_model.User, issueIDs []int64, before issueProjectColumns) {
	after, err := loadIssueProjectColumns(ctx, issueIDs)
	if err != nil {
		log.Error("loadIssueProjectColumns: %v", err)
		return
	}
	for issueID := range container.SetOf(issueIDs...) {
		projectIDs := container.Set[int64]{}
		for projectID := range before[issueID] {
			projectIDs.Add(projectID)
		}
		for projectID := range after[issueID] {
			projectIDs.Add(projectID)
		}
		var issue *issues_model.Issue
		for projectID := range projectIDs {
			oldColumnID, inBefore := before[issueID][projectID]
			newColumnID, inAfter := after[issueID][projectID]
			if inBefore && inAfter && oldColumnID == newColumnID {
				continue
			}
			if issue == nil {
				if issue, err = issues_model.GetIssueByID(ctx, issueID); err != nil {
					log.Error("GetIssueByID: %v", err)
					break
				}
			}
			if err := notifyProjectCardChange(ctx, doer, issue, projectID, before[issueID], after[issueID]); err != nil {
				log.Error("notifyProjectCardChange for issue %d in project %d: %v", issueID, projectID, err)
			}
		}
	}
}

func notifyProjectCardChange(ctx context.Context, doer *user_model.User, issue *issues_model.Issue, projectID int64, before, after map[int64]int64) error {
	project, err := project_model.GetProjectByID(ctx, projectID)
	if err != nil {
		return err
	}
	getColumn := func(columnID int64) (*project_model.Column, error) {
		if columnID == 0 {
			return project.MustDefaultColumn(ctx)
		}
		return project_model.GetColumn(ctx, columnID)
	}
	var oldColumn, newColumn *project_model.Column
	if columnID, ok := before[projectID]; ok {
		if oldColumn, err = getColumn(columnID); err != nil {
			return err
		}
	}
	if columnID, ok := after[projectID]; ok {
		if newColumn, err = getColumn(columnID); err != nil {
			return err
		}
	}
	if oldColumn != nil && newColumn != nil && oldColumn.ID == newColumn.ID {
		return nil // a legacy "no column" row resolved to the default column
	}
	notify_service.IssueChangeProjectColumn(ctx, doer, issue, project, oldColumn, newColumn)
	return nil
}
