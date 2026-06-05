package login

import (
	"database/sql"
	"testing"
)

// mt inserts a message_thread row. from/to are account ids; lastFrom is
// last_message_from. Returns the thread id.
func mt(t *testing.T, db *sql.DB, from, to, lastFrom int) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	                     VALUES (?, ?, ?, 's')`, to, from, lastFrom)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// msg inserts a message row with the given created stamp (” = CURRENT_TIMESTAMP default).
func msg(t *testing.T, db *sql.DB, thread int64, created string, deleted bool) {
	t.Helper()
	del := sql.NullString{}
	if deleted {
		del = sql.NullString{String: created, Valid: true}
	}
	var err error
	if created == "" {
		_, err = db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content, deleted)
		                  VALUES (?, 1, '', 'm', ?)`, thread, del)
	} else {
		_, err = db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content, created, deleted)
		                  VALUES (?, 1, '', 'm', ?, ?)`, thread, created, del)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// st inserts a message_status row for (thread, account) with optional
// read/deleted stamps (” = NULL).
func st(t *testing.T, db *sql.DB, thread int64, account int, read, deleted string) {
	t.Helper()
	toNull := func(s string) sql.NullString {
		if s == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: s, Valid: true}
	}
	if _, err := db.Exec(`INSERT INTO message_status (thread_id, account_id, "read", deleted)
	                      VALUES (?, ?, ?, ?)`, thread, account, toNull(read), toNull(deleted)); err != nil {
		t.Fatal(err)
	}
}

// TestGetUnreadMessageCount pins the TS Messages.ts:3-37 unread
// semantics, row by row. Viewer is account id 2; threads run from
// account 1 → account 2 unless stated.
func TestGetUnreadMessageCount(t *testing.T) {
	const viewer = 2

	cases := []struct {
		name string
		seed func(t *testing.T, db *sql.DB)
		want int
	}{
		{"unread thread counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
		}, 1},
		{"read after last message not counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "2026-06-05 11:00:00", "")
		}, 0},
		{"read before last message counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "2026-06-05 09:00:00", "")
		}, 1},
		{"status-deleted after last message not counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "", "2026-06-05 11:00:00")
		}, 0},
		{"status-deleted before last message counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "", "2026-06-05 09:00:00")
		}, 1},
		{"own-last-message thread excluded", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, viewer) // last_message_from = viewer
			msg(t, db, th, "2026-06-05 10:00:00", false)
		}, 0},
		{"deleted messages excluded from last-message", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			msg(t, db, th, "2026-06-05 12:00:00", true) // deleted newest
			st(t, db, th, viewer, "2026-06-05 11:00:00", "")
			// last non-deleted = 10:00 < read 11:00 → not unread
		}, 0},
		{"thread not involving viewer excluded", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, 3, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
		}, 0},
		{"empty tables", func(t *testing.T, db *sql.DB) {}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := createTestDB(t)
			tc.seed(t, db)
			got, err := getUnreadMessageCount(t.Context(), db, viewer)
			if err != nil {
				t.Fatalf("getUnreadMessageCount: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
