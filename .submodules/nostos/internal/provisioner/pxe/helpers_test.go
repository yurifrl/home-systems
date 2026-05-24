package pxe

import "time"

func timeZero() time.Time {
	return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
}
