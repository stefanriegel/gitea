// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import "gitea.dev/modules/json"

// HookProjectCardAction is the action of a project_card event
type HookProjectCardAction string

const (
	// HookProjectCardCreated is sent when an issue is added to a project
	HookProjectCardCreated HookProjectCardAction = "created"
	// HookProjectCardMoved is sent when an issue moves to another column of a project
	HookProjectCardMoved HookProjectCardAction = "moved"
	// HookProjectCardDeleted is sent when an issue is removed from a project
	HookProjectCardDeleted HookProjectCardAction = "deleted"
)

// ProjectCardRef identifies a project or a project column
type ProjectCardRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// ProjectCardPayload represents a project_card event: an issue or pull request added to,
// moved within or removed from a project board
type ProjectCardPayload struct {
	Action HookProjectCardAction `json:"action"`
	// The issue or pull request on the board
	Issue   *Issue          `json:"issue"`
	Project *ProjectCardRef `json:"project"`
	// The column the card is in now, or was in before it was deleted
	Column *ProjectCardRef `json:"column"`
	// The column the card was moved from, only for "moved"
	FromColumn *ProjectCardRef `json:"from_column,omitempty"`
	Repository *Repository     `json:"repository"`
	Sender     *User           `json:"sender"`
}

// JSONPayload implements Payloader
func (p *ProjectCardPayload) JSONPayload() ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}
