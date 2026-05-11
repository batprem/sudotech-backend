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

type BoardHandler struct {
	DB *sqlx.DB
}

func (h *BoardHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/boards", h.List)
	rg.POST("/boards", h.Create)
	rg.GET("/boards/:id", h.Get)
	rg.PUT("/boards/:id", h.Update)
	rg.DELETE("/boards/:id", h.Delete)
}

// loadOwnedBoard fetches the board by id scoped to userID.
// Returns sql.ErrNoRows for both "doesn't exist" and "not yours" to avoid leaking existence.
func loadOwnedBoard(ctx context.Context, db *sqlx.DB, id int64, userID int64) (models.Board, error) {
	var b models.Board
	err := db.GetContext(ctx, &b,
		`SELECT id, user_id, title, created_at, updated_at FROM boards WHERE id = ? AND user_id = ?`,
		id, userID)
	return b, err
}

func (h *BoardHandler) List(c *gin.Context) {
	uid := auth.UserID(c)
	var boards []models.Board
	if err := h.DB.SelectContext(c.Request.Context(), &boards,
		`SELECT id, user_id, title, created_at, updated_at FROM boards WHERE user_id = ? ORDER BY updated_at DESC`,
		uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if boards == nil {
		boards = []models.Board{}
	}
	c.JSON(http.StatusOK, boards)
}

func (h *BoardHandler) Create(c *gin.Context) {
	uid := auth.UserID(c)

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
	if len(body.Title) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 80 characters or fewer"})
		return
	}

	tx, err := h.DB.BeginTxx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.ExecContext(c.Request.Context(),
		`INSERT INTO boards (user_id, title, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		uid, body.Title, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	boardID, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Insert three default columns
	defaultCols := []string{"To Do", "Doing", "Done"}
	for i, title := range defaultCols {
		_, err := tx.ExecContext(c.Request.Context(),
			`INSERT INTO columns (board_id, title, position, created_at) VALUES (?, ?, ?, ?)`,
			boardID, title, i, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	board := models.Board{
		ID:        boardID,
		UserID:    uid,
		Title:     body.Title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	c.JSON(http.StatusCreated, board)
}

func (h *BoardHandler) Get(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	board, err := loadOwnedBoard(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "board not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var columns []models.Column
	if err := h.DB.SelectContext(c.Request.Context(), &columns,
		`SELECT id, board_id, title, position, created_at FROM columns WHERE board_id = ? ORDER BY position`,
		id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if columns == nil {
		columns = []models.Column{}
	}

	cards := []models.Card{}
	if len(columns) > 0 {
		colIDs := make([]int64, len(columns))
		for i, col := range columns {
			colIDs[i] = col.ID
		}
		query, args, err := sqlx.In(
			`SELECT id, column_id, title, description, due_date, position, created_at, updated_at
			 FROM cards WHERE column_id IN (?) ORDER BY column_id, position`,
			colIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		query = h.DB.Rebind(query)
		if err := h.DB.SelectContext(c.Request.Context(), &cards, query, args...); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for i := range cards {
			cards[i].Labels = []int64{}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"board":   board,
		"columns": columns,
		"cards":   cards,
		"labels":  []struct{}{},
	})
}

func (h *BoardHandler) Update(c *gin.Context) {
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
	if len(body.Title) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 80 characters or fewer"})
		return
	}

	// Confirm ownership first
	if _, err := loadOwnedBoard(c.Request.Context(), h.DB, id, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "board not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	_, err := h.DB.ExecContext(c.Request.Context(),
		`UPDATE boards SET title = ?, updated_at = ? WHERE id = ?`,
		body.Title, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	board, err := loadOwnedBoard(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, board)
}

func (h *BoardHandler) Delete(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	if _, err := loadOwnedBoard(c.Request.Context(), h.DB, id, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "board not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.DB.ExecContext(c.Request.Context(),
		`DELETE FROM boards WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
