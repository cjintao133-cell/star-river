package main

import (
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		fmt.Println("open err", err)
		return
	}
	defer db.Close()
	_, err = db.Exec(`CREATE VIRTUAL TABLE t USING fts5(body)`)
	if err != nil {
		fmt.Println("FTS5: NOT AVAILABLE ->", err)
		return
	}
	db.Exec(`INSERT INTO t(body) VALUES ('error E_QUOTA_4021 quota exceeded'),('cost controls and spend limits')`)
	rows, _ := db.Query(`SELECT body, bm25(t) FROM t WHERE t MATCH 'quota' ORDER BY bm25(t)`)
	found := false
	for rows.Next() {
		var b string
		var s float64
		rows.Scan(&b, &s)
		fmt.Println("FTS5 OK ->", b)
		found = true
	}
	if !found {
		fmt.Println("FTS5 table created but query empty")
	}
	// also check sqlite version + whether vec available
	var v string
	db.QueryRow("select sqlite_version()").Scan(&v)
	fmt.Println("sqlite version:", v)
}
