// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package actions

import (
	"slices"

	"gitea.dev/modules/actions/jobparser"
	"gitea.dev/modules/log"
	api "gitea.dev/modules/structs"
)

func matchProjectCardEvent(payload *api.ProjectCardPayload, evt *jobparser.Event) bool {
	matchTimes := 0
	for cond, vals := range evt.Acts() {
		switch cond {
		case "types":
			if slices.Contains(vals, string(payload.Action)) {
				matchTimes++
			}
		default:
			log.Warn("project_card event unsupported condition %q", cond)
		}
	}
	return matchTimes == len(evt.Acts())
}
