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
	// player will have right-click report abuse in chat, mute option in report abuse interface
	OpLoginOKWithRights = protocol.Operation{
		Opcode:      18,
		PayloadSize: 0,
		Name:        "LoginOKWithRights",
	}
	// OpLoginOKSupermod is sent when staffModLevel >= 2.
	// TS World.ts:943-949 (244 pin 9aadcec4): >=2 → byte 19 (supermod/admin).
	OpLoginOKSupermod = protocol.Operation{
		Opcode:      19,
		PayloadSize: 0,
		Name:        "LoginOKSupermod",
	}
)
