package familio

import (
	"testing"

	. "github.com/onsi/gomega"
)

func intp(v int) *int { return &v }

func TestEventDateFromRange(t *testing.T) {
	cases := []struct {
		name string
		in   *DateRange
		want EventDate
	}{
		{
			name: "nil is an unknown gregorian equal date",
			in:   nil,
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeEqual},
		},
		{
			name: "exact year only",
			in:   &DateRange{Year: 1846},
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeEqual,
				First: &DatePart{Year: 1846, Type: calendarGregorian}},
		},
		{
			name: "circa maps to about",
			in:   &DateRange{Year: 1846, Circa: true},
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeAbout,
				First: &DatePart{Year: 1846, Type: calendarGregorian}},
		},
		{
			name: "before bound",
			in:   &DateRange{Year: 1910, Range: RangeBefore},
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeBefore,
				First: &DatePart{Year: 1910, Type: calendarGregorian}},
		},
		{
			name: "after bound",
			in:   &DateRange{Year: 1897, Range: RangeAfter},
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeAfter,
				First: &DatePart{Year: 1897, Type: calendarGregorian}},
		},
		{
			name: "between range with two ends",
			in:   &DateRange{Year: 1846, Range: RangeBetween, EndYear: intp(1850), EndMonth: intp(3)},
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeBetween,
				First:  &DatePart{Year: 1846, Type: calendarGregorian},
				Second: &DatePart{Year: 1850, Month: intp(3), Type: calendarGregorian}},
		},
		{
			name: "julian calendar on both endpoints",
			in:   &DateRange{Year: 1846, Calendar: calendarJulian, Range: RangeBetween, EndYear: intp(1850)},
			want: EventDate{Calendar: calendarJulian, Type: dateTypeBetween,
				First:  &DatePart{Year: 1846, Type: calendarJulian},
				Second: &DatePart{Year: 1850, Type: calendarJulian}},
		},
		{
			name: "month and day on the first part",
			in:   &DateRange{Year: 1846, Month: intp(2), Day: intp(10)},
			want: EventDate{Calendar: calendarGregorian, Type: dateTypeEqual,
				First: &DatePart{Year: 1846, Month: intp(2), Day: intp(10), Type: calendarGregorian}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(EventDateFromRange(tc.in)).To(Equal(tc.want))
		})
	}
}

func TestRangeFromEventDateRoundTrip(t *testing.T) {
	cases := []*DateRange{
		nil,
		{Year: 1846},
		{Year: 1846, Month: intp(2), Day: intp(10)},
		{Year: 1846, Circa: true},
		{Year: 1910, Range: RangeBefore},
		{Year: 1897, Range: RangeAfter},
		{Year: 1846, Range: RangeBetween, EndYear: intp(1850), EndMonth: intp(3)},
		{Year: 1846, Calendar: calendarJulian},
		{Year: 1846, Calendar: calendarJulian, Range: RangeBetween, EndYear: intp(1850)},
	}
	for _, in := range cases {
		RegisterTestingT(t)
		Expect(RangeFromEventDate(EventDateFromRange(in))).To(Equal(in))
	}
}
