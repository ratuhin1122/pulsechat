package store

import (
	"bytes"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"os"
	"pulsechat/internal/storage"
	"pulsechat/internal/store/db"
	"testing"
)

func TestPersonalDeletionIntegration(t *testing.T) {
	for _, kind := range []string{"dm", "group"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx := discoveryStore(t)
			self, err := s.CreateUser(ctx, db.CreateUserParams{Username: "self", Email: "self@example.test", PasswordHash: "hash"})
			if err != nil {
				t.Fatal(err)
			}
			peer, err := s.CreateUser(ctx, db.CreateUserParams{Username: "peer", Email: "peer@example.test", PasswordHash: "hash"})
			if err != nil {
				t.Fatal(err)
			}
			room, err := s.GetOrCreateDirectRoom(ctx, self.ID, peer.ID)
			if kind == "group" {
				room, err = s.CreateGroupRoom(ctx, "Public", self.ID, true)
				if err == nil {
					_, err = s.JoinGroupRoom(ctx, db.JoinGroupRoomParams{RoomID: room.ID, UserID: peer.ID})
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			old, err := s.CreateMemberMessage(ctx, db.CreateMemberMessageParams{RoomID: room.ID, SenderID: peer.ID, Body: "old"})
			if err != nil {
				t.Fatal(err)
			}
			attachment, err := s.CreateAttachment(ctx, db.CreateAttachmentParams{MessageID: old.ID, RoomID: room.ID, SenderID: peer.ID, Kind: "document", OriginalName: "test.pdf", StoredName: "test.pdf", MimeType: "application/pdf", SizeBytes: 100})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.ClearChat(ctx, room.ID, self.ID); err != nil {
				t.Fatal(err)
			}
			listed, err := s.ListRoomsForUser(ctx, db.ListRoomsForUserParams{UserID: self.ID, Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range listed {
				if r.ID == room.ID {
					t.Fatal("deleted room is listed")
				}
			}
			if _, err = s.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: room.ID, UserID: self.ID}); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("hidden membership err=%v", err)
			}
			if _, err = s.GetRoomMember(ctx, db.GetRoomMemberParams{RoomID: room.ID, UserID: peer.ID}); err != nil {
				t.Fatal("peer lost membership", err)
			}
			if visible, err := s.AttachmentVisible(ctx, attachment.ID, self.ID); err != nil || visible {
				t.Fatalf("deleted file visible=%v err=%v", visible, err)
			}
			if visible, err := s.AttachmentVisible(ctx, attachment.ID, peer.ID); err != nil || !visible {
				t.Fatalf("peer file visible=%v err=%v", visible, err)
			}
			if kind == "dm" {
				reopened, err := s.GetOrCreateDirectRoom(ctx, self.ID, peer.ID)
				if err != nil || reopened.ID != room.ID {
					t.Fatalf("reopened=%v err=%v", reopened.ID, err)
				}
			} else {
				if _, err = s.JoinGroupRoom(ctx, db.JoinGroupRoomParams{RoomID: room.ID, UserID: self.ID}); err != nil {
					t.Fatal(err)
				}
			}
			history, err := s.ListRoomHistory(ctx, db.ListRoomHistoryParams{RoomID: room.ID, UserID: self.ID, PageLimit: 50})
			if err != nil || len(history) != 0 {
				t.Fatalf("old history returned=%d err=%v", len(history), err)
			}
			newer, err := s.CreateMemberMessage(ctx, db.CreateMemberMessageParams{RoomID: room.ID, SenderID: peer.ID, Body: "new"})
			if err != nil {
				t.Fatal(err)
			}
			history, err = s.ListRoomHistory(ctx, db.ListRoomHistoryParams{RoomID: room.ID, UserID: self.ID, PageLimit: 50})
			if err != nil || len(history) != 1 || history[0].ID != newer.ID {
				t.Fatalf("new history=%v err=%v", history, err)
			}
			peerHistory, err := s.ListRoomHistory(ctx, db.ListRoomHistoryParams{RoomID: room.ID, UserID: peer.ID, PageLimit: 50})
			if err != nil || len(peerHistory) != 2 {
				t.Fatalf("peer history=%d err=%v", len(peerHistory), err)
			}
			if visible, err := s.AttachmentVisible(ctx, attachment.ID, self.ID); err != nil || visible {
				t.Fatalf("reopened old file visible=%v err=%v", visible, err)
			}
			late, err := s.CreateMessage(ctx, db.CreateMessageParams{RoomID: room.ID, SenderID: peer.ID, Type: "document"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Pool.Exec(ctx, "UPDATE attachments SET message_id=$1 WHERE id=$2", late.ID, attachment.ID); err != nil {
				t.Fatal(err)
			}
			if visible, err := s.MessageVisible(ctx, late.ID, self.ID); err != nil || visible {
				t.Fatalf("old upload published late visible=%v err=%v", visible, err)
			}
			history, err = s.ListRoomHistory(ctx, db.ListRoomHistoryParams{RoomID: room.ID, UserID: self.ID, PageLimit: 50})
			if err != nil || len(history) != 1 {
				t.Fatalf("late upload resurrected history: %d %v", len(history), err)
			}
			summaries, err := s.ListRoomSummaries(ctx, db.ListRoomSummariesParams{UserID: self.ID, RoomIds: []pgtype.UUID{room.ID}})
			if err != nil || len(summaries) != 1 || summaries[0].LastMessageID != newer.ID {
				t.Fatalf("summary resurrected upload: %v %v", summaries, err)
			}

		})
	}
}

func TestDeleteAccountFilesIntegration(t *testing.T) {
	s, ctx := discoveryStore(t)
	disk, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	self, err := s.CreateUser(ctx, db.CreateUserParams{Username: "self", Email: "self@example.test", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := s.CreateUser(ctx, db.CreateUserParams{Username: "peer", Email: "peer@example.test", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := s.GetOrCreateDirectRoom(ctx, self.ID, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	type object struct {
		area storage.Area
		name string
	}
	var objects []object
	for _, kind := range []string{"image", "document", "voice"} {
		msg, err := s.CreateMessage(ctx, db.CreateMessageParams{RoomID: room.ID, SenderID: self.ID, Type: kind})
		if err != nil {
			t.Fatal(err)
		}
		names := map[storage.Area][]string{}
		for _, area := range []storage.Area{storage.Quarantine, storage.Clean} {
			for range 2 {
				o, err := disk.Save(ctx, area, bytes.NewReader([]byte("private file")))
				if err != nil {
					t.Fatal(err)
				}
				objects = append(objects, object{area, o.Name})
				names[area] = append(names[area], o.Name)
			}
		}
		_, err = s.Pool.Exec(ctx, `INSERT INTO attachments(message_id,room_id,sender_id,kind,original_name,stored_name,mime_type,size_bytes,status,thumbnail_name,quarantine_name)
   VALUES($1,$2,$3,$4,'file',$5,'application/octet-stream',12,'ready',$6,$7)`, msg.ID, room.ID, self.ID, kind, names[storage.Clean][0], names[storage.Clean][1], names[storage.Quarantine][0])
		if err != nil {
			t.Fatal(err)
		}
		// Only referenced files are owned by the account; track those three objects.
		objects = objects[:len(objects)-4]
		objects = append(objects, object{storage.Clean, names[storage.Clean][0]}, object{storage.Clean, names[storage.Clean][1]}, object{storage.Quarantine, names[storage.Quarantine][0]})
	}
	if _, err = s.CreateMemberMessage(ctx, db.CreateMemberMessageParams{RoomID: room.ID, SenderID: peer.ID, Body: "keep peer message"}); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAccount(ctx, self.ID, disk); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetUser(ctx, self.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("account remains: %v", err)
	}
	for _, o := range objects {
		file, err := disk.Open(ctx, o.area, o.name)
		if err == nil {
			file.Close()
			t.Fatalf("file remains: %s", o.name)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	var ownMessages, ownAttachments, peerMessages, outbox int
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE sender_id=$1", self.ID).Scan(&ownMessages); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM attachments WHERE sender_id=$1", self.ID).Scan(&ownAttachments); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE sender_id=$1", peer.ID).Scan(&peerMessages); err != nil {
		t.Fatal(err)
	}
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM storage_deletions").Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if ownMessages != 0 || ownAttachments != 0 || peerMessages != 1 || outbox != 0 {
		t.Fatalf("messages=%d attachments=%d peer=%d outbox=%d", ownMessages, ownAttachments, peerMessages, outbox)
	}
}

type failingCleanupStorage struct{ storage.Storage }

func (failingCleanupStorage) Delete(context.Context, storage.Area, string) error {
	return errors.New("temporary storage failure")
}
func TestAccountCleanupRetryIntegration(t *testing.T) {
	s, ctx := discoveryStore(t)
	disk, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	self, err := s.CreateUser(ctx, db.CreateUserParams{Username: "self", Email: "self@example.test", PasswordHash: "hash"})
	if err != nil {
		t.Fatal(err)
	}
	room, err := s.CreateGroupRoom(ctx, "Group", self.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	file, err := disk.Save(ctx, storage.Quarantine, bytes.NewReader([]byte("pending")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreatePendingUpload(ctx, db.CreatePendingUploadParams{RoomID: room.ID, SenderID: self.ID, Kind: "document", OriginalName: "file.pdf", StoredName: file.Name, MimeType: "application/pdf", SizeBytes: file.Size})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAccount(ctx, self.ID, failingCleanupStorage{disk}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetUser(ctx, self.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("account was not deleted", err)
	}
	var pending int
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM storage_deletions").Scan(&pending); err != nil || pending == 0 {
		t.Fatalf("outbox=%d err=%v", pending, err)
	}
	if err = s.CleanupDeletedFiles(ctx, disk); err != nil {
		t.Fatal(err)
	}
	if file, err := disk.Open(ctx, storage.Quarantine, file.Name); err == nil {
		file.Close()
		t.Fatal("pending file survived cleanup retry")
	}
}
