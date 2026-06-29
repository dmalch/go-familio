package familio

// Date type qualifiers and the second calendar, complementing the
// calendarGregorian/dateTypeEqual/dateTypeBetween constants in person.go. These
// are familio's complex-date `type` vocabulary (see issue #5 / API.md): a single
// whole-date qualifier, not a per-endpoint flag.
const (
	calendarJulian = "julian"

	dateTypeAbout  = "about"
	dateTypeBefore = "before"
	dateTypeAfter  = "after"
)

// Range kinds accepted on a DateRange (the provider's user-facing vocabulary,
// mirroring the sibling terraform-provider-genealogy date model).
const (
	RangeBefore  = "before"
	RangeAfter   = "after"
	RangeBetween = "between"
)

// DateRange is the provider's domain value object for a genealogical date: a
// primary {Year,Month,Day}, an optional approximation (Circa) or bound/range
// (Range plus the End* second date), and a Calendar. It mirrors the date model
// of the sibling terraform-provider-genealogy so configs read the same across
// both providers.
//
// familio's wire shape (EventDate{calendar,type,first,second}) is an
// infrastructure detail; translate with EventDateFromRange / RangeFromEventDate.
// familio carries a single whole-date `type`, so Circa maps to type "about" and
// Range to before/after/between — the two are mutually exclusive on familio and
// the schema rejects combining them (familio has no per-endpoint approximation,
// so EndCirca is accepted only for cross-provider config symmetry and never
// reaches the wire).
type DateRange struct {
	Year  int
	Month *int
	Day   *int
	Circa bool

	Range    string // "" | RangeBefore | RangeAfter | RangeBetween
	EndYear  *int
	EndMonth *int
	EndDay   *int
	EndCirca bool

	Calendar string // "" => gregorian
}

// EventDateFromRange translates the domain DateRange into familio's wire
// EventDate. A nil DateRange is an unknown date (no parts). This is the single
// place that knows familio's calendar/type/first/second encoding.
func EventDateFromRange(r *DateRange) EventDate {
	if r == nil {
		return EventDate{Calendar: calendarGregorian, Type: dateTypeEqual}
	}

	calendar := r.Calendar
	if calendar == "" {
		calendar = calendarGregorian
	}
	first := &DatePart{Year: r.Year, Month: r.Month, Day: r.Day, Type: calendar}

	switch r.Range {
	case RangeBefore:
		return EventDate{Calendar: calendar, Type: dateTypeBefore, First: first}
	case RangeAfter:
		return EventDate{Calendar: calendar, Type: dateTypeAfter, First: first}
	case RangeBetween:
		second := &DatePart{Year: derefInt(r.EndYear), Month: r.EndMonth, Day: r.EndDay, Type: calendar}
		return EventDate{Calendar: calendar, Type: dateTypeBetween, First: first, Second: second}
	default:
		dateType := dateTypeEqual
		if r.Circa {
			dateType = dateTypeAbout
		}
		return EventDate{Calendar: calendar, Type: dateType, First: first}
	}
}

// RangeFromEventDate translates a wire EventDate back into the domain DateRange
// for read-back. It returns nil when the date is unknown (no first part).
// gregorian is the default calendar, so it surfaces as an empty Calendar (no
// spurious diff against a config that omits calendar).
func RangeFromEventDate(date EventDate) *DateRange {
	if date.First == nil {
		return nil
	}
	r := &DateRange{Year: date.First.Year, Month: date.First.Month, Day: date.First.Day}
	if date.Calendar == calendarJulian {
		r.Calendar = calendarJulian
	}

	switch date.Type {
	case dateTypeAbout:
		r.Circa = true
	case dateTypeBefore:
		r.Range = RangeBefore
	case dateTypeAfter:
		r.Range = RangeAfter
	case dateTypeBetween:
		r.Range = RangeBetween
		if date.Second != nil {
			year := date.Second.Year
			r.EndYear = &year
			r.EndMonth = date.Second.Month
			r.EndDay = date.Second.Day
		}
	}
	return r
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
