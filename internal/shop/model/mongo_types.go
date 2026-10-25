package model

// GeoPoint 表示 GeoJSON Point，坐标顺序为 [lng, lat]。
type GeoPoint struct {
	Type        string    `bson:"type"`
	Coordinates []float64 `bson:"coordinates"`
}

// GeoPolygon 表示 GeoJSON Polygon。
type GeoPolygon struct {
	Type        string        `bson:"type"`
	Coordinates [][][]float64 `bson:"coordinates"`
}
