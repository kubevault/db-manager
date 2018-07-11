package framework

import (
	"testing"
	"time"
	_ "strings"
	"fmt"
	"math"
	"strings"
)

func TestGetDateString(t *testing.T) {
	tm := time.Now()
	tm.Nanosecond()
	res := GetDateString(tm)
	y := processValue(tm.Year(), 4)
	mon := processValue(int(tm.Month()), 2)
	d := processValue(tm.Day(), 2)
	h := processValue(tm.Hour(), 2)
	min := processValue(tm.Minute(), 2)
	s := processValue(tm.Second(), 2)
	n := processValue(tm.Nanosecond(), 9)

	exp := fmt.Sprintf("%s-%s-%sT%s:%s:%s.%s%%2B06:00", y, mon, d, h, min, s, n)
	if res != exp {
		t.Errorf("Sum was incorrect, got: \"%s\", want: \"%s\".", res, exp)
	}
}

func processValue(val, reqLen int) string {
	if val == 0 {
		return strings.Repeat("0", reqLen)
	}
	digNum := int(math.Floor(math.Log10(float64(val))) + 1)

	return fmt.Sprintf("%s%d", strings.Repeat("0", (reqLen - digNum)), val)
}
