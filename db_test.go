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
	if err := db.InitArchivesTable(); err != nil {
		t.Fatalf("InitArchivesTable: %v", err)
	}
	if err := db.InitNotesTable(); err != nil {
		t.Fatalf("InitNotesTable: %v", err)
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

func TestBuildCategoryTree_HidesDuplicateFlatGroupLabels(t *testing.T) {
	groups := BuildCategoryTree([]CategoryInfo{
		{Name: "crime", Sites: "Japan Today"},
		{Name: "business_economy", Sites: "Example"},
	})
	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %#v", groups)
	}

	flatSeen := false
	nestedSeen := false
	for _, group := range groups {
		if group.Label == "Crime" && len(group.Pills) == 1 && group.Pills[0].Label == "crime" {
			flatSeen = true
			if group.ShowLabel {
				t.Fatalf("flat duplicate category should hide group label: %#v", group)
			}
		}
		if group.Label == "Business" && len(group.Pills) == 1 && group.Pills[0].Label == "economy" {
			nestedSeen = true
			if !group.ShowLabel {
				t.Fatalf("hierarchical category should show group label: %#v", group)
			}
		}
	}
	if !flatSeen || !nestedSeen {
		t.Fatalf("did not find expected flat/nested business cases: %#v", groups)
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
	if err := db.InitArchivesTable(); err != nil {
		t.Fatalf("InitArchivesTable: %v", err)
	}
	if err := db.InitNotesTable(); err != nil {
		t.Fatalf("InitNotesTable: %v", err)
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

func TestQueryArticles_ReadsOnlyOrdersByReadAt(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := testUser(t, db, "reader")
	if err := db.MarkRead(user.ID, 1); err != nil {
		t.Fatalf("MarkRead 1: %v", err)
	}
	if err := db.MarkRead(user.ID, 3); err != nil {
		t.Fatalf("MarkRead 3: %v", err)
	}
	if _, err := db.Exec(`UPDATE article_reads SET read_at = ? WHERE user_id = ? AND article_id = ?`, "2026-04-20 10:00:00", user.ID, 1); err != nil {
		t.Fatalf("set read_at 1: %v", err)
	}
	if _, err := db.Exec(`UPDATE article_reads SET read_at = ? WHERE user_id = ? AND article_id = ?`, "2026-04-21 10:00:00", user.ID, 3); err != nil {
		t.Fatalf("set read_at 3: %v", err)
	}

	arts, err := db.QueryArticles(QueryParams{UserID: user.ID, ReadsOnly: true, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles ReadsOnly: %v", err)
	}
	if len(arts) != 2 {
		t.Fatalf("want 2 read articles, got %d", len(arts))
	}
	if arts[0].ID != 3 || arts[0].ReadAt != "2026-04-21 10:00:00" {
		t.Fatalf("want latest read first, got %#v", arts[0])
	}
	if arts[1].ID != 1 || arts[1].ReadAt != "2026-04-20 10:00:00" {
		t.Fatalf("want older read second, got %#v", arts[1])
	}
}

func TestQueryArticles_ArchiveHidesFromDefaultAndShowsArchivedOnly(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := testUser(t, db, "archiver")
	if err := db.MarkArchived(user.ID, 1); err != nil {
		t.Fatalf("MarkArchived: %v", err)
	}

	active, err := db.QueryArticles(QueryParams{UserID: user.ID, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles active: %v", err)
	}
	for _, a := range active {
		if a.ID == 1 {
			t.Fatalf("archived article should be hidden from default feed, got %#v", a)
		}
	}

	archived, err := db.QueryArticles(QueryParams{UserID: user.ID, ArchivedOnly: true, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles archived: %v", err)
	}
	if len(archived) != 1 || archived[0].ID != 1 || !archived[0].Archived {
		t.Fatalf("want only archived article 1, got %#v", archived)
	}

	if err := db.UnmarkArchived(user.ID, 1); err != nil {
		t.Fatalf("UnmarkArchived: %v", err)
	}
	active, err = db.QueryArticles(QueryParams{UserID: user.ID, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles after unarchive: %v", err)
	}
	found := false
	for _, a := range active {
		if a.ID == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("unarchived article should return to default feed")
	}
}

func TestArticleNotesAreUserScopedAndSearchable(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	userA := testUser(t, db, "notea")
	userB := testUser(t, db, "noteb")
	if err := db.SaveNote(userA.ID, 2, "follow up on solar battery angle"); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}

	article, err := db.GetArticleByID(2, userA.ID)
	if err != nil {
		t.Fatalf("GetArticleByID user A: %v", err)
	}
	if article.Note != "follow up on solar battery angle" {
		t.Fatalf("want user A note, got %q", article.Note)
	}
	article, err = db.GetArticleByID(2, userB.ID)
	if err != nil {
		t.Fatalf("GetArticleByID user B: %v", err)
	}
	if article.Note != "" {
		t.Fatalf("user B should not see user A note, got %q", article.Note)
	}

	arts, err := db.QueryArticles(QueryParams{UserID: userA.ID, Q: "solar battery", Fields: []string{"notes"}, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles notes search: %v", err)
	}
	if len(arts) != 1 || arts[0].ID != 2 || arts[0].Note == "" {
		t.Fatalf("want article 2 from notes search, got %#v", arts)
	}

	arts, err = db.QueryArticles(QueryParams{UserID: userB.ID, Q: "solar battery", Fields: []string{"notes"}, Limit: 20})
	if err != nil {
		t.Fatalf("QueryArticles user B notes search: %v", err)
	}
	if len(arts) != 0 {
		t.Fatalf("user B should not match user A note, got %#v", arts)
	}
}

func TestSaveNoteEmptyDeletesNote(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	user := testUser(t, db, "noteclear")
	if err := db.SaveNote(user.ID, 1, "temporary note"); err != nil {
		t.Fatalf("SaveNote: %v", err)
	}
	if err := db.SaveNote(user.ID, 1, "   "); err != nil {
		t.Fatalf("SaveNote empty: %v", err)
	}
	article, err := db.GetArticleByID(1, user.ID)
	if err != nil {
		t.Fatalf("GetArticleByID: %v", err)
	}
	if article.Note != "" {
		t.Fatalf("want note deleted, got %q", article.Note)
	}
}
