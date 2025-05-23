package models

import (
	"time"
)

// Booster represents and active booster a player has
type Booster struct {
	ID        string
	Type      string
	Value     float64
	ExpiersAt time.Time
	Source    string
}

// Player repcrenset a player's profile data stored presistently in MongoDB

type Player struct {
	UUID               string
	Username           string
	Team               string
	TotalPlaytimeTicks float64
	DeltaPlayTimeTicks float64
	Banned             bool
	BanExpiresAt       *time.Time
	ActiveBoosters     []Booster
	LastLoginAt        *time.Time
	CreatedAt          *time.Time
	IsNewPlayer        bool `json:"-" bson:"-"`
}
