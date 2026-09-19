package repository

import (
	"errors"
	"fmt"

	"fiber-otdr-fault-localization/backend/internal/dto"
	"fiber-otdr-fault-localization/backend/internal/model"
	"gorm.io/gorm"
)

type EventRepository struct{ db *gorm.DB }

func (r *EventRepository) ReplaceForTrace(traceID uint, events []model.EventMarker) error {
	if err := r.db.Where("trace_id = ? AND reviewed = ?", traceID, false).Delete(&model.EventMarker{}).Error; err != nil {
		return fmt.Errorf("clear unreviewed events: %w", err)
	}
	if len(events) == 0 {
		return nil
	}
	if err := r.db.Create(&events).Error; err != nil {
		return fmt.Errorf("create detected events: %w", err)
	}
	return nil
}

func (r *EventRepository) Get(id uint) (model.EventMarker, error) {
	var event model.EventMarker
	if err := r.db.First(&event, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return event, ErrNotFound
		}
		return event, fmt.Errorf("get event marker: %w", err)
	}
	return event, nil
}

func (r *EventRepository) List(query dto.EventQuery) ([]model.EventMarker, int64, error) {
	db := r.db.Model(&model.EventMarker{})
	if query.TraceID != nil {
		db = db.Where("trace_id = ?", *query.TraceID)
	}
	if query.RouteID != nil {
		db = db.Where("trace_id IN (?)", r.db.Model(&model.TraceCapture{}).Select("id").Where("route_id = ?", *query.RouteID))
	}
	if query.Type.Valid() {
		db = db.Where("event_type = ?", query.Type)
	}
	if query.Reviewed != nil {
		db = db.Where("reviewed = ?", *query.Reviewed)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}
	var events []model.EventMarker
	if err := db.Order("trace_id DESC, distance_m ASC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&events).Error; err != nil {
		return nil, 0, fmt.Errorf("list events: %w", err)
	}
	return events, total, nil
}

func (r *EventRepository) ForTrace(traceID uint) ([]model.EventMarker, error) {
	var events []model.EventMarker
	if err := r.db.Where("trace_id = ?", traceID).Order("distance_m ASC").Find(&events).Error; err != nil {
		return nil, fmt.Errorf("list events for trace: %w", err)
	}
	return events, nil
}

// Review applies a manual judgment guarded by the event version so that two
// concurrent reviews of the same event cannot both succeed.
func (r *EventRepository) Review(event *model.EventMarker, expectedVersion uint) error {
	result := r.db.Model(&model.EventMarker{}).
		Where("id = ? AND version = ?", event.ID, expectedVersion).
		Updates(map[string]any{
			"event_type": event.EventType, "distance_m": event.DistanceM, "reviewed": true,
			"review_note": event.ReviewNote, "reviewed_by": event.ReviewedBy, "reviewed_at": event.ReviewedAt,
			"revision_count": event.RevisionCount, "version": gorm.Expr("version + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("review event marker: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (r *EventRepository) CreateRevision(revision *model.EventRevision) error {
	if err := r.db.Create(revision).Error; err != nil {
		return fmt.Errorf("create event revision: %w", err)
	}
	return nil
}

func (r *EventRepository) ListRevisions(eventID uint) ([]model.EventRevision, error) {
	var revisions []model.EventRevision
	if err := r.db.Where("event_id = ?", eventID).Order("revision_no DESC").Find(&revisions).Error; err != nil {
		return nil, fmt.Errorf("list event revisions: %w", err)
	}
	return revisions, nil
}

// LatestRevisions loads the newest revision per event in one query so lists
// can show the most recent change without N+1 lookups.
func (r *EventRepository) LatestRevisions(eventIDs []uint) (map[uint]model.EventRevision, error) {
	latest := make(map[uint]model.EventRevision, len(eventIDs))
	if len(eventIDs) == 0 {
		return latest, nil
	}
	sub := r.db.Model(&model.EventRevision{}).Select("event_id, MAX(revision_no) AS revision_no").Where("event_id IN ?", eventIDs).Group("event_id")
	var revisions []model.EventRevision
	if err := r.db.Joins("JOIN (?) latest ON latest.event_id = event_revisions.event_id AND latest.revision_no = event_revisions.revision_no", sub).Find(&revisions).Error; err != nil {
		return nil, fmt.Errorf("list latest event revisions: %w", err)
	}
	for _, revision := range revisions {
		latest[revision.EventID] = revision
	}
	return latest, nil
}
