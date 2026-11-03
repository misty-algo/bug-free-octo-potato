package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// poiCacheEntry 是关键词检索结果的进程内缓存项。
type poiCacheEntry struct {
	results []*entity.POI
	expire  time.Time
}

// NominatimPOIRepository 是地点检索的在线实现。
//
// ADAPTER: real —— 服务端代理调用 OpenStreetMap Nominatim（免 Key，
// 覆盖全国省市区县/乡镇/村庄/道路/POI），浏览器不再直连境外服务，
// 避免跨域与网络不可达问题。Nominatim 不可用时回退 Photon，
// 两者都失败时回退内置本地地点，保证演示链路不中断。
//
// 性能优化（相对原串行实现）：
//   - 关键词结果进程内缓存（默认 10 分钟），重复搜索不再打外部网络；
//   - Nominatim / Photon 并发请求，谁先返回有效结果用谁，避免串行最坏 ~12s；
//   - 单次请求超时降到 3s，更快回退到本地内置地点。
type NominatimPOIRepository struct {
	fallback *LocalPOIRepository
	client   *http.Client
	mu       sync.Mutex
	cache    map[string]poiCacheEntry
	cacheTTL time.Duration
}

// NewNominatimPOIRepository 创建在线地点检索实现。
func NewNominatimPOIRepository() *NominatimPOIRepository {
	return &NominatimPOIRepository{
		fallback: NewLocalPOIRepository(),
		client:   &http.Client{Timeout: 3 * time.Second},
		cache:    make(map[string]poiCacheEntry),
		cacheTTL: 10 * time.Minute,
	}
}

// Search 按关键词检索全国地点。
func (r *NominatimPOIRepository) Search(ctx context.Context, keyword, city string, lng, lat float64, limit int) ([]*entity.POI, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return r.fallback.Search(ctx, keyword, city, lng, lat, limit)
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	// 命中缓存直接返回，省去外部网络往返。
	r.mu.Lock()
	if e, ok := r.cache[keyword]; ok && time.Now().Before(e.expire) {
		r.mu.Unlock()
		return e.results, nil
	}
	r.mu.Unlock()

	type searchResult struct {
		pois []*entity.POI
		err  error
	}
	nomCh := make(chan searchResult, 1)
	phtCh := make(chan searchResult, 1)
	// 并发请求两个在线源，谁先返回有效结果就用谁。
	go func() {
		pois, err := r.searchNominatim(ctx, keyword, limit, lng, lat)
		nomCh <- searchResult{pois, err}
	}()
	go func() {
		pois, err := r.searchPhoton(ctx, keyword, limit, lng, lat)
		phtCh <- searchResult{pois, err}
	}()
	nom := <-nomCh
	pht := <-phtCh

	list := nom.pois
	if len(list) == 0 {
		list = pht.pois
	}
	if len(list) == 0 {
		// 双源都不可达：回退内置地点，保证可继续下单演示。
		list, _ = r.fallback.Search(ctx, keyword, city, lng, lat, limit)
	}
	// 仅缓存真实在线结果；失败回退不缓存，网络恢复即可立即生效。
	if len(nom.pois) > 0 || len(pht.pois) > 0 {
		r.mu.Lock()
		r.cache[keyword] = poiCacheEntry{results: list, expire: time.Now().Add(r.cacheTTL)}
		r.mu.Unlock()
	}
	return list, nil
}

// Recommend 返回附近上车点。在线源没有"上车点推荐"概念，
// 推荐位仍使用内置常用地点。
func (r *NominatimPOIRepository) Recommend(ctx context.Context, lng, lat float64, limit int) ([]*entity.POI, error) {
	return r.fallback.Recommend(ctx, lng, lat, limit)
}

// nominatimItem 是 Nominatim search 响应的条目。
type nominatimItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Lon         string `json:"lon"`
	Lat         string `json:"lat"`
	Address     struct {
		City     string `json:"city"`
		Town     string `json:"town"`
		County   string `json:"county"`
		Village  string `json:"village"`
		State    string `json:"state"`
		Province string `json:"province"`
	} `json:"address"`
}

// searchNominatim 调用 Nominatim 检索（中文结果）。
// lng/lat 非零时附加经纬度做就近偏好（proximity），提升本地结果排序。
func (r *NominatimPOIRepository) searchNominatim(ctx context.Context, keyword string, limit int, lng, lat float64) ([]*entity.POI, error) {
	q := url.Values{}
	q.Set("format", "jsonv2")
	q.Set("countrycodes", "cn")
	q.Set("accept-language", "zh-CN")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("q", keyword)
	if lat != 0 || lng != 0 {
		q.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
		q.Set("lon", strconv.FormatFloat(lng, 'f', 6, 64))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://nominatim.openstreetmap.org/search?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// Nominatim 使用条款要求携带可识别的 User-Agent。
	req.Header.Set("User-Agent", "huazhu-ride-demo/1.0")
	req.Header.Set("Accept", "application/json")
	body, err := r.do(req)
	if err != nil {
		return nil, err
	}
	return r.decodePOIs(body, func(raw json.RawMessage) ([]*entity.POI, error) {
		var items []nominatimItem
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		out := make([]*entity.POI, 0, len(items))
		for i, it := range items {
			lng, lat, ok := parseCoord(it.Lon, it.Lat)
			if !ok {
				continue
			}
			out = append(out, &entity.POI{
				ID:   fmt.Sprintf("osm_%d_%s_%s", i, it.Lon, it.Lat),
				Name: firstNonEmpty(it.Name, strings.Split(it.DisplayName, ",")[0]),
				Addr: briefAddr(it.DisplayName),
				City: firstNonEmpty(it.Address.City, it.Address.Town, it.Address.County,
					it.Address.Village, it.Address.State, it.Address.Province),
				Lng: lng, Lat: lat,
			})
		}
		return out, nil
	})
}

// photonItem 是 Photon 检索响应的条目。
type photonItem struct {
	Properties struct {
		Name    string `json:"name"`
		City    string `json:"city"`
		State   string `json:"state"`
		Country string `json:"country"`
		Street  string `json:"street"`
		District string `json:"district"`
	} `json:"properties"`
	Geometry struct {
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
}

// searchPhoton 调用 Photon（komoot，Nominatim 的检索分支）作回退源。
// lng/lat 非零时附加经纬度做就近偏好。
func (r *NominatimPOIRepository) searchPhoton(ctx context.Context, keyword string, limit int, lng, lat float64) ([]*entity.POI, error) {
	q := url.Values{}
	q.Set("lang", "default")
	q.Set("limit", strconv.Itoa(limit))
	q.Set("q", keyword)
	if lat != 0 || lng != 0 {
		q.Set("lat", strconv.FormatFloat(lat, 'f', 6, 64))
		q.Set("lon", strconv.FormatFloat(lng, 'f', 6, 64))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://photon.komoot.io/api/?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "huazhu-ride-demo/1.0")
	req.Header.Set("Accept", "application/json")
	body, err := r.do(req)
	if err != nil {
		return nil, err
	}
	return r.decodePOIs(body, func(raw json.RawMessage) ([]*entity.POI, error) {
		var body struct {
			Features []photonItem `json:"features"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		out := make([]*entity.POI, 0, len(body.Features))
		for i, f := range body.Features {
			if len(f.Geometry.Coordinates) < 2 {
				continue
			}
			name := firstNonEmpty(f.Properties.Name, f.Properties.Street, f.Properties.District)
			if name == "" {
				continue
			}
			out = append(out, &entity.POI{
				ID:   fmt.Sprintf("pht_%d_%v_%v", i, f.Geometry.Coordinates[0], f.Geometry.Coordinates[1]),
				Name: name,
				Addr: strings.Join(filterEmpty(f.Properties.District, f.Properties.City, f.Properties.State), " · "),
				City: firstNonEmpty(f.Properties.City, f.Properties.State),
				Lng:  f.Geometry.Coordinates[0],
				Lat:  f.Geometry.Coordinates[1],
			})
		}
		return out, nil
	})
}

// do 发送请求并返回响应体。
func (r *NominatimPOIRepository) do(req *http.Request) ([]byte, error) {
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poi upstream status %d", resp.StatusCode)
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 2048)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// decodePOIs 把响应体交给 decode 解析为 POI 列表。
func (r *NominatimPOIRepository) decodePOIs(body []byte, decode func(json.RawMessage) ([]*entity.POI, error)) ([]*entity.POI, error) {
	return decode(json.RawMessage(body))
}

// parseCoord 解析 Nominatim 的字符串经纬度。
func parseCoord(lon, lat string) (float64, float64, bool) {
	lngV, err1 := strconv.ParseFloat(lon, 64)
	latV, err2 := strconv.ParseFloat(lat, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lngV, latV, true
}

// briefAddr 把完整 display_name 压缩成省市区三级摘要。
func briefAddr(display string) string {
	parts := strings.Split(display, ",")
	if len(parts) > 4 {
		parts = parts[:4]
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// filterEmpty 过滤空字符串。
func filterEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}

// 编译期接口契约校验。
var _ entity.POIRepository = (*NominatimPOIRepository)(nil)
