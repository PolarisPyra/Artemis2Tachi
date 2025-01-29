package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type BatchManualLampChuni string

const (
	AllJusticeCritical BatchManualLampChuni = "ALL JUSTICE CRITICAL"
	AllJustice         BatchManualLampChuni = "ALL JUSTICE"
	FullCombo          BatchManualLampChuni = "FULL COMBO"
	Clear              BatchManualLampChuni = "CLEAR"
	Failed             BatchManualLampChuni = "FAILED"
)

type BatchManualScoreChuni struct {
	Identifier   string               `json:"identifier"`
	MatchType    string               `json:"matchType"`
	Score        int                  `json:"score"`
	Lamp         BatchManualLampChuni `json:"lamp"`
	Difficulty   string               `json:"difficulty"`
	TimeAchieved *int64               `json:"timeAchieved,omitempty"`
	Judgements   *struct {
		JCrit   int `json:"jcrit"`
		Justice int `json:"justice"`
		Attack  int `json:"attack"`
		Miss    int `json:"miss"`
	} `json:"judgements,omitempty"`
	Optional *struct {
		MaxCombo int `json:"maxCombo"`
	} `json:"optional,omitempty"`
}

type BatchManualImportChuni struct {
	Meta struct {
		Game     string `json:"game"`
		Playtype string `json:"playtype"`
		Service  string `json:"service"`
	} `json:"meta"`
	Scores  []BatchManualScoreChuni `json:"scores"`
	Classes *struct {
		Dan    *string `json:"dan,omitempty"`
		Emblem *string `json:"emblem,omitempty"`
	} `json:"classes,omitempty"`
}

func fetchChuniTachiExport(db *sql.DB, userID string) (*BatchManualImportChuni, error) {
	var tachiExport BatchManualImportChuni

	// Fetch profile data
	var classEmblemBase, classEmblemMedal int
	err := db.QueryRow("SELECT classEmblemBase, classEmblemMedal FROM chuni_profile_data WHERE user = ?", userID).Scan(&classEmblemBase, &classEmblemMedal)
	if err != nil {
		return nil, err
	}

	// Fetch playlog data
	rows, err := db.Query("SELECT romVersion, userPlayDate, musicId, level, score, maxCombo, judgeGuilty, judgeAttack, judgeJustice, judgeCritical, judgeHeaven, isFullCombo, isAllJustice, isClear FROM chuni_score_playlog WHERE user = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var playlog struct {
			RomVersion    sql.NullString
			UserPlayDate  sql.NullString
			MusicID       sql.NullInt64
			Level         sql.NullInt64
			Score         sql.NullInt64
			MaxCombo      sql.NullInt64
			JudgeGuilty   sql.NullInt64
			JudgeAttack   sql.NullInt64
			JudgeJustice  sql.NullInt64
			JudgeCritical sql.NullInt64
			JudgeHeaven   sql.NullInt64
			IsFullCombo   sql.NullBool
			IsAllJustice  sql.NullBool
			IsClear       sql.NullBool
		}

		err := rows.Scan(&playlog.RomVersion, &playlog.UserPlayDate, &playlog.MusicID, &playlog.Level, &playlog.Score, &playlog.MaxCombo, &playlog.JudgeGuilty, &playlog.JudgeAttack, &playlog.JudgeJustice, &playlog.JudgeCritical, &playlog.JudgeHeaven, &playlog.IsFullCombo, &playlog.IsAllJustice, &playlog.IsClear)
		if err != nil {
			return nil, err
		}

		if !playlog.RomVersion.Valid || !playlog.MusicID.Valid || !playlog.Level.Valid || !playlog.Score.Valid || !playlog.JudgeJustice.Valid || !playlog.IsAllJustice.Valid || !playlog.IsFullCombo.Valid || !playlog.IsClear.Valid {
			continue
		}

		// Filter out WORLD'S END scores
		if (playlog.RomVersion.String[:2] == "1." && playlog.Level.Int64 == 4) || (playlog.RomVersion.String[:2] == "2." && playlog.Level.Int64 == 5) {
			continue
		}

		var lamp BatchManualLampChuni
		if playlog.IsAllJustice.Bool && playlog.JudgeJustice.Int64 == 0 {
			lamp = AllJusticeCritical
		} else if playlog.IsAllJustice.Bool {
			lamp = AllJustice
		} else if playlog.IsFullCombo.Bool {
			lamp = FullCombo
		} else if playlog.IsClear.Bool {
			lamp = Clear
		} else {
			lamp = Failed
		}

		tachiScore := BatchManualScoreChuni{
			Identifier: fmt.Sprintf("%d", playlog.MusicID.Int64),
			MatchType:  "inGameID",
			Score:      int(playlog.Score.Int64),
			Lamp:       lamp,
			Difficulty: []string{"BASIC", "ADVANCED", "EXPERT", "MASTER", "ULTIMA"}[playlog.Level.Int64],
		}

		if playlog.UserPlayDate.Valid {
			playDate, err := time.Parse("2006-01-02 15:04:05", playlog.UserPlayDate.String)
			if err == nil {
				tachiScore.TimeAchieved = new(int64)
				*tachiScore.TimeAchieved = playDate.Unix()
			}
		}

		if playlog.JudgeCritical.Valid && playlog.JudgeJustice.Valid && playlog.JudgeAttack.Valid && playlog.JudgeGuilty.Valid {
			tachiScore.Judgements = &struct {
				JCrit   int `json:"jcrit"`
				Justice int `json:"justice"`
				Attack  int `json:"attack"`
				Miss    int `json:"miss"`
			}{
				JCrit:   int(playlog.JudgeHeaven.Int64 + playlog.JudgeCritical.Int64),
				Justice: int(playlog.JudgeJustice.Int64),
				Attack:  int(playlog.JudgeAttack.Int64),
				Miss:    int(playlog.JudgeGuilty.Int64),
			}
		}

		if playlog.MaxCombo.Valid {
			tachiScore.Optional = &struct {
				MaxCombo int `json:"maxCombo"`
			}{
				MaxCombo: int(playlog.MaxCombo.Int64),
			}
		}

		tachiExport.Scores = append(tachiExport.Scores, tachiScore)
	}

	tachiExport.Meta.Game = "chunithm"
	tachiExport.Meta.Playtype = "Single"
	tachiExport.Meta.Service = "Cozynet"

	tachiExport.Classes = &struct {
		Dan    *string `json:"dan,omitempty"`
		Emblem *string `json:"emblem,omitempty"`
	}{
		Dan:    getChuniTachiClass(classEmblemBase),
		Emblem: getChuniTachiClass(classEmblemMedal),
	}

	return &tachiExport, nil
}

func getChuniTachiClass(class int) *string {
	tachiClasses := []string{"", "DAN_I", "DAN_II", "DAN_III", "DAN_IV", "DAN_V", "DAN_INFINITE"}
	if class >= 0 && class < len(tachiClasses) {
		return &tachiClasses[class]
	}
	return nil
}

func exportChuniToTachi(tachiExport *BatchManualImportChuni) error {
	if err := os.MkdirAll("exports", 0755); err != nil {
		return fmt.Errorf("failed to create exports directory: %w", err)
	}

	file, err := json.MarshalIndent(tachiExport, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal export data: %w", err)
	}

	if err := os.WriteFile("exports/chuni_tachi_export.json", file, 0644); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	return nil
}
