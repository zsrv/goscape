package account

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
)

type adminSearchData struct {
	Query   string
	Results []PortalAccount
}

type adminAccountData struct {
	Acct       *PortalAccount
	Groups     []string
	Approved   bool
	Identities []Identity
	Characters []Character
	Audit      []AuditEntry
}

type adminAuditData struct {
	Target  string
	Entries []AuditEntry
}

func (p *portal) handleAdminSearch(w http.ResponseWriter, r *http.Request) {
	data := adminSearchData{Query: r.URL.Query().Get("q")}
	if data.Query != "" {
		results, err := p.store.SearchAccounts(r.Context(), data.Query)
		if err != nil {
			p.log.Error("admin search", slog.Any("err", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		data.Results = results
	}
	p.render(w, r, "admin_search.html", data)
}

// adminTarget loads the {id} path account or writes 404.
func (p *portal) adminTarget(w http.ResponseWriter, r *http.Request) (*PortalAccount, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	acct, err := p.store.AccountByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		p.log.Error("admin target", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil, false
	}
	return acct, true
}

func (p *portal) handleAdminAccount(w http.ResponseWriter, r *http.Request) {
	acct, ok := p.adminTarget(w, r)
	if !ok {
		return
	}
	groups, err := p.store.GroupsByAccount(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ids, err := p.store.IdentitiesByAccount(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chars, err := p.store.CharactersByAccount(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	audit, err := p.store.RecentAudit(r.Context(), 25, fmt.Sprintf("account:%d", acct.ID))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	approved := false
	for _, g := range groups {
		if g == GroupManuallyApproved {
			approved = true
		}
	}
	p.render(w, r, "admin_account.html", adminAccountData{
		Acct: acct, Groups: groups, Approved: approved,
		Identities: ids, Characters: chars, Audit: audit,
	})
}

// adminAction wraps the shared shape of the POST actions: resolve
// target, run the mutation, audit as the acting admin, bounce back.
func (p *portal) adminAction(action string, mutate func(r *http.Request, target *PortalAccount) (details string, err error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, ok := p.adminTarget(w, r)
		if !ok {
			return
		}
		details, err := mutate(r, target)
		if err != nil {
			p.log.Error("admin action failed", slog.String("action", action), slog.Any("err", err))
			http.Redirect(w, r, fmt.Sprintf("/admin/accounts/%d?msg=Action+failed:+%s", target.ID, action), http.StatusFound)
			return
		}
		admin := ctxAccount(r)
		if err := p.store.AppendAudit(r.Context(), admin.ID, action,
			fmt.Sprintf("account:%d", target.ID), details); err != nil {
			p.log.Warn("audit failed", slog.Any("err", err))
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/accounts/%d?msg=Done", target.ID), http.StatusFound)
	}
}

func (p *portal) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	entries, err := p.store.RecentAudit(r.Context(), 100, target)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.render(w, r, "admin_audit.html", adminAuditData{Target: target, Entries: entries})
}
