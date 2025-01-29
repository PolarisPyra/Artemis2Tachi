package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type BatchManualLampGeki string

const (
	TIME_OFFSET                       = 9 * 3600 * 1000
	AllBreak      BatchManualLampGeki = "ALL BREAK"
	FullComboGeki BatchManualLampGeki = "FULL COMBO"
	FullBell      BatchManualLampGeki = "FULL BELL"
	ClearGeki     BatchManualLampGeki = "CLEAR"
	Loss          BatchManualLampGeki = "LOSS"
)

var DIFFICULTY_MAP = map[int]string{
	0:  "BASIC",
	1:  "ADVANCED",
	2:  "EXPERT",
	3:  "MASTER",
	10: "LUNATIC",
}

type BatchManualScoreGeki struct {
	Identifier   string              `json:"identifier"`
	MatchType    string              `json:"matchType"`
	Score        int                 `json:"score"`
	Lamp         BatchManualLampGeki `json:"lamp"`
	Difficulty   string              `json:"difficulty"`
	TimeAchieved *int64              `json:"timeAchieved,omitempty"`
	Judgements   *struct {
		CBreak int `json:"cbreak"`
		Break  int `json:"break"`
		Hit    int `json:"hit"`
		Miss   int `json:"miss"`
	} `json:"judgements,omitempty"`
	Optional *struct {
		MaxCombo       int `json:"maxCombo"`
		Damage         int `json:"damage"`
		BellCount      int `json:"bellCount"`
		TotalBellCount int `json:"totalBellCount"`
		PlatScore      int `json:"platScore"`
	} `json:"optional,omitempty"`
}

type BatchManualImportGeki struct {
	Meta struct {
		Game     string `json:"game"`
		Playtype string `json:"playtype"`
		Service  string `json:"service"`
	} `json:"meta"`
	Scores []BatchManualScoreGeki `json:"scores"`
}

func fetchOngekiExport(db *sql.DB, userID string) (*BatchManualImportGeki, error) {
	var tachiExport BatchManualImportGeki
	tachiExport.Meta.Game = "ongeki"
	tachiExport.Meta.Playtype = "Single"
	tachiExport.Meta.Service = "batch-artemis-export"
	tachiExport.Scores = []BatchManualScoreGeki{} // Initialize slice to avoid `null` in JSON

	rows, err := db.Query(`
		SELECT 
			userPlayDate, musicId, clearStatus, level as difficulty,
			techScore, maxCombo, judgeMiss, judgeHit, judgeBreak,
			judgeCriticalBreak, bellCount, damageCount, isFullCombo,
			isFullBell, isAllBreak, platinumScore, totalBellCount
		FROM ongeki_score_playlog
		WHERE user = ?
		ORDER BY userPlayDate
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlog: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var playlog struct {
			UserPlayDate       sql.NullString
			MusicID            sql.NullInt32
			ClearStatus        sql.NullInt32
			Difficulty         sql.NullInt32
			TechScore          sql.NullInt32
			MaxCombo           sql.NullInt32
			JudgeMiss          sql.NullInt32
			JudgeHit           sql.NullInt32
			JudgeBreak         sql.NullInt32
			JudgeCriticalBreak sql.NullInt32
			BellCount          sql.NullInt32
			DamageCount        sql.NullInt32
			IsFullCombo        sql.NullInt32
			IsFullBell         sql.NullInt32
			IsAllBreak         sql.NullInt32
			PlatinumScore      sql.NullInt32
			TotalBellCount     sql.NullInt32
		}

		err := rows.Scan(
			&playlog.UserPlayDate, &playlog.MusicID, &playlog.ClearStatus, &playlog.Difficulty,
			&playlog.TechScore, &playlog.MaxCombo, &playlog.JudgeMiss, &playlog.JudgeHit,
			&playlog.JudgeBreak, &playlog.JudgeCriticalBreak, &playlog.BellCount,
			&playlog.DamageCount, &playlog.IsFullCombo, &playlog.IsFullBell,
			&playlog.IsAllBreak, &playlog.PlatinumScore, &playlog.TotalBellCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan playlog row: %v", err)
		}

		// Determine lamp status
		var lamp BatchManualLampGeki
		switch {
		case playlog.IsAllBreak.Valid && playlog.IsAllBreak.Int32 == 1:
			lamp = AllBreak
		case playlog.IsFullCombo.Valid && playlog.IsFullCombo.Int32 == 1:
			lamp = FullComboGeki
		case playlog.IsFullBell.Valid && playlog.IsFullBell.Int32 == 1:
			lamp = FullBell
		case playlog.ClearStatus.Valid && playlog.ClearStatus.Int32 > 0:
			lamp = ClearGeki
		default:
			lamp = Loss
		}

		// Convert difficulty
		difficulty, ok := DIFFICULTY_MAP[int(playlog.Difficulty.Int32)]
		if !ok {
			difficulty = "UNKNOWN"
		}

		// Convert timestamps (handling possible NULL values)
		var timeAchieved *int64
		if playlog.UserPlayDate.Valid {
			parsedTime, err := time.Parse("2006-01-02 15:04:05", playlog.UserPlayDate.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse userPlayDate: %v", err)
			}
			timestamp := parsedTime.UnixMilli() + TIME_OFFSET
			timeAchieved = &timestamp
		}
		// Construct score object
		score := BatchManualScoreGeki{
			Identifier:   fmt.Sprintf("%d", playlog.MusicID.Int32),
			MatchType:    "inGameID",
			Score:        int(playlog.TechScore.Int32),
			Lamp:         lamp,
			Difficulty:   difficulty,
			TimeAchieved: timeAchieved,
		}

		// Add judgements
		score.Judgements = &struct {
			CBreak int `json:"cbreak"`
			Break  int `json:"break"`
			Hit    int `json:"hit"`
			Miss   int `json:"miss"`
		}{
			CBreak: int(playlog.JudgeCriticalBreak.Int32),
			Break:  int(playlog.JudgeBreak.Int32),
			Hit:    int(playlog.JudgeHit.Int32),
			Miss:   int(playlog.JudgeMiss.Int32),
		}

		// Add optional fields
		score.Optional = &struct {
			MaxCombo       int `json:"maxCombo"`
			Damage         int `json:"damage"`
			BellCount      int `json:"bellCount"`
			TotalBellCount int `json:"totalBellCount"`
			PlatScore      int `json:"platScore"`
		}{
			MaxCombo:       int(playlog.MaxCombo.Int32),
			Damage:         int(playlog.DamageCount.Int32),
			BellCount:      int(playlog.BellCount.Int32),
			TotalBellCount: int(playlog.TotalBellCount.Int32),
			PlatScore:      int(playlog.PlatinumScore.Int32),
		}

		// Append the score to the list
		tachiExport.Scores = append(tachiExport.Scores, score)
	}

	return &tachiExport, nil
}

func exportOngekiToTachi(tachiExportGeki *BatchManualImportGeki) error {
	if err := os.MkdirAll("exports", 0755); err != nil {
		return fmt.Errorf("failed to create exports directory: %w", err)
	}

	file, err := json.MarshalIndent(tachiExportGeki, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal export data: %w", err)
	}

	if err := os.WriteFile("exports/ongeki_tachi_export.json", file, 0644); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	return nil
}
