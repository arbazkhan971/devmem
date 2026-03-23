package memory_test

import (
	"fmt"
	"testing"
)

func TestTrackFile_StoresCorrectly(t *testing.T) {
	store := newTestStore(t)

	f, err := store.CreateFeature("feat-track", "Track files test")
	if err != nil {
		t.Fatalf("CreateFeature: %v", err)
	}
	sess, err := store.CreateSession(f.ID, "test-tool")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Track a file
	if err := store.TrackFile(f.ID, sess.ID, "internal/memory/files.go", "modified"); err != nil {
		t.Fatalf("TrackFile: %v", err)
	}

	// Verify it was stored
	files, err := store.GetFilesTouched(f.ID, 10)
	if err != nil {
		t.Fatalf("GetFilesTouched: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "internal/memory/files.go" {
		t.Errorf("expected path 'internal/memory/files.go', got %q", files[0].Path)
	}
	if files[0].Action != "modified" {
		t.Errorf("expected action 'modified', got %q", files[0].Action)
	}
	if files[0].FeatureID != f.ID {
		t.Errorf("expected feature ID %q, got %q", f.ID, files[0].FeatureID)
	}
	if files[0].SessionID != sess.ID {
		t.Errorf("expected session ID %q, got %q", sess.ID, files[0].SessionID)
	}
	if files[0].FirstSeen == "" {
		t.Error("expected non-empty FirstSeen")
	}
}

func TestTrackFile_DefaultAction(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-default-action", "Default action test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	// Track with empty action should default to "modified"
	if err := store.TrackFile(f.ID, sess.ID, "main.go", ""); err != nil {
		t.Fatalf("TrackFile: %v", err)
	}

	files, err := store.GetFilesTouched(f.ID, 10)
	if err != nil {
		t.Fatalf("GetFilesTouched: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Action != "modified" {
		t.Errorf("expected default action 'modified', got %q", files[0].Action)
	}
}

func TestGetFilesTouched_ReturnsCorrectFiles(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-files", "Files test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	// Track multiple files
	paths := []string{"file1.go", "file2.go", "file3.go"}
	for _, p := range paths {
		if err := store.TrackFile(f.ID, sess.ID, p, "added"); err != nil {
			t.Fatalf("TrackFile(%s): %v", p, err)
		}
	}

	files, err := store.GetFilesTouched(f.ID, 10)
	if err != nil {
		t.Fatalf("GetFilesTouched: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// Verify all paths are present
	gotPaths := make(map[string]bool)
	for _, fr := range files {
		gotPaths[fr.Path] = true
	}
	for _, p := range paths {
		if !gotPaths[p] {
			t.Errorf("missing path %q in results", p)
		}
	}
}

func TestGetFilesTouched_Limit(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-limit", "Limit test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	for i := 0; i < 5; i++ {
		store.TrackFile(f.ID, sess.ID, fmt.Sprintf("file%d.go", i), "modified")
	}

	files, err := store.GetFilesTouched(f.ID, 2)
	if err != nil {
		t.Fatalf("GetFilesTouched: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files (limited), got %d", len(files))
	}
}

func TestGetFilesTouched_FiltersByFeature(t *testing.T) {
	store := newTestStore(t)

	fa, _ := store.CreateFeature("feat-a", "A")
	fb, _ := store.CreateFeature("feat-b", "B")
	sess, _ := store.CreateSession(fa.ID, "test-tool")

	store.TrackFile(fa.ID, sess.ID, "a.go", "modified")
	store.TrackFile(fb.ID, "", "b.go", "modified")

	files, err := store.GetFilesTouched(fa.ID, 10)
	if err != nil {
		t.Fatalf("GetFilesTouched: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file for feat-a, got %d", len(files))
	}
	if files[0].Path != "a.go" {
		t.Errorf("expected 'a.go', got %q", files[0].Path)
	}
}

func TestTrackFile_DuplicateDoesNotError(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-dup", "Duplicate test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	// Track the same file twice with same feature+session+path
	if err := store.TrackFile(f.ID, sess.ID, "main.go", "modified"); err != nil {
		t.Fatalf("first TrackFile: %v", err)
	}
	if err := store.TrackFile(f.ID, sess.ID, "main.go", "modified"); err != nil {
		t.Fatalf("second TrackFile (duplicate) should not error: %v", err)
	}

	// Should still only have one record
	files, err := store.GetFilesTouched(f.ID, 10)
	if err != nil {
		t.Fatalf("GetFilesTouched: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (duplicate ignored), got %d", len(files))
	}
}

func TestGetSessionFiles(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-sess-files", "Session files test")
	s1, _ := store.CreateSession(f.ID, "tool-1")
	s2, _ := store.CreateSession(f.ID, "tool-2")

	store.TrackFile(f.ID, s1.ID, "file1.go", "modified")
	store.TrackFile(f.ID, s1.ID, "file2.go", "added")
	store.TrackFile(f.ID, s2.ID, "file3.go", "deleted")

	// Get files for session 1
	files, err := store.GetSessionFiles(s1.ID)
	if err != nil {
		t.Fatalf("GetSessionFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files for session 1, got %d", len(files))
	}

	// Get files for session 2
	files2, err := store.GetSessionFiles(s2.ID)
	if err != nil {
		t.Fatalf("GetSessionFiles: %v", err)
	}
	if len(files2) != 1 {
		t.Fatalf("expected 1 file for session 2, got %d", len(files2))
	}
	if files2[0].Path != "file3.go" {
		t.Errorf("expected 'file3.go', got %q", files2[0].Path)
	}
	if files2[0].Action != "deleted" {
		t.Errorf("expected action 'deleted', got %q", files2[0].Action)
	}
}

func TestTrackFile_Actions(t *testing.T) {
	store := newTestStore(t)

	f, _ := store.CreateFeature("feat-actions", "Actions test")
	sess, _ := store.CreateSession(f.ID, "test-tool")

	for _, action := range []string{"modified", "added", "deleted"} {
		t.Run(action, func(t *testing.T) {
			path := action + "_file.go"
			if err := store.TrackFile(f.ID, sess.ID, path, action); err != nil {
				t.Fatalf("TrackFile(%s): %v", action, err)
			}
			files, _ := store.GetSessionFiles(sess.ID)
			found := false
			for _, fr := range files {
				if fr.Path == path && fr.Action == action {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("did not find file %q with action %q", path, action)
			}
		})
	}
}
