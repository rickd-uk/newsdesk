package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Article struct {
	ID          int
	Site        string
	URL         string
	Category    string
	Title       string
	Author      string
	PublishDate string
	Tags        string
	Content     string
	ScrapedAt   string
}

type QueryParams struct {
	Q        string
	Site     string
	Category string
	Offset   int
	Limit    int
}

type DB struct {
	*sql.DB
}

func OpenDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA journal_mode=WAL;")
	return &DB{db}, nil
}

func (db *DB) InitFTS() error {
	_, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts
		USING fts5(title, body, tags)`)
	if err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM articles_fts").Scan(&count)
	if count == 0 {
		_, err = db.Exec(`INSERT INTO articles_fts(rowid, title, body, tags)
			SELECT id, COALESCE(title,''), COALESCE(content,''), COALESCE(tags,'')
			FROM articles`)
		if err != nil {
			return fmt.Errorf("populate fts: %w", err)
		}
	}
	return nil
}

func (db *DB) GetSites() ([]string, error) {
	rows, err := db.Query(
		"SELECT DISTINCT site FROM articles WHERE site != '' ORDER BY site")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return out, nil
}

func (db *DB) GetCategories() ([]string, error) {
	rows, err := db.Query(
		"SELECT DISTINCT category FROM articles WHERE category != '' ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		out = append(out, c)
	}
	return out, nil
}

func (db *DB) QueryArticles(p QueryParams) ([]Article, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if p.Q != "" {
		q := `SELECT COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
			COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
			COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
			COALESCE(a.scraped_at,'')
			FROM articles_fts f
			JOIN articles a ON a.id = f.rowid
			WHERE articles_fts MATCH ?`
		args := []interface{}{strings.TrimSpace(p.Q) + "*"}
		if p.Site != "" {
			q += " AND a.site = ?"
			args = append(args, p.Site)
		}
		if p.Category != "" {
			q += " AND a.category = ?"
			args = append(args, p.Category)
		}
		q += " ORDER BY a.publish_date DESC, a.scraped_at DESC LIMIT ? OFFSET ?"
		args = append(args, p.Limit, p.Offset)
		rows, err = db.Query(q, args...)
	} else {
		q := `SELECT COALESCE(id,0), COALESCE(site,''), COALESCE(url,''),
			COALESCE(category,''), COALESCE(title,''), COALESCE(author,''),
			COALESCE(publish_date,''), COALESCE(tags,''), COALESCE(content,''),
			COALESCE(scraped_at,'') FROM articles WHERE 1=1`
		args := []interface{}{}
		if p.Site != "" {
			q += " AND site = ?"
			args = append(args, p.Site)
		}
		if p.Category != "" {
			q += " AND category = ?"
			args = append(args, p.Category)
		}
		q += " ORDER BY publish_date DESC, scraped_at DESC LIMIT ? OFFSET ?"
		args = append(args, p.Limit, p.Offset)
		rows, err = db.Query(q, args...)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
			&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) GetArticleByID(id int) (*Article, error) {
	row := db.QueryRow(`SELECT COALESCE(id,0), COALESCE(site,''), COALESCE(url,''),
		COALESCE(category,''), COALESCE(title,''), COALESCE(author,''),
		COALESCE(publish_date,''), COALESCE(tags,''), COALESCE(content,''),
		COALESCE(scraped_at,'') FROM articles WHERE id = ?`, id)
	var a Article
	if err := row.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
		&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt); err != nil {
		return nil, err
	}
	return &a, nil
}
