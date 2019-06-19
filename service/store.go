package service

import (
	"database/sql"
	"errors"

	"github.com/gdotgordon/ipverify/types"

	// Load sqlite3 driver
	_ "github.com/mattn/go-sqlite3"

	"go.uber.org/zap"
)

const sqlAdditem = `
	INSERT INTO items(
		Uuid,
		Username,
		Ipaddr,
        Unix
    ) values(?, ?, ?, ?)`

type Store interface {
	AddRecord(types.VerifyRequest) error
	GetAllRowsForUser(username string) ([]types.VerifyRequest, error)
	GetPriorNext(username string, timestamp int64) (*types.VerifyRequest, *types.VerifyRequest, error)
	Shutdown()
}

type SQLiteStore struct {
	db      *sql.DB
	addStmt *sql.Stmt
	log     *zap.SugaredLogger
}

func NewSQLiteStore(filepath string, log *zap.SugaredLogger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("Unable to open database")
	}

	if err := createTable(db, log); err != nil {
		return nil, err
	}
	addStmt, err := db.Prepare(sqlAdditem)
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db, addStmt: addStmt, log: log}, nil
}

func (sqs *SQLiteStore) AddRecord(item types.VerifyRequest) error {
	_, err := sqs.addStmt.Exec(item.EventUUID, item.Username, item.IPAddress, item.UnixTimestamp)
	if err != nil {
		return err
	}
	sqs.log.Debugw("added db row", "item", item)
	return nil
}

func (sqs *SQLiteStore) GetAllRowsForUser(username string) ([]types.VerifyRequest, error) {
	sql_readall := `
		SELECT Uuid, Username, Ipaddr, Unix FROM items
		WHERE Username = ?
        ORDER BY Unix DESC
        `

	rows, err := sqs.db.Query(sql_readall, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.VerifyRequest
	for rows.Next() {
		item := types.VerifyRequest{}
		err2 := rows.Scan(&item.EventUUID, &item.Username, &item.IPAddress, &item.UnixTimestamp)
		if err2 != nil {
			panic(err2)
		}
		result = append(result, item)
	}
	return result, nil
}

func (sqs *SQLiteStore) GetPriorNext(username string,
	timestamp int64) (*types.VerifyRequest, *types.VerifyRequest, error) {
	sql_readprev := `
        SELECT Uuid, Username, Ipaddr, Unix FROM items
        WHERE Username = ? AND Unix < ?
        ORDER BY Unix DESC LIMIT 1
		`
	rows, err := sqs.db.Query(sql_readprev, username, timestamp)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var prev *types.VerifyRequest
	for rows.Next() {
		item := types.VerifyRequest{}
		err2 := rows.Scan(&item.EventUUID, &item.Username, &item.IPAddress, &item.UnixTimestamp)
		if err2 != nil {
			panic(err2)
		}
		prev = &item
	}
	return prev, nil, nil
}

func (sqs *SQLiteStore) Shutdown() {
	if err := sqs.addStmt.Close(); err != nil {
		sqs.log.Warnw("sqlite prepared statement close", "error", err)
	}
	if err := sqs.db.Close(); err != nil {
		sqs.log.Warnw("sqlite shutdown error", "error", err)
	}
}

func createTable(db *sql.DB, log *zap.SugaredLogger) error {
	// create table if not exists
	sql_table := `
	CREATE TABLE IF NOT EXISTS items(
			Uuid TEXT NOT NULL PRIMARY KEY,
			Username TEXT NOT NULL,
			Ipaddr TEXT NOT NULL,
			Unix INT NOT NULL
	);
	`
	_, err := db.Exec(sql_table)
	if err != nil {
		return err
	}
	log.Infow("Created table", "name", "items")
	return nil
}
