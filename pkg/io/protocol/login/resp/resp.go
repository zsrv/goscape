package login

import "github.com/zsrv/goscape/pkg/io/protocol"

var (
	// client waits 2 seconds then tries logging in again
	OpRespTryAgainLater = protocol.Operation{
		Opcode:      1,
		PayloadSize: 0,
		Name:        "OpRespTryAgainLater",
	}
	OpRespLoginSuccess = protocol.Operation{
		Opcode:      2,
		PayloadSize: 0,
		Name:        "OpRespLoginSuccess",
	}
	OpRespInvalidUsernameOrPassword = protocol.Operation{
		// Invalid username or password.
		Opcode:      3,
		PayloadSize: 0,
		Name:        "OpRespInvalidUsernameOrPassword",
	}
	OpRespAccountDisabled = protocol.Operation{
		// Your account has been disabled.
		// Please check your message-centre for details.
		Opcode:      4,
		PayloadSize: 0,
		Name:        "OpRespAccountDisabled",
	}
	OpRespAlreadyLoggedIn = protocol.Operation{
		// Your account is already logged in.
		// Try again in 60 secs...
		Opcode:      5,
		PayloadSize: 0,
		Name:        "OpRespAlreadyLoggedIn",
	}
	OpRespGameUpdated = protocol.Operation{
		// RuneScape has been updated!
		// Please reload this page.
		Opcode:      6,
		PayloadSize: 0,
		Name:        "OpRespGameUpdated",
	}
	OpRespWorldFull = protocol.Operation{
		// This world is full.
		// Please use a different world.
		Opcode:      7,
		PayloadSize: 0,
		Name:        "OpRespWorldFull",
	}
	OpRespLoginServerOffline = protocol.Operation{
		// Unable to connect.
		// Login server offline.
		Opcode:      8,
		PayloadSize: 0,
		Name:        "OpRespLoginServerOffline",
	}
	OpRespConnectionLimitExceeded = protocol.Operation{
		// Login limit exceeded.
		// Too many connections from your address.
		Opcode:      9,
		PayloadSize: 0,
		Name:        "OpRespConnectionLimitExceeded",
	}
	OpRespBadSessionId = protocol.Operation{
		// Unable to connect.
		// Bad session id.
		Opcode:      10,
		PayloadSize: 0,
		Name:        "OpRespBadSessionId",
	}
	OpRespLoginServerRejectedSession = protocol.Operation{
		// Login server rejected session.
		// Please try again.
		Opcode:      11,
		PayloadSize: 0,
		Name:        "OpRespLoginServerRejectedSession",
	}
	OpRespMembersWorld = protocol.Operation{
		// You need a members account to login to this world.
		// Please subscribe, or use a different world.
		Opcode:      12,
		PayloadSize: 0,
		Name:        "OpRespMembersWorld",
	}
	OpRespLoginFailed = protocol.Operation{
		// Could not complete login.
		// Please try using a different world.
		Opcode:      13,
		PayloadSize: 0,
		Name:        "OpRespLoginFailed",
	}
	OpRespServerBeingUpdated = protocol.Operation{
		// The server is being updated.
		// Please wait 1 minute and try again.
		Opcode:      14,
		PayloadSize: 0,
		Name:        "OpRespServerBeingUpdated",
	}
	OpRespReconnected = protocol.Operation{
		Opcode:      15,
		PayloadSize: 0,
		Name:        "OpRespReconnected",
	}
	// maybe too many login attempts to one account globally/from any ip
	OpRespLoginAttemptsExceeded = protocol.Operation{
		// Login attempts exceeded.
		// Please wait 1 minute and try again.
		Opcode:      16,
		PayloadSize: 0,
		Name:        "OpRespLoginAttemptsExceeded",
	}
	OpRespMembersOnlyArea = protocol.Operation{
		// You are standing in a members-only area.
		// To play on this world move to a free area first
		Opcode:      17,
		PayloadSize: 0,
		Name:        "OpRespMembersOnlyArea",
	}
	// player will have right-click report abuse in chat, mute option in report abuse interface
	OpRespLoginSuccessWithRights = protocol.Operation{
		Opcode:      18,
		PayloadSize: 0,
		Name:        "OpRespLoginSuccessWithRights",
	}
)
