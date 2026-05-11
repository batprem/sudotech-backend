package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"todo-list/backend/auth"
	"todo-list/backend/models"
)

type ColumnHandler struct {
	DB *sqlx.DB
}

func (h *ColumnHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/boards/:bid/columns", h.Create)
	rg.PUT("/columns/:id", h.Update)
	rg.PATCH("/columns/:id/move", h.Move)
	rg.DELETE("/columns/:id", h.Delete)
}

// loadOwnedColumn fetches the column and verifies ownership through the board.
// Returns sql.ErrNoRows when the column is missing or the board is not owned.
func loadOwnedColumn(ctx context.Context, db *sqlx.DB, columnID int64, userID int64) (models.Column, error) {
	var col models.Column
	err := db.GetContext(ctx, &col, `
		SELECT c.id, c.board_id, c.title, c.position, c.created_at
		FROM columns c
		JOIN boards b ON b.id = c.board_id
		WHERE c.id = ? AND b.user_id = ?`,
		columnID, userID)
	return col, err
}

func (h *ColumnHandler) Create(c *gin.Context) {
	uid := auth.UserID(c)

	var bid int64
	if err := parseID(c, "bid", &bid); err != nil {
		return
	}

	// Confirm ownership of the parent board.
	if _, err := loadOwnedBoard(c.Request.Context(), h.DB, bid, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "board not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if len(body.Title) > 40 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 40 characters or fewer"})
		return
	}

	// Append at max(position) + 1.
	var nextPos int
	err := h.DB.GetContext(c.Request.Context(), &nextPos,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM columns WHERE board_id = ?`, bid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	res, err := h.DB.ExecContext(c.Request.Context(),
		`INSERT INTO columns (board_id, title, position, created_at) VALUES (?, ?, ?, ?)`,
		bid, body.Title, nextPos, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	colID, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	col := models.Column{
		ID:        colID,
		BoardID:   bid,
		Title:     body.Title,
		Position:  nextPos,
		CreatedAt: now,
	}
	c.JSON(http.StatusCreated, col)
}

func (h *ColumnHandler) Update(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if len(body.Title) > 40 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 40 characters or fewer"})
		return
	}

	col, err := loadOwnedColumn(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "column not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.DB.ExecContext(c.Request.Context(),
		`UPDATE columns SET title = ? WHERE id = ?`, body.Title, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	col.Title = body.Title
	c.JSON(http.StatusOK, col)
}

func (h *ColumnHandler) Move(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	var body struct {
		Position int `json:"position"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	col, err := loadOwnedColumn(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "column not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Load all columns for this board ordered by position.
	var cols []models.Column
	if err := h.DB.SelectContext(c.Request.Context(), &cols,
		`SELECT id, board_id, title, position, created_at FROM columns WHERE board_id = ? ORDER BY position`,
		col.BoardID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Splice: remove the moving column, insert at the clamped target index.
	targetIdx := body.Position
	if targetIdx < 0 {
		targetIdx = 0
	}
	if targetIdx >= len(cols) {
		targetIdx = len(cols) - 1
	}

	// Remove moving column from its current place.
	var moving models.Column
	ordered := make([]models.Column, 0, len(cols))
	for _, c2 := range cols {
		if c2.ID == id {
			moving = c2
		} else {
			ordered = append(ordered, c2)
		}
	}
	// Insert at target index.
	result := make([]models.Column, 0, len(cols))
	result = append(result, ordered[:targetIdx]...)
	result = append(result, moving)
	result = append(result, ordered[targetIdx:]...)

	// Rewrite positions 0…N-1 in a transaction.
	tx, err := h.DB.BeginTxx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	for i := range result {
		result[i].Position = i
		if _, err := tx.ExecContext(c.Request.Context(),
			`UPDATE columns SET position = ? WHERE id = ?`, i, result[i].ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"columns": result})
}

func (h *ColumnHandler) Delete(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	if _, err := loadOwnedColumn(c.Request.Context(), h.DB, id, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "column not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reject if the column has any cards (cards table exists per schema.sql).
	var cardCount int
	if err := h.DB.GetContext(c.Request.Context(), &cardCount,
		`SELECT COUNT(*) FROM cards WHERE column_id = ?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cardCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "column has cards"})
		return
	}

	if _, err := h.DB.ExecContext(c.Request.Context(),
		`DELETE FROM columns WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
