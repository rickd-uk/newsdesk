package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"unicode"

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
	Read        bool
	ReadAt      string
	Favorited   bool
	Archived    bool
	Note        string
}

type ArticleHighlight struct {
	ID          int
	UserID      int
	ArticleID   int
	Site        string
	Title       string
	PublishDate string
	Snippet     string
	Prefix      string
	Suffix      string
	CreatedAt   string
}

type QueryParams struct {
	UserID        int
	Q             string
	Site          string
	Category      string
	Author        string
	DateFrom      string
	DateTo        string
	Fields        []string // "title","body","tags","notes" — nil/empty means all
	HideRead      bool
	ReadsOnly     bool
	FavoritesOnly bool
	ArchivedOnly  bool
	HideNotes     bool
	Offset        int
	Limit         int
}

func buildFTSPrefix(fields []string) string {
	valid := map[string]bool{"title": true, "body": true, "tags": true}
	var cols []string
	for _, f := range fields {
		if valid[f] {
			cols = append(cols, f)
		}
	}
	if len(cols) == 0 || len(cols) == 3 {
		return ""
	}
	if len(cols) == 1 {
		return cols[0] + ":"
	}
	return "{" + strings.Join(cols, " ") + "}:"
}

func buildFTSQuery(fields []string, raw string) string {
	var terms []string
	for _, term := range strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		term = strings.TrimSpace(term)
		if term != "" {
			terms = append(terms, term+"*")
		}
	}
	if len(terms) == 0 {
		return ""
	}
	return buildFTSPrefix(fields) + strings.Join(terms, " ")
}

func searchTerms(raw string) []string {
	var terms []string
	for _, term := range strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		term = strings.TrimSpace(term)
		if term != "" {
			terms = append(terms, term)
		}
	}
	return terms
}

func articleSearchFields(fields []string) ([]string, bool) {
	if len(fields) == 0 {
		return nil, true
	}
	valid := map[string]bool{"title": true, "body": true, "tags": true}
	var out []string
	for _, f := range fields {
		if valid[f] {
			out = append(out, f)
		}
	}
	return out, len(out) > 0
}

func searchIncludesNotes(fields []string) bool {
	if len(fields) == 0 {
		return true
	}
	for _, f := range fields {
		if f == "notes" {
			return true
		}
	}
	return false
}

func appendSearchCondition(q string, args []interface{}, p QueryParams) (string, []interface{}, bool) {
	raw := strings.TrimSpace(p.Q)
	if raw == "" {
		return q, args, true
	}
	terms := searchTerms(raw)
	if len(terms) == 0 {
		return q + " AND 0=1", args, false
	}

	var clauses []string
	if fields, ok := articleSearchFields(p.Fields); ok {
		if ftsQ := buildFTSQuery(fields, raw); ftsQ != "" {
			clauses = append(clauses, "a.id IN (SELECT rowid FROM articles_fts WHERE articles_fts MATCH ?)")
			args = append(args, ftsQ)
		}
	}
	if searchIncludesNotes(p.Fields) && p.UserID != 0 {
		var noteClauses []string
		for _, term := range terms {
			noteClauses = append(noteClauses, "n.note LIKE ?")
			args = append(args, "%"+term+"%")
		}
		clauses = append(clauses, "("+strings.Join(noteClauses, " AND ")+")")
	}
	if len(clauses) == 0 {
		return q + " AND 0=1", args, false
	}
	return q + " AND (" + strings.Join(clauses, " OR ") + ")", args, true
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
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("warn: WAL mode not set: %v", err)
	}
	return &DB{db}, nil
}

// InitFTS creates or repairs the standalone FTS5 index from articles.
func (db *DB) InitFTS() error {
	if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts
		USING fts5(title, body, tags)`); err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}

	var articleCount, ftsCount, missingCount, orphanCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM articles").Scan(&articleCount); err != nil {
		return fmt.Errorf("check article count: %w", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM articles_fts").Scan(&ftsCount); err != nil {
		return fmt.Errorf("check fts count: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM articles a
		LEFT JOIN articles_fts f ON f.rowid = a.id
		WHERE f.rowid IS NULL`).Scan(&missingCount); err != nil {
		return fmt.Errorf("check missing fts rows: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*)
		FROM articles_fts f
		LEFT JOIN articles a ON a.id = f.rowid
		WHERE a.id IS NULL`).Scan(&orphanCount); err != nil {
		return fmt.Errorf("check orphan fts rows: %w", err)
	}

	if articleCount != ftsCount || missingCount > 0 || orphanCount > 0 {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin fts rebuild: %w", err)
		}
		if _, err = tx.Exec("DROP TABLE IF EXISTS articles_fts"); err != nil {
			tx.Rollback()
			return fmt.Errorf("drop stale fts table: %w", err)
		}
		if _, err = tx.Exec(`CREATE VIRTUAL TABLE articles_fts
			USING fts5(title, body, tags)`); err != nil {
			tx.Rollback()
			return fmt.Errorf("recreate fts table: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO articles_fts(rowid, title, body, tags)
			SELECT id, COALESCE(title,''), COALESCE(content,''), COALESCE(tags,'')
			FROM articles`); err != nil {
			tx.Rollback()
			return fmt.Errorf("populate fts: %w", err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit fts rebuild: %w", err)
		}
	}
	return nil
}

func (db *DB) InitFavoritesTable() error {
	return db.initArticleStateTable("article_favorites", "favorited_at")
}

func (db *DB) InitArchivesTable() error {
	return db.initArticleStateTable("article_archives", "archived_at")
}

func (db *DB) InitNotesTable() error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS article_notes (
		user_id INTEGER NOT NULL,
		article_id INTEGER NOT NULL,
		note TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(user_id, article_id),
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_article_notes_article_id ON article_notes(article_id)`); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_article_notes_user_updated ON article_notes(user_id, updated_at DESC)`)
	return err
}

func (db *DB) InitHighlightsTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS article_highlights (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		article_id INTEGER NOT NULL,
		snippet TEXT NOT NULL,
		prefix TEXT,
		suffix TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	)`)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_article_highlights_user_created ON article_highlights(user_id, created_at DESC)`); err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_article_highlights_article ON article_highlights(article_id)`)
	return err
}

func (db *DB) SaveHighlight(userID, articleID int, snippet, prefix, suffix string) (*ArticleHighlight, error) {
	snippet = strings.TrimSpace(snippet)
	if snippet == "" {
		return nil, fmt.Errorf("empty highlight")
	}
	res, err := db.Exec(`INSERT INTO article_highlights(user_id, article_id, snippet, prefix, suffix)
		VALUES(?, ?, ?, ?, ?)`, userID, articleID, snippet, prefix, suffix)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetHighlight(userID, int(id))
}

func (db *DB) DeleteHighlight(userID, id int) error {
	_, err := db.Exec(`DELETE FROM article_highlights WHERE user_id = ? AND id = ?`, userID, id)
	return err
}

func (db *DB) GetHighlight(userID, id int) (*ArticleHighlight, error) {
	row := db.QueryRow(`SELECT h.id, h.user_id, h.article_id,
		COALESCE(a.site,''), COALESCE(a.title,''), COALESCE(a.publish_date,''),
		COALESCE(h.snippet,''), COALESCE(h.prefix,''), COALESCE(h.suffix,''), COALESCE(h.created_at,'')
		FROM article_highlights h
		LEFT JOIN articles a ON a.id = h.article_id
		WHERE h.user_id = ? AND h.id = ?`, userID, id)
	var h ArticleHighlight
	if err := row.Scan(&h.ID, &h.UserID, &h.ArticleID, &h.Site, &h.Title, &h.PublishDate, &h.Snippet, &h.Prefix, &h.Suffix, &h.CreatedAt); err != nil {
		return nil, err
	}
	return &h, nil
}

func (db *DB) GetHighlightsForArticle(userID, articleID int) ([]ArticleHighlight, error) {
	rows, err := db.Query(`SELECT h.id, h.user_id, h.article_id,
		COALESCE(a.site,''), COALESCE(a.title,''), COALESCE(a.publish_date,''),
		COALESCE(h.snippet,''), COALESCE(h.prefix,''), COALESCE(h.suffix,''), COALESCE(h.created_at,'')
		FROM article_highlights h
		LEFT JOIN articles a ON a.id = h.article_id
		WHERE h.user_id = ? AND h.article_id = ?
		ORDER BY h.id`, userID, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHighlights(rows)
}

func (db *DB) GetHighlightsForUser(userID int) ([]ArticleHighlight, error) {
	rows, err := db.Query(`SELECT h.id, h.user_id, h.article_id,
		COALESCE(a.site,''), COALESCE(a.title,''), COALESCE(a.publish_date,''),
		COALESCE(h.snippet,''), COALESCE(h.prefix,''), COALESCE(h.suffix,''), COALESCE(h.created_at,'')
		FROM article_highlights h
		LEFT JOIN articles a ON a.id = h.article_id
		WHERE h.user_id = ?
		ORDER BY h.created_at DESC, h.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHighlights(rows)
}

func scanHighlights(rows *sql.Rows) ([]ArticleHighlight, error) {
	var out []ArticleHighlight
	for rows.Next() {
		var h ArticleHighlight
		if err := rows.Scan(&h.ID, &h.UserID, &h.ArticleID, &h.Site, &h.Title, &h.PublishDate, &h.Snippet, &h.Prefix, &h.Suffix, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (db *DB) SaveNote(userID, id int, note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		_, err := db.Exec(`DELETE FROM article_notes WHERE user_id = ? AND article_id = ?`, userID, id)
		return err
	}
	_, err := db.Exec(`INSERT INTO article_notes(user_id, article_id, note, updated_at)
		VALUES(?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, article_id) DO UPDATE SET note = excluded.note, updated_at = CURRENT_TIMESTAMP`,
		userID, id, note)
	return err
}

func (db *DB) MarkFavorite(userID, id int) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO article_favorites(user_id, article_id) VALUES(?, ?)`, userID, id)
	return err
}

func (db *DB) UnmarkFavorite(userID, id int) error {
	_, err := db.Exec(`DELETE FROM article_favorites WHERE user_id = ? AND article_id = ?`, userID, id)
	return err
}

func (db *DB) InitReadTable() error {
	return db.initArticleStateTable("article_reads", "read_at")
}

func (db *DB) MarkRead(userID, id int) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO article_reads(user_id, article_id) VALUES(?, ?)`, userID, id)
	return err
}

func (db *DB) MarkUnread(userID, id int) error {
	_, err := db.Exec(`DELETE FROM article_reads WHERE user_id = ? AND article_id = ?`, userID, id)
	return err
}

func (db *DB) MarkArchived(userID, id int) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO article_archives(user_id, article_id) VALUES(?, ?)`, userID, id)
	return err
}

func (db *DB) UnmarkArchived(userID, id int) error {
	_, err := db.Exec(`DELETE FROM article_archives WHERE user_id = ? AND article_id = ?`, userID, id)
	return err
}

func (db *DB) initArticleStateTable(tableName, timestampCol string) error {
	hasUserID, err := db.tableHasColumn(tableName, "user_id")
	if err != nil {
		return err
	}
	if !hasUserID {
		exists, err := db.tableExists(tableName)
		if err != nil {
			return err
		}
		if exists {
			legacyName := tableName + "_legacy_global"
			if _, err := db.Exec("ALTER TABLE " + tableName + " RENAME TO " + legacyName); err != nil {
				return fmt.Errorf("migrate %s: %w", tableName, err)
			}
			log.Printf("migrated legacy global table %s to %s; new rows are now user-scoped", tableName, legacyName)
		}
	}
	_, err = db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		user_id INTEGER NOT NULL,
		article_id INTEGER NOT NULL,
		%s DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(user_id, article_id),
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	)`, tableName, timestampCol))
	if err != nil {
		return err
	}
	if _, err = db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_article_id ON %s(article_id)`, tableName, tableName)); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_user_%s ON %s(user_id, %s DESC)`, tableName, timestampCol, tableName, timestampCol))
	return err
}

func (db *DB) tableExists(tableName string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count)
	return count > 0, err
}

func (db *DB) tableHasColumn(tableName, col string) (bool, error) {
	exists, err := db.tableExists(tableName)
	if err != nil || !exists {
		return false, err
	}
	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
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
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

type CategoryInfo struct {
	Name  string
	Sites string // comma-separated site names
}

func (db *DB) GetCategoryInfos() ([]CategoryInfo, error) {
	rows, err := db.Query(
		`SELECT category, GROUP_CONCAT(DISTINCT site) FROM articles
		 WHERE category != '' GROUP BY category ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CategoryInfo
	for rows.Next() {
		var ci CategoryInfo
		if err := rows.Scan(&ci.Name, &ci.Sites); err != nil {
			return nil, err
		}
		out = append(out, ci)
	}
	return out, rows.Err()
}

func (db *DB) CountArticles(p QueryParams) (int, error) {
	var count int
	q := `SELECT COUNT(*) FROM articles a
	      LEFT JOIN article_reads r ON a.id = r.article_id AND r.user_id = ?
	      LEFT JOIN article_favorites fav ON a.id = fav.article_id AND fav.user_id = ?
	      LEFT JOIN article_archives ar ON a.id = ar.article_id AND ar.user_id = ?
	      LEFT JOIN article_notes n ON a.id = n.article_id AND n.user_id = ?
	      WHERE 1=1`
	args := []interface{}{p.UserID, p.UserID, p.UserID, p.UserID}
	var ok bool
	q, args, ok = appendSearchCondition(q, args, p)
	if !ok {
		return 0, nil
	}
	if p.Site != "" {
		q += " AND a.site = ?"
		args = append(args, p.Site)
	}
	if p.Category != "" {
		q += " AND a.category = ?"
		args = append(args, p.Category)
	}
	if p.Author != "" {
		q += " AND a.author LIKE ?"
		args = append(args, "%"+p.Author+"%")
	}
	if p.DateFrom != "" {
		q += " AND (date(a.publish_date) IS NULL OR date(a.publish_date) >= ?)"
		args = append(args, p.DateFrom)
	}
	if p.DateTo != "" {
		q += " AND (date(a.publish_date) IS NULL OR date(a.publish_date) <= ?)"
		args = append(args, p.DateTo)
	}
	if p.HideRead {
		q += " AND r.article_id IS NULL"
	}
	if p.ReadsOnly {
		q += " AND r.article_id IS NOT NULL"
	}
	if p.FavoritesOnly {
		q += " AND fav.article_id IS NOT NULL"
	}
	if p.ArchivedOnly {
		q += " AND ar.article_id IS NOT NULL"
	} else if p.UserID != 0 {
		q += " AND ar.article_id IS NULL"
	}
	return count, db.QueryRow(q, args...).Scan(&count)
}

// CategoryPill is a leaf node in the category hierarchy.
type CategoryPill struct {
	Value string // full original name (DB filter value)
	Label string // display portion
	Sites string // comma-separated sites
}

type CategorySubGroup struct {
	Label string
	Pills []CategoryPill
}

type CategoryGroup struct {
	Label     string
	ShowLabel bool
	Pills     []CategoryPill // direct children (no subgroup)
	SubGroups []CategorySubGroup
}

// BuildCategoryTree converts flat CategoryInfos into a display hierarchy.
// "business_economy" → Group "Business" / pill "economy"
// "news_japan_history" → Group "News" / SubGroup "Japan" / pill "history"
func BuildCategoryTree(cats []CategoryInfo) []CategoryGroup {
	type entry struct {
		label    string
		pills    []CategoryPill
		subMap   map[string]*CategorySubGroup
		subOrder []string
	}
	groupMap := map[string]*entry{}
	var groupOrder []string

	for _, cat := range cats {
		parts := strings.SplitN(cat.Name, "_", 3)
		groupLabel := capitalize(parts[0])

		if _, ok := groupMap[groupLabel]; !ok {
			groupMap[groupLabel] = &entry{label: groupLabel, subMap: map[string]*CategorySubGroup{}}
			groupOrder = append(groupOrder, groupLabel)
		}
		g := groupMap[groupLabel]
		pill := CategoryPill{Value: cat.Name, Sites: cat.Sites}

		switch len(parts) {
		case 1:
			pill.Label = parts[0]
			g.pills = append(g.pills, pill)
		case 2:
			pill.Label = parts[1]
			g.pills = append(g.pills, pill)
		default: // 3 parts: group_subgroup_label
			subLabel := capitalize(parts[1])
			pill.Label = parts[2]
			if _, ok := g.subMap[subLabel]; !ok {
				g.subMap[subLabel] = &CategorySubGroup{Label: subLabel}
				g.subOrder = append(g.subOrder, subLabel)
			}
			g.subMap[subLabel].Pills = append(g.subMap[subLabel].Pills, pill)
		}
	}

	result := make([]CategoryGroup, 0, len(groupOrder))
	for _, label := range groupOrder {
		g := groupMap[label]
		cg := CategoryGroup{Label: g.label, Pills: g.pills}
		for _, sub := range g.subOrder {
			cg.SubGroups = append(cg.SubGroups, *g.subMap[sub])
		}
		cg.ShowLabel = len(cg.SubGroups) > 0 || len(cg.Pills) > 1 ||
			(len(cg.Pills) == 1 && !strings.EqualFold(cg.Label, cg.Pills[0].Label))
		result = append(result, cg)
	}
	return result
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (db *DB) QueryArticles(p QueryParams) ([]Article, error) {
	if p.Limit <= 0 {
		p.Limit = 20
	}
	q := `SELECT COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
			COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
			COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
			COALESCE(a.scraped_at,''),
			CASE WHEN r.article_id IS NOT NULL THEN 1 ELSE 0 END,
			COALESCE(r.read_at,''),
			CASE WHEN fav.article_id IS NOT NULL THEN 1 ELSE 0 END,
			CASE WHEN ar.article_id IS NOT NULL THEN 1 ELSE 0 END,
			COALESCE(n.note, '')
			FROM articles a
			LEFT JOIN article_reads r ON a.id = r.article_id AND r.user_id = ?
			LEFT JOIN article_favorites fav ON a.id = fav.article_id AND fav.user_id = ?
			LEFT JOIN article_archives ar ON a.id = ar.article_id AND ar.user_id = ?
			LEFT JOIN article_notes n ON a.id = n.article_id AND n.user_id = ?
			WHERE 1=1`
	args := []interface{}{p.UserID, p.UserID, p.UserID, p.UserID}
	var ok bool
	q, args, ok = appendSearchCondition(q, args, p)
	if !ok {
		return nil, nil
	}
	if p.Site != "" {
		q += " AND a.site = ?"
		args = append(args, p.Site)
	}
	if p.Category != "" {
		q += " AND a.category = ?"
		args = append(args, p.Category)
	}
	if p.Author != "" {
		q += " AND a.author LIKE ?"
		args = append(args, "%"+p.Author+"%")
	}
	if p.DateFrom != "" {
		q += " AND (date(a.publish_date) IS NULL OR date(a.publish_date) >= ?)"
		args = append(args, p.DateFrom)
	}
	if p.DateTo != "" {
		q += " AND (date(a.publish_date) IS NULL OR date(a.publish_date) <= ?)"
		args = append(args, p.DateTo)
	}
	if p.HideRead {
		q += " AND r.article_id IS NULL"
	}
	if p.ReadsOnly {
		q += " AND r.article_id IS NOT NULL"
	}
	if p.FavoritesOnly {
		q += " AND fav.article_id IS NOT NULL"
	}
	if p.ArchivedOnly {
		q += " AND ar.article_id IS NOT NULL"
	} else if p.UserID != 0 {
		q += " AND ar.article_id IS NULL"
	}
	if p.ReadsOnly {
		q += " ORDER BY r.read_at DESC, a.publish_date DESC, a.scraped_at DESC LIMIT ? OFFSET ?"
	} else {
		q += " ORDER BY a.publish_date DESC, a.scraped_at DESC LIMIT ? OFFSET ?"
	}
	args = append(args, p.Limit, p.Offset)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
			&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt,
			&a.Read, &a.ReadAt, &a.Favorited, &a.Archived, &a.Note); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) GetArticleByID(id, userID int) (*Article, error) {
	row := db.QueryRow(`SELECT COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
		COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
		COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
		COALESCE(a.scraped_at,''),
		CASE WHEN r.article_id IS NOT NULL THEN 1 ELSE 0 END,
		COALESCE(r.read_at,''),
		CASE WHEN fav.article_id IS NOT NULL THEN 1 ELSE 0 END,
		CASE WHEN ar.article_id IS NOT NULL THEN 1 ELSE 0 END,
		COALESCE(n.note, '')
		FROM articles a
		LEFT JOIN article_reads r ON a.id = r.article_id AND r.user_id = ?
		LEFT JOIN article_favorites fav ON a.id = fav.article_id AND fav.user_id = ?
		LEFT JOIN article_archives ar ON a.id = ar.article_id AND ar.user_id = ?
		LEFT JOIN article_notes n ON a.id = n.article_id AND n.user_id = ?
		WHERE a.id = ?`, userID, userID, userID, userID, id)
	var a Article
	if err := row.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
		&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt,
		&a.Read, &a.ReadAt, &a.Favorited, &a.Archived, &a.Note); err != nil {
		return nil, err
	}
	return &a, nil
}
