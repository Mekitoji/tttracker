package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"tttracker/internal/db"
)

const ts = "2026-06-09T08:00:00Z"

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.Migrate(d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func insertProject(t *testing.T, d *sql.DB, key string) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO projects(key, name, created_at, updated_at) VALUES(?, ?, ?, ?)`,
		key, key+" project", ts, ts,
	)
	if err != nil {
		t.Fatalf("insert project: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func insertTicket(t *testing.T, d *sql.DB, projectID int64, number int, title string) int64 {
	t.Helper()
	res, err := d.Exec(
		`INSERT INTO tickets(project_id, number, title, type, status, priority, created_at, updated_at)
		 VALUES(?, ?, ?, 'task', 'todo', 'medium', ?, ?)`,
		projectID, number, title, ts, ts,
	)
	if err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func searchCount(t *testing.T, d *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM ticket_search WHERE ticket_search MATCH ?`, query,
	).Scan(&n); err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	return n
}

func TestMigrateCreatesSchema(t *testing.T) {
	d := openTestDB(t)

	want := []string{
		"projects", "tickets", "subtasks", "comments",
		"attachments", "activity_events", "ticket_search",
	}
	for _, name := range want {
		var got string
		err := d.QueryRow(
			`SELECT name FROM sqlite_master WHERE name = ?`, name,
		).Scan(&got)
		if err != nil {
			t.Fatalf("expected object %q to exist: %v", name, err)
		}
	}

	// Migrations must be idempotent.
	if err := db.Migrate(d); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

// GATE 1a: the FTS index must follow insert/update/delete on tickets.
func TestFTS5SyncOnWrite(t *testing.T) {
	d := openTestDB(t)
	pid := insertProject(t, d, "PET")
	tid := insertTicket(t, d, pid, 1, "Fix SQLite migration bug")

	if got := searchCount(t, d, "migration"); got != 1 {
		t.Fatalf("after insert: want 1 hit, got %d", got)
	}

	if _, err := d.Exec(`UPDATE tickets SET title = 'Polish board view' WHERE id = ?`, tid); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := searchCount(t, d, "migration"); got != 0 {
		t.Fatalf("after update: stale term should be gone, got %d", got)
	}
	if got := searchCount(t, d, "board"); got != 1 {
		t.Fatalf("after update: new term should be found, got %d", got)
	}

	if _, err := d.Exec(`DELETE FROM tickets WHERE id = ?`, tid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := searchCount(t, d, "board"); got != 0 {
		t.Fatalf("after delete: index should be empty, got %d", got)
	}
}

// GATE 1b: deleting a project cascades to its tickets AND the FTS sync trigger
// must still fire (this is why recursive_triggers is enabled). Querying the FTS
// table directly catches a stale index that a JOIN to tickets would hide.
func TestFTS5SurvivesCascadeDelete(t *testing.T) {
	d := openTestDB(t)
	pid := insertProject(t, d, "PET")
	insertTicket(t, d, pid, 1, "zzcascadeterm marker")

	if got := searchCount(t, d, "zzcascadeterm"); got != 1 {
		t.Fatalf("precondition: want 1 hit, got %d", got)
	}

	if _, err := d.Exec(`DELETE FROM projects WHERE id = ?`, pid); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	var tickets int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tickets`).Scan(&tickets); err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if tickets != 0 {
		t.Fatalf("cascade delete failed: %d tickets remain (foreign_keys off?)", tickets)
	}

	if got := searchCount(t, d, "zzcascadeterm"); got != 0 {
		t.Fatalf("FTS index is stale after cascade delete: got %d (recursive_triggers off?)", got)
	}
}
