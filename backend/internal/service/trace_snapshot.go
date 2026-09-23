package service

import "fiber-otdr-fault-localization/backend/internal/model"

// traceConversionParams holds the length/refractive index/launch offset that a
// trace's event distances must be converted with. Traces imported after the
// launch offset feature carry an immutable snapshot; traces imported before it
// fall back to the current route parameters and are treated as having a zero
// launch offset.
type traceConversionParams struct {
	LengthM         float64
	RefractiveIndex float64
	LaunchOffsetM   float64
}

func effectiveConversionParams(trace model.TraceCapture, route model.FiberRoute) traceConversionParams {
	params := traceConversionParams{
		LengthM:         trace.SnapshotLengthM,
		RefractiveIndex: trace.SnapshotRefractiveIndex,
		LaunchOffsetM:   trace.SnapshotLaunchOffsetM,
	}
	if params.LengthM <= 0 {
		params.LengthM = route.LengthM
	}
	if params.RefractiveIndex < 1.3 || params.RefractiveIndex > 1.7 {
		params.RefractiveIndex = route.RefractiveIndex
	}
	return params
}
