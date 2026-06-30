package prowlarr

import (
	"net/http"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type apiHandler struct {
	lazyDB lazy.Lazy[*gorm.DB]
}

func (apiHandler) Key() string { return "prowlarr_api" }

func (h apiHandler) Apply(e *gin.Engine) error {
	db, err := h.lazyDB.Get()
	if err != nil {
		return err
	}

	g := e.Group("/api/prowlarr")
	g.GET("", makeGetHandler(db))
	g.PUT("", makePutHandler(db))

	return nil
}

type configResponse struct {
	Enabled   bool      `json:"enabled"`
	BaseURL   string    `json:"baseUrl"`
	APIKey    string    `json:"apiKey"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type configRequest struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

func makeGetHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cfg model.ProwlarrConfig
		if err := db.WithContext(c.Request.Context()).First(&cfg).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, configResponse{
			Enabled:   cfg.Enabled,
			BaseURL:   cfg.BaseURL,
			APIKey:    cfg.APIKey,
			UpdatedAt: cfg.UpdatedAt,
		})
	}
}

func makePutHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req configRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		cfg := model.ProwlarrConfig{
			ID:        1,
			Enabled:   req.Enabled,
			BaseURL:   req.BaseURL,
			APIKey:    req.APIKey,
			UpdatedAt: time.Now(),
		}

		if err := db.WithContext(c.Request.Context()).Save(&cfg).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		if req.Enabled {
			src := model.TorrentSource{Key: sourceKey, Name: "Prowlarr"}
			db.WithContext(ctx).Where(model.TorrentSource{Key: sourceKey}).FirstOrCreate(&src)
		} else {
			db.WithContext(ctx).Where("key = ?", sourceKey).Delete(&model.TorrentSource{})
		}

		c.JSON(http.StatusOK, configResponse{
			Enabled:   cfg.Enabled,
			BaseURL:   cfg.BaseURL,
			APIKey:    cfg.APIKey,
			UpdatedAt: cfg.UpdatedAt,
		})
	}
}
