package gamehistory

import (
	"encoding/json"
	"strconv"
	"strings"

	"gorm.io/datatypes"
)

const (
	EndReasonCompleted = "completed"
	EndReasonAbandoned = "abandoned"
)

func ParseUserIDFromPlayerID(playerID string) (int64, bool) {
	idx := strings.LastIndex(playerID, "-")
	if idx < 0 || idx == len(playerID)-1 {
		return 0, false
	}
	v, err := strconv.ParseInt(playerID[idx+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func MarshalToJSON(v interface{}) (datatypes.JSON, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}
