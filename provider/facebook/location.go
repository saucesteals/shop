package facebook

type coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (c coordinate) valid() bool {
	return c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}
