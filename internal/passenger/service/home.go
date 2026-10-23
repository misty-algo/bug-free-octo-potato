package service

import (
	"context"
	"sort"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// 车型定义。车型是否可用由计价域配置决定，乘客端只读。
const (
	// CarTypeExpress 为快车。
	CarTypeExpress = 1
	// CarTypeCarpool 为拼车。
	CarTypeCarpool = 2
	// CarTypePremium 为专车。
	CarTypePremium = 3
)

// CarTypeCatalog 描述一个车型的静态信息。
type CarTypeCatalog struct {
	CarType int
	Name    string
	// MinFee 为起步价，单位分。
	MinFee int64
	// Available 标识该车型在当前城市是否已启用。
	Available bool
	// UnavailableReason 在不可用时说明原因，前端据此置灰。
	UnavailableReason string
}

// CarTypeProvider 读取城市可用车型，由计价域提供。
type CarTypeProvider interface {
	// ListByCity 返回该城市已启用的车型。
	ListByCity(ctx context.Context, city string) ([]*CarTypeCatalog, error)
}

// defaultCarTypes 是没有配置计价域时的兜底车型。
func defaultCarTypes() []*CarTypeCatalog {
	return []*CarTypeCatalog{
		{CarType: CarTypeExpress, Name: "快车", MinFee: 1300, Available: true},
		{CarType: CarTypeCarpool, Name: "拼车", MinFee: 800, Available: true},
		{CarType: CarTypePremium, Name: "专车", MinFee: 2500, Available: true},
	}
}

// Home 返回首页合并数据。
//
// 一次返回车型、首次快速入口、常用地址、历史地址与恢复入口，
// 避免前端为拼首屏发起多次请求。
func (s *Service) Home(ctx context.Context, cmd dto.HomeCommand) (*dto.HomeDTO, error) {
	out := &dto.HomeDTO{
		CarTypes:        []dto.CarTypeDTO{},
		QuickEntries:    []dto.QuickEntryDTO{},
		CommonAddresses: []dto.AddressDTO{},
		HistoryAddresses: []dto.AddressDTO{},
		RestoreTarget:   "home",
	}

	// 车型：计价域不可用时退回兜底车型，保证首页不白屏。
	catalog := defaultCarTypes()
	if s.carTypes != nil {
		if list, err := s.carTypes.ListByCity(ctx, ""); err == nil && len(list) > 0 {
			catalog = list
		}
	}
	for _, item := range catalog {
		out.CarTypes = append(out.CarTypes, dto.CarTypeDTO{
			CarType:           item.CarType,
			Name:              item.Name,
			Available:         item.Available,
			UnavailableReason: item.UnavailableReason,
			MinFee:            item.MinFee,
		})
	}

	// 首次快速入口：只返回该账号没用过的，按配置顺序排列，最多四条。
	entries, err := s.quick.ListEnabled(ctx, s.now().UTC())
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	if cmd.AccountID != "" {
		ids, err := s.quick.ListUsedEntryIDs(ctx, cmd.AccountID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			used[id] = true
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Sort < entries[j].Sort })
	count := 0
	for _, entry := range entries {
		if used[entry.EntryID] {
			continue
		}
		out.QuickEntries = append(out.QuickEntries, dto.QuickEntryDTO{
			EntryID: entry.EntryID,
			Title:   entry.Title,
			Icon:    entry.Icon,
			Action:  entry.Action,
			Target:  entry.Target,
		})
		count++
		if count >= 4 {
			break
		}
	}

	// 常用地址最多二十条，历史地址取前十条。
	if cmd.AccountID != "" {
		common, err := s.addresses.ListByType(ctx, cmd.AccountID, entity.AddressTypeHome, entity.AddressLimit)
		if err != nil {
			return nil, err
		}
		company, err := s.addresses.ListByType(ctx, cmd.AccountID, entity.AddressTypeCompany, entity.AddressLimit)
		if err != nil {
			return nil, err
		}
		other, err := s.addresses.ListByType(ctx, cmd.AccountID, entity.AddressTypeOther, entity.AddressLimit)
		if err != nil {
			return nil, err
		}
		for _, list := range [][]*entity.Address{common, company, other} {
			for _, address := range list {
				out.CommonAddresses = append(out.CommonAddresses, toAddressDTO(address))
			}
		}
		history, err := s.addresses.ListByType(ctx, cmd.AccountID, entity.AddressTypeHistory, 10)
		if err != nil {
			return nil, err
		}
		// 历史地址按选择次数与最近使用时间排序。
		sort.SliceStable(history, func(i, j int) bool {
			if history[i].UseCount != history[j].UseCount {
				return history[i].UseCount > history[j].UseCount
			}
			return history[i].LastUsedAt.After(history[j].LastUsedAt)
		})
		for _, address := range history {
			out.HistoryAddresses = append(out.HistoryAddresses, toAddressDTO(address))
		}
	}

	// 恢复入口：存在进行中订单时展示。
	if cmd.AccountID != "" {
		if active, err := s.orders.FindActive(ctx, cmd.AccountID); err == nil && active != nil {
			out.HasActiveOrder = true
			out.ActiveOrderID = active.ID
			out.RestoreTarget = "order_detail"
		}
	}
	return out, nil
}

// SearchPOI 检索地点。带坐标时按距离由近到远排序，
// 无坐标时仍返回结果但不展示虚假距离。
func (s *Service) SearchPOI(ctx context.Context, cmd dto.SearchPOICommand) ([]*dto.POIDTO, error) {
	limit := cmd.Limit
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	list, err := s.pois.Search(ctx, cmd.Keyword, cmd.City, cmd.Lng, cmd.Lat, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.POIDTO, 0, len(list))
	for _, poi := range list {
		item := &dto.POIDTO{
			ID:    poi.ID,
			Name:  poi.Name,
			Addr:  poi.Addr,
			City:  poi.City,
			Lng:   poi.Lng,
			Lat:   poi.Lat,
		}
		// 没有定位坐标时距离不可算，MUST NOT 返回虚假距离。
		if cmd.HasCoord {
			item.HasDistance = true
			item.DistanceM = poi.DistanceM
		}
		out = append(out, item)
	}
	return out, nil
}

// UseQuickEntry 记录入口已用过。
//
// 游客点击只做前端引导，不落已用标记；记录需要登录。
func (s *Service) UseQuickEntry(ctx context.Context, accountID, entryID string) error {
	if accountID == "" {
		return entity.ErrUnauthorized
	}
	if entryID == "" {
		return entity.ErrInvalidStateTransition
	}
	return s.quick.MarkUsed(ctx, accountID, entryID, s.now().UTC())
}

// toAddressDTO 把地址实体转成出参。
func toAddressDTO(address *entity.Address) dto.AddressDTO {
	return dto.AddressDTO{
		ID:       address.ID,
		Type:     string(address.Type),
		Tag:      address.Tag,
		Name:     address.Name,
		Addr:     address.Addr,
		POIID:    address.POIID,
		Lng:      address.Lng,
		Lat:      address.Lat,
		UseCount: address.UseCount,
	}
}
