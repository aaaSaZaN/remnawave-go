package cron

import (
	"context"
	"time"

	"remnawave-go/internal/database"

	"gorm.io/gorm"
)

type Scheduler struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Scheduler {
	return &Scheduler{db: db}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkExpiredUsers()
			s.checkTrafficLimits()
		}
	}
}

func (s *Scheduler) checkExpiredUsers() {
	now := time.Now()
	s.db.Model(&database.User{}).
		Where("status = ? AND expire_at <= ?", "ACTIVE", now).
		Update("status", "EXPIRED")
}

func (s *Scheduler) checkTrafficLimits() {
	var users []database.User
	s.db.Preload("Traffic").
		Where("status = ? AND traffic_limit_bytes > 0", "ACTIVE").
		Find(&users)

	for _, u := range users {
		if u.Traffic != nil && u.Traffic.UsedTrafficBytes >= u.TrafficLimitBytes {
			s.db.Model(&database.User{}).Where("id = ?", u.ID).Update("status", "LIMITED")
		}
	}
}
