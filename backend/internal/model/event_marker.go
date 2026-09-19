package model

import (
	"time"

	"fiber-otdr-fault-localization/backend/internal/constants"
)

type EventMarker struct {
	ID                       uint                `gorm:"primaryKey" json:"id"`
	TraceID                  uint                `gorm:"not null;index" json:"trace_id"`
	DistanceM                float64             `gorm:"not null" json:"distance_m"`
	EventType                constants.EventType `gorm:"size:24;not null;index;check:event_type_allowed,event_type IN ('connector','splice','bend','break','end','unknown')" json:"event_type"`
	InsertionLossDB          float64             `gorm:"not null" json:"insertion_loss_db"`
	ReflectanceDB            float64             `gorm:"not null" json:"reflectance_db"`
	Confidence               float64             `gorm:"not null" json:"confidence"`
	AlgorithmEventType       constants.EventType `gorm:"size:24;not null;check:algorithm_event_type_allowed,algorithm_event_type IN ('connector','splice','bend','break','end','unknown')" json:"algorithm_event_type"`
	AlgorithmDistanceM       float64             `gorm:"not null" json:"algorithm_distance_m"`
	AlgorithmInsertionLossDB float64             `gorm:"not null" json:"algorithm_insertion_loss_db"`
	Reviewed                 bool                `gorm:"not null;default:false;index" json:"reviewed"`
	ReviewNote               string              `gorm:"size:1000" json:"review_note"`
	ReviewedBy               *uint               `json:"reviewed_by"`
	ReviewedAt               *time.Time          `json:"reviewed_at"`
	RevisionCount            uint                `gorm:"not null;default:0" json:"revision_count"`
	Version                  uint                `gorm:"not null;default:1" json:"version"`
	LatestRevision           *EventRevision      `gorm:"-" json:"latest_revision,omitempty"`
	CreatedAt                time.Time           `json:"created_at"`
	UpdatedAt                time.Time           `json:"updated_at"`
}

// EventRevision preserves one superseded manual review judgment so that
// re-reviewing an event never erases the previous decision or its timestamp.
type EventRevision struct {
	ID                 uint                `gorm:"primaryKey" json:"id"`
	EventID            uint                `gorm:"not null;index" json:"event_id"`
	RevisionNo         uint                `gorm:"not null" json:"revision_no"`
	PreviousEventType  constants.EventType `gorm:"size:24;not null" json:"previous_event_type"`
	PreviousDistanceM  float64             `gorm:"not null" json:"previous_distance_m"`
	PreviousReviewNote string              `gorm:"size:1000" json:"previous_review_note"`
	PreviousReviewedBy *uint               `json:"previous_reviewed_by"`
	PreviousReviewedAt *time.Time          `json:"previous_reviewed_at"`
	NewEventType       constants.EventType `gorm:"size:24;not null" json:"new_event_type"`
	NewDistanceM       float64             `gorm:"not null" json:"new_distance_m"`
	NewReviewNote      string              `gorm:"size:1000" json:"new_review_note"`
	RevisedBy          uint                `gorm:"not null" json:"revised_by"`
	CreatedAt          time.Time           `gorm:"index" json:"created_at"`
}
