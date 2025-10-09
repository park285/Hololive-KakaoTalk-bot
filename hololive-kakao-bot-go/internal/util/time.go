package util

import "time"

var kstLocation *time.Location

func init() {
	var err error
	kstLocation, err = time.LoadLocation("Asia/Seoul")
	if err != nil {
		// Fallback to UTC+9
		kstLocation = time.FixedZone("KST", 9*60*60)
	}
}

// ToKST converts any time to KST timezone
func ToKST(t time.Time) time.Time {
	return t.In(kstLocation)
}

// FormatKST formats time in KST with given layout
func FormatKST(t time.Time, layout string) string {
	return t.In(kstLocation).Format(layout)
}

// NowKST returns current time in KST
func NowKST() time.Time {
	return time.Now().In(kstLocation)
}
