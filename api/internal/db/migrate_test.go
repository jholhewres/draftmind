package db

import (
	"testing"
	"testing/fstest"
)

func TestReadMigrationsSortsAndSkipsOthers(t *testing.T) {
	fsys := fstest.MapFS{
		"m/0002_b.sql": {Data: []byte("b")},
		"m/0001_a.sql": {Data: []byte("a")},
		"m/README.md":  {Data: []byte("x")},
	}
	got, err := readMigrations(fsys, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "0001_a.sql" || got[1].Name != "0002_b.sql" {
		t.Fatalf("unexpected order: %+v", got)
	}
}

func TestEmbeddedMigrationsLoad(t *testing.T) {
	migs, err := Migrations()
	if err != nil || len(migs) == 0 {
		t.Fatalf("embedded migrations: %v, %d", err, len(migs))
	}
}
