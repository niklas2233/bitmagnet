package rssimporter

import (
	"net/http"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type apiHandler struct {
	lazyDB lazy.Lazy[*gorm.DB]
}

func (apiHandler) Key() string {
	return "rss_feeds_api"
}

func (h apiHandler) Apply(e *gin.Engine) error {
	db, err := h.lazyDB.Get()
	if err != nil {
		return err
	}
	handler := &rssFeedsHandler{db: db}
	group := e.Group("/api/rss-feeds")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.DELETE("/:source", handler.deleteBySource)
	return nil
}

type rssFeedsHandler struct {
	db *gorm.DB
}

type rssFeedResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

func (h *rssFeedsHandler) list(c *gin.Context) {
	var feeds []model.RssFeed
	if err := h.db.WithContext(c.Request.Context()).Find(&feeds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]rssFeedResponse, len(feeds))
	for i, f := range feeds {
		result[i] = rssFeedResponse{ID: f.ID, URL: f.URL, Source: f.Source, CreatedAt: f.CreatedAt}
	}
	c.JSON(http.StatusOK, result)
}

type createFeedRequest struct {
	URL    string `binding:"required" json:"url"`
	Source string `binding:"required" json:"source"`
}

func (h *rssFeedsHandler) create(c *gin.Context) {
	var req createFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	feed := model.RssFeed{ID: uuid.New().String(), URL: req.URL, Source: req.Source}
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&feed).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.TorrentSource{
			Key:  req.Source,
			Name: req.Source,
		}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, rssFeedResponse{
		ID: feed.ID, URL: feed.URL, Source: feed.Source, CreatedAt: feed.CreatedAt,
	})
}

func (h *rssFeedsHandler) deleteBySource(c *gin.Context) {
	source := c.Param("source")
	var rowsAffected int64
	err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("source = ?", source).Delete(&model.RssFeed{})
		if res.Error != nil {
			return res.Error
		}
		rowsAffected = res.RowsAffected
		if rowsAffected == 0 {
			return nil
		}
		return tx.Where("key = ?", source).Delete(&model.TorrentSource{}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "feed not found"})
		return
	}
	c.Status(http.StatusNoContent)
}
