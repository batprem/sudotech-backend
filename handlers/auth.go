package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"

	"todo-list/backend/auth"
	"todo-list/backend/models"
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type AuthHandler struct {
	DB   *sqlx.DB
	Auth *auth.Service
}

func (h *AuthHandler) Register(api *gin.RouterGroup) {
	api.POST("/register", h.Signup)
	api.POST("/login", h.Login)
	api.GET("/me", h.Auth.RequireUser(), h.Me)
}

func (h *AuthHandler) Signup(c *gin.Context) {
	var in models.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !usernameRE.MatchString(in.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username must be alphanumeric or underscore"})
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	res, err := h.DB.ExecContext(c.Request.Context(),
		`INSERT INTO users (username, password_hash) VALUES (?, ?)`,
		in.Username, hash)
	if err != nil {
		var sqlErr sqlite3.Error
		if errors.As(err, &sqlErr) && sqlErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	user := models.PublicUser{ID: id, Username: in.Username}
	token, err := h.Auth.Sign(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, models.AuthResponse{Token: token, User: user})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var in models.LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in.Username = strings.TrimSpace(in.Username)

	var user models.User
	err := h.DB.GetContext(c.Request.Context(), &user,
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`,
		in.Username)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !auth.VerifyPassword(user.PasswordHash, in.Password)) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	token, err := h.Auth.Sign(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.AuthResponse{Token: token, User: user.Public()})
}

func (h *AuthHandler) Me(c *gin.Context) {
	uid := auth.UserID(c)
	var user models.User
	if err := h.DB.GetContext(c.Request.Context(), &user,
		`SELECT id, username, password_hash, created_at FROM users WHERE id = ?`, uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user no longer exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user.Public())
}

