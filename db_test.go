package main

import (
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE articles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		url TEXT, fingerprint TEXT UNIQUE,
		title TEXT, author TEXT, publish_date TEXT,
		tags TEXT, content TEXT, scraped_at DATETIME,
		category TEXT, site TEXT
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO articles
		(site, url, fingerprint, title, author, publish_date, tags, content, category) VALUES
		('Japan Times','http://ex.com/1','fp1','Article One','Auth A','2026-04-01','tech,science','First content paragraph.\n\nSecond paragraph.','tech'),
		('The Guardian','http://ex.com/2','fp2','Article Two','','2026-04-02','','Guardian content here','uk-news'),
		('Japan Times','http://ex.com/3','fp3','Article Three','Auth B','2026-03-15','','Old article content','health')`)
	if err != nil {
		t.Fatalf("insert fixtures: %v", err)
	}
	if err := db.InitFTS(); err != nil {
		t.Fatalf("InitFTS: %v", err)
	}
	return db
}

func TestGetSites(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	sites, err := db.GetSites()
	if err != nil {
		t.Fatalf("GetSites: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("want 2 sites, got %d: %v", len(sites), sites)
	}
}

func TestGetCategories(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	cats, err := db.GetCategories()
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("want 3 categories, got %d: %v", len(cats), cats)
	}
}

func TestQueryArticles_NoFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("want 3 articles, got %d", len(arts))
	}
	// Newest first
	if arts[0].PublishDate != "2026-04-02" {
		t.Fatalf("want newest first, got %s", arts[0].PublishDate)
	}
}

func TestQueryArticles_SiteFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Site: "Japan Times", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("want 2 Japan Times articles, got %d", len(arts))
	}
}

func TestQueryArticles_Search(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Q: "Guardian", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles search: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 result for 'Guardian', got %d", len(arts))
	}
}

func TestQueryArticles_Offset(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Offset: 2, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 article at offset 2, got %d", len(arts))
	}
}

func TestGetArticleByID(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	a, err := db.GetArticleByID(1)
	if err != nil {
		t.Fatalf("GetArticleByID: %v", err)
	}
	if a.Title != "Article One" {
		t.Fatalf("want 'Article One', got %q", a.Title)
	}
}

func TestGetArticleByID_NotFound(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	_, err := db.GetArticleByID(9999)
	if err == nil {
		t.Fatal("want error for missing ID, got nil")
	}
}

func TestQueryArticles_CombinedFilter(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Site: "Japan Times", Category: "tech", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles combined: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 article for Japan Times+tech, got %d", len(arts))
	}
	if arts[0].Title != "Article One" {
		t.Fatalf("want 'Article One', got %q", arts[0].Title)
	}
}
