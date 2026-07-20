package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// recordingAccountService captures metadata + requests, returns canned data.
type recordingAccountService struct {
	accountpb.UnimplementedAccountServiceServer
	gotAuth []string
}

func (s *recordingAccountService) record(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.gotAuth = md.Get("authorization")
}

func (s *recordingAccountService) SearchAccounts(ctx context.Context, req *accountpb.SearchAccountsRequest) (*accountpb.SearchAccountsResponse, error) {
	s.record(ctx)
	return &accountpb.SearchAccountsResponse{Accounts: []*accountpb.AccountSummary{{
		Id: 7, Email: "a@example.com", EmailVerified: true, Status: "active", CharacterCount: 2,
	}}}, nil
}

func (s *recordingAccountService) SetGroupMembership(ctx context.Context, req *accountpb.SetGroupMembershipRequest) (*emptypb.Empty, error) {
	s.record(ctx)
	return &emptypb.Empty{}, nil
}

func (s *recordingAccountService) AdminResetPassword(ctx context.Context, req *accountpb.AdminResetPasswordRequest) (*accountpb.AdminResetPasswordResponse, error) {
	s.record(ctx)
	return &accountpb.AdminResetPasswordResponse{ResetUrl: "http://portal/reset-password?token=abc"}, nil
}

func startStubAccountServer(t *testing.T) (addr string, stub *recordingAccountService) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stub = &recordingAccountService{}
	srv := grpc.NewServer()
	accountpb.RegisterAccountServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), stub
}

func TestAccountVerb_SearchAndAuth(t *testing.T) {
	addr, stub := startStubAccountServer(t)
	var out, errOut bytes.Buffer
	code := runAccount([]string{"-addr", addr, "-token", "sekrit", "search", "a@"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "a@example.com") || !strings.Contains(out.String(), "7") {
		t.Fatalf("output: %q", out.String())
	}
	if len(stub.gotAuth) != 1 || stub.gotAuth[0] != "Bearer sekrit" {
		t.Fatalf("auth metadata: %v", stub.gotAuth)
	}
}

func TestAccountVerb_ApproveAndResetPassword(t *testing.T) {
	addr, _ := startStubAccountServer(t)
	var out, errOut bytes.Buffer
	if code := runAccount([]string{"-addr", addr, "-token", "x", "approve", "7"}, &out, &errOut); code != 0 {
		t.Fatalf("approve exit=%d stderr=%s", code, errOut.String())
	}
	out.Reset()
	if code := runAccount([]string{"-addr", addr, "-token", "x", "reset-password", "7"}, &out, &errOut); code != 0 {
		t.Fatalf("reset exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "http://portal/reset-password?token=abc") {
		t.Fatalf("reset output: %q", out.String())
	}
}

func TestAccountVerb_UsageErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runAccount(nil, &out, &errOut); code != 2 {
		t.Fatalf("no args: exit=%d", code)
	}
	if code := runAccount([]string{"-addr", "127.0.0.1:1", "frobnicate"}, &out, &errOut); code != 2 {
		t.Fatalf("unknown sub: exit=%d", code)
	}
	if code := runAccount([]string{"-addr", "127.0.0.1:1", "approve", "not-a-number"}, &out, &errOut); code != 2 {
		t.Fatalf("bad id: exit=%d", code)
	}
}
