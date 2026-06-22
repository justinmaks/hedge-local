package store

import "testing"

func TestProjectUpsert_new(t *testing.T) {
	s := tempDB(t)
	id, err := s.ProjectUpsert("/home/user/repo", "repo")
	if err != nil {
		t.Fatalf("ProjectUpsert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero project ID")
	}
}

func TestProjectUpsert_idempotent(t *testing.T) {
	s := tempDB(t)
	id1, _ := s.ProjectUpsert("/home/user/repo", "repo")
	id2, _ := s.ProjectUpsert("/home/user/repo", "repo")
	if id1 != id2 {
		t.Errorf("expected same ID on re-upsert: got %d then %d", id1, id2)
	}
}

func TestProjectUpsert_differentPaths(t *testing.T) {
	s := tempDB(t)
	id1, _ := s.ProjectUpsert("/home/user/repo-a", "repo-a")
	id2, _ := s.ProjectUpsert("/home/user/repo-b", "repo-b")
	if id1 == id2 {
		t.Error("expected different IDs for different paths")
	}
}
