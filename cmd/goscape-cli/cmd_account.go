package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// runAccount is the `account` verb: a thin client over the
// AccountService admin gRPC surface (modules/account). Requires the
// server's account.admin_token via -token or GOSCAPE_ACCOUNT_TOKEN.
func runAccount(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("account", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:2005", "AccountService gRPC address")
	token := fs.String("token", "", "admin bearer token (default: GOSCAPE_ACCOUNT_TOKEN env)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, `usage: goscape-cli account [-addr host:port] [-token T] <subcommand> [args]

subcommands:
  search <query>                                  list matching accounts
  show <account-id|email>                         full account detail
  approve <account-id> | unapprove <account-id>   manually_approved on/off
  disable <account-id> | enable <account-id>      account status
  unlink <account-id> <provider>                  soft-revoke (burn) a linked identity
  release-identity <provider> <provider-user-id>  hard-delete (free) an identity
  reset-password <account-id>                     mint + print a reset URL
  bootstrap-admin <email>                         add account to the admin group`)
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *token == "" {
		*token = os.Getenv("GOSCAPE_ACCOUNT_TOKEN")
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	sub, subArgs := rest[0], rest[1:]

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(stderr, "account: dial %s: %v\n", *addr, err)
		return 1
	}
	defer conn.Close()
	client := accountpb.NewAccountServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*token)
	}

	parseID := func(s string) (int64, bool) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id < 1 {
			fmt.Fprintf(stderr, "account: %q is not an account id\n", s)
			return 0, false
		}
		return id, true
	}
	need := func(n int) bool {
		if len(subArgs) != n {
			fs.Usage()
			return false
		}
		return true
	}
	setGroup := func(group string, member bool) int {
		if !need(1) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		if _, err := client.SetGroupMembership(ctx, &accountpb.SetGroupMembershipRequest{
			AccountId: id, Group: group, Member: member}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "account %d: %s=%v\n", id, group, member)
		return 0
	}
	setStatus := func(status string) int {
		if !need(1) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		if _, err := client.SetAccountStatus(ctx, &accountpb.SetAccountStatusRequest{
			AccountId: id, Status: status}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "account %d: status=%s\n", id, status)
		return 0
	}

	switch sub {
	case "search":
		if !need(1) {
			return 2
		}
		resp, err := client.SearchAccounts(ctx, &accountpb.SearchAccountsRequest{Query: subArgs[0]})
		if err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%-8s %-32s %-9s %-9s %s\n", "ID", "EMAIL", "VERIFIED", "STATUS", "CHARS")
		for _, a := range resp.Accounts {
			fmt.Fprintf(stdout, "%-8d %-32s %-9v %-9s %d\n", a.Id, a.Email, a.EmailVerified, a.Status, a.CharacterCount)
		}
		return 0

	case "show":
		if !need(1) {
			return 2
		}
		req := &accountpb.GetAccountRequest{}
		if strings.Contains(subArgs[0], "@") {
			req.Email = subArgs[0]
		} else {
			id, ok := parseID(subArgs[0])
			if !ok {
				return 2
			}
			req.Id = id
		}
		resp, err := client.GetAccount(ctx, req)
		if err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		a := resp.Account
		fmt.Fprintf(stdout, "id: %d\nemail: %s (verified=%v)\nstatus: %s\ngroups: %s\n",
			a.Id, a.Email, a.EmailVerified, a.Status, strings.Join(resp.Groups, ", "))
		for _, id := range resp.Identities {
			state := "linked"
			if id.Revoked {
				state = "REVOKED"
			}
			fmt.Fprintf(stdout, "identity: %s %s (%s) [%s]\n", id.Provider, id.ProviderUserId, id.ProviderUsername, state)
		}
		for _, ch := range resp.Characters {
			fmt.Fprintf(stdout, "character: %s (id=%d game_account=%d)\n", ch.Username, ch.Id, ch.GameAccountId)
		}
		return 0

	case "approve":
		return setGroup("manually_approved", true)
	case "unapprove":
		return setGroup("manually_approved", false)
	case "disable":
		return setStatus("disabled")
	case "enable":
		return setStatus("active")

	case "unlink":
		if !need(2) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		if _, err := client.UnlinkIdentity(ctx, &accountpb.UnlinkIdentityRequest{
			AccountId: id, Provider: subArgs[1]}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "account %d: %s identity revoked (burned — use release-identity to free it)\n", id, subArgs[1])
		return 0

	case "release-identity":
		if !need(2) {
			return 2
		}
		if _, err := client.ReleaseIdentity(ctx, &accountpb.ReleaseIdentityRequest{
			Provider: subArgs[0], ProviderUserId: subArgs[1]}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "identity %s:%s released\n", subArgs[0], subArgs[1])
		return 0

	case "reset-password":
		if !need(1) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		resp, err := client.AdminResetPassword(ctx, &accountpb.AdminResetPasswordRequest{AccountId: id})
		if err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", resp.ResetUrl)
		return 0

	case "bootstrap-admin":
		if !need(1) {
			return 2
		}
		if _, err := client.BootstrapAdmin(ctx, &accountpb.BootstrapAdminRequest{Email: subArgs[0]}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s is now an admin\n", subArgs[0])
		return 0

	default:
		fmt.Fprintf(stderr, "account: unknown subcommand %q\n", sub)
		fs.Usage()
		return 2
	}
}
