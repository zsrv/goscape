package friends

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// handler implements friendspb.FriendsServiceServer.
type handler struct {
	friendspb.UnimplementedFriendsServiceServer

	repo *Repository
	cfg  Config
	log  *slog.Logger
}

// ensureWorld lazy-inits the world's player slot if not already known.
// TS-faithful behavior: FriendServer.ts:108-115 (and similar branches)
// lazily call initializeWorld on the first non-WorldConnect message
// from a new world. Kept permanently.
//
// NAI-S1-D-LAZY-WORLDINIT — for reviewer traceability; not retired.
func (h *handler) ensureWorld(worldId int32) {
	h.repo.initializeWorldIfAbsent(worldId, h.cfg.WorldPlayerLimit)
}

// WorldConnect validates the profile and initializes the world's slot.
// Mirrors TS FriendServer WORLD_CONNECT (FriendServer.ts:89-106).
// Re-init by the same world resets that world's player counter to 0.
func (h *handler) WorldConnect(_ context.Context, req *friendspb.WorldConnectRequest) (*emptypb.Empty, error) {
	if req.Profile != h.cfg.NodeProfile {
		return nil, status.Errorf(codes.InvalidArgument,
			"profile mismatch: got %q, want %q", req.Profile, h.cfg.NodeProfile)
	}
	h.repo.InitializeWorld(req.WorldId, h.cfg.WorldPlayerLimit)
	return &emptypb.Empty{}, nil
}
