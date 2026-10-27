package service

import "testing"

// resolvePoint 决定派单地点：事件坐标可用时以事件为准，否则回落演示默认值。
func TestResolvePointPrefersEventCoordinates(t *testing.T) {
	// 乘客真实在天府三街，演示默认值在春熙路——两者相距约 12 公里。
	const fbName, fbLat, fbLng = "春熙路", 30.6570, 104.0810

	// 1. 事件携带可用坐标：必须用事件值，否则司机会被派到错误的上车点。
	name, lat, lng := resolvePoint(&eventLatLng{Name: "天府三街", Lat: 30.5470, Lng: 104.0665},
		fbName, fbLat, fbLng, "上车点")
	if name != "天府三街" || lat != 30.5470 || lng != 104.0665 {
		t.Fatalf("got (%s,%v,%v), want event coordinates", name, lat, lng)
	}

	// 2. 坐标为空：回落默认值，保证老版本事件（无坐标）仍可派单。
	name, lat, lng = resolvePoint(nil, fbName, fbLat, fbLng, "上车点")
	if name != fbName || lat != fbLat || lng != fbLng {
		t.Fatalf("got (%s,%v,%v), want fallback", name, lat, lng)
	}

	// 3. 零值坐标代表「未上报」，不是「在几内亚湾」：必须回落而不是派到 (0,0)。
	name, lat, lng = resolvePoint(&eventLatLng{Lat: 0, Lng: 0}, fbName, fbLat, fbLng, "上车点")
	if name != fbName || lat != fbLat || lng != fbLng {
		t.Fatalf("zero coordinate must fall back, got (%s,%v,%v)", name, lat, lng)
	}

	// 4. 越界坐标：同样不可用。
	name, lat, lng = resolvePoint(&eventLatLng{Lat: 91, Lng: 200}, fbName, fbLat, fbLng, "上车点")
	if name != fbName || lat != fbLat || lng != fbLng {
		t.Fatalf("out-of-range coordinate must fall back, got (%s,%v,%v)", name, lat, lng)
	}

	// 5. 有坐标但缺名称：用缺省名称兜底，坐标仍取事件值。
	name, lat, lng = resolvePoint(&eventLatLng{Lat: 30.5470, Lng: 104.0665}, fbName, fbLat, fbLng, "上车点")
	if name != "上车点" || lat != 30.5470 || lng != 104.0665 {
		t.Fatalf("got (%s,%v,%v), want default name with event coordinates", name, lat, lng)
	}
}
