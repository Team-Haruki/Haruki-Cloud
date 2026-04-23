package datafiles

import _ "embed"

//go:embed custom_room_pt.csv
var customRoomPTCSV string

func CustomRoomPTCSV() string {
	return customRoomPTCSV
}
