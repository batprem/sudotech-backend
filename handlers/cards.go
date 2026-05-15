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
