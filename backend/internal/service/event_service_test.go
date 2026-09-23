package service

import (
	"encoding/json"
	"testing"

	"fiber-otdr-fault-localization/backend/internal/dto"
	"fiber-otdr-fault-localization/backend/internal/model"
	"fiber-otdr-fault-localization/backend/internal/repository"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newEventTestStore(t *testing.T) *repository.Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:event-service?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.FiberRoute{}, &model.TraceCapture{}, &model.EventMarker{}, &model.LocalizationCase{}, &model.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	return repository.NewStore(db)
}

func stepTracePoints() []float64 {
	pts := make([]float64, 400)
	for i := range pts {
		pts[i] = 28 - float64(i)*0.02
		if i >= 50 {
			pts[i] -= 3
		}
	}
	return pts
}

func TestDetectLegacyTraceTreatsMissingOffsetAsZero(t *testing.T) {
	store := newEventTestStore(t)
	// Route currently carries a launch offset; legacy traces never saved one.
	route := model.FiberRoute{RouteCode: "LEGACY1", Name: "legacy", LengthM: 500, RefractiveIndex: 1.468, LaunchOffsetM: 150, LaunchConnector: "SC/APC", RouteStatus: "active"}
	if err := store.Routes.Create(&route); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(stepTracePoints())
	legacy := model.TraceCapture{RouteID: route.ID, WavelengthNM: 1550, PulseWidthNS: 100, SampleIntervalNS: 20, RawPointsJSON: datatypes.JSON(raw), NoiseFloorDB: 17, DenoiseWindow: 5, PeakThresholdDB: 0.8, MergeWindow: 3, UploadedBy: 1}
	if err := store.Traces.Create(&legacy); err != nil {
		t.Fatal(err)
	}

	svc := NewEventService(store)
	summary, err := svc.Detect(legacy.ID, dto.DetectEventsRequest{}, Actor{ID: 1, Username: "analyst", Role: "analyst", RequestID: "req-legacy"})
	if err != nil {
		t.Fatalf("legacy detection must not be rejected: %v", err)
	}
	if summary.DetectedCount != 1 {
		t.Fatalf("expected 1 event, got %d", summary.DetectedCount)
	}
	events, err := store.Events.ForTrace(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Sample 50 at 20 ns / 1.468 is ~102.11 m raw. With the missing offset
	// treated as 0 it stays in-bounds; subtracting 150 m would skip it.
	if events[0].DistanceM < 100 || events[0].DistanceM > 105 {
		t.Fatalf("legacy event distance %.2f not converted with zero offset", events[0].DistanceM)
	}
}

func TestDetectRejectsAndPreservesPreviousEvents(t *testing.T) {
	store := newEventTestStore(t)
	route := model.FiberRoute{RouteCode: "REJECT1", Name: "reject", LengthM: 500, RefractiveIndex: 1.468, LaunchOffsetM: 0, LaunchConnector: "SC/APC", RouteStatus: "active"}
	if err := store.Routes.Create(&route); err != nil {
		t.Fatal(err)
	}
	pts := make([]float64, 400)
	for i := range pts {
		pts[i] = 28 - float64(i)*0.02
		if i >= 250 { // ~510.56 m: within 500? no -> beyond length, rejected run
			pts[i] -= 4.5
		}
	}
	raw, _ := json.Marshal(pts)
	trace := model.TraceCapture{RouteID: route.ID, WavelengthNM: 1550, PulseWidthNS: 100, SampleIntervalNS: 20, SnapshotLengthM: 500, SnapshotRefractiveIndex: 1.468, SnapshotLaunchOffsetM: 0, RawPointsJSON: datatypes.JSON(raw), NoiseFloorDB: 17, DenoiseWindow: 5, PeakThresholdDB: 4, MergeWindow: 3, UploadedBy: 1}
	if err := store.Traces.Create(&trace); err != nil {
		t.Fatal(err)
	}
	// Seed a previous detection result that must survive the rejection.
	previous := model.EventMarker{TraceID: trace.ID, DistanceM: 123.45, EventType: "splice", InsertionLossDB: 1, AlgorithmEventType: "splice", AlgorithmDistanceM: 123.45, AlgorithmInsertionLossDB: 1}
	if err := store.Events.ReplaceForTrace(trace.ID, []model.EventMarker{previous}); err != nil {
		t.Fatal(err)
	}
	svc := NewEventService(store)
	if _, err := svc.Detect(trace.ID, dto.DetectEventsRequest{PeakThresholdDB: 1}, Actor{ID: 1, Username: "analyst", Role: "analyst", RequestID: "req-reject"}); err == nil {
		t.Fatal("expected detection rejection for out-of-bounds event")
	}
	events, err := store.Events.ForTrace(trace.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].DistanceM != 123.45 {
		t.Fatalf("previous events must be retained, got %#v", events)
	}
}
