/*
 * Banner Bard: Banner-serving discord bot, sire.
 *
 * db.go - Database utilities. This provides the wrapper between the
 * SQLite3 database and the rest of the bot (specifically,
 * banner-bard.go and scheduler.go). If you're adding new information
 * for banner bard to remember, try to make a nice wrapper function
 * here.
 *
 *
 * This program uses the BSD 3-Clause license. You can find details under
 * the file LICENSE or under <https://opensource.org/licenses/BSD-3-Clause>.
 */
package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

var sqlDb *sql.DB

const (
	SqlNoRows     = "no rows in result set"
	SqlForeignKey = "FOREIGN KEY constraint failed"
	DatabaseFile  = "./banner-bard.db"
)

type Tag struct {
	Name     string
	AuthorID string
	Url      string
}

func openDb() error {
	var err error
	sqlDb, err = sql.Open("sqlite3", DatabaseFile)

	// Pragmas

	if err == nil {
		_, err = sqlDb.Exec("PRAGMA foreign_keys = true")
	}

	// Table initialization

	if err == nil {
		_, err = sqlDb.Exec(`
CREATE TABLE IF NOT EXISTS tag (
  name TEXT PRIMARY KEY,
  authorID TEXT NOT NULL,
  url TEXT NOT NULL
)`)
	}

	if err == nil {
		_, err = sqlDb.Exec(`
CREATE TABLE IF NOT EXISTS playlist (
  name TEXT NOT NULL,
  tag TEXT NOT NULL REFERENCES tag(name) ON DELETE CASCADE,
  timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (name, tag)
)`)
	}

	return err
}

func closeDbOrPanic() {
	err := sqlDb.Close()

	if err != nil {
		panic(err)
	}
}

// Sql utils

func rollbackOrDie(tx *sql.Tx, name string) {
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		logger.Fatalf("%s: unable to rollback: %s",
			name, rollbackErr.Error())
	}
}

// Tags

func namedTag(name string) (tag Tag, err error) {
	tag.Name = name
	err = sqlDb.
		QueryRow("SELECT url, authorID FROM tag WHERE name=?",
			name).
		Scan(&tag.Url, &tag.AuthorID)

	return tag, err
}

func insertTag(name string, authorID string, url string) (err error) {
	_, err = sqlDb.
		Exec("INSERT OR REPLACE INTO tag (name, authorID, url) VALUES (?,?,?)",
			name, authorID, url)
	return err
}

func delTag(name string) (err error) {
	_, err = sqlDb.Exec("DELETE FROM tag WHERE name=?", name)
	return err
}

func tagExists(name string) (bool, error) {
	var count int
	err := sqlDb.
		QueryRow("SELECT COUNT(*) FROM tag WHERE name=?",
			name).
		Scan(&count)
	return count > 0, err
}

func allTags() (taglist []Tag, err error) {
	var rows *sql.Rows

	rows, err = sqlDb.Query("SELECT name, authorID, url FROM tag ORDER BY name")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var tag Tag
		err = rows.Scan(&tag.Name, &tag.AuthorID, &tag.Url)
		if err != nil {
			break
		}

		taglist = append(taglist, tag)
	}

	return taglist, err
}

func clearTags() error {
	_, err := sqlDb.Exec("DELETE FROM tag")
	return err
}

// Playlists

func clearPlaylist(playlist string) error {
	_, err := sqlDb.Exec("DELETE FROM playlist WHERE name=?", playlist)
	return err
}

func appendPlaylist(playlist string, tags []string) error {
	tx, err := sqlDb.Begin()
	if err != nil {
		return err
	}

	for _, tag := range tags {
		_, err := tx.Exec("INSERT INTO playlist (name, tag) VALUES (?,?)",
			playlist, tag)

		if err != nil {
			rollbackOrDie(tx, "appendPlaylist")
			return err
		}
	}

	return tx.Commit()
}

func editPlaylist(playlist string, tags []string) error {
	tx, err := sqlDb.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM playlist WHERE name=?", playlist)
	if err != nil {
		rollbackOrDie(tx, "editPlaylist")
		return err
	}

	for _, tag := range tags {
		_, err = tx.Exec("INSERT INTO playlist (name, tag) VALUES (?, ?)",
			playlist, tag)

		if err != nil {
			rollbackOrDie(tx, "editPlaylist")
			return err
		}
	}

	return tx.Commit()
}

func reducePlaylist(playlist string, tags []string) error {
	tx, err := sqlDb.Begin()
	if err != nil {
		return err
	}

	for _, tag := range tags {
		_, err := tx.Exec("DELETE FROM playlist WHERE name=? AND tag=?",
			playlist, tag)
		if err != nil {
			rollbackOrDie(tx, "reducePlaylist")
			return err
		}
	}

	return nil
}

func allPlaylists() (playlists []string, err error) {
	var rows *sql.Rows

	rows, err = sqlDb.Query("SELECT DISTINCT name FROM playlist")
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var playlist string
		err = rows.Scan(&playlist)
		if err != nil {
			break
		}

		playlists = append(playlists, playlist)
	}

	return playlists, err
}

func playlistTags(playlist string) (tags []string, err error) {
	var rows *sql.Rows

	rows, err = sqlDb.Query(
		"SELECT tag FROM playlist WHERE name=? ORDER BY timestamp", playlist)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var tag string
		err = rows.Scan(&tag)
		if err != nil {
			break
		}

		tags = append(tags, tag)
	}

	return tags, err
}

func playlistExists(name string) (bool, error) {
	var count int
	err := sqlDb.
		QueryRow("SELECT COUNT(*) FROM playlist WHERE name=?",
			name).
		Scan(&count)
	return count > 0, err
}
