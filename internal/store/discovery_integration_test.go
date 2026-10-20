package store

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulsechat/internal/store/db"
)

func discoveryStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("PULSECHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PULSECHAT_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "discovery_test_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
		_ = admin.Close(cleanup)
	})
	paths, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(source)); err != nil {
			t.Fatalf("migration %s: %v", path, err)
		}
	}
	return &Store{Pool: pool, Queries: db.New(pool)}, ctx
}

func TestDiscoveryAndReadIntegration(t *testing.T) {
	s, ctx := discoveryStore(t)
	newUser := func(name string) db.User {
		t.Helper()
		u, err := s.CreateUser(ctx, db.CreateUserParams{Username: name, Email: name + "@example.test", PasswordHash: "test"})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	self, peer := newUser("alice_self"), newUser("alice_peer")
	for i := range 25 {
		newUser("alice_" + string(rune('a'+i)))
	}
	users, err := s.SearchUsers(ctx, db.SearchUsersParams{ViewerID: self.ID, Search: "ALICE"})
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 20 {
		t.Fatalf("user limit=%d", len(users))
	}
	for _, u := range users {
		if u.ID == self.ID || u.DisplayName == "" {
			t.Fatalf("invalid user %+v", u)
		}
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE users SET display_name='River Person' WHERE id=$1", peer.ID); err != nil {
		t.Fatal(err)
	}
	users, err = s.SearchUsers(ctx, db.SearchUsersParams{ViewerID: self.ID, Search: "river"})
	if err != nil || len(users) != 1 || users[0].DisplayName != "River Person" {
		t.Fatalf("display search=%v err=%v", users, err)
	}
	public, err := s.CreateGroupRoom(ctx, "Team one", self.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateGroupRoom(ctx, "Team two", self.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	private, err := s.CreateGroupRoom(ctx, "Team private", self.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	dm, err := s.GetOrCreateDirectRoom(ctx, self.ID, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE rooms SET is_public=true WHERE id=$1", dm.ID); err == nil {
		t.Fatal("public DM accepted")
	}
	for offset, want := range []pgtype.UUID{public.ID, second.ID} {
		rooms, err := s.ListPublicRooms(ctx, db.ListPublicRoomsParams{Search: "team", PageLimit: 1, PageOffset: int32(offset)})
		if err != nil || len(rooms) != 1 || rooms[0].ID != want {
			t.Fatalf("public page=%v err=%v", rooms, err)
		}
	}
	if _, err := s.AddRoomMember(ctx, db.AddRoomMemberParams{RoomID: public.ID, UserID: peer.ID}); err != nil {
		t.Fatal(err)
	}
	summary := func() db.ListRoomSummariesRow {
		t.Helper()
		rows, err := s.ListRoomSummaries(ctx, db.ListRoomSummariesParams{UserID: self.ID, RoomIds: []pgtype.UUID{public.ID}})
		if err != nil || len(rows) != 1 {
			t.Fatalf("summary=%v err=%v", rows, err)
		}
		return rows[0]
	}
	if row := summary(); row.UnreadCount != 0 || row.LastMessageID.Valid {
		t.Fatalf("empty summary=%+v", row)
	}
	base := time.Now().UTC().Truncate(time.Second)
	message := func(sender pgtype.UUID, offset time.Duration) db.Message {
		t.Helper()
		m, err := s.CreateMessage(ctx, db.CreateMessageParams{RoomID: public.ID, SenderID: sender, Type: "text", Body: strings.Repeat("x", 400)})
		if err != nil {
			t.Fatal(err)
		}
		m.CreatedAt = pgtype.Timestamptz{Time: base.Add(offset), Valid: true}
		if _, err := s.Pool.Exec(ctx, "UPDATE messages SET created_at=$1 WHERE id=$2", m.CreatedAt, m.ID); err != nil {
			t.Fatal(err)
		}
		return m
	}
	first := message(peer.ID, 0)
	_ = message(self.ID, time.Second)
	last := message(peer.ID, 2*time.Second)
	if row := summary(); row.UnreadCount != 2 || row.LastMessageID != last.ID || len(row.LastBody) != 280 {
		t.Fatalf("unread/preview=%+v", row)
	}
	mark := func(id pgtype.UUID) pgtype.UUID {
		t.Helper()
		got, err := s.MarkRoomRead(ctx, db.SetRoomReadMarkerParams{RoomID: public.ID, UserID: self.ID, MessageID: id})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if mark(first.ID) != first.ID || summary().UnreadCount != 1 {
		t.Fatal("partial read failed")
	}
	if mark(last.ID) != last.ID || mark(first.ID) != last.ID || summary().UnreadCount != 0 {
		t.Fatal("read marker regressed")
	}
	for _, p := range []db.SetRoomReadMarkerParams{
		{RoomID: private.ID, UserID: self.ID, MessageID: first.ID},
		{RoomID: private.ID, UserID: peer.ID, MessageID: first.ID},
	} {
		if _, err := s.MarkRoomRead(ctx, p); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("invalid marker error=%v", err)
		}
	}
	if _, err := s.Pool.Exec(ctx, "UPDATE room_members SET last_read_message_id=NULL WHERE room_id=$1 AND user_id=$2", public.ID, self.ID); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := range 20 {
		group.Go(func() {
			id := first.ID
			if i%2 == 0 {
				id = last.ID
			}
			if _, err := s.MarkRoomRead(ctx, db.SetRoomReadMarkerParams{RoomID: public.ID, UserID: self.ID, MessageID: id}); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if summary().UnreadCount != 0 {
		t.Fatal("concurrent markers regressed")
	}
	a, b := message(peer.ID, 3*time.Second), message(peer.ID, 3*time.Second)
	if a.ID.String() > b.ID.String() {
		a, b = b, a
	}
	mark(a.ID)
	if summary().UnreadCount != 1 {
		t.Fatal("equal timestamp UUID ordering failed")
	}
	mark(b.ID)
	if mark(a.ID) != b.ID {
		t.Fatal("equal timestamp regression")
	}
	if _, err := s.Pool.Exec(ctx, "DELETE FROM messages WHERE id=$1", b.ID); err != nil {
		t.Fatal(err)
	}
	member, err := s.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: public.ID, UserID: self.ID})
	if err != nil || member.LastReadMessageID.Valid {
		t.Fatalf("deleted marker=%+v err=%v", member, err)
	}
	for _, name := range []string{"000007_read_markers.down.sql", "000006_discovery.down.sql", "000006_discovery.up.sql", "000007_read_markers.up.sql"} {
		source, err := os.ReadFile(filepath.Join("../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Pool.Exec(ctx, string(source)); err != nil {
			t.Fatalf("migration roundtrip %s: %v", name, err)
		}
	}
}
