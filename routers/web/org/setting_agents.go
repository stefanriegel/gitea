// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package org

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"gitea.dev/modules/setting"
	"gitea.dev/modules/templates"
	shared_user "gitea.dev/routers/web/shared/user"
	"gitea.dev/services/context"
)

const tplSettingsAgents templates.TplName = "org/settings/agents"

// The "Agent-Konten" page is a thin UI over the agent scheduler, a separate service configured in
// [riegel] AGENT_SCHEDULER_URL / AGENT_SCHEDULER_TOKEN. It owns accounts, logins, usage and the queue.

func agentScheduler() (string, string) {
	sec := setting.CfgProvider.Section("riegel")
	return sec.Key("AGENT_SCHEDULER_URL").String(), sec.Key("AGENT_SCHEDULER_TOKEN").String()
}

var agentClient = &http.Client{Timeout: 40 * time.Second}

func callAgentScheduler(method, path string, body, out any) error {
	base, token := agentScheduler()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+"/api"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := agentClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("%s", e.Error)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type agentWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    int64   `json:"resets_at"`
}

type agentTicket struct {
	Repo      string   `json:"repo"`
	Number    int64    `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Mode      string   `json:"mode"`
	BlockedBy []string `json:"blocked_by"`
}

type agentAccount struct {
	ID           string                 `json:"id"`
	Engine       string                 `json:"engine"`
	DisplayName  string                 `json:"display_name"`
	BotLogin     string                 `json:"bot_login"`
	Enabled      bool                   `json:"enabled"`
	LoggedIn     bool                   `json:"logged_in"`
	LoginError   string                 `json:"login_error"`
	LimitedUntil int64                  `json:"limited_until"`
	Usage        map[string]agentWindow `json:"usage"`
	UsageAt      int64                  `json:"usage_at"`
	Running      *struct {
		Repo  string `json:"repo"`
		Issue int64  `json:"issue"`
		Since int64  `json:"since"`
	} `json:"running"`
	Login *struct {
		URL   string `json:"url"`
		Code  string `json:"code"`
		State string `json:"state"`
	} `json:"login"`
}

type agentStatus struct {
	Accounts []agentAccount `json:"accounts"`
	Next     []agentTicket  `json:"next"`
	Blocked  []agentTicket  `json:"blocked"`
	Updated  int64          `json:"updated"`
	Error    string         `json:"error"`
}

// agentAccountView is what the template shows: everything pre-digested, German, at a glance
type agentAccountView struct {
	agentAccount
	State, StateColor string
	Measured          bool
	HasFiveHour       bool
	HasWeek           bool
	FiveHour, Week    int
	FiveHourReset     string
	WeekReset         string
	RunningLink       string
}

func percent(w agentWindow, now int64) int {
	if w.ResetsAt <= now {
		return 0
	}
	return int(w.Utilization*100 + 0.5)
}

func clock(ts int64) string {
	if ts == 0 {
		return ""
	}
	t := time.Unix(ts, 0).Local()
	if t.Sub(time.Now()) > 20*time.Hour {
		return t.Format("02.01. 15:04")
	}
	return t.Format("15:04")
}

// resetText says when a window starts again at 0 %: "um 15:04", "morgen um 03:00", "am 13.10. um 03:00"
func resetText(ts, now int64) string {
	if ts <= now {
		return ""
	}
	t, n := time.Unix(ts, 0).Local(), time.Unix(now, 0).Local()
	switch {
	case t.Format("2006-01-02") == n.Format("2006-01-02"):
		return t.Format("um 15:04")
	case t.Format("2006-01-02") == n.AddDate(0, 0, 1).Format("2006-01-02"):
		return t.Format("morgen um 15:04")
	default:
		return t.Format("am 02.01. um 15:04")
	}
}

func viewAccount(a agentAccount, now int64) agentAccountView {
	v := agentAccountView{agentAccount: a}
	v.FiveHour, v.Week = percent(a.Usage["five_hour"], now), percent(a.Usage["seven_day"], now)
	v.Measured = a.UsageAt > 0
	_, v.HasFiveHour = a.Usage["five_hour"]
	_, v.HasWeek = a.Usage["seven_day"]
	v.FiveHourReset, v.WeekReset = resetText(a.Usage["five_hour"].ResetsAt, now), resetText(a.Usage["seven_day"].ResetsAt, now)
	switch {
	case !a.LoggedIn:
		v.State, v.StateColor = "nicht angemeldet", "red"
	case a.LoginError != "":
		v.State, v.StateColor = "neu anmelden", "red"
	case !a.Enabled:
		v.State, v.StateColor = "pausiert", "grey"
	case a.Running != nil:
		v.State, v.StateColor = "arbeitet", "blue"
		v.RunningLink = fmt.Sprintf("%s%s/issues/%d", setting.AppSubURL+"/", a.Running.Repo, a.Running.Issue)
	case a.LimitedUntil > now:
		v.State, v.StateColor = "Limit bis "+clock(a.LimitedUntil), "orange"
	case v.FiveHour >= 90 || v.Week >= 90:
		v.State, v.StateColor = "fast voll", "orange"
	default:
		v.State, v.StateColor = "bereit", "green"
	}
	return v
}

// AgentAccounts shows the subscription accounts the agents run on, their usage and the queue
func AgentAccounts(ctx *context.Context) {
	ctx.Data["Title"] = "Agent-Konten"
	ctx.Data["PageIsOrgSettings"] = true
	ctx.Data["PageIsSettingsAgents"] = true
	if _, err := shared_user.RenderUserOrgHeader(ctx); err != nil {
		ctx.ServerError("RenderUserOrgHeader", err)
		return
	}
	var st agentStatus
	if err := callAgentScheduler(http.MethodGet, "/status", nil, &st); err != nil {
		ctx.Data["SchedulerError"] = err.Error()
	}
	now := time.Now().Unix()
	views := make([]agentAccountView, 0, len(st.Accounts))
	for _, a := range st.Accounts {
		views = append(views, viewAccount(a, now))
	}
	ctx.Data["Accounts"] = views
	ctx.Data["Next"] = st.Next
	ctx.Data["Blocked"] = st.Blocked
	ctx.Data["Updated"] = clock(st.Updated)
	ctx.Data["LoopError"] = st.Error
	ctx.HTML(http.StatusOK, tplSettingsAgents)
}

// AgentAccountPost stores a Claude token, starts a Codex device login, or pauses/resumes an account
func AgentAccountPost(ctx *context.Context) {
	account, action := ctx.PathParam("account"), ctx.PathParam("action")
	back := ctx.ContextUser.OrganisationLink() + "/settings/agents"
	var body any
	switch action {
	case "claude-token":
		body = map[string]string{"token": ctx.FormString("token")}
	case "codex-login", "claude-login":
		body = map[string]string{}
	case "claude-code":
		body = map[string]string{"code": ctx.FormString("code")}
	case "enabled":
		body = map[string]bool{"enabled": ctx.FormBool("enabled")}
	default:
		ctx.NotFound(nil)
		return
	}
	if err := callAgentScheduler(http.MethodPost, "/accounts/"+url.PathEscape(account)+"/"+action, body, nil); err != nil {
		ctx.Flash.Error(err.Error())
	} else if action == "claude-token" || action == "claude-code" {
		ctx.Flash.Success("Angemeldet.")
	}
	ctx.Redirect(back)
}
