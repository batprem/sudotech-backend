package models

import "time"

type Card struct {
	ID          int64      `db:"id"          json:"id"`
	ColumnID    int64      `db:"column_id"   json:"column_id"`
	Title       string     `db:"title"       json:"title"`
	Description string     `db:"description" json:"description"`
	DueDate     *time.Time `db:"due_date"    json:"due_date"`
	Position    int        `db:"position"    json:"position"`
	Labels      []int64    `db:"-"           json:"labels"`
	CreatedAt   time.Time  `db:"created_at"  json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"  json:"updated_at"`
}
