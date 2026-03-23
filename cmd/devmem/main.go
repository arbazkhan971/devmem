package main

import (
	"fmt"
	"log"
	"os"

	"github.com/arbaz/devmem/internal/git"
	devmem "github.com/arbaz/devmem/internal/mcp"
	"github.com/arbaz/devmem/internal/storage"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}

	gitRoot, err := git.FindGitRoot(cwd)
	if err != nil {
		log.Fatalf("not a git repository: %v", err)
	}

	memDir, err := git.EnsureMemoryDir(gitRoot)
	if err != nil {
		log.Fatalf("setup memory directory: %v", err)
	}

	dbPath := memDir + "/memory.db"
	db, err := storage.NewDB(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	fmt.Fprintf(os.Stderr, "devmem: initialized at %s (project: %s)\n", memDir, git.ProjectName(gitRoot))

	srv := devmem.NewServer(db, gitRoot)
	if err := srv.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
