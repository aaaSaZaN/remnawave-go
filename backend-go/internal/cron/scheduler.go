package cron

import (
	"context"
	"time"

	"remnawave-go/internal/database"

	"gorm.io/gorm"
)

type UserSyncer interface {
	SyncUserToNodes(user *database.User, prevVlessUUID *string)
	RemoveUserFromNodes(user *database.User)
}

type Scheduler struct {
	db     *gorm.DB
	syncer UserSyncer
}

func New(db *gorm.DB, syncer ...UserSyncer) *Scheduler {
	var s UserSyncer
	if len(syncer) > 0 {
		s = syncer[0]
	}
	return &Scheduler{db: db, syncer: s}
}

func (s *Scheduler) SetSyncer(syncer UserSyncer) {
	s.syncer = syncer
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
	var expiredUsers []database.User
	if err := s.db.Where("status = ? AND expire_at <= ?", "ACTIVE", now).Find(&expiredUsers).Error; err != nil {
		return
	}
	if len(expiredUsers) == 0 {
		return
	}

	var ids []uint64
	for _, u := range expiredUsers {
		ids = append(ids, u.ID)
	}
	s.db.Model(&database.User{}).Where("id IN ?", ids).Update("status", "EXPIRED")

	if s.syncer != nil {
		for i := range expiredUsers {
			expiredUsers[i].Status = "EXPIRED"
			s.syncer.SyncUserToNodes(&expiredUsers[i], nil)
		}
	}
}

func (s *Scheduler) checkTrafficLimits() {
	var users []database.User
	if err := s.db.Preload("Traffic").
		Where("status = ? AND traffic_limit_bytes > 0", "ACTIVE").
		Find(&users).Error; err != nil {
		return
	}

	for i := range users {
		u := &users[i]
		if u.Traffic != nil && u.Traffic.UsedTrafficBytes >= u.TrafficLimitBytes {
			s.db.Model(&database.User{}).Where("id = ?", u.ID).Update("status", "LIMITED")
			if s.syncer != nil {
				u.Status = "LIMITED"
				s.syncer.SyncUserToNodes(u, nil)
			}
		}
	}
}
