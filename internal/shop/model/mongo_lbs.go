package model

import "time"

const citiesMongoCollectionName = "cities"

// CitiesDocument 是 cities 集合（城市）的 MongoDB 文档模型。
type CitiesDocument struct {
	ID               string     `bson:"_id"`
	Code             string     `bson:"code"`
	Name             string     `bson:"name"`
	Province         string     `bson:"province"`
	Center           GeoPoint   `bson:"center"`
	Boundary         GeoPolygon `bson:"boundary"`
	OpenStatus       string     `bson:"open_status"`
	OpenedAt         time.Time  `bson:"opened_at"`
	ServiceTimeStart string     `bson:"service_time_start"`
	ServiceTimeEnd   string     `bson:"service_time_end"`
	Timezone         string     `bson:"timezone"`
	ProductCodes     []string   `bson:"product_codes"`
	Hotline          string     `bson:"hotline"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (CitiesDocument) MongoCollectionName() string {
	return citiesMongoCollectionName
}

const poisMongoCollectionName = "pois"

// PoisDocument 是 pois 集合（POI 与推荐上车点）的 MongoDB 文档模型。
type PoisDocument struct {
	ID                string   `bson:"_id"`
	PoiID             string   `bson:"poi_id"`
	Name              string   `bson:"name"`
	Address           string   `bson:"address"`
	Location          GeoPoint `bson:"location"`
	CityCode          string   `bson:"city_code"`
	Category          string   `bson:"category"`
	IsRecommendPickup bool     `bson:"is_recommend_pickup"`
	ParentPoiID       string   `bson:"parent_poi_id"`
	GuideImages       []string `bson:"guide_images"`
	GuideText         string   `bson:"guide_text"`
	HeatScore         int64    `bson:"heat_score"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (PoisDocument) MongoCollectionName() string {
	return poisMongoCollectionName
}

const geoFencesMongoCollectionName = "geo_fences"

// GeoFencesDocument 是 geo_fences 集合（电子围栏）的 MongoDB 文档模型。
type GeoFencesDocument struct {
	ID             string     `bson:"_id"`
	Name           string     `bson:"name"`
	CityCode       string     `bson:"city_code"`
	Type           string     `bson:"type"`
	Geometry       GeoPolygon `bson:"geometry"`
	Status         string     `bson:"status"`
	EffectiveStart time.Time  `bson:"effective_start"`
	EffectiveEnd   time.Time  `bson:"effective_end"`
	Action         string     `bson:"action"`
	Remark         string     `bson:"remark"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (GeoFencesDocument) MongoCollectionName() string {
	return geoFencesMongoCollectionName
}
