package rotating

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if !assert.NoError(t, err, `time.LoadLocation should succeed`) {
		return
	}

	t.Run("Hourly interval", func(t *testing.T) {
		testcases := []struct {
			Time     time.Time
			Interval time.Duration
			Expected time.Time
		}{
			{
				Time:     time.Date(2021, 1, 1, 0, 5, 0, 0, time.UTC),
				Interval: time.Hour,
				Expected: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				Time:     time.Date(2021, 1, 1, 1, 0, 0, 0, time.UTC),
				Interval: time.Hour,
				Expected: time.Date(2021, 1, 1, 1, 0, 0, 0, time.UTC),
			},
			{
				Time:     time.Date(2021, 1, 1, 0, 5, 30, 30, tokyo),
				Interval: time.Minute,
				Expected: time.Date(2021, 1, 1, 0, 5, 0, 0, tokyo),
			},
			{
				Time:     time.Date(2021, 1, 1, 0, 10, 45, 3000, tokyo),
				Interval: time.Minute,
				Expected: time.Date(2021, 1, 1, 0, 10, 0, 0, tokyo),
			},
		}

		for _, tc := range testcases {
			tc := tc
			t.Run(fmt.Sprintf("%s", tc.Time), func(t *testing.T) {
				if !assert.Equal(t, tc.Expected, truncate(tc.Time, tc.Interval)) {
					return
				}
			})
		}
	})
}
