package service

import (
	"testing"

	"fiber-otdr-fault-localization/backend/internal/model"
)

func TestEffectiveConversionParamsUsesSnapshotForNewTraces(t *testing.T) {
	route := model.FiberRoute{LengthM: 9000, RefractiveIndex: 1.467, LaunchOffsetM: 300}
	trace := model.TraceCapture{SnapshotLengthM: 8000, SnapshotRefractiveIndex: 1.468, SnapshotLaunchOffsetM: 120}
	got := effectiveConversionParams(trace, route)
	if got.LengthM != 8000 || got.RefractiveIndex != 1.468 || got.LaunchOffsetM != 120 {
		t.Fatalf("expected snapshot params, got %+v", got)
	}
}

func TestEffectiveConversionParamsTreatsLegacyTraceOffsetAsZero(t *testing.T) {
	route := model.FiberRoute{LengthM: 9000, RefractiveIndex: 1.467, LaunchOffsetM: 300}
	legacy := model.TraceCapture{}
	got := effectiveConversionParams(legacy, route)
	if got.LengthM != 9000 {
		t.Fatalf("legacy trace length should fall back to route, got %v", got.LengthM)
	}
	if got.RefractiveIndex != 1.467 {
		t.Fatalf("legacy trace refractive index should fall back to route, got %v", got.RefractiveIndex)
	}
	if got.LaunchOffsetM != 0 {
		t.Fatalf("legacy trace launch offset must be treated as 0, got %v", got.LaunchOffsetM)
	}
}
