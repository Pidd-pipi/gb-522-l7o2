package model

import (
	"time"

	"gorm.io/datatypes"
)

type TraceCapture struct {
	ID               uint    `gorm:"primaryKey" json:"id"`
	RouteID          uint    `gorm:"not null;index" json:"route_id"`
	WavelengthNM     int     `gorm:"not null" json:"wavelength_nm"`
	PulseWidthNS     float64 `gorm:"not null" json:"pulse_width_ns"`
	SampleIntervalNS float64 `gorm:"not null" json:"sample_interval_ns"`
	// Snapshot of the route conversion parameters captured at import time.
	// Event distances of this trace are always converted from this snapshot,
	// never from the route parameters that may change later.
	SnapshotLengthM         float64        `gorm:"not null;default:0" json:"snapshot_length_m"`
	SnapshotRefractiveIndex float64        `gorm:"not null;default:0" json:"snapshot_refractive_index"`
	SnapshotLaunchOffsetM   float64        `gorm:"not null;default:0" json:"snapshot_launch_offset_m"`
	RawPointsJSON           datatypes.JSON `gorm:"type:jsonb;not null" json:"raw_points_json"`
	ProcessedJSON           datatypes.JSON `gorm:"type:jsonb" json:"processed_points_json"`
	NoiseFloorDB            float64        `gorm:"not null" json:"noise_floor_db"`
	DenoiseWindow           int            `gorm:"not null;default:5" json:"denoise_window"`
	PeakThresholdDB         float64        `gorm:"not null;default:0.8" json:"peak_threshold_db"`
	MergeWindow             int            `gorm:"not null;default:3" json:"merge_window"`
	CapturedAt              time.Time      `gorm:"not null;index" json:"captured_at"`
	UploadedBy              uint           `gorm:"not null;index" json:"uploaded_by"`
	CreatedAt               time.Time      `json:"created_at"`
	Route                   FiberRoute     `gorm:"foreignKey:RouteID" json:"-"`
}
