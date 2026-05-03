package station

// HistoryDays is the size of the daily-history window returned alongside each
// station. Both server and client agree on this length; days are oldest-first.
const HistoryDays = 30

// DailyHistory holds one station's daily-max price for one fuel over the most
// recent HistoryDays days. Index 0 is the oldest day; index HistoryDays-1 is
// "today" (the day containing the window's end timestamp). nil means no price
// is known for that day.
type DailyHistory [HistoryDays]*float64

type Station struct {
	ID           int64
	Name         string
	Updated      int64 // Unix timestamp
	PostalCode   string
	Address      string
	OpeningHours string
	Town         string
	City         string
	State        string
	Gasoil       *float64
	Petrol95     *float64
	Petrol98     *float64
	GLP          *float64
	Lat          float64
	Lng          float64
}
