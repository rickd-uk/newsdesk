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
	if err := db.InitUsersTable(); err != nil {
		t.Fatalf("InitUsersTable: %v", err)
	}
	if err := db.InitSessionsTable(); err != nil {
		t.Fatalf("InitSessionsTable: %v", err)
	}
	if err := db.InitReadTable(); err != nil {
		t.Fatalf("InitReadTable: %v", err)
	}
	if err := db.InitFavoritesTable(); err != nil {
		t.Fatalf("InitFavoritesTable: %v", err)
	}
	return db
}

func testUser(t *testing.T, db *DB, username string) *User {
	t.Helper()
	user, err := db.CreateUser(username, "", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return user
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

func TestGetCategoryInfos(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	cats, err := db.GetCategoryInfos()
	if err != nil {
		t.Fatalf("GetCategoryInfos: %v", err)
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

func TestQueryArticles_SearchPrefixesEveryTerm(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	arts, err := db.QueryArticles(QueryParams{Q: "Guard cont", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles search: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("want 1 result for 'Guard cont', got %d", len(arts))
	}
	if arts[0].Title != "Article Two" {
		t.Fatalf("want Article Two, got %q", arts[0].Title)
	}
}

func TestInitFTSRebuildsStaleIndex(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

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
		(site, url, fingerprint, title, content) VALUES
		('Japan Times','http://ex.com/1','fp1','Indexed Article','already indexed'),
		('Ars Technica','http://ex.com/2','fp2','Missing Article','fresh ars content')`)
	if err != nil {
		t.Fatalf("insert fixtures: %v", err)
	}
	_, err = db.Exec(`CREATE VIRTUAL TABLE articles_fts USING fts5(title, body, tags)`)
	if err != nil {
		t.Fatalf("create stale fts: %v", err)
	}
	_, err = db.Exec(`INSERT INTO articles_fts(rowid, title, body, tags)
		VALUES (1, 'Indexed Article', 'already indexed', '')`)
	if err != nil {
		t.Fatalf("insert stale fts: %v", err)
	}

	if err := db.InitFTS(); err != nil {
		t.Fatalf("InitFTS: %v", err)
	}

	var articleCount, ftsCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM articles").Scan(&articleCount); err != nil {
		t.Fatalf("article count: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM articles_fts").Scan(&ftsCount); err != nil {
		t.Fatalf("fts count: %v", err)
	}
	if articleCount != 2 || ftsCount != articleCount {
		t.Fatalf("want 2 synced rows, got articles=%d fts=%d", articleCount, ftsCount)
	}

	if err := db.InitReadTable(); err != nil {
		t.Fatalf("InitReadTable: %v", err)
	}
	if err := db.InitFavoritesTable(); err != nil {
		t.Fatalf("InitFavoritesTable: %v", err)
	}
	arts, err := db.QueryArticles(QueryParams{Q: "fresh ars", Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles search: %v", err)
	}
	if len(arts) != 1 || arts[0].Site != "Ars Technica" {
		t.Fatalf("want rebuilt Ars Technica row, got %#v", arts)
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
	a, err := db.GetArticleByID(1, 0)
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
	_, err := db.GetArticleByID(9999, 0)
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

func TestQueryArticles_ReadAndFavoriteAreUserScoped(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	userA := testUser(t, db, "usera")
	userB := testUser(t, db, "userb")

	if err := db.MarkRead(userA.ID, 1); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := db.MarkFavorite(userA.ID, 1); err != nil {
		t.Fatalf("MarkFavorite: %v", err)
	}

	artsA, err := db.QueryArticles(QueryParams{UserID: userA.ID, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles user A: %v", err)
	}
	artsB, err := db.QueryArticles(QueryParams{UserID: userB.ID, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles user B: %v", err)
	}

	var gotA, gotB *Article
	for i := range artsA {
		if artsA[i].ID == 1 {
			gotA = &artsA[i]
		}
	}
	for i := range artsB {
		if artsB[i].ID == 1 {
			gotB = &artsB[i]
		}
	}
	if gotA == nil || !gotA.Read || !gotA.Favorited {
		t.Fatalf("user A should see article 1 as read and favorited, got %#v", gotA)
	}
	if gotB == nil || gotB.Read || gotB.Favorited {
		t.Fatalf("user B should not inherit user A state, got %#v", gotB)
	}
}
