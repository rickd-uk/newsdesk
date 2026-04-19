package main

import (
	"database/sql"
	"fmt"
	"log"
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
	Read        bool
	Favorited   bool
}

type QueryParams struct {
	Q             string
	Site          string
	Category      string
	Author        string
	DateFrom      string
	DateTo        string
	Fields        []string // "title","body","tags" — nil/empty means all
	HideRead      bool
	FavoritesOnly bool
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

// InitFTS creates a standalone FTS5 index populated once at startup.
// New articles inserted after startup are not searchable until restart.
func (db *DB) InitFTS() error {
	_, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts
		USING fts5(title, body, tags)`)
	if err != nil {
		return fmt.Errorf("create fts table: %w", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM articles_fts").Scan(&count); err != nil {
		return fmt.Errorf("check fts count: %w", err)
	}
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

func (db *DB) InitFavoritesTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS article_favorites (
		article_id INTEGER PRIMARY KEY,
		favorited_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (db *DB) MarkFavorite(id int) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO article_favorites(article_id) VALUES(?)`, id)
	return err
}

func (db *DB) UnmarkFavorite(id int) error {
	_, err := db.Exec(`DELETE FROM article_favorites WHERE article_id = ?`, id)
	return err
}

func (db *DB) InitReadTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS article_reads (
		article_id INTEGER PRIMARY KEY,
		read_at    DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

func (db *DB) MarkRead(id int) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO article_reads(article_id) VALUES(?)`, id)
	return err
}

func (db *DB) MarkUnread(id int) error {
	_, err := db.Exec(`DELETE FROM article_reads WHERE article_id = ?`, id)
	return err
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
	searchQ := strings.TrimSpace(p.Q)
	if searchQ != "" {
		ftsQ := buildFTSPrefix(p.Fields) + searchQ + "*"
		q := `SELECT COUNT(*) FROM articles_fts f
		      JOIN articles a ON a.id = f.rowid
		      LEFT JOIN article_reads r ON a.id = r.article_id
		      LEFT JOIN article_favorites fav ON a.id = fav.article_id
		      WHERE articles_fts MATCH ?`
		args := []interface{}{ftsQ}
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
		if p.FavoritesOnly {
			q += " AND fav.article_id IS NOT NULL"
		}
		return count, db.QueryRow(q, args...).Scan(&count)
	}
	q := `SELECT COUNT(*) FROM articles a
	      LEFT JOIN article_reads r ON a.id = r.article_id
	      LEFT JOIN article_favorites fav ON a.id = fav.article_id
	      WHERE 1=1`
	args := []interface{}{}
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
	if p.FavoritesOnly {
		q += " AND fav.article_id IS NOT NULL"
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
	var (
		rows *sql.Rows
		err  error
	)

	searchQ := strings.TrimSpace(p.Q)
	if searchQ != "" {
		ftsQ := buildFTSPrefix(p.Fields) + searchQ + "*"
		q := `SELECT COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
			COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
			COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
			COALESCE(a.scraped_at,''),
			CASE WHEN r.article_id IS NOT NULL THEN 1 ELSE 0 END,
			CASE WHEN fav.article_id IS NOT NULL THEN 1 ELSE 0 END
			FROM articles_fts f
			JOIN articles a ON a.id = f.rowid
			LEFT JOIN article_reads r ON a.id = r.article_id
			LEFT JOIN article_favorites fav ON a.id = fav.article_id
			WHERE articles_fts MATCH ?`
		args := []interface{}{ftsQ}
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
		if p.FavoritesOnly {
			q += " AND fav.article_id IS NOT NULL"
		}
		q += " ORDER BY a.publish_date DESC, a.scraped_at DESC LIMIT ? OFFSET ?"
		args = append(args, p.Limit, p.Offset)
		rows, err = db.Query(q, args...)
	} else {
		q := `SELECT COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
			COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
			COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
			COALESCE(a.scraped_at,''),
			CASE WHEN r.article_id IS NOT NULL THEN 1 ELSE 0 END,
			CASE WHEN fav.article_id IS NOT NULL THEN 1 ELSE 0 END
			FROM articles a
			LEFT JOIN article_reads r ON a.id = r.article_id
			LEFT JOIN article_favorites fav ON a.id = fav.article_id
			WHERE 1=1`
		args := []interface{}{}
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
		if p.FavoritesOnly {
			q += " AND fav.article_id IS NOT NULL"
		}
		q += " ORDER BY a.publish_date DESC, a.scraped_at DESC LIMIT ? OFFSET ?"
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
			&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt,
			&a.Read, &a.Favorited); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

func (db *DB) GetArticleByID(id int) (*Article, error) {
	row := db.QueryRow(`SELECT COALESCE(a.id,0), COALESCE(a.site,''), COALESCE(a.url,''),
		COALESCE(a.category,''), COALESCE(a.title,''), COALESCE(a.author,''),
		COALESCE(a.publish_date,''), COALESCE(a.tags,''), COALESCE(a.content,''),
		COALESCE(a.scraped_at,''),
		CASE WHEN r.article_id IS NOT NULL THEN 1 ELSE 0 END,
		CASE WHEN fav.article_id IS NOT NULL THEN 1 ELSE 0 END
		FROM articles a
		LEFT JOIN article_reads r ON a.id = r.article_id
		LEFT JOIN article_favorites fav ON a.id = fav.article_id
		WHERE a.id = ?`, id)
	var a Article
	if err := row.Scan(&a.ID, &a.Site, &a.URL, &a.Category, &a.Title,
		&a.Author, &a.PublishDate, &a.Tags, &a.Content, &a.ScrapedAt,
		&a.Read, &a.Favorited); err != nil {
		return nil, err
	}
	return &a, nil
}
