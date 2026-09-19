package service

import (
	"testing"
	"time"

	"fiber-otdr-fault-localization/backend/internal/constants"
	"fiber-otdr-fault-localization/backend/internal/dto"
	"fiber-otdr-fault-localization/backend/internal/model"
	"fiber-otdr-fault-localization/backend/internal/repository"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func reviewTestStore(t *testing.T) (*repository.Store, model.FiberRoute, model.TraceCapture, model.EventMarker) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:event-review-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.FiberRoute{}, &model.TraceCapture{}, &model.EventMarker{}, &model.EventRevision{}, &model.LocalizationCase{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	store := repository.NewStore(db)
	route := model.FiberRoute{RouteCode: "R-REV", Name: "修订测试线路", LengthM: 5000, RefractiveIndex: 1.468, LaunchConnector: "SC/APC", RouteStatus: "active"}
	if err := db.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	trace := model.TraceCapture{RouteID: route.ID, WavelengthNM: 1550, PulseWidthNS: 100, SampleIntervalNS: 1, RawPointsJSON: datatypes.JSON([]byte("[]")), CapturedAt: time.Now(), UploadedBy: 1}
	if err := db.Create(&trace).Error; err != nil {
		t.Fatal(err)
	}
	event := model.EventMarker{TraceID: trace.ID, DistanceM: 1200, EventType: constants.EventBend, InsertionLossDB: 1.2, ReflectanceDB: -40, Confidence: 0.9, AlgorithmEventType: constants.EventSplice, AlgorithmDistanceM: 1200, AlgorithmInsertionLossDB: 1.2, Version: 1}
	if err := db.Create(&event).Error; err != nil {
		t.Fatal(err)
	}
	return store, route, trace, event
}

func reviewActor(id uint, name string) Actor {
	return Actor{ID: id, Username: name, Role: constants.RoleReviewer, RequestID: "req-" + name}
}

func distancePtr(v float64) *float64 { return &v }

func TestReviewCreatesTrailAndKeepsPreviousDecision(t *testing.T) {
	store, _, _, event := reviewTestStore(t)
	svc := NewEventService(store)
	actor := reviewActor(7, "reviewer")

	first, err := svc.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBreak, DistanceM: distancePtr(1300), ReviewNote: "首次判定为断点", Version: 1}, actor)
	if err != nil {
		t.Fatalf("first review: %v", err)
	}
	updated := first.Event.(model.EventMarker)
	if updated.RevisionCount != 1 || updated.Version != 2 || first.Revision.PrevEventType != nil {
		t.Fatalf("unexpected first revision: %+v", first)
	}

	// Second review must carry the old human decision forward in the trail.
	second, err := svc.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBend, DistanceM: distancePtr(1320), ReviewNote: "复核后改回弯曲", Version: 2}, reviewActor(8, "admin"))
	if err != nil {
		t.Fatalf("second review: %v", err)
	}
	rev := second.Revision
	if rev.RevisionNo != 2 || rev.PrevEventType == nil || *rev.PrevEventType != constants.EventBreak {
		t.Fatalf("previous decision not preserved: %+v", rev)
	}
	if rev.PrevReviewNote != "首次判定为断点" || rev.PrevReviewedBy == nil || *rev.PrevReviewedBy != 7 || rev.PrevReviewedAt == nil {
		t.Fatalf("previous review metadata not preserved: %+v", rev)
	}

	history, err := svc.Revisions(event.ID)
	if err != nil || len(history) != 2 || history[0].RevisionNo != 2 || history[1].RevisionNo != 1 {
		t.Fatalf("revision history mismatch: %v %+v", err, history)
	}
}

func TestConcurrentRevisionSucceedsOnce(t *testing.T) {
	store, _, _, event := reviewTestStore(t)
	svc := NewEventService(store)
	if _, err := svc.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBreak, ReviewNote: "并发修订内容A", Version: 1}, reviewActor(7, "r1")); err != nil {
		t.Fatalf("winner review: %v", err)
	}
	_, err := svc.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBend, ReviewNote: "并发修订内容B", Version: 1}, reviewActor(8, "r2"))
	if appErr, ok := err.(*AppError); !ok || appErr.Code != CodeConflict {
		t.Fatalf("expected STATE_CONFLICT for stale version, got %v", err)
	}
	history, err := svc.Revisions(event.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("exactly one revision must be recorded, got %d (%v)", len(history), err)
	}
}

func makeCase(t *testing.T, store *repository.Store, routeID, traceID uint, status constants.CaseStatus, conclusion string) model.LocalizationCase {
	t.Helper()
	item := model.LocalizationCase{RouteID: routeID, BaselineTraceID: traceID, CurrentTraceID: traceID, CaseStatus: status, ParametersJSON: datatypes.JSON([]byte(`{}`)), DifferencesJSON: datatypes.JSON([]byte("[]")), Conclusion: conclusion, Version: 2, CreatedBy: 1}
	if status == constants.CaseConfirmed {
		item.ReviewerID = uintPtr(7)
	}
	if err := store.DB.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	return item
}

func TestReviewInvalidatesOpenReferencingCase(t *testing.T) {
	store, route, trace, event := reviewTestStore(t)
	makeCase(t, store, route.ID, trace.ID, constants.CasePendingReview, "")
	makeCase(t, store, route.ID, trace.ID, constants.CaseConfirmed, "已形成的结论")

	svc := NewEventService(store)
	result, err := svc.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBreak, ReviewNote: "修订导致案例失效", Version: 1}, reviewActor(7, "reviewer"))
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if len(result.InvalidatedCases) != 2 {
		t.Fatalf("expected 2 invalidated cases, got %d", len(result.InvalidatedCases))
	}
	for _, invalidation := range result.InvalidatedCases {
		got, err := store.Cases.Get(invalidation.CaseID)
		if err != nil {
			t.Fatal(err)
		}
		if got.CaseStatus != constants.CaseDraft || !got.RequiresReanalysis {
			t.Fatalf("case %d not returned to draft: %+v", got.ID, got)
		}
		if got.AnalysisError == "" || got.Conclusion != "" || got.ReviewerID != nil {
			t.Fatalf("invalidation reason / stale conclusion not reset on case %d: %+v", got.ID, got)
		}
	}
}

func TestReviewRejectedWhenClosedCaseReferencesTrace(t *testing.T) {
	store, route, trace, event := reviewTestStore(t)
	makeCase(t, store, route.ID, trace.ID, constants.CaseClosed, "不可变结论")
	svc := NewEventService(store)
	_, err := svc.Review(event.ID, dto.ReviewEventRequest{EventType: constants.EventBreak, ReviewNote: "不应允许的修订", Version: 1}, reviewActor(7, "reviewer"))
	if appErr, ok := err.(*AppError); !ok || appErr.Code != CodeConflict {
		t.Fatalf("expected STATE_CONFLICT due to closed case, got %v", err)
	}
	// Event must remain untouched and no revision trail written.
	reloaded, _ := store.Events.Get(event.ID)
	if reloaded.Version != 1 || reloaded.Reviewed {
		t.Fatalf("event modified despite closed case: %+v", reloaded)
	}
	history, _ := store.Events.ListRevisions(event.ID)
	if len(history) != 0 {
		t.Fatalf("expected no revision entries, got %d", len(history))
	}
}

func uintPtr(v uint) *uint { return &v }
