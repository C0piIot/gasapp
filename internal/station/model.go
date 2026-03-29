package station

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
