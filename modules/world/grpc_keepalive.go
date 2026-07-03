package world

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// clientKeepaliveParams is the keepalive contract for both bridge
// clients (login, friends). Without it a NAT/firewall dropping
// connection state without RST leaves the subscriber streams blocked in
// Recv() forever — the reconnect supervisors only run on stream errors
// (arch-29.2).
func clientKeepaliveParams() keepalive.ClientParameters {
	return keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	}
}

func worldClientKeepalive() grpc.DialOption {
	return grpc.WithKeepaliveParams(clientKeepaliveParams())
}
