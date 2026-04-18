package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
)

//go:embed templates static
var embeddedFiles embed.FS

func main() {
	dbPath := flag.String("db", "articles.db", "Path to SQLite database")
	addr := flag.String("addr", ":8080", "Address to listen on")
	flag.Parse()

	db, err := OpenDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.InitFTS(); err != nil {
		log.Fatalf("init fts: %v", err)
	}
	if err := db.InitReadTable(); err != nil {
		log.Fatalf("init read table: %v", err)
	}
	if err := db.InitFavoritesTable(); err != nil {
		log.Fatalf("init favorites table: %v", err)
	}

	tmpl, err := mustParseTemplatesFS(embeddedFiles)
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	srv := &Server{db: db, tmpl: tmpl}

	staticFS, _ := fs.Sub(embeddedFiles, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	http.HandleFunc("/articles", srv.handleArticles)
	http.HandleFunc("/article/", srv.handleArticleDispatch)
	http.HandleFunc("/", srv.handleIndex)

	fmt.Printf("article-viewer listening on %s\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
