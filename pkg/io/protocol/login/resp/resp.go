package resp

import "github.com/zsrv/goscape/pkg/io/protocol"

var (
	// client waits 2 seconds then tries logging in again
	OpTryAgain = protocol.Operation{
		Opcode:      1,
		PayloadSize: 0,
		Name:        "TryAgain",
	}
	OpOK = protocol.Operation{
		Opcode:      2,
		PayloadSize: 0,
		Name:        "OK",
	}
	OpInvalidUsernameOrPassword = protocol.Operation{
		// Invalid username or password.
		Opcode:      3,
		PayloadSize: 0,
		Name:        "INVALID_USERNAME_OR_PASSWORD",
	}
	OpBanned = protocol.Operation{
		// Your account has been disabled.
		// Please check your message-centre for details.
		Opcode:      4,
		PayloadSize: 0,
		Name:        "BANNED",
	}
	OpDuplicate = protocol.Operation{
		// Your account is already logged in.
		// Try again in 60 secs...
		Opcode:      5,
		PayloadSize: 0,
		Name:        "DUPLICATE",
	}
	OpClientOutOfDate = protocol.Operation{
		// RuneScape has been updated!
		// Please reload this page.
		Opcode:      6,
		PayloadSize: 0,
		Name:        "CLIENT_OUT_OF_DATE",
	}
	OpServerFull = protocol.Operation{
		// This world is full.
		// Please use a different world.
		Opcode:      7,
		PayloadSize: 0,
		Name:        "SERVER_FULL",
	}
	OpLoginServerOffline = protocol.Operation{
		// Unable to connect.
		// Login server offline.
		Opcode:      8,
		PayloadSize: 0,
		Name:        "LOGINSERVER_OFFLINE",
	}
	OpIPLimit = protocol.Operation{
		// Login limit exceeded.
		// Too many connections from your address.
		Opcode:      9,
		PayloadSize: 0,
		Name:        "IP_LIMIT",
	}
	OpBadSessionID = protocol.Operation{
		// Unable to connect.
		// Bad session id.
		Opcode:      10,
		PayloadSize: 0,
		Name:        "BadSessionID",
	}
	OpLoginServerRejected = protocol.Operation{
		// Login server rejected session.
		// Please try again.
		Opcode:      11,
		PayloadSize: 0,
		Name:        "LoginServerRejected",
	}
	OpNeedMembersAccount = protocol.Operation{
		// You need a members account to login to this world.
		// Please subscribe, or use a different world.
		Opcode:      12,
		PayloadSize: 0,
		Name:        "NEED_MEMBERS_ACCOUNT",
	}
	OpInvalidSave = protocol.Operation{
		// Could not complete login.
		// Please try using a different world.
		Opcode:      13,
		PayloadSize: 0,
		Name:        "INVALID_SAVE",
	}
	OpUpdateInProgress = protocol.Operation{
		// The server is being updated.
		// Please wait 1 minute and try again.
		Opcode:      14,
		PayloadSize: 0,
		Name:        "UPDATE_IN_PROGRESS",
	}
	OpReconnectOK = protocol.Operation{
		Opcode:      15,
		PayloadSize: 0,
		Name:        "RECONNECT_OK",
	}
	OpTooManyAttempts = protocol.Operation{
		// Login attempts exceeded.
		// Please wait 1 minute and try again.
		Opcode:      16,
		PayloadSize: 0,
		Name:        "TOO_MANY_ATTEMPTS",
	}
	OpMembersOnlyArea = protocol.Operation{
		// You are standing in a members-only area.
		// To play on this world move to a free area first
		Opcode:      17,
		PayloadSize: 0,
		Name:        "MembersOnlyArea",
	}
	OpHopTimer = protocol.Operation{
		// You have only just left another world
		// Your profile will be transfered in: <n> seconds
		//
		// rev-254 A4: the only login reject with a payload — one extra
		// byte carrying min(255, remaining/1000) seconds (TS
		// World.ts:1861-1866 @2e3bcf43 renders LoginServer response 10
		// as [21, Math.min(255, remaining/1000)]). The Java client
		// (Client.java @2e629784, response-21 branch) reads that byte
		// and counts it down on the title screen before auto-retrying.
		Opcode:      21,
		PayloadSize: 1,
		Name:        "HOP_TIMER",
	}
	// Replies 18 (LoginOKWithRights) and 19 (LoginOKSupermod) were removed
	// at 254: the staff tier now rides inside the always-opcode-2 login OK
	// reply [2, min(staffModLevel,2), 1] (TS World.ts:946-950 @43e02957;
	// the 254 client has no 18/19 login branches — Client.java @2e629784
	// routes them to "Unexpected server response").
)
