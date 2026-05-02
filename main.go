package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

//go:embed templates static
var embeddedFiles embed.FS

func main() {
	dbPath := flag.String("db", "articles.db", "Path to SQLite database")
	addr := flag.String("addr", ":8080", "Address to listen on")
	allowSignups := flag.Bool("allow-signups", envBool("NEWSDESK_ALLOW_SIGNUPS"), "Allow new user signup")
	flag.Parse()

	db, err := OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.InitFTS(); err != nil {
		log.Fatalf("init fts: %v", err)
	}
	if err := db.InitUsersTable(); err != nil {
		log.Fatalf("init users table: %v", err)
	}
	if err := db.InitSessionsTable(); err != nil {
		log.Fatalf("init sessions table: %v", err)
	}
	if err := db.InitReadTable(); err != nil {
		log.Fatalf("init read table: %v", err)
	}
	if err := db.InitFavoritesTable(); err != nil {
		log.Fatalf("init favorites table: %v", err)
	}
	if err := db.InitArchivesTable(); err != nil {
		log.Fatalf("init archives table: %v", err)
	}
	if err := db.InitNotesTable(); err != nil {
		log.Fatalf("init notes table: %v", err)
	}

	tmpl, err := mustParseTemplatesFS(embeddedFiles)
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	srv := &Server{db: db, tmpl: tmpl, signupEnabled: *allowSignups}

	staticFS, _ := fs.Sub(embeddedFiles, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/articles", srv.handleArticles)
	http.HandleFunc("/article/", srv.handleArticleDispatch)
	http.HandleFunc("/signup", srv.handleSignup)
	http.HandleFunc("/login", srv.handleLogin)
	http.HandleFunc("/logout", srv.handleLogout)
	http.HandleFunc("/", srv.handleIndex)

	fmt.Printf("article-viewer listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func envBool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
