// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package org

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestViewAgentAccount(t *testing.T) {
	now := int64(1_000_000)
	base := agentAccount{ID: "claude-a", Enabled: true, LoggedIn: true, Usage: map[string]agentWindow{
		"five_hour": {Utilization: 0.42, ResetsAt: now + 60},
		"seven_day": {Utilization: 0.95, ResetsAt: now - 1}, // window already reset
	}}
	v := viewAccount(base, now)
	assert.Equal(t, 42, v.FiveHour)
	assert.Equal(t, 0, v.Week)
	assert.Equal(t, "bereit", v.State)

	a := base
	a.LoggedIn = false
	assert.Equal(t, "nicht angemeldet", viewAccount(a, now).State)
	a = base
	a.LimitedUntil = now + 600
	assert.Equal(t, "orange", viewAccount(a, now).StateColor)
	a = base
	a.Enabled = false
	assert.Equal(t, "pausiert", viewAccount(a, now).State)
	a = base
	a.Usage = map[string]agentWindow{"seven_day": {Utilization: 0.93, ResetsAt: now + 100}}
	assert.Equal(t, "fast voll", viewAccount(a, now).State)
	a = base
	a.Runs = []agentRun{{Repo: "riegel/x", Issue: 1}, {Repo: "riegel/y", Issue: 2}}
	assert.Equal(t, "arbeitet", viewAccount(a, now).State)
}
