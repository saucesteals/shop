package facebook

type coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type optionalCoordinate struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (c coordinate) valid() bool {
	return c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}

func (c optionalCoordinate) coordinate() (coordinate, bool) {
	if c.Latitude == nil || c.Longitude == nil {
		return coordinate{}, false
	}
	point := coordinate{
		Latitude:  *c.Latitude,
		Longitude: *c.Longitude,
	}

	return point, point.valid()
}
