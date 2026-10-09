// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package actions

import (
	"testing"

	api "gitea.dev/modules/structs"
	webhook_module "gitea.dev/modules/webhook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectMatchedProjectCard(t *testing.T) {
	for yamlOn, expected := range map[string]detectResult{
		"on: project_card":                           detectMatched,
		"on:\n  project_card:\n    types: [moved]":   detectMatched,
		"on:\n  project_card:\n    types: [created]": detectNotApplicable,
		"on: issues": detectNotApplicable,
	} {
		evts, err := GetEventsFromContent(fullWorkflowContent(yamlOn))
		require.NoError(t, err)
		require.Len(t, evts, 1)
		payload := &api.ProjectCardPayload{Action: api.HookProjectCardMoved}
		assert.Equal(t, expected, detectWorkflowMatch(t.Context(), nil, nil, webhook_module.HookEventProjectCard, payload, evts[0]), yamlOn)
	}
	assert.True(t, IsDefaultBranchWorkflow(webhook_module.HookEventProjectCard))
}
