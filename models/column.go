package models

import "time"

type Column struct {
	ID        int64     `db:"id"         json:"id"`
	BoardID   int64     `db:"board_id"   json:"board_id"`
	Title     string    `db:"title"      json:"title"`
	Position  int       `db:"position"   json:"position"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
