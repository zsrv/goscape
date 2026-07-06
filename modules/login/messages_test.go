package login

import (
	"database/sql"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// mt inserts a message_thread row. from/to are account ids; lastFrom is
// last_message_from. Returns the thread id via INSERT ... RETURNING —
// the dialect-uniform id-retrieval form (pgx/v5 stdlib's LastInsertId
// always errors); same idiom as db.go's insertAccount.
func mt(t *testing.T, db *gamedb.DB, from, to, lastFrom int) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	               VALUES (?, ?, ?, 's') RETURNING id`), to, from, lastFrom,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// msg inserts a message row with the given created stamp (zero value =
// CURRENT_TIMESTAMP default).
func msg(t *testing.T, db *gamedb.DB, thread int64, created time.Time, deleted bool) {
	t.Helper()
	del := sql.NullTime{}
	if deleted {
		del = sql.NullTime{Time: created, Valid: true}
	}
	var err error
	if created.IsZero() {
		_, err = db.Exec(db.Rebind(`INSERT INTO message (thread_id, sender_id, sender_ip, content, deleted)
		                  VALUES (?, 1, '', 'm', ?)`), thread, del)
	} else {
		_, err = db.Exec(db.Rebind(`INSERT INTO message (thread_id, sender_id, sender_ip, content, created, deleted)
		                  VALUES (?, 1, '', 'm', ?, ?)`), thread, created, del)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// st inserts a message_status row for (thread, account) with optional
// read/deleted stamps (zero value = NULL).
func st(t *testing.T, db *gamedb.DB, thread int64, account int, read, deleted time.Time) {
	t.Helper()
	toNull := func(tm time.Time) sql.NullTime {
		if tm.IsZero() {
			return sql.NullTime{}
		}
		return sql.NullTime{Time: tm, Valid: true}
	}
	if _, err := db.Exec(db.Rebind(`INSERT INTO message_status (thread_id, account_id, "read", deleted)
	                      VALUES (?, ?, ?, ?)`), thread, account, toNull(read), toNull(deleted)); err != nil {
		t.Fatal(err)
	}
}

// TestGetUnreadMessageCount pins the TS Messages.ts:3-37 unread
// semantics, row by row. Viewer is account id 2; threads run from
// account 1 → account 2 unless stated.
func TestGetUnreadMessageCount(t *testing.T) {
	const viewer = 2

	at := func(hour int) time.Time { return time.Date(2026, 6, 5, hour, 0, 0, 0, time.UTC) }

	cases := []struct {
		name string
		seed func(t *testing.T, db *gamedb.DB)
		want int
	}{
		{"unread thread counted", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, at(10), false)
		}, 1},
		{"read after last message not counted", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, at(10), false)
			st(t, db, th, viewer, at(11), time.Time{})
		}, 0},
		{"read before last message counted", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, at(10), false)
			st(t, db, th, viewer, at(9), time.Time{})
		}, 1},
		{"status-deleted after last message not counted", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, at(10), false)
			st(t, db, th, viewer, time.Time{}, at(11))
		}, 0},
		{"status-deleted before last message counted", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, at(10), false)
			st(t, db, th, viewer, time.Time{}, at(9))
		}, 1},
		{"own-last-message thread excluded", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, viewer) // last_message_from = viewer
			msg(t, db, th, at(10), false)
		}, 0},
		{"deleted messages excluded from last-message", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, at(10), false)
			msg(t, db, th, at(12), true) // deleted newest
			st(t, db, th, viewer, at(11), time.Time{})
			// last non-deleted = 10:00 < read 11:00 → not unread
		}, 0},
		{"thread not involving viewer excluded", func(t *testing.T, db *gamedb.DB) {
			th := mt(t, db, 1, 3, 1)
			msg(t, db, th, at(10), false)
		}, 0},
		{"empty tables", func(t *testing.T, db *gamedb.DB) {}, 0},
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
