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

type CardHandler struct {
	DB *sqlx.DB
}

func (h *CardHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/columns/:cid/cards", h.Create)
	rg.GET("/cards/:id", h.Get)
	rg.PUT("/cards/:id", h.Update)
	rg.PATCH("/cards/:id/move", h.Move)
	rg.DELETE("/cards/:id", h.Delete)
}

// loadOwnedCard fetches the card and verifies ownership through column → board.
// Returns sql.ErrNoRows when the card is missing or the board is not owned by userID.
func loadOwnedCard(ctx context.Context, db *sqlx.DB, cardID int64, userID int64) (models.Card, error) {
	var card models.Card
	err := db.GetContext(ctx, &card, `
		SELECT ca.id, ca.column_id, ca.title, ca.description, ca.due_date, ca.position, ca.created_at, ca.updated_at
		FROM cards ca
		JOIN columns co ON co.id = ca.column_id
		JOIN boards b   ON b.id  = co.board_id
		WHERE ca.id = ? AND b.user_id = ?`,
		cardID, userID)
	return card, err
}

func (h *CardHandler) Create(c *gin.Context) {
	uid := auth.UserID(c)

	var cid int64
	if err := parseID(c, "cid", &cid); err != nil {
		return
	}

	if _, err := loadOwnedColumn(c.Request.Context(), h.DB, cid, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "column not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var body struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		DueDate     *time.Time `json:"due_date"`
		Labels      []int64    `json:"labels"`
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
	if len(body.Title) > 120 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 120 characters or fewer"})
		return
	}
	if len(body.Description) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description must be 4000 characters or fewer"})
		return
	}

	var nextPos int
	if err := h.DB.GetContext(c.Request.Context(), &nextPos,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM cards WHERE column_id = ?`, cid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	res, err := h.DB.ExecContext(c.Request.Context(),
		`INSERT INTO cards (column_id, title, description, due_date, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cid, body.Title, body.Description, body.DueDate, nextPos, now, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	cardID, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	card := models.Card{
		ID:          cardID,
		ColumnID:    cid,
		Title:       body.Title,
		Description: body.Description,
		DueDate:     body.DueDate,
		Position:    nextPos,
		Labels:      []int64{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	c.JSON(http.StatusCreated, card)
}

func (h *CardHandler) Get(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	card, err := loadOwnedCard(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	card.Labels = []int64{}
	c.JSON(http.StatusOK, card)
}

func (h *CardHandler) Update(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	card, err := loadOwnedCard(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var body struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		DueDate     *time.Time `json:"due_date"`
		Labels      []int64    `json:"labels"`
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
	if len(body.Title) > 120 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title must be 120 characters or fewer"})
		return
	}
	if len(body.Description) > 4000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "description must be 4000 characters or fewer"})
		return
	}

	now := time.Now().UTC()
	if _, err := h.DB.ExecContext(c.Request.Context(),
		`UPDATE cards SET title = ?, description = ?, due_date = ?, updated_at = ? WHERE id = ?`,
		body.Title, body.Description, body.DueDate, now, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	card.Title = body.Title
	card.Description = body.Description
	card.DueDate = body.DueDate
	card.UpdatedAt = now
	card.Labels = []int64{}
	c.JSON(http.StatusOK, card)
}

func (h *CardHandler) Move(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	var body struct {
		ColumnID int64 `json:"column_id"`
		Position int   `json:"position"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	card, err := loadOwnedCard(c.Request.Context(), h.DB, id, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	destCol, err := loadOwnedColumn(c.Request.Context(), h.DB, body.ColumnID, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "column not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sourceCol, err := loadOwnedColumn(c.Request.Context(), h.DB, card.ColumnID, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sourceCol.BoardID != destCol.BoardID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "column not on this board"})
		return
	}

	sameColumn := sourceCol.ID == destCol.ID

	tx, err := h.DB.BeginTxx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	const selectByColumn = `
		SELECT id, column_id, title, description, due_date, position, created_at, updated_at
		FROM cards WHERE column_id = ? ORDER BY position`

	var srcCards []models.Card
	if err := tx.SelectContext(c.Request.Context(), &srcCards, selectByColumn, sourceCol.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Pull the moving card out of the source list.
	var moving models.Card
	srcRemaining := make([]models.Card, 0, len(srcCards))
	for _, ca := range srcCards {
		if ca.ID == id {
			moving = ca
		} else {
			srcRemaining = append(srcRemaining, ca)
		}
	}

	// destBase is the destination list *without* the moving card.
	var destBase []models.Card
	if sameColumn {
		destBase = srcRemaining
	} else {
		if err := tx.SelectContext(c.Request.Context(), &destBase, selectByColumn, destCol.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Clamp the target index to [0, len(destBase)].
	target := body.Position
	if target < 0 {
		target = 0
	}
	if target > len(destBase) {
		target = len(destBase)
	}

	moving.ColumnID = destCol.ID

	destResult := make([]models.Card, 0, len(destBase)+1)
	destResult = append(destResult, destBase[:target]...)
	destResult = append(destResult, moving)
	destResult = append(destResult, destBase[target:]...)

	now := time.Now().UTC()

	for i := range destResult {
		destResult[i].Position = i
		destResult[i].Labels = []int64{}
		if destResult[i].ID == id {
			destResult[i].ColumnID = destCol.ID
			destResult[i].UpdatedAt = now
			if _, err := tx.ExecContext(c.Request.Context(),
				`UPDATE cards SET column_id = ?, position = ?, updated_at = ? WHERE id = ?`,
				destCol.ID, i, now, destResult[i].ID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			if _, err := tx.ExecContext(c.Request.Context(),
				`UPDATE cards SET position = ? WHERE id = ?`, i, destResult[i].ID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	// For same-column moves, source == dest. For cross-column, close the gap in source.
	var sourceResult []models.Card
	if sameColumn {
		sourceResult = destResult
	} else {
		sourceResult = make([]models.Card, len(srcRemaining))
		for i, ca := range srcRemaining {
			ca.Position = i
			ca.Labels = []int64{}
			sourceResult[i] = ca
			if _, err := tx.ExecContext(c.Request.Context(),
				`UPDATE cards SET position = ? WHERE id = ?`, i, ca.ID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"source": sourceResult,
		"dest":   destResult,
	})
}

func (h *CardHandler) Delete(c *gin.Context) {
	uid := auth.UserID(c)

	var id int64
	if err := parseID(c, "id", &id); err != nil {
		return
	}

	if _, err := loadOwnedCard(c.Request.Context(), h.DB, id, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "card not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.DB.ExecContext(c.Request.Context(),
		`DELETE FROM cards WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
