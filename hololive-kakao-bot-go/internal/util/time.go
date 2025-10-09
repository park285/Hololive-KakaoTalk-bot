package util

import "time"

var kstLocation *time.Location

func init() {
	var err error
	kstLocation, err = time.LoadLocation("Asia/Seoul")
	if err != nil {
		kstLocation = time.FixedZone("KST", 9*60*60)
	}
}

func ToKST(t time.Time) time.Time {
	return t.In(kstLocation)
}

func FormatKST(t time.Time, layout string) string {
	return t.In(kstLocation).Format(layout)
}

func NowKST() time.Time {
	return time.Now().In(kstLocation)
}
