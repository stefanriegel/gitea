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
	"regexp"
	"slices"
	"strings"
	"time"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	org_model "gitea.dev/models/organization"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/optional"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/templates"
	"gitea.dev/modules/util"
	shared_user "gitea.dev/routers/web/shared/user"
	"gitea.dev/services/context"
	org_service "gitea.dev/services/org"
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

type agentRun struct {
	Repo  string `json:"repo"`
	Issue int64  `json:"issue"`
	Since int64  `json:"since"`
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
	Runs         []agentRun             `json:"runs"`
	Roles        []string               `json:"roles"`    // modes it takes, empty = all
	Priority     int                    `json:"priority"` // 1 is asked first
	Slots        int                    `json:"slots"`
	BaseURL      string                 `json:"base_url"` // engine "openai" only
	Models       map[string]string      `json:"models"`   // engine "openai": small, large
	Login        *struct {
		URL   string `json:"url"`
		Code  string `json:"code"`
		State string `json:"state"`
	} `json:"login"`
}

type agentUsageRow struct {
	Name   string  `json:"name"`
	Repo   string  `json:"repo"`
	Issue  int64   `json:"issue"`
	Runs   int     `json:"runs"`
	Tokens int64   `json:"tokens"`
	Cost   float64 `json:"cost"`
}

type agentUsage struct {
	Repos    []agentUsageRow `json:"repos"`
	Modes    []agentUsageRow `json:"modes"`
	Accounts []agentUsageRow `json:"accounts"`
	Top      []agentUsageRow `json:"top"`
	Total    agentUsageRow   `json:"total"`
}

type agentStatus struct {
	UsageWeek agentUsage     `json:"usage_week"`
	Accounts  []agentAccount `json:"accounts"`
	Next      []agentTicket  `json:"next"`
	Blocked   []agentTicket  `json:"blocked"`
	Updated   int64          `json:"updated"`
	Error     string         `json:"error"`
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
	HasRole           map[string]bool
	RoleText          string
}

// usageView is one table row as the page shows it: German labels, readable numbers
type usageView struct {
	Name, Link, Tokens, Cost string
	Runs                     int
}

var agentModeNames = map[string]string{"work": "Umsetzen (Implementierer)", "review": "Review", "triage": "Einsortieren (Triage)", "grill": "Klären (Planer)"}

// agentModes is the order the page lists the roles in
var agentModes = []string{"triage", "grill", "work", "review"}

func readableTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return strings.Replace(fmt.Sprintf("%.1f Mio.", float64(n)/1_000_000), ".", ",", 1)
	case n >= 1000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprint(n)
	}
}

func usageRows(rows []agentUsageRow, names map[string]string) []usageView {
	out := make([]usageView, 0, len(rows))
	for _, r := range rows {
		name := r.Name
		if n, ok := names[name]; ok {
			name = n
		}
		v := usageView{Name: name, Runs: r.Runs, Tokens: readableTokens(r.Tokens), Cost: strings.Replace(fmt.Sprintf("%.2f $", r.Cost), ".", ",", 1)}
		if r.Repo != "" {
			v.Name = fmt.Sprintf("%s#%d", r.Repo, r.Issue)
			v.Link = fmt.Sprintf("%s%s/issues/%d", setting.AppSubURL+"/", r.Repo, r.Issue)
		}
		out = append(out, v)
	}
	return out
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
	v.HasRole = map[string]bool{}
	names := make([]string, 0, len(a.Roles))
	for _, m := range agentModes {
		if len(a.Roles) == 0 || slices.Contains(a.Roles, m) {
			v.HasRole[m] = true
			names = append(names, agentModeNames[m])
		}
	}
	v.RoleText = strings.Join(names, ", ")
	if len(a.Roles) == 0 {
		v.RoleText = "alle Schritte"
	}
	switch {
	case !a.LoggedIn && a.Engine == "openai":
		v.State, v.StateColor = "Key fehlt", "red"
	case !a.LoggedIn:
		v.State, v.StateColor = "nicht angemeldet", "red"
	case a.LoginError != "":
		v.State, v.StateColor = "neu anmelden", "red"
	case !a.Enabled:
		v.State, v.StateColor = "pausiert", "grey"
	case len(a.Runs) > 0:
		v.State, v.StateColor = "arbeitet", "blue"
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
	ctx.Data["AgentModes"] = agentModes
	ctx.Data["AgentModeNames"] = agentModeNames
	ctx.Data["Next"] = st.Next
	ctx.Data["Blocked"] = st.Blocked
	ctx.Data["Updated"] = clock(st.Updated)
	ctx.Data["LoopError"] = st.Error
	ctx.Data["UsageTotal"] = usageRows([]agentUsageRow{st.UsageWeek.Total}, nil)[0]
	ctx.Data["UsageRepos"] = usageRows(st.UsageWeek.Repos, nil)
	ctx.Data["UsageModes"] = usageRows(st.UsageWeek.Modes, agentModeNames)
	ctx.Data["UsageTop"] = usageRows(st.UsageWeek.Top, nil)
	ctx.HTML(http.StatusOK, tplSettingsAgents)
}

// AgentAccountPost stores a Claude token, starts a Codex device login, pauses/resumes an account, or sets its
// roles/priority/slots or OpenAI-compatible endpoint
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
	case "delete":
		agentAccountDelete(ctx, account, back)
		return
	case "settings":
		prio, slots := ctx.FormInt("priority"), ctx.FormInt("slots") // parses the form: FormStrings alone only parses multipart
		body = map[string]any{"roles": ctx.FormStrings("roles"), "priority": prio, "slots": slots}
	case "openai":
		body = map[string]string{"base_url": ctx.FormTrim("base_url"), "api_key": ctx.FormTrim("api_key"),
			"small": ctx.FormTrim("small"), "large": ctx.FormTrim("large")}
	default:
		ctx.NotFound(nil)
		return
	}
	if err := callAgentScheduler(http.MethodPost, "/accounts/"+url.PathEscape(account)+"/"+action, body, nil); err != nil {
		ctx.Flash.Error(err.Error())
	} else if action == "claude-token" || action == "claude-code" {
		ctx.Flash.Success("Angemeldet.")
	} else if action == "settings" || action == "openai" {
		ctx.Flash.Success("Gespeichert.")
	}
	ctx.Redirect(back)
}

// agentLoginRe: the bot's user name is also the account id ("claude-c", "hetzner-b")
var agentLoginRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

var agentEngines = map[string]string{"claude": "Claude-Abo", "codex": "Codex-Abo (ChatGPT)", "openai": "OpenAI-kompatibel"}

// createAgentBot makes the bot user an agent account works as: member of team "agents" (write on every repo of the
// org), with a token for the runs. Only a new user: a token is never minted for an existing one.
func createAgentBot(ctx *context.Context, login, name, engine string) (string, error) {
	if _, err := user_model.GetUserByName(ctx, login); err == nil {
		return "", fmt.Errorf("Den Benutzer %q gibt es schon, bitte eine andere Kennung.", login)
	} else if !user_model.IsErrUserNotExist(err) {
		return "", err
	}
	team, err := org_model.GetTeam(ctx, ctx.ContextUser.ID, "agents")
	if err != nil {
		return "", fmt.Errorf("Team \"agents\" fehlt in dieser Organisation: %w", err)
	}
	visibility := api.VisibleTypeLimited
	u := &user_model.User{Name: login, FullName: name, Email: login + "@noreply.localhost", Passwd: util.CryptoRandomString(40),
		LoginType: auth_model.Plain, Description: "Coding-Agent (" + agentEngines[engine] + ")"}
	if err := user_model.AdminCreateUser(ctx, u, &user_model.Meta{}, &user_model.CreateUserOverwriteOptions{
		IsActive: optional.Some(true), Visibility: &visibility}); err != nil {
		return "", err
	}
	if err := org_service.AddTeamMember(ctx, team, u); err != nil {
		return "", err
	}
	scope, err := auth_model.AccessTokenScope("write:repository,write:issue,read:organization,read:user").Normalize()
	if err != nil {
		return "", err
	}
	t := &auth_model.AccessToken{UID: u.ID, Name: "agent", Scope: scope}
	if err := auth_model.NewAccessToken(ctx, t); err != nil {
		return "", err
	}
	return t.Token, nil
}

// AgentAccountCreate adds an account: its bot user, then the scheduler entry (and an OpenAI-compatible endpoint).
// Claude and Codex accounts log in afterwards with the row's "Anmelden".
func AgentAccountCreate(ctx *context.Context) {
	back := ctx.ContextUser.OrganisationLink() + "/settings/agents"
	engine, login, name := ctx.FormTrim("engine"), strings.ToLower(ctx.FormTrim("login")), ctx.FormTrim("display_name")
	if _, ok := agentEngines[engine]; !ok || !agentLoginRe.MatchString(login) || name == "" {
		ctx.Flash.Error("Bitte Typ, Kennung (klein, z. B. claude-c) und Anzeigename angeben.")
		ctx.Redirect(back)
		return
	}
	// the scheduler must answer before a bot user is made, else a retry finds the name taken
	// ponytail: a scheduler error after this check leaves the bot without account; then delete the user in the admin panel
	if err := callAgentScheduler(http.MethodGet, "/status", nil, nil); err != nil {
		ctx.Flash.Error("Scheduler nicht erreichbar: " + err.Error())
		ctx.Redirect(back)
		return
	}
	token, err := createAgentBot(ctx, login, name, engine)
	if err == nil {
		err = callAgentScheduler(http.MethodPost, "/accounts", map[string]string{
			"id": login, "engine": engine, "display_name": name, "bot_login": login, "bot_token": token}, nil)
	}
	if err == nil && engine == "openai" && ctx.FormTrim("base_url") != "" {
		err = callAgentScheduler(http.MethodPost, "/accounts/"+url.PathEscape(login)+"/openai", map[string]string{
			"base_url": ctx.FormTrim("base_url"), "api_key": ctx.FormTrim("api_key"),
			"small": ctx.FormTrim("small"), "large": ctx.FormTrim("large")}, nil)
	}
	if err != nil {
		ctx.Flash.Error(err.Error())
	} else if engine == "openai" {
		ctx.Flash.Success("Konto " + name + " angelegt. Rollen und Priorität stellst du in seiner Zeile ein.")
	} else {
		ctx.Flash.Success("Konto " + name + " angelegt. Jetzt in seiner Zeile „Anmelden“.")
	}
	ctx.Redirect(back)
}

// retireAgentBot takes a deleted account's bot out of service: out of team "agents", all tokens gone. The user
// stays, so its comments and pull requests keep their author. Never touches an admin or an org owner.
func retireAgentBot(ctx *context.Context, login string) error {
	u, err := user_model.GetUserByName(ctx, login)
	if user_model.IsErrUserNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	team, err := org_model.GetTeam(ctx, ctx.ContextUser.ID, "agents")
	if err != nil {
		return err
	}
	owner, err := org_model.IsOrganizationOwner(ctx, ctx.ContextUser.ID, u.ID)
	if err != nil {
		return err
	}
	member, err := org_model.IsTeamMember(ctx, ctx.ContextUser.ID, team.ID, u.ID)
	if err != nil {
		return err
	}
	if u.IsAdmin || owner || !member {
		return fmt.Errorf("@%s ist kein reiner Agent-Bot, den Benutzer habe ich nicht angefasst", login)
	}
	if err := org_service.RemoveTeamMember(ctx, team, u); err != nil {
		return err
	}
	tokens, err := db.Find[auth_model.AccessToken](ctx, auth_model.ListAccessTokensOptions{UserID: u.ID})
	if err != nil {
		return err
	}
	for _, t := range tokens {
		if err := auth_model.DeleteAccessTokenByID(ctx, t.ID, u.ID); err != nil {
			return err
		}
	}
	return nil
}

// agentAccountDelete removes the account from the scheduler (refused while it runs), then retires its bot
func agentAccountDelete(ctx *context.Context, account, back string) {
	if !ctx.FormBool("confirm") {
		ctx.Flash.Error("Zum Löschen bitte das Häkchen setzen.")
		ctx.Redirect(back)
		return
	}
	var st agentStatus
	if err := callAgentScheduler(http.MethodGet, "/status", nil, &st); err != nil {
		ctx.Flash.Error("Scheduler nicht erreichbar: " + err.Error())
		ctx.Redirect(back)
		return
	}
	i := slices.IndexFunc(st.Accounts, func(a agentAccount) bool { return a.ID == account })
	if i < 0 {
		ctx.NotFound(nil)
		return
	}
	a := st.Accounts[i]
	err := callAgentScheduler(http.MethodPost, "/accounts/"+url.PathEscape(account)+"/delete", map[string]string{}, nil)
	if err == nil {
		if err = retireAgentBot(ctx, a.BotLogin); err != nil {
			err = fmt.Errorf("Konto gelöscht, aber: %w", err)
		}
	}
	if err != nil {
		ctx.Flash.Error(err.Error())
	} else {
		ctx.Flash.Success("Konto " + a.DisplayName + " gelöscht, @" + a.BotLogin + " ist aus dem Team „agents“ und hat keine Tokens mehr.")
	}
	ctx.Redirect(back)
}
