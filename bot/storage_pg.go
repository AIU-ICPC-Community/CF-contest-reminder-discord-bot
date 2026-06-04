package bot

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

var useDB bool
var dbPool *pgxpool.Pool

func initDB() {
	url := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if url == "" {
		useDB = false
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		log.Printf("failed to connect to database: %v", err)
		useDB = false
		return
	}

	pool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		log.Printf("failed to connect to database: %v", err)
		useDB = false
		return
	}

	dbPool = pool
	useDB = true
	log.Println("connected to DATABASE_URL; using Postgres for sent-state storage")

	create := `CREATE TABLE IF NOT EXISTS sent_contests (
        environment text NOT NULL,
        contest_id integer NOT NULL,
        sent_at timestamptz DEFAULT now(),
        PRIMARY KEY (environment, contest_id)
    )`
	if _, err := dbPool.Exec(ctx, create); err != nil {
		log.Printf("failed to create sent_contests table: %v", err)
	}
}

func loadSentContestIDsDB(env string) (map[int]struct{}, error) {
	result := make(map[int]struct{})
	if !useDB || dbPool == nil {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := dbPool.Query(ctx, "SELECT contest_id FROM sent_contests WHERE environment=$1", env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func saveSentContestIDsDB(sentContestIDs map[int]struct{}, env string) error {
	if !useDB || dbPool == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := dbPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // safe to call

	stmt := `INSERT INTO sent_contests (environment, contest_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	for contestID := range sentContestIDs {
		if _, err := tx.Exec(ctx, stmt, env, contestID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
