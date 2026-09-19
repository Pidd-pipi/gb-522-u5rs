package dto

import (
	"time"

	"fiber-otdr-fault-localization/backend/internal/constants"
)

type ReviewEventRequest struct {
	EventType  constants.EventType `json:"event_type" validate:"required"`
	DistanceM  *float64            `json:"distance_m" validate:"omitempty,gte=0"`
	ReviewNote string              `json:"review_note" validate:"required,min=3,max=1000"`
	Version    uint                `json:"version" validate:"required,gt=0"`
}

type EventQuery struct {
	TraceID  *uint
	RouteID  *uint
	Type     constants.EventType
	Reviewed *bool
	Page     int
	PageSize int
}

type DetectionSummary struct {
	TraceID       uint    `json:"trace_id"`
	DetectedCount int     `json:"detected_count"`
	NoiseFloorDB  float64 `json:"noise_floor_db"`
	ThresholdDB   float64 `json:"threshold_db"`
	RejectedCount int     `json:"rejected_out_of_bounds"`
}

// EventRevisionView is one immutable step of an event's human review trail.
type EventRevisionView struct {
	ID             uint                 `json:"id"`
	RevisionNo     int                  `json:"revision_no"`
	EventType      constants.EventType  `json:"event_type"`
	DistanceM      float64              `json:"distance_m"`
	ReviewNote     string               `json:"review_note"`
	ReviewedBy     uint                 `json:"reviewed_by"`
	ReviewerName   string               `json:"reviewer_name"`
	ReviewedAt     time.Time            `json:"reviewed_at"`
	PrevEventType  *constants.EventType `json:"prev_event_type,omitempty"`
	PrevDistanceM  *float64             `json:"prev_distance_m,omitempty"`
	PrevReviewNote string               `json:"prev_review_note,omitempty"`
	PrevReviewedBy *uint                `json:"prev_reviewed_by,omitempty"`
	PrevReviewer   string               `json:"prev_reviewer_name,omitempty"`
	PrevReviewedAt *time.Time           `json:"prev_reviewed_at,omitempty"`
}

// ReviewEventResult returns the updated event plus the new revision step and
// any cases that were invalidated, so callers can show the closure immediately.
type ReviewEventResult struct {
	Event            any                `json:"event"`
	Revision         EventRevisionView  `json:"revision"`
	InvalidatedCases []CaseInvalidation `json:"invalidated_cases"`
}

type CaseInvalidation struct {
	CaseID  uint   `json:"case_id"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
	Version uint   `json:"version"`
}
